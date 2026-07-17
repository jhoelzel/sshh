package configstore

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
	"shh-h/internal/domain/profile"
)

const (
	profilesFilename    = "profiles.json"
	currentSchema       = 1
	legacyBackupSuffix  = ".v0.bak"
	configDirectoryMode = 0o700
	configFileMode      = 0o600
)

var ErrConflict = apperror.New(apperror.CodeConflict, "Profiles changed outside shh-h; reload before saving.")

type document struct {
	Version  int               `json:"version"`
	Revision uint64            `json:"revision"`
	Profiles []profile.Profile `json:"profiles"`
}

type Store struct {
	mu             sync.Mutex
	profilesPath   string
	revision       uint64
	fingerprint    [sha256.Size]byte
	hasFingerprint bool
	loaded         bool
}

func New(appID string) (*Store, error) {
	if appID == "" {
		return nil, errors.New("app id is required")
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user config directory: %w", err)
	}

	return NewAt(filepath.Join(configDir, appID, profilesFilename)), nil
}

func NewAt(path string) *Store {
	return &Store{profilesPath: path}
}

func (s *Store) LoadProfiles() ([]profile.Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.profilesPath)
	if errors.Is(err, os.ErrNotExist) {
		profiles := defaultProfiles(time.Now())
		s.loaded = true
		s.revision = 0
		s.hasFingerprint = false
		if err := s.saveProfilesLocked(profiles); err != nil {
			return nil, err
		}
		return cloneProfiles(profiles), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read profiles: %w", err)
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, errors.New("decode profiles: empty configuration")
	}

	if trimmed[0] == '[' {
		var profiles []profile.Profile
		if err := decodeSingleJSON(data, &profiles); err != nil {
			return nil, fmt.Errorf("decode legacy profiles: %w", err)
		}
		profiles, err = validateProfiles(profiles, time.Now())
		if err != nil {
			return nil, err
		}
		if err := s.backupLegacy(data); err != nil {
			return nil, err
		}
		s.loaded = true
		s.revision = 0
		s.fingerprint = sha256.Sum256(data)
		s.hasFingerprint = true
		if err := s.saveProfilesLocked(profiles); err != nil {
			return nil, fmt.Errorf("migrate legacy profiles: %w", err)
		}
		return cloneProfiles(profiles), nil
	}

	var persisted document
	if err := decodeSingleJSON(data, &persisted); err != nil {
		return nil, fmt.Errorf("decode profiles: %w", err)
	}
	if persisted.Version != currentSchema {
		return nil, fmt.Errorf("unsupported profiles schema version %d", persisted.Version)
	}
	profiles, err := validateProfiles(persisted.Profiles, time.Now())
	if err != nil {
		return nil, err
	}

	s.loaded = true
	s.revision = persisted.Revision
	s.fingerprint = sha256.Sum256(data)
	s.hasFingerprint = true
	return cloneProfiles(profiles), nil
}

func (s *Store) SaveProfiles(profiles []profile.Profile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	validated, err := validateProfiles(profiles, time.Now())
	if err != nil {
		return err
	}
	if !s.loaded {
		if _, err := os.Stat(s.profilesPath); err == nil {
			return ErrConflict
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect profiles before saving: %w", err)
		}
		s.loaded = true
	}
	return s.saveProfilesLocked(validated)
}

func (s *Store) saveProfilesLocked(profiles []profile.Profile) error {
	if err := s.ensureUnchanged(); err != nil {
		return err
	}

	next := document{Version: currentSchema, Revision: s.revision + 1, Profiles: profiles}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return fmt.Errorf("encode profiles: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(s.profilesPath)
	if err := os.MkdirAll(dir, configDirectoryMode); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	tempFile, err := os.CreateTemp(dir, ".profiles-*.json")
	if err != nil {
		return fmt.Errorf("create temp profiles file: %w", err)
	}
	tempName := tempFile.Name()
	defer os.Remove(tempName)

	if err := tempFile.Chmod(configFileMode); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("protect profiles temp file: %w", err)
	}
	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("write profiles temp file: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("sync profiles temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close profiles temp file: %w", err)
	}
	if err := os.Rename(tempName, s.profilesPath); err != nil {
		return fmt.Errorf("replace profiles file: %w", err)
	}
	if err := os.Chmod(s.profilesPath, configFileMode); err != nil {
		return fmt.Errorf("protect profiles file: %w", err)
	}
	if err := syncDirectory(dir); err != nil {
		return fmt.Errorf("sync profiles directory: %w", err)
	}

	s.revision = next.Revision
	s.fingerprint = sha256.Sum256(data)
	s.hasFingerprint = true
	return nil
}

func (s *Store) ensureUnchanged() error {
	if !s.hasFingerprint {
		if _, err := os.Stat(s.profilesPath); err == nil {
			return ErrConflict
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect profiles: %w", err)
		}
		return nil
	}

	current, err := os.ReadFile(s.profilesPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrConflict
		}
		return fmt.Errorf("read profiles before saving: %w", err)
	}
	if sha256.Sum256(current) != s.fingerprint {
		return ErrConflict
	}
	return nil
}

func (s *Store) backupLegacy(data []byte) error {
	backupPath := s.profilesPath + legacyBackupSuffix
	file, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, configFileMode)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create legacy profiles backup: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write legacy profiles backup: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync legacy profiles backup: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close legacy profiles backup: %w", err)
	}
	return nil
}

func validateProfiles(profiles []profile.Profile, now time.Time) ([]profile.Profile, error) {
	result := cloneProfiles(profiles)
	ids := make(map[string]struct{}, len(result))
	names := make(map[string]struct{}, len(result))
	for i := range result {
		result[i] = result[i].WithDefaults(now)
		if err := result[i].Validate(); err != nil {
			return nil, fmt.Errorf("profile %q: %w", result[i].ID, err)
		}
		if _, exists := ids[result[i].ID]; exists {
			return nil, fmt.Errorf("duplicate profile id %q", result[i].ID)
		}
		ids[result[i].ID] = struct{}{}
		nameKey := strings.ToLower(result[i].Name)
		if _, exists := names[nameKey]; exists {
			return nil, fmt.Errorf("duplicate profile name %q", result[i].Name)
		}
		names[nameKey] = struct{}{}
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

func cloneProfiles(profiles []profile.Profile) []profile.Profile {
	result := make([]profile.Profile, len(profiles))
	for i, item := range profiles {
		result[i] = item
		result[i].Arguments = append([]string(nil), item.Arguments...)
		result[i].Tags = append([]string(nil), item.Tags...)
		if item.Environment != nil {
			result[i].Environment = make(map[string]string, len(item.Environment))
			for key, value := range item.Environment {
				result[i].Environment[key] = value
			}
		}
	}
	return result
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func defaultProfiles(now time.Time) []profile.Profile {
	return []profile.Profile{
		{
			ID:        "local-shell",
			Name:      "Local Shell",
			Protocol:  profile.ProtocolLocal,
			Tags:      []string{"local"},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
}
