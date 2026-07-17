package workspacestore

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"shh-h/internal/apperror"
	"shh-h/internal/domain/workspace"
)

const (
	filename      = "workspaces.json"
	currentSchema = 1
	directoryMode = 0o700
	fileMode      = 0o600
)

var ErrConflict = apperror.New(apperror.CodeConflict, "Workspace layouts changed outside shh-h; reload before saving.")

type document struct {
	Version  int                `json:"version"`
	Revision uint64             `json:"revision"`
	Layouts  []workspace.Layout `json:"layouts"`
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

func NewAt(path string) *Store {
	return &Store{path: path}
}

func (store *Store) LoadLayouts() ([]workspace.Layout, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	data, err := os.ReadFile(store.path)
	if errors.Is(err, os.ErrNotExist) {
		store.loaded = true
		store.revision = 0
		store.hasFingerprint = false
		return []workspace.Layout{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read workspace layouts: %w", err)
	}
	var persisted document
	if err := decodeSingleJSON(data, &persisted); err != nil {
		return nil, fmt.Errorf("decode workspace layouts: %w", err)
	}
	if persisted.Version != currentSchema {
		return nil, fmt.Errorf("unsupported workspace layouts schema version %d", persisted.Version)
	}
	validated, err := validate(persisted.Layouts, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	store.loaded = true
	store.revision = persisted.Revision
	store.fingerprint = sha256.Sum256(data)
	store.hasFingerprint = true
	return validated, nil
}

func (store *Store) SaveLayouts(layouts []workspace.Layout) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	validated, err := validate(layouts, time.Now().UTC())
	if err != nil {
		return err
	}
	if !store.loaded {
		if _, err := os.Stat(store.path); err == nil {
			return ErrConflict
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect workspace layouts before saving: %w", err)
		}
		store.loaded = true
	}
	if err := store.ensureUnchanged(); err != nil {
		return err
	}

	next := document{Version: currentSchema, Revision: store.revision + 1, Layouts: validated}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return fmt.Errorf("encode workspace layouts: %w", err)
	}
	data = append(data, '\n')
	directory := filepath.Dir(store.path)
	if err := os.MkdirAll(directory, directoryMode); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".workspaces-*.json")
	if err != nil {
		return fmt.Errorf("create workspace layouts temp file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(fileMode); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect workspace layouts temp file: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write workspace layouts temp file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync workspace layouts temp file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close workspace layouts temp file: %w", err)
	}
	if err := os.Rename(temporaryPath, store.path); err != nil {
		return fmt.Errorf("replace workspace layouts file: %w", err)
	}
	if err := os.Chmod(store.path, fileMode); err != nil {
		return fmt.Errorf("protect workspace layouts file: %w", err)
	}
	if err := syncDirectory(directory); err != nil {
		return fmt.Errorf("sync workspace layouts directory: %w", err)
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
			return fmt.Errorf("inspect workspace layouts: %w", err)
		}
		return nil
	}
	data, err := os.ReadFile(store.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrConflict
		}
		return fmt.Errorf("read workspace layouts before saving: %w", err)
	}
	if sha256.Sum256(data) != store.fingerprint {
		return ErrConflict
	}
	return nil
}

func validate(layouts []workspace.Layout, now time.Time) ([]workspace.Layout, error) {
	result := make([]workspace.Layout, len(layouts))
	ids := make(map[string]struct{}, len(layouts))
	names := make(map[string]struct{}, len(layouts))
	for index, layout := range layouts {
		layout = layout.WithDefaults(now)
		if err := layout.Validate(); err != nil {
			return nil, fmt.Errorf("workspace layout %q: %w", layout.ID, err)
		}
		if _, exists := ids[layout.ID]; exists {
			return nil, fmt.Errorf("duplicate workspace layout id %q", layout.ID)
		}
		ids[layout.ID] = struct{}{}
		nameKey := strings.ToLower(layout.Name)
		if _, exists := names[nameKey]; exists {
			return nil, fmt.Errorf("duplicate workspace layout name %q", layout.Name)
		}
		names[nameKey] = struct{}{}
		result[index] = layout
	}
	return result, nil
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

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
