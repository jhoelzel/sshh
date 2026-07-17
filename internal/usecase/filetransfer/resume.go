package filetransfer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	filedomain "shh-h/internal/domain/filetransfer"
	"shh-h/internal/port"
)

const maxResumeErrorBytes = 4096

type ResumeRepository interface {
	LoadResumes() ([]filedomain.ResumeRecord, error)
	SaveResumes([]filedomain.ResumeRecord) error
}

func NewManagerWithResumeRepository(factory port.RemoteFilesystemFactory, repository ResumeRepository) (*Manager, error) {
	manager := NewManager(factory)
	if repository == nil {
		return manager, nil
	}
	records, err := repository.LoadResumes()
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		if _, exists := manager.resumes[record.ID]; exists {
			return nil, fmt.Errorf("duplicate transfer resume id %q", record.ID)
		}
		manager.resumes[record.ID] = record
	}
	manager.resumeRepo = repository
	return manager, nil
}

func (m *Manager) TransferResumes(leaseID, sessionID string) ([]filedomain.TransferResume, error) {
	runtime, err := m.session(leaseID, sessionID)
	if err != nil {
		return nil, err
	}
	m.resumeMu.Lock()
	records := make([]filedomain.ResumeRecord, 0, len(m.resumes))
	for _, record := range m.resumes {
		if record.ProfileID != runtime.snapshot.ProfileID {
			continue
		}
		if _, active := m.activeResumes[record.ID]; active {
			continue
		}
		records = append(records, record)
	}
	m.resumeMu.Unlock()
	sort.Slice(records, func(left, right int) bool { return records[left].UpdatedAt > records[right].UpdatedAt })
	result := make([]filedomain.TransferResume, 0, len(records))
	for _, record := range records {
		result = append(result, m.inspectResume(runtime, record))
	}
	return result, nil
}

func (m *Manager) ResumeTransfer(leaseID, sessionID, resumeID string) (filedomain.Transfer, error) {
	runtime, err := m.session(leaseID, sessionID)
	if err != nil {
		return filedomain.Transfer{}, err
	}
	record, err := m.beginResume(resumeID, runtime.snapshot.ProfileID)
	if err != nil {
		return filedomain.Transfer{}, err
	}
	if err := m.reserveResumeDestination(runtime, record); err != nil {
		m.endActiveResume(record.ID)
		return filedomain.Transfer{}, err
	}
	operation := func(transfer *runtimeTransfer) error {
		if record.Direction == filedomain.DirectionDownload {
			return m.resumeDownload(runtime, transfer, record)
		}
		return m.resumeUpload(runtime, transfer, record)
	}
	transfer, err := m.startTransferAt(
		runtime, record.Direction, record.Source, record.Destination,
		record.ID, record.Bytes, record.Total, operation,
	)
	if err != nil {
		m.endActiveResume(record.ID)
	}
	return transfer, err
}

func (m *Manager) DiscardTransferResume(leaseID, sessionID, resumeID string) error {
	runtime, err := m.session(leaseID, sessionID)
	if err != nil {
		return err
	}
	record, err := m.beginResume(resumeID, runtime.snapshot.ProfileID)
	if err != nil {
		return err
	}
	defer m.endActiveResume(record.ID)
	if record.Direction == filedomain.DirectionDownload {
		if err := os.Remove(record.PartialPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove partial download: %w", err)
		}
	} else if err := runtime.filesystem.Remove(runtime.ctx, record.PartialPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove partial upload: %w", err)
	}
	if err := m.deleteResume(record.ID); err != nil {
		return fmt.Errorf("remove transfer resume metadata: %w", err)
	}
	return nil
}

