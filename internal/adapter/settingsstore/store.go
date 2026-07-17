package settingsstore

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

	settingsdomain "shh-h/internal/domain/settings"
)

const (
	filename      = "settings.json"
	currentSchema = 3
	directoryMode = 0o700
	fileMode      = 0o600
)

var ErrConflict = errors.New("settings changed outside shh-h; reload before saving")

type document struct {
	Version  int                     `json:"version"`
	Revision uint64                  `json:"revision"`
	Settings settingsdomain.Settings `json:"settings"`
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

func (s *Store) LoadSettings() (settingsdomain.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		s.loaded = true
		s.revision = 0
		s.hasFingerprint = false
		defaults := settingsdomain.Defaults()
		if err := s.saveLocked(defaults); err != nil {
			return settingsdomain.Settings{}, err
		}
		return defaults, nil
	}
	if err != nil {
		return settingsdomain.Settings{}, fmt.Errorf("read settings: %w", err)
	}
	var persisted document
	if err := decodeSingleJSON(data, &persisted); err != nil {
		return settingsdomain.Settings{}, fmt.Errorf("decode settings: %w", err)
	}
	switch persisted.Version {
	case 1:
		persisted.Settings.Notifications = settingsdomain.Defaults().Notifications
		persisted.Settings.Transfers = settingsdomain.Defaults().Transfers
	case 2:
		persisted.Settings.Transfers = settingsdomain.Defaults().Transfers
	case currentSchema:
	default:
		return settingsdomain.Settings{}, fmt.Errorf("unsupported settings schema version %d", persisted.Version)
	}
	if err := persisted.Settings.Validate(); err != nil {
		return settingsdomain.Settings{}, fmt.Errorf("validate settings: %w", err)
	}
	s.loaded = true
	s.revision = persisted.Revision
	s.fingerprint = sha256.Sum256(data)
	s.hasFingerprint = true
	return persisted.Settings, nil
}

func (s *Store) SaveSettings(value settingsdomain.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := value.Validate(); err != nil {
		return err
	}
	if !s.loaded {
		if _, err := os.Stat(s.path); err == nil {
			return ErrConflict
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect settings before saving: %w", err)
		}
		s.loaded = true
	}
	return s.saveLocked(value)
}

func (s *Store) saveLocked(value settingsdomain.Settings) error {
	if err := s.ensureUnchanged(); err != nil {
		return err
	}
	next := document{Version: currentSchema, Revision: s.revision + 1, Settings: value}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	data = append(data, '\n')
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, directoryMode); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".settings-*.json")
	if err != nil {
		return fmt.Errorf("create settings temp file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(fileMode); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect settings temp file: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write settings temp file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync settings temp file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close settings temp file: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("replace settings file: %w", err)
	}
	if err := os.Chmod(s.path, fileMode); err != nil {
		return fmt.Errorf("protect settings file: %w", err)
	}
	if err := syncDirectory(directory); err != nil {
		return fmt.Errorf("sync settings directory: %w", err)
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
			return fmt.Errorf("inspect settings: %w", err)
		}
		return nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrConflict
		}
		return fmt.Errorf("read settings before saving: %w", err)
	}
	if sha256.Sum256(data) != s.fingerprint {
		return ErrConflict
	}
	return nil
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
