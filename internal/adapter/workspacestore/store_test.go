package workspacestore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"shh-h/internal/domain/workspace"
)

func TestStoreRoundTripPermissionsAndConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "workspaces.json")
	store := NewAt(path)
	loaded, err := store.LoadLayouts()
	if err != nil || len(loaded) != 0 {
		t.Fatalf("load empty store: layouts=%#v err=%v", loaded, err)
	}
	layouts := []workspace.Layout{{
		ID:   "operations",
		Name: "Operations",
		Tabs: []workspace.Tab{{ProfileID: "profile-1", Title: "Production", Endpoint: "prod.example:22"}},
	}}
	if err := store.SaveLayouts(layouts); err != nil {
		t.Fatalf("save layouts: %v", err)
	}
	reloaded, err := NewAt(path).LoadLayouts()
	if err != nil || len(reloaded) != 1 || reloaded[0].Name != "Operations" {
		t.Fatalf("reload layouts: layouts=%#v err=%v", reloaded, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat layouts: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("unexpected layouts mode %o", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read layouts: %v", err)
	}
	if err := os.WriteFile(path, append(data, ' '), 0o600); err != nil {
		t.Fatalf("change layouts externally: %v", err)
	}
	if err := store.SaveLayouts(layouts); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestStoreRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspaces.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"revision":1,"layouts":[],"extra":true}`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := NewAt(path).LoadLayouts(); err == nil {
		t.Fatal("expected unknown field to be rejected")
	}
}