func (m *Manager) resumeDownload(runtime *runtimeSession, transfer *runtimeTransfer, record filedomain.ResumeRecord) (resultErr error) {
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, m.captureResumeFailure(runtime, transfer, record, resultErr))
		}
	}()
	partialInfo, err := m.validateDownloadResume(runtime, record)
	if err != nil {
		return err
	}
	offset := partialInfo.Size()
	transfer.setProgress(offset, record.Total)
	m.publish(transfer.snapshotValue())
	source, total, err := runtime.filesystem.OpenRead(transfer.ctx, record.Source, offset)
	if err != nil {
		return err
	}
	defer source.Close()
	if total != record.Total {
		return errors.New("remote source size changed before resume")
	}
	destination, err := os.OpenFile(record.PartialPath, os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open partial download: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = destination.Close()
		}
	}()
	openedInfo, err := destination.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(partialInfo, openedInfo) {
		return errors.New("partial download changed while it was being opened")
	}
	if _, err := destination.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("seek partial download: %w", err)
	}
	if err := m.copy(transfer, destination, source); err != nil {
		return err
	}
	currentSource, err := runtime.filesystem.Stat(transfer.ctx, record.Source)
	if err != nil {
		return fmt.Errorf("recheck remote source: %w", err)
	}
	if !remoteSourceMatchesRecord(currentSource, record) {
		return errors.New("remote source changed during resumed download")
	}
	if err := destination.Sync(); err != nil {
		return fmt.Errorf("sync resumed download: %w", err)
	}
	if err := destination.Close(); err != nil {
		return fmt.Errorf("close resumed download: %w", err)
	}
	closed = true
	if !record.Overwrite {
		exists, err := localPathExists(record.Destination)
		if err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("%w during resumed download", ErrDestinationExists)
		}
	}
	currentPartial, err := os.Lstat(record.PartialPath)
	if err != nil || !currentPartial.Mode().IsRegular() || !os.SameFile(partialInfo, currentPartial) {
		return errors.New("partial download path changed before publication")
	}
	if err := os.Rename(record.PartialPath, record.Destination); err != nil {
		return fmt.Errorf("finish resumed download: %w", err)
	}
	_ = m.deleteResume(record.ID)
	return nil
}

func (m *Manager) resumeUpload(runtime *runtimeSession, transfer *runtimeTransfer, record filedomain.ResumeRecord) (resultErr error) {
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, m.captureResumeFailure(runtime, transfer, record, resultErr))
		}
	}()
	source, err := os.Open(record.Source)
	if err != nil {
		return fmt.Errorf("open local source: %w", err)
	}
	defer source.Close()
	sourceInfo, sourceDigest, err := digestOpenFile(transfer.ctx, source)
	if err != nil {
		return err
	}
	if !localSourceMatchesRecord(sourceInfo, sourceDigest, record) {
		return errors.New("local source changed since the upload was interrupted")
	}
	partial, err := runtime.filesystem.Stat(transfer.ctx, record.PartialPath)
	if err != nil {
		return fmt.Errorf("inspect partial upload: %w", err)
	}
	if partial.Directory || partial.Symlink || !os.FileMode(partial.Mode).IsRegular() || partial.Size < 0 || partial.Size > record.Total {
		return errors.New("partial upload is not a valid resumable file")
	}
	offset := partial.Size
	localPrefix, err := digestFilePrefix(transfer.ctx, source, offset)
	if err != nil {
		return fmt.Errorf("checksum local upload prefix: %w", err)
	}
	remotePrefix, err := m.digestRemoteFile(transfer.ctx, runtime, record.PartialPath, offset)
	if err != nil {
		return fmt.Errorf("checksum partial upload: %w", err)
	}
	if localPrefix != remotePrefix {
		return errors.New("partial upload checksum does not match the local source")
	}
	if _, err := source.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("seek local upload source: %w", err)
	}
	transfer.setProgress(offset, record.Total)
	m.publish(transfer.snapshotValue())
	destination, err := runtime.filesystem.OpenWrite(transfer.ctx, record.PartialPath, offset)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = destination.Close()
		}
	}()
	if err := m.copy(transfer, destination, source); err != nil {
		return err
	}
	if err := destination.Close(); err != nil {
		return fmt.Errorf("close resumed upload: %w", err)
	}
	closed = true
	currentSource, err := os.Stat(record.Source)
	if err != nil || !sameLocalSource(sourceInfo, currentSource) {
		return errors.New("local source changed during resumed upload")
	}
	remoteDigest, err := m.digestRemoteFile(transfer.ctx, runtime, record.PartialPath, record.Total)
	if err != nil {
		return fmt.Errorf("verify resumed upload: %w", err)
	}
	if remoteDigest != record.SourceSHA256 {
		return errors.New("completed upload checksum does not match the local source")
	}
	if !record.Overwrite {
		exists, err := remotePathExists(runtime, record.Destination)
		if err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("%w during resumed upload", ErrDestinationExists)
		}
	}
	if err := runtime.filesystem.Rename(runtime.ctx, record.PartialPath, record.Destination); err != nil {
		return fmt.Errorf("finish resumed upload: %w", err)
	}
	_ = m.deleteResume(record.ID)
	return nil
}

