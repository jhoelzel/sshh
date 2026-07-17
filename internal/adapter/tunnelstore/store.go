package tunnelstore

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

	"shh-h/internal/domain/tunnel"
)

const (
	filename      = "tunnels.json"
	currentSchema = 1
	directoryMode = 0o700
	fileMode      = 0o600
)

var ErrConflict = errors.New("tunnels changed outside shh-h; reload before saving")

type document struct {
	Version  int             `json:"version"`
	Revision uint64          `json:"revision"`
	Tunnels  []tunnel.Config `json:"tunnels"`
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

func (s *Store) LoadTunnels() ([]tunnel.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		s.loaded = true
		s.revision = 0
		s.hasFingerprint = false
		return []tunnel.Config{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read tunnels: %w", err)
	}
	var persisted document
	if err := decodeSingleJSON(data, &persisted); err != nil {
		return nil, fmt.Errorf("decode tunnels: %w", err)
	}
	if persisted.Version != currentSchema {
		return nil, fmt.Errorf("unsupported tunnels schema version %d", persisted.Version)
	}
	validated, err := validate(persisted.Tunnels, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	s.loaded = true
	s.revision = persisted.Revision
	s.fingerprint = sha256.Sum256(data)
	s.hasFingerprint = true
	return validated, nil
}

func (s *Store) SaveTunnels(configs []tunnel.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	validated, err := validate(configs, time.Now().UTC())
	if err != nil {
		return err
	}
	if !s.loaded {
		if _, err := os.Stat(s.path); err == nil {
			return ErrConflict
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect tunnels before saving: %w", err)
		}
		s.loaded = true
	}
	if err := s.ensureUnchanged(); err != nil {
		return err
	}

	next := document{Version: currentSchema, Revision: s.revision + 1, Tunnels: validated}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return fmt.Errorf("encode tunnels: %w", err)
	}
	data = append(data, '\n')
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, directoryMode); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".tunnels-*.json")
	if err != nil {
		return fmt.Errorf("create tunnels temp file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(fileMode); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect tunnels temp file: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write tunnels temp file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync tunnels temp file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close tunnels temp file: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("replace tunnels file: %w", err)
	}
	if err := os.Chmod(s.path, fileMode); err != nil {
		return fmt.Errorf("protect tunnels file: %w", err)
	}
	if err := syncDirectory(directory); err != nil {
		return fmt.Errorf("sync tunnels directory: %w", err)
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
			return fmt.Errorf("inspect tunnels: %w", err)
		}
		return nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrConflict
		}
		return fmt.Errorf("read tunnels before saving: %w", err)
	}
	if sha256.Sum256(data) != s.fingerprint {
		return ErrConflict
	}
	return nil
}

func validate(configs []tunnel.Config, now time.Time) ([]tunnel.Config, error) {
	result := append([]tunnel.Config{}, configs...)
	ids := make(map[string]struct{}, len(result))
	names := make(map[string]struct{}, len(result))
	for index := range result {
		result[index] = result[index].WithDefaults(now)
		if err := result[index].Validate(); err != nil {
			return nil, fmt.Errorf("tunnel %q: %w", result[index].ID, err)
		}
		if _, exists := ids[result[index].ID]; exists {
			return nil, fmt.Errorf("duplicate tunnel id %q", result[index].ID)
		}
		ids[result[index].ID] = struct{}{}
		name := strings.ToLower(result[index].Name)
		if _, exists := names[name]; exists {
			return nil, fmt.Errorf("duplicate tunnel name %q", result[index].Name)
		}
		names[name] = struct{}{}
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
