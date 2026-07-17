package filetransfer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"sync"
	"time"

	filedomain "shh-h/internal/domain/filetransfer"
	"shh-h/internal/domain/profile"
	"shh-h/internal/port"
)

const (
	transferConcurrency = 2
	copyBufferSize      = 128 * 1024
	progressInterval    = 100 * time.Millisecond
	maxTransferHistory  = 200
)

type Sink interface {
	PublishTransfer(filedomain.Transfer)
}

type Manager struct {
	mu        sync.RWMutex
	factory   port.RemoteFilesystemFactory
	sink      Sink
	sessions  map[string]*runtimeSession
	transfers map[string]*runtimeTransfer
	workers   chan struct{}
}

type runtimeSession struct {
	snapshot   filedomain.Session
	filesystem port.RemoteFilesystem
	ctx        context.Context
	cancel     context.CancelFunc
	closeOnce  sync.Once
	closeErr   error
}

type runtimeTransfer struct {
	mu       sync.RWMutex
	snapshot filedomain.Transfer
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
}

func NewManager(factory port.RemoteFilesystemFactory) *Manager {
	return &Manager{
		factory: factory, sessions: make(map[string]*runtimeSession),
		transfers: make(map[string]*runtimeTransfer), workers: make(chan struct{}, transferConcurrency),
	}
}

func (m *Manager) SetSink(sink Sink) {
	m.mu.Lock()
	m.sink = sink
	m.mu.Unlock()
}

