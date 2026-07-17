package transferstore

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"shh-h/internal/apperror"
	filedomain "shh-h/internal/domain/filetransfer"
)

const (
	filename      = "transfer-resumes.json"
	currentSchema = 1
	directoryMode = 0o700
	fileMode      = 0o600
	maxRecords    = 200
	maxPathBytes  = 4096
	maxErrorBytes = 4096
)

var ErrConflict = apperror.New(apperror.CodeConflict, "Transfer resume metadata changed outside shh-h; reload before saving.")

type document struct {
	Version  int                       `json:"version"`
	Revision uint64                    `json:"revision"`
	Resumes  []filedomain.ResumeRecord `json:"resumes"`
}

type Store struct {
	mu             sync.Mutex
	path           string
	revision       uint64
	fingerprint    [sha256.Size]byte
	hasFingerprint bool
	loaded         bool
}

func New(appID string) (*Store, error) {
	if strings.TrimSpace(appID) == "" {
		return nil, errors.New("app id is required")
	}
	directory, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user config directory: %w", err)
	}
	return NewAt(filepath.Join(directory, appID, filename)), nil
}

func NewAt(storePath string) *Store {
	return &Store{path: storePath}
}

func (store *Store) LoadResumes() ([]filedomain.ResumeRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	data, err := os.ReadFile(store.path)
	if errors.Is(err, os.ErrNotExist) {
		store.loaded = true
		store.revision = 0
		store.hasFingerprint = false
		return []filedomain.ResumeRecord{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read transfer resume metadata: %w", err)
	}
	var persisted document
	if err := decodeSingleJSON(data, &persisted); err != nil {
		return nil, fmt.Errorf("decode transfer resume metadata: %w", err)
	}
	if persisted.Version != currentSchema {
		return nil, fmt.Errorf("unsupported transfer resume schema version %d", persisted.Version)
	}
	validated, err := validateRecords(persisted.Resumes)
	if err != nil {
		return nil, err
	}
	store.loaded = true
	store.revision = persisted.Revision
	store.fingerprint = sha256.Sum256(data)
	store.hasFingerprint = true
	return validated, nil
}

func (store *Store) SaveResumes(records []filedomain.ResumeRecord) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	validated, err := validateRecords(records)
	if err != nil {
		return err
	}
	if !store.loaded {
		if _, err := os.Stat(store.path); err == nil {
			return ErrConflict
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect transfer resume metadata before saving: %w", err)
		}
		store.loaded = true
	}
	if err := store.ensureUnchanged(); err != nil {
		return err
	}

	next := document{Version: currentSchema, Revision: store.revision + 1, Resumes: validated}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return fmt.Errorf("encode transfer resume metadata: %w", err)
	}
	data = append(data, '\n')
	directory := filepath.Dir(store.path)
	if err := os.MkdirAll(directory, directoryMode); err != nil {
		return fmt.Errorf("create transfer resume directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".transfer-resumes-*.json")
	if err != nil {
		return fmt.Errorf("create transfer resume temp file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(fileMode); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect transfer resume temp file: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write transfer resume temp file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync transfer resume temp file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close transfer resume temp file: %w", err)
	}
	if err := os.Rename(temporaryPath, store.path); err != nil {
		return fmt.Errorf("replace transfer resume metadata: %w", err)
	}
	if err := os.Chmod(store.path, fileMode); err != nil {
		return fmt.Errorf("protect transfer resume metadata: %w", err)
	}
	if err := syncDirectory(directory); err != nil {
		return fmt.Errorf("sync transfer resume directory: %w", err)
	}
	store.revision = next.Revision
	store.fingerprint = sha256.Sum256(data)
	store.hasFingerprint = true
	return nil
}

func (store *Store) ensureUnchanged() error {
	if !store.hasFingerprint {
		if _, err := os.Stat(store.path); err == nil {
			return ErrConflict
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect transfer resume metadata: %w", err)
		}
		return nil
	}
	data, err := os.ReadFile(store.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrConflict
		}
		return fmt.Errorf("read transfer resume metadata before saving: %w", err)
	}
	if sha256.Sum256(data) != store.fingerprint {
		return ErrConflict
	}
	return nil
}

func validateRecords(records []filedomain.ResumeRecord) ([]filedomain.ResumeRecord, error) {
	if len(records) > maxRecords {
		return nil, fmt.Errorf("transfer resume metadata exceeds the %d item limit", maxRecords)
	}
	result := append([]filedomain.ResumeRecord(nil), records...)
	ids := make(map[string]struct{}, len(result))
	for index, record := range result {
		if err := validateRecord(record); err != nil {
			return nil, fmt.Errorf("transfer resume %q: %w", record.ID, err)
		}
		if _, exists := ids[record.ID]; exists {
			return nil, fmt.Errorf("duplicate transfer resume id %q", record.ID)
		}
		ids[record.ID] = struct{}{}
		result[index] = record
	}
	sort.Slice(result, func(left, right int) bool { return result[left].ID < result[right].ID })
	return result, nil
}

