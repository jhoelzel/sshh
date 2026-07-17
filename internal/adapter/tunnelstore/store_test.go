package tunnelstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"shh-h/internal/domain/tunnel"
)

func TestStoreRoundTripAndConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "tunnels.json")
	store := NewAt(path)
	loaded, err := store.LoadTunnels()
	if err != nil || len(loaded) != 0 {
		t.Fatalf("load empty store: configs=%#v err=%v", loaded, err)
	}
	configs := []tunnel.Config{{
		ID: "forward", Name: "Database", ProfileID: "server", Kind: tunnel.KindLocal,
		BindAddress: "127.0.0.1", BindPort: 0, DestinationHost: "db.internal", DestinationPort: 5432,
	}}
	if err := store.SaveTunnels(configs); err != nil {
		t.Fatalf("save tunnels: %v", err)
	}
	reloaded, err := NewAt(path).LoadTunnels()
	if err != nil || len(reloaded) != 1 || reloaded[0].Name != "Database" {
		t.Fatalf("reload tunnels: configs=%#v err=%v", reloaded, err)
	}
	if mode := fileModeOf(t, path); mode != 0o600 {
		t.Fatalf("unexpected tunnel file mode: %o", mode)
	}
	if err := os.WriteFile(path, append(mustRead(t, path), ' '), 0o600); err != nil {
		t.Fatalf("change tunnels externally: %v", err)
	}
	if err := store.SaveTunnels(configs); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestStoreRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tunnels.json")
	data := []byte(`{"version":1,"revision":1,"tunnels":[],"surprise":true}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := NewAt(path).LoadTunnels(); err == nil {
		t.Fatal("expected unknown field to be rejected")
	}
}

func fileModeOf(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Mode().Perm()
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}