func (m *Manager) Open(ctx context.Context, leaseID string, selected profile.Profile, credentials port.SSHCredentials) (filedomain.Session, error) {
	if leaseID == "" {
		return filedomain.Session{}, errors.New("frontend lease is required")
	}
	if selected.Protocol != profile.ProtocolSSH {
		return filedomain.Session{}, errors.New("sftp requires an ssh profile")
	}
	if err := selected.Validate(); err != nil {
		return filedomain.Session{}, err
	}
	if m.factory == nil {
		return filedomain.Session{}, errors.New("sftp support is unavailable")
	}

	runtimeContext, cancel := context.WithCancel(ctx)
	filesystem, err := m.factory.OpenRemoteFilesystem(runtimeContext, port.SSHTerminalSpec{
		Host: selected.Host, Port: selected.Port, Username: selected.Username,
		Authentication: selected.Authentication, IdentityFile: selected.IdentityFile,
		Credentials: credentials,
	})
	clear(credentials.Password)
	clear(credentials.Passphrase)
	if err != nil {
		cancel()
		return filedomain.Session{}, err
	}
	id, err := newID()
	if err != nil {
		cancel()
		_ = filesystem.Close()
		return filedomain.Session{}, err
	}
	snapshot := filedomain.Session{
		ID: id, LeaseID: leaseID, ProfileID: selected.ID,
		Root: filesystem.WorkingDirectory(), OpenedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	runtime := &runtimeSession{snapshot: snapshot, filesystem: filesystem, ctx: runtimeContext, cancel: cancel}
	m.mu.Lock()
	m.sessions[id] = runtime
	m.mu.Unlock()
	return snapshot, nil
}

func (m *Manager) List(leaseID, sessionID, remotePath string) ([]filedomain.Entry, error) {
	runtime, err := m.session(leaseID, sessionID)
	if err != nil {
		return nil, err
	}
	return runtime.filesystem.ReadDirectory(runtime.ctx, remotePath)
}

func (m *Manager) CreateDirectory(leaseID, sessionID, remotePath string) error {
	runtime, err := m.session(leaseID, sessionID)
	if err != nil {
		return err
	}
	return runtime.filesystem.CreateDirectory(runtime.ctx, remotePath)
}

func (m *Manager) Rename(leaseID, sessionID, source, destination string) error {
	runtime, err := m.session(leaseID, sessionID)
	if err != nil {
		return err
	}
	return runtime.filesystem.Rename(runtime.ctx, source, destination)
}

func (m *Manager) Remove(leaseID, sessionID, remotePath string) error {
	runtime, err := m.session(leaseID, sessionID)
	if err != nil {
		return err
	}
	return runtime.filesystem.Remove(runtime.ctx, remotePath)
}

func (m *Manager) Chmod(leaseID, sessionID, remotePath string, mode uint32) error {
	if mode > 0o7777 {
		return errors.New("invalid remote file mode")
	}
	runtime, err := m.session(leaseID, sessionID)
	if err != nil {
		return err
	}
	return runtime.filesystem.Chmod(runtime.ctx, remotePath, os.FileMode(mode))
}

func (m *Manager) StartDownload(leaseID, sessionID, remotePath, localPath string, overwrite bool) (filedomain.Transfer, error) {
	runtime, err := m.session(leaseID, sessionID)
	if err != nil {
		return filedomain.Transfer{}, err
	}
	localPath = filepath.Clean(localPath)
	if !overwrite {
		if _, err := os.Stat(localPath); err == nil {
			return filedomain.Transfer{}, errors.New("local destination already exists")
		} else if !errors.Is(err, os.ErrNotExist) {
			return filedomain.Transfer{}, fmt.Errorf("inspect local destination: %w", err)
		}
	}
	return m.startTransfer(runtime, filedomain.DirectionDownload, remotePath, localPath, func(transfer *runtimeTransfer) error {
		return m.download(runtime, transfer, remotePath, localPath, overwrite)
	})
}

func (m *Manager) StartUpload(leaseID, sessionID, localPath, remotePath string, overwrite bool) (filedomain.Transfer, error) {
	runtime, err := m.session(leaseID, sessionID)
	if err != nil {
		return filedomain.Transfer{}, err
	}
	info, err := os.Stat(localPath)
	if err != nil {
		return filedomain.Transfer{}, fmt.Errorf("inspect local source: %w", err)
	}
	if !info.Mode().IsRegular() {
		return filedomain.Transfer{}, errors.New("local source is not a regular file")
	}
	if !overwrite {
		if _, err := runtime.filesystem.Stat(runtime.ctx, remotePath); err == nil {
			return filedomain.Transfer{}, errors.New("remote destination already exists")
		}
	}
	return m.startTransfer(runtime, filedomain.DirectionUpload, localPath, remotePath, func(transfer *runtimeTransfer) error {
		transfer.setTotal(info.Size())
		return m.upload(runtime, transfer, localPath, remotePath, overwrite)
	})
}

func (m *Manager) Transfers(leaseID string) []filedomain.Transfer {
	m.mu.RLock()
	result := make([]filedomain.Transfer, 0, len(m.transfers))
	for _, transfer := range m.transfers {
		if snapshot := transfer.snapshotValue(); snapshot.LeaseID == leaseID {
			result = append(result, snapshot)
		}
	}
	m.mu.RUnlock()
	sort.Slice(result, func(left, right int) bool { return result[left].StartedAt > result[right].StartedAt })
	return result
}

func (m *Manager) CancelTransfer(leaseID, transferID string) error {
	m.mu.RLock()
	transfer := m.transfers[transferID]
	m.mu.RUnlock()
	if transfer == nil || transfer.snapshotValue().LeaseID != leaseID {
		return errors.New("transfer not found")
	}
	transfer.cancel()
	return nil
}

func (m *Manager) CloseSession(leaseID, sessionID string) error {
	runtime, err := m.session(leaseID, sessionID)
	if err != nil {
		return err
	}
	m.closeSession(runtime)
	m.mu.Lock()
	delete(m.sessions, sessionID)
	m.mu.Unlock()
	return runtime.closeErr
}

func (m *Manager) CloseLease(leaseID string) {
	m.mu.RLock()
	sessions := make([]*runtimeSession, 0)
	for _, runtime := range m.sessions {
		if runtime.snapshot.LeaseID == leaseID {
			sessions = append(sessions, runtime)
		}
	}
	m.mu.RUnlock()
	for _, runtime := range sessions {
		m.closeSession(runtime)
		m.mu.Lock()
		delete(m.sessions, runtime.snapshot.ID)
		m.mu.Unlock()
	}
}

func (m *Manager) Shutdown() {
	m.mu.RLock()
	sessions := make([]*runtimeSession, 0, len(m.sessions))
	for _, runtime := range m.sessions {
		sessions = append(sessions, runtime)
	}
	m.mu.RUnlock()
	for _, runtime := range sessions {
		m.closeSession(runtime)
	}
	m.mu.Lock()
	clear(m.sessions)
	m.mu.Unlock()
}

func (m *Manager) LiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

func (m *Manager) startTransfer(runtime *runtimeSession, direction filedomain.Direction, source, destination string, operation func(*runtimeTransfer) error) (filedomain.Transfer, error) {
	id, err := newID()
	if err != nil {
		return filedomain.Transfer{}, err
	}
	ctx, cancel := context.WithCancel(runtime.ctx)
	snapshot := filedomain.Transfer{
		ID: id, LeaseID: runtime.snapshot.LeaseID, SessionID: runtime.snapshot.ID,
		Direction: direction, Source: source, Destination: destination, State: filedomain.TransferQueued,
	}
	transfer := &runtimeTransfer{snapshot: snapshot, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	m.mu.Lock()
	m.transfers[id] = transfer
	m.mu.Unlock()
	m.publish(transfer.snapshotValue())
	go m.runTransfer(transfer, operation)
	return snapshot, nil
}

func (m *Manager) runTransfer(transfer *runtimeTransfer, operation func(*runtimeTransfer) error) {
	defer close(transfer.done)
	select {
	case m.workers <- struct{}{}:
		defer func() { <-m.workers }()
	case <-transfer.ctx.Done():
		transfer.finish(filedomain.TransferCancelled, "")
		m.publish(transfer.snapshotValue())
		m.pruneTransferHistory()
		return
	}
	transfer.start()
	m.publish(transfer.snapshotValue())
	err := operation(transfer)
	if errors.Is(err, context.Canceled) || errors.Is(transfer.ctx.Err(), context.Canceled) {
		transfer.finish(filedomain.TransferCancelled, "")
	} else if err != nil {
		transfer.finish(filedomain.TransferFailed, err.Error())
	} else {
		transfer.finish(filedomain.TransferCompleted, "")
	}
	m.publish(transfer.snapshotValue())
	m.pruneTransferHistory()
}

func (m *Manager) download(runtime *runtimeSession, transfer *runtimeTransfer, remotePath, localPath string, overwrite bool) error {
	source, total, err := runtime.filesystem.OpenRead(transfer.ctx, remotePath)
	if err != nil {
		return err
	}
	defer source.Close()
	transfer.setTotal(total)
	temporaryPath := filepath.Join(filepath.Dir(localPath), "."+filepath.Base(localPath)+".shhh-part-"+transfer.snapshot.ID)
	destination, err := os.OpenFile(temporaryPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create partial download: %w", err)
	}
	defer os.Remove(temporaryPath)
	if err := m.copy(transfer, destination, source); err != nil {
		_ = destination.Close()
		return err
	}
	if err := destination.Sync(); err != nil {
		_ = destination.Close()
		return fmt.Errorf("sync partial download: %w", err)
	}
	if err := destination.Close(); err != nil {
		return fmt.Errorf("close partial download: %w", err)
	}
	if !overwrite {
		if _, err := os.Stat(localPath); err == nil {
			return errors.New("local destination appeared during download")
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect local destination: %w", err)
		}
	}
	if err := os.Rename(temporaryPath, localPath); err != nil {
		return fmt.Errorf("finish download: %w", err)
	}
	return nil
}

func (m *Manager) upload(runtime *runtimeSession, transfer *runtimeTransfer, localPath, remotePath string, overwrite bool) error {
	source, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local source: %w", err)
	}
	defer source.Close()
	temporaryPath := path.Join(path.Dir(remotePath), "."+path.Base(remotePath)+".shhh-part-"+transfer.snapshot.ID)
	destination, err := runtime.filesystem.OpenWrite(transfer.ctx, temporaryPath)
	if err != nil {
		return err
	}
	removeTemporary := true
	defer func() {
		_ = destination.Close()
		if removeTemporary {
			_ = runtime.filesystem.Remove(context.Background(), temporaryPath)
		}
	}()
	if err := m.copy(transfer, destination, source); err != nil {
		return err
	}
	if err := destination.Close(); err != nil {
		return fmt.Errorf("close remote upload: %w", err)
	}
	if !overwrite {
		if _, err := runtime.filesystem.Stat(runtime.ctx, remotePath); err == nil {
			return errors.New("remote destination appeared during upload")
		}
	}
	if err := runtime.filesystem.Rename(runtime.ctx, temporaryPath, remotePath); err != nil {
		return err
	}
	removeTemporary = false
	return nil
}

func (m *Manager) copy(transfer *runtimeTransfer, destination io.Writer, source io.Reader) error {
	buffer := make([]byte, copyBufferSize)
	lastPublish := time.Now()
	for {
		if err := transfer.ctx.Err(); err != nil {
			return err
		}
		read, readErr := source.Read(buffer)
		if read > 0 {
			written, writeErr := destination.Write(buffer[:read])
			if writeErr != nil {
				return writeErr
			}
			if written != read {
				return io.ErrShortWrite
			}
			transfer.addBytes(int64(written))
			if time.Since(lastPublish) >= progressInterval {
				m.publish(transfer.snapshotValue())
				lastPublish = time.Now()
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func (m *Manager) closeSession(runtime *runtimeSession) {
	runtime.closeOnce.Do(func() {
		runtime.cancel()
		m.mu.RLock()
		transfers := make([]*runtimeTransfer, 0)
		for _, transfer := range m.transfers {
			if transfer.snapshotValue().SessionID == runtime.snapshot.ID {
				transfers = append(transfers, transfer)
			}
		}
		m.mu.RUnlock()
		for _, transfer := range transfers {
			transfer.cancel()
		}
		runtime.closeErr = runtime.filesystem.Close()
		for _, transfer := range transfers {
			<-transfer.done
		}
	})
}

func (m *Manager) session(leaseID, sessionID string) (*runtimeSession, error) {
	m.mu.RLock()
	runtime := m.sessions[sessionID]
	m.mu.RUnlock()
	if runtime == nil {
		return nil, errors.New("sftp session not found")
	}
	if runtime.snapshot.LeaseID != leaseID {
		return nil, errors.New("sftp session belongs to another frontend lease")
	}
	return runtime, nil
}

func (m *Manager) publish(snapshot filedomain.Transfer) {
	m.mu.RLock()
	sink := m.sink
	m.mu.RUnlock()
	if sink != nil {
		sink.PublishTransfer(snapshot)
	}
}

func (m *Manager) pruneTransferHistory() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.transfers) <= maxTransferHistory {
		return
	}
	type completedTransfer struct {
		id       string
		finished string
	}
	completed := make([]completedTransfer, 0, len(m.transfers))
	for id, transfer := range m.transfers {
		snapshot := transfer.snapshotValue()
		if snapshot.State != filedomain.TransferQueued && snapshot.State != filedomain.TransferRunning {
			completed = append(completed, completedTransfer{id: id, finished: snapshot.FinishedAt})
		}
	}
	sort.Slice(completed, func(left, right int) bool { return completed[left].finished < completed[right].finished })
	remove := len(m.transfers) - maxTransferHistory
	if remove > len(completed) {
		remove = len(completed)
	}
	for _, candidate := range completed[:remove] {
		delete(m.transfers, candidate.id)
	}
}

func (t *runtimeTransfer) snapshotValue() filedomain.Transfer {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.snapshot
}

func (t *runtimeTransfer) start() {
	t.mu.Lock()
	t.snapshot.State = filedomain.TransferRunning
	t.snapshot.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	t.mu.Unlock()
}

func (t *runtimeTransfer) setTotal(total int64) {
	t.mu.Lock()
	t.snapshot.Total = total
	t.mu.Unlock()
}

func (t *runtimeTransfer) addBytes(count int64) {
	t.mu.Lock()
	t.snapshot.Bytes += count
	t.mu.Unlock()
}

func (t *runtimeTransfer) finish(state filedomain.TransferState, message string) {
	t.mu.Lock()
	t.snapshot.State = state
	t.snapshot.Message = message
	t.snapshot.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	t.mu.Unlock()
}

func newID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate file operation id: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}