func (m *Manager) inspectResume(runtime *runtimeSession, record filedomain.ResumeRecord) filedomain.TransferResume {
	summary := filedomain.TransferResume{
		ID: record.ID, ProfileID: record.ProfileID, Direction: record.Direction,
		Source: record.Source, Destination: record.Destination, Bytes: record.Bytes, Total: record.Total,
		Available: true, Message: record.LastError, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
	if record.Direction == filedomain.DirectionDownload {
		partial, err := os.Lstat(record.PartialPath)
		if err != nil || !partial.Mode().IsRegular() || partial.Size() > record.Total {
			summary.Available = false
			summary.Message = "Partial download is no longer available"
			return summary
		}
		summary.Bytes = partial.Size()
		source, err := runtime.filesystem.Stat(runtime.ctx, record.Source)
		if err != nil || !remoteSourceMatchesRecord(source, record) {
			summary.Available = false
			summary.Message = "Remote source changed since the interruption"
			return summary
		}
		if !record.Overwrite {
			if exists, err := localPathExists(record.Destination); err != nil || exists {
				summary.Available = false
				summary.Message = "Download destination now exists"
			}
		}
		return summary
	}

	source, err := os.Stat(record.Source)
	if err != nil || !localSourceMatchesRecord(source, record.SourceSHA256, record) {
		summary.Available = false
		summary.Message = "Local source changed since the interruption"
		return summary
	}
	partial, err := runtime.filesystem.Stat(runtime.ctx, record.PartialPath)
	if err != nil || partial.Directory || partial.Symlink || partial.Size > record.Total {
		summary.Available = false
		summary.Message = "Partial upload is no longer available"
		return summary
	}
	summary.Bytes = partial.Size
	if !record.Overwrite {
		if exists, err := remotePathExists(runtime, record.Destination); err != nil || exists {
			summary.Available = false
			summary.Message = "Upload destination now exists"
		}
	}
	return summary
}

func (m *Manager) validateDownloadResume(runtime *runtimeSession, record filedomain.ResumeRecord) (os.FileInfo, error) {
	partial, err := os.Lstat(record.PartialPath)
	if err != nil {
		return nil, fmt.Errorf("inspect partial download: %w", err)
	}
	if !partial.Mode().IsRegular() || partial.Size() < 0 || partial.Size() > record.Total {
		return nil, errors.New("partial download is not a valid resumable file")
	}
	source, err := runtime.filesystem.Stat(runtime.ctx, record.Source)
	if err != nil {
		return nil, fmt.Errorf("inspect remote source: %w", err)
	}
	if !remoteSourceMatchesRecord(source, record) {
		return nil, errors.New("remote source changed since the download was interrupted")
	}
	return partial, nil
}

func (m *Manager) reserveResumeDestination(runtime *runtimeSession, record filedomain.ResumeRecord) error {
	if !m.tryReserveDestination(record.Direction, runtime.snapshot.ID, record.Destination) {
		return ErrDestinationExists
	}
	if record.Overwrite {
		return nil
	}
	var exists bool
	var err error
	if record.Direction == filedomain.DirectionDownload {
		exists, err = localPathExists(record.Destination)
	} else {
		exists, err = remotePathExists(runtime, record.Destination)
	}
	if err != nil {
		m.releaseDestination(record.Direction, runtime.snapshot.ID, record.Destination)
		return err
	}
	if exists {
		m.releaseDestination(record.Direction, runtime.snapshot.ID, record.Destination)
		return ErrDestinationExists
	}
	return nil
}

func (m *Manager) captureResumeFailure(runtime *runtimeSession, transfer *runtimeTransfer, record filedomain.ResumeRecord, cause error) error {
	if record.Direction == filedomain.DirectionDownload {
		record.Bytes = localFileSize(record.PartialPath, transfer.snapshotValue().Bytes)
	} else {
		record.Bytes = m.remoteFileSize(runtime, record.PartialPath, transfer.snapshotValue().Bytes)
	}
	record.LastError = boundedResumeError(cause)
	record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	transfer.setResume(record.ID, record.Bytes)
	if err := m.upsertResume(record); err != nil {
		return fmt.Errorf("update transfer resume metadata: %w", err)
	}
	return nil
}

func (m *Manager) createResume(record filedomain.ResumeRecord) error {
	m.resumeMu.Lock()
	defer m.resumeMu.Unlock()
	if _, exists := m.resumes[record.ID]; exists {
		return errors.New("transfer resume already exists")
	}
	m.resumes[record.ID] = record
	m.activeResumes[record.ID] = struct{}{}
	if err := m.persistResumesLocked(); err != nil {
		delete(m.resumes, record.ID)
		delete(m.activeResumes, record.ID)
		return err
	}
	return nil
}

func (m *Manager) upsertResume(record filedomain.ResumeRecord) error {
	m.resumeMu.Lock()
	defer m.resumeMu.Unlock()
	previous, existed := m.resumes[record.ID]
	m.resumes[record.ID] = record
	if err := m.persistResumesLocked(); err != nil {
		if existed {
			m.resumes[record.ID] = previous
		} else {
			delete(m.resumes, record.ID)
		}
		return err
	}
	return nil
}

func (m *Manager) deleteResume(id string) error {
	m.resumeMu.Lock()
	defer m.resumeMu.Unlock()
	previous, existed := m.resumes[id]
	if !existed {
		return nil
	}
	delete(m.resumes, id)
	if err := m.persistResumesLocked(); err != nil {
		m.resumes[id] = previous
		return err
	}
	return nil
}

func (m *Manager) beginResume(id, profileID string) (filedomain.ResumeRecord, error) {
	m.resumeMu.Lock()
	defer m.resumeMu.Unlock()
	record, exists := m.resumes[id]
	if !exists || record.ProfileID != profileID {
		return filedomain.ResumeRecord{}, errors.New("transfer resume not found")
	}
	if _, active := m.activeResumes[id]; active {
		return filedomain.ResumeRecord{}, errors.New("transfer resume is already active")
	}
	m.activeResumes[id] = struct{}{}
	return record, nil
}

func (m *Manager) endActiveResume(id string) {
	m.resumeMu.Lock()
	delete(m.activeResumes, id)
	m.resumeMu.Unlock()
}

func (m *Manager) persistResumesLocked() error {
	if m.resumeRepo == nil {
		return nil
	}
	records := make([]filedomain.ResumeRecord, 0, len(m.resumes))
	for _, record := range m.resumes {
		records = append(records, record)
	}
	sort.Slice(records, func(left, right int) bool { return records[left].ID < records[right].ID })
	return m.resumeRepo.SaveResumes(records)
}

func (m *Manager) remoteFileSize(runtime *runtimeSession, remotePath string, fallback int64) int64 {
	entry, err := runtime.filesystem.Stat(runtime.ctx, remotePath)
	if err == nil && entry.Size >= 0 {
		return entry.Size
	}
	return fallback
}

func (m *Manager) digestRemoteFile(ctx context.Context, runtime *runtimeSession, remotePath string, expected int64) (string, error) {
	reader, total, err := runtime.filesystem.OpenRead(ctx, remotePath, 0)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	if total != expected {
		return "", fmt.Errorf("remote file size %d does not match expected size %d", total, expected)
	}
	return digestReader(ctx, reader, expected)
}

func digestOpenFile(ctx context.Context, file *os.File) (os.FileInfo, string, error) {
	before, err := file.Stat()
	if err != nil {
		return nil, "", fmt.Errorf("inspect local source: %w", err)
	}
	if !before.Mode().IsRegular() {
		return nil, "", errors.New("local source is not a regular file")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, "", fmt.Errorf("seek local source: %w", err)
	}
	digest, err := digestReader(ctx, file, before.Size())
	if err != nil {
		return nil, "", fmt.Errorf("checksum local source: %w", err)
	}
	after, err := file.Stat()
	if err != nil || !sameLocalSource(before, after) {
		return nil, "", errors.New("local source changed while it was being checksummed")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, "", fmt.Errorf("rewind local source: %w", err)
	}
	return before, digest, nil
}

func digestFilePrefix(ctx context.Context, file *os.File, size int64) (string, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	return digestReader(ctx, file, size)
}

func digestReader(ctx context.Context, reader io.Reader, size int64) (string, error) {
	if size < 0 {
		return "", errors.New("checksum size cannot be negative")
	}
	hash := sha256.New()
	buffer := make([]byte, copyBufferSize)
	remaining := size
	for remaining > 0 {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		chunk := int64(len(buffer))
		if remaining < chunk {
			chunk = remaining
		}
		read, err := io.ReadFull(reader, buffer[:chunk])
		if read > 0 {
			_, _ = hash.Write(buffer[:read])
			remaining -= int64(read)
		}
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func sameRemoteSource(expected, current filedomain.Entry) bool {
	return !current.Directory && !current.Symlink && os.FileMode(current.Mode).IsRegular() &&
		expected.Size == current.Size && expected.ModifiedAt == current.ModifiedAt
}

func remoteSourceMatchesRecord(current filedomain.Entry, record filedomain.ResumeRecord) bool {
	return !current.Directory && !current.Symlink && os.FileMode(current.Mode).IsRegular() &&
		current.Size == record.SourceSize && current.ModifiedAt == record.SourceModifiedAt
}

func sameLocalSource(expected, current os.FileInfo) bool {
	return expected != nil && current != nil && current.Mode().IsRegular() &&
		expected.Size() == current.Size() && expected.ModTime().Equal(current.ModTime())
}

func localSourceMatchesRecord(current os.FileInfo, digest string, record filedomain.ResumeRecord) bool {
	return current != nil && current.Mode().IsRegular() && current.Size() == record.SourceSize &&
		current.ModTime().UTC().Format(time.RFC3339Nano) == record.SourceModifiedAt && digest == record.SourceSHA256
}

func localFileSize(localPath string, fallback int64) int64 {
	info, err := os.Lstat(localPath)
	if err == nil && info.Mode().IsRegular() && info.Size() >= 0 {
		return info.Size()
	}
	return fallback
}

func localPartialPath(destination, id string) string {
	return filepath.Clean(filepath.Join(filepath.Dir(destination), "."+filepath.Base(destination)+".shhh-part-"+id))
}

func remotePartialPath(destination, id string) string {
	return path.Clean(path.Join(path.Dir(destination), "."+path.Base(destination)+".shhh-part-"+id))
}

func boundedResumeError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if len(message) <= maxResumeErrorBytes {
		return message
	}
	message = message[:maxResumeErrorBytes]
	for !utf8.ValidString(message) {
		message = message[:len(message)-1]
	}
	return message
}
