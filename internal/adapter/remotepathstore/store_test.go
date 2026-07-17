package remotepathstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"shh-h/internal/domain/remotepath"
)

func TestStoreRoundTripPermissionsAndConflict(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "config", "remote-path-favorites.json")
	store := NewAt(storePath)
	loaded, err := store.LoadFavorites()
	if err != nil || len(loaded) != 0 {
		t.Fatalf("load empty store: favorites=%#v err=%v", loaded, err)
	}
	favorites := []remotepath.Favorite{{ID: "favorite", ProfileID: "profile", Path: "/srv/app"}}
	if err := store.SaveFavorites(favorites); err != nil {
		t.Fatalf("save favorites: %v", err)
	}
	reloaded, err := NewAt(storePath).LoadFavorites()
	if err != nil || len(reloaded) != 1 || reloaded[0].Path != "/srv/app" {
		t.Fatalf("reload favorites: favorites=%#v err=%v", reloaded, err)
	}
	info, err := os.Stat(storePath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("favorite store permissions: info=%v err=%v", info, err)
	}
	data, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("read favorite store: %v", err)
	}
	if err := os.WriteFile(storePath, append(data, ' '), 0o600); err != nil {
		t.Fatalf("change favorite store externally: %v", err)
	}
	if err := store.SaveFavorites(favorites); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestStoreRejectsUnknownAndDuplicateFavorites(t *testing.T) {
	unknownPath := filepath.Join(t.TempDir(), "unknown.json")
	if err := os.WriteFile(unknownPath, []byte(`{"version":1,"revision":1,"favorites":[],"extra":true}`), 0o600); err != nil {
		t.Fatalf("write unknown fixture: %v", err)
	}
	if _, err := NewAt(unknownPath).LoadFavorites(); err == nil {
		t.Fatal("expected unknown field rejection")
	}

	store := NewAt(filepath.Join(t.TempDir(), "favorites.json"))
	if _, err := store.LoadFavorites(); err != nil {
		t.Fatalf("load empty store: %v", err)
	}
	duplicate := []remotepath.Favorite{
		{ID: "one", ProfileID: "profile", Path: "/srv/app"},
		{ID: "two", ProfileID: "profile", Path: "/srv/./app"},
	}
	if err := store.SaveFavorites(duplicate); err == nil {
		t.Fatal("expected canonical duplicate rejection")
	}
}
