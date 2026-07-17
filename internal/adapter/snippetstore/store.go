package snippetstore

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

	"shh-h/internal/domain/snippet"
)

const (
	filename      = "snippets.json"
	currentSchema = 1
	directoryMode = 0o700
	fileMode      = 0o600
)

var ErrConflict = errors.New("snippets changed outside shh-h; reload before saving")

type document struct {
	Version  int               `json:"version"`
	Revision uint64            `json:"revision"`
	Snippets []snippet.Snippet `json:"snippets"`
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

func (s *Store) LoadSnippets() ([]snippet.Snippet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		s.loaded = true
		s.revision = 0
		s.hasFingerprint = false
		return []snippet.Snippet{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read snippets: %w", err)
	}
	var persisted document
	if err := decodeSingleJSON(data, &persisted); err != nil {
		return nil, fmt.Errorf("decode snippets: %w", err)
	}
	if persisted.Version != currentSchema {
		return nil, fmt.Errorf("unsupported snippets schema version %d", persisted.Version)
	}
	validated, err := validate(persisted.Snippets, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	s.loaded = true
	s.revision = persisted.Revision
	s.fingerprint = sha256.Sum256(data)
	s.hasFingerprint = true
	return validated, nil
}

func (s *Store) SaveSnippets(snippets []snippet.Snippet) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	validated, err := validate(snippets, time.Now().UTC())
	if err != nil {
		return err
	}
	if !s.loaded {
		if _, err := os.Stat(s.path); err == nil {
			return ErrConflict
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect snippets before saving: %w", err)
		}
		s.loaded = true
	}
	if err := s.ensureUnchanged(); err != nil {
		return err
	}

	next := document{Version: currentSchema, Revision: s.revision + 1, Snippets: validated}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return fmt.Errorf("encode snippets: %w", err)
	}
	data = append(data, '\n')
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, directoryMode); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".snippets-*.json")
	if err != nil {
		return fmt.Errorf("create snippets temp file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(fileMode); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect snippets temp file: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write snippets temp file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync snippets temp file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close snippets temp file: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("replace snippets file: %w", err)
	}
	if err := os.Chmod(s.path, fileMode); err != nil {
		return fmt.Errorf("protect snippets file: %w", err)
	}
	if err := syncDirectory(directory); err != nil {
		return fmt.Errorf("sync snippets directory: %w", err)
	}
	s.revision = next.Revision
	s.fingerprint = sha256.Sum256(data)
	s.hasFingerprint = true
	return nil
}

func (s *Store) ensureUnchanged() error {
	if !s.hasFingerprint {
		if _, err := os.Stat(s.path); err == nil {
			return ErrConflict
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect snippets: %w", err)
		}
		return nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrConflict
		}
		return fmt.Errorf("read snippets before saving: %w", err)
	}
	if sha256.Sum256(data) != s.fingerprint {
		return ErrConflict
	}
	return nil
}

func validate(items []snippet.Snippet, now time.Time) ([]snippet.Snippet, error) {
	result := make([]snippet.Snippet, len(items))
	ids := make(map[string]struct{}, len(items))
	names := make(map[string]struct{}, len(items))
	for index, item := range items {
		item.Tags = append([]string(nil), item.Tags...)
		item = item.WithDefaults(now)
		if err := item.Validate(); err != nil {
			return nil, fmt.Errorf("snippet %q: %w", item.ID, err)
		}
		if _, exists := ids[item.ID]; exists {
			return nil, fmt.Errorf("duplicate snippet id %q", item.ID)
		}
		ids[item.ID] = struct{}{}
		nameKey := strings.ToLower(item.Name)
		if _, exists := names[nameKey]; exists {
			return nil, fmt.Errorf("duplicate snippet name %q", item.Name)
		}
		names[nameKey] = struct{}{}
		result[index] = item
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