func validateRecord(record filedomain.ResumeRecord) error {
	decoded, err := hex.DecodeString(record.ID)
	if err != nil || len(decoded) != 16 || record.ID != strings.ToLower(record.ID) {
		return errors.New("id must be 32 lowercase hexadecimal characters")
	}
	if err := validateText("profile id", record.ProfileID, 256); err != nil {
		return err
	}
	if record.Direction != filedomain.DirectionDownload && record.Direction != filedomain.DirectionUpload {
		return fmt.Errorf("unsupported direction %q", record.Direction)
	}
	for label, value := range map[string]string{
		"source": record.Source, "destination": record.Destination, "partial path": record.PartialPath,
	} {
		if err := validateText(label, value, maxPathBytes); err != nil {
			return err
		}
	}
	if record.Total < 0 || record.SourceSize != record.Total {
		return errors.New("source size and total must match and cannot be negative")
	}
	if record.Bytes < 0 || record.Bytes > record.Total {
		return errors.New("resume byte count is outside the source size")
	}
	if _, err := time.Parse(time.RFC3339Nano, record.SourceModifiedAt); err != nil {
		return errors.New("source modification time is invalid")
	}
	created, err := time.Parse(time.RFC3339Nano, record.CreatedAt)
	if err != nil {
		return errors.New("creation time is invalid")
	}
	updated, err := time.Parse(time.RFC3339Nano, record.UpdatedAt)
	if err != nil || updated.Before(created) {
		return errors.New("update time is invalid")
	}
	if len(record.LastError) > maxErrorBytes {
		return fmt.Errorf("last error exceeds %d bytes", maxErrorBytes)
	}

	if record.Direction == filedomain.DirectionDownload {
		if !path.IsAbs(record.Source) || !filepath.IsAbs(record.Destination) || !filepath.IsAbs(record.PartialPath) {
			return errors.New("download paths must be absolute")
		}
		if path.Clean(record.Source) != record.Source || filepath.Clean(record.Destination) != record.Destination || filepath.Clean(record.PartialPath) != record.PartialPath {
			return errors.New("download paths must be canonical")
		}
		if record.SourceSHA256 != "" {
			return errors.New("download metadata must not contain a source digest")
		}
		expected := localPartialPath(record.Destination, record.ID)
		if filepath.Clean(record.PartialPath) != expected {
			return errors.New("download partial path does not match its destination and id")
		}
		return nil
	}

	if !filepath.IsAbs(record.Source) || !path.IsAbs(record.Destination) || !path.IsAbs(record.PartialPath) {
		return errors.New("upload paths must be absolute")
	}
	if filepath.Clean(record.Source) != record.Source || path.Clean(record.Destination) != record.Destination || path.Clean(record.PartialPath) != record.PartialPath {
		return errors.New("upload paths must be canonical")
	}
	if len(record.SourceSHA256) != sha256.Size*2 {
		return errors.New("upload source digest must be a SHA-256 value")
	}
	if _, err := hex.DecodeString(record.SourceSHA256); err != nil || record.SourceSHA256 != strings.ToLower(record.SourceSHA256) {
		return errors.New("upload source digest must be lowercase hexadecimal")
	}
	expected := remotePartialPath(record.Destination, record.ID)
	if path.Clean(record.PartialPath) != expected {
		return errors.New("upload partial path does not match its destination and id")
	}
	return nil
}

func validateText(label, value string, maximum int) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", label)
	}
	if len(value) > maximum {
		return fmt.Errorf("%s exceeds %d bytes", label, maximum)
	}
	for _, character := range value {
		if character == 0 || unicode.IsControl(character) {
			return fmt.Errorf("%s contains control characters", label)
		}
	}
	return nil
}

func localPartialPath(destination, id string) string {
	return filepath.Clean(filepath.Join(filepath.Dir(destination), "."+filepath.Base(destination)+".shhh-part-"+id))
}

func remotePartialPath(destination, id string) string {
	return path.Clean(path.Join(path.Dir(destination), "."+path.Base(destination)+".shhh-part-"+id))
}

func decodeSingleJSON(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("unexpected trailing JSON value")
}

func syncDirectory(directoryPath string) error {
	directory, err := os.Open(directoryPath)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
