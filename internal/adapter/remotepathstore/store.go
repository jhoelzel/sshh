package remotepathstore

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

	"shh-h/internal/domain/remotepath"
)

const (
	filename      = "remote-path-favorites.json"
	currentSchema = 1
	directoryMode = 0o700
	fileMode      = 0o600
)

var ErrConflict = errors.New("remote path favorites changed outside shh-h; reload before saving")

type document struct {
	Version   int                   `json:"version"`
	Revision  uint64                `json:"revision"`
	Favorites []remotepath.Favorite `json:"favorites"`
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

func (store *Store) LoadFavorites() ([]remotepath.Favorite, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	data, err := os.ReadFile(store.path)
	if errors.Is(err, os.ErrNotExist) {
		store.loaded = true
		store.revision = 0
		store.hasFingerprint = false
		return []remotepath.Favorite{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read remote path favorites: %w", err)
	}
	var persisted document
	if err := decodeSingleJSON(data, &persisted); err != nil {
		return nil, fmt.Errorf("decode remote path favorites: %w", err)
	}
	if persisted.Version != currentSchema {
		return nil, fmt.Errorf("unsupported remote path favorites schema version %d", persisted.Version)
	}
	validated, err := validate(persisted.Favorites, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	store.loaded = true
	store.revision = persisted.Revision
	store.fingerprint = sha256.Sum256(data)
	store.hasFingerprint = true
	return validated, nil
}

func (store *Store) SaveFavorites(favorites []remotepath.Favorite) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	validated, err := validate(favorites, time.Now().UTC())
	if err != nil {
		return err
	}
	if !store.loaded {
		if _, err := os.Stat(store.path); err == nil {
			return ErrConflict
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect remote path favorites before saving: %w", err)
		}
		store.loaded = true
	}
	if err := store.ensureUnchanged(); err != nil {
		return err
	}

	next := document{Version: currentSchema, Revision: store.revision + 1, Favorites: validated}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return fmt.Errorf("encode remote path favorites: %w", err)
	}
	data = append(data, '\n')
	directory := filepath.Dir(store.path)
	if err := os.MkdirAll(directory, directoryMode); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".remote-path-favorites-*.json")
	if err != nil {
		return fmt.Errorf("create remote path favorites temp file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(fileMode); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect remote path favorites temp file: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write remote path favorites temp file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync remote path favorites temp file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close remote path favorites temp file: %w", err)
	}
	if err := os.Rename(temporaryPath, store.path); err != nil {
		return fmt.Errorf("replace remote path favorites file: %w", err)
	}
	if err := os.Chmod(store.path, fileMode); err != nil {
		return fmt.Errorf("protect remote path favorites file: %w", err)
	}
	if err := syncDirectory(directory); err != nil {
		return fmt.Errorf("sync remote path favorites directory: %w", err)
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
			return fmt.Errorf("inspect remote path favorites: %w", err)
		}
		return nil
	}
	data, err := os.ReadFile(store.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrConflict
		}
		return fmt.Errorf("read remote path favorites before saving: %w", err)
	}
	if sha256.Sum256(data) != store.fingerprint {
		return ErrConflict
	}
	return nil
}

func validate(favorites []remotepath.Favorite, now time.Time) ([]remotepath.Favorite, error) {
	if len(favorites) > remotepath.MaxFavorites {
		return nil, fmt.Errorf("remote path favorites exceed the %d item limit", remotepath.MaxFavorites)
	}
	result := make([]remotepath.Favorite, len(favorites))
	ids := make(map[string]struct{}, len(favorites))
	paths := make(map[string]struct{}, len(favorites))
	for index, favorite := range favorites {
		favorite = favorite.WithDefaults(now)
		if err := favorite.Validate(); err != nil {
			return nil, fmt.Errorf("remote path favorite %q: %w", favorite.ID, err)
		}
		if _, exists := ids[favorite.ID]; exists {
			return nil, fmt.Errorf("duplicate remote path favorite id %q", favorite.ID)
		}
		ids[favorite.ID] = struct{}{}
		pathKey := favorite.ProfileID + "\x00" + favorite.Path
		if _, exists := paths[pathKey]; exists {
			return nil, fmt.Errorf("duplicate remote path favorite for profile %q and path %q", favorite.ProfileID, favorite.Path)
		}
		paths[pathKey] = struct{}{}
		result[index] = favorite
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
