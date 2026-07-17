package configstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"shh-h/internal/domain/profile"
)

func TestLoadProfilesCreatesLocalDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	store := NewAt(path)

	profiles, err := store.LoadProfiles()
	if err != nil {
		t.Fatalf("load profiles: %v", err)
	}
	if len(profiles) != 1 || profiles[0].Protocol != profile.ProtocolLocal {
		t.Fatalf("expected one local default profile, got %#v", profiles)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read created config: %v", err)
	}
	var persisted document
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("decode created config: %v", err)
	}
	if persisted.Version != currentSchema || persisted.Revision != 1 {
		t.Fatalf("unexpected created document: %#v", persisted)
	}
}

func TestSaveProfilesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	store := NewAt(path)
	profiles := []profile.Profile{
		{ID: "prod", Name: "Production", Protocol: profile.ProtocolSSH, Host: "prod.example.com", Username: "deploy"},
	}

	if err := store.SaveProfiles(profiles); err != nil {
		t.Fatalf("save profiles: %v", err)
	}

	loaded, err := store.LoadProfiles()
	if err != nil {
		t.Fatalf("load profiles: %v", err)
	}
	if got := loaded[0].Endpoint(); got != "deploy@prod.example.com:22" {
		t.Fatalf("unexpected endpoint: %q", got)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat profiles: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected private profile permissions, got %o", info.Mode().Perm())
	}
}

func TestLoadProfilesRejectsTrailingJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	if err := os.WriteFile(path, []byte("[]\n{}\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if _, err := NewAt(path).LoadProfiles(); err == nil {
		t.Fatal("expected trailing JSON to be rejected")
	}
}

func TestLoadProfilesMigratesLegacyArrayWithBackup(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "profiles.json")
	legacy := []byte(`[{"id":"local","name":"Local","protocol":"local","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`)
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatalf("write legacy fixture: %v", err)
	}

	profiles, err := NewAt(path).LoadProfiles()
	if err != nil {
		t.Fatalf("migrate profiles: %v", err)
	}
	if len(profiles) != 1 || profiles[0].ID != "local" {
		t.Fatalf("unexpected migrated profiles: %#v", profiles)
	}
	backup, err := os.ReadFile(path + legacyBackupSuffix)
	if err != nil {
		t.Fatalf("read migration backup: %v", err)
	}
	if string(backup) != string(legacy) {
		t.Fatal("migration backup does not match the source document")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migrated config: %v", err)
	}
	var persisted document
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("decode migrated config: %v", err)
	}
	if persisted.Version != currentSchema || persisted.Revision != 1 {
		t.Fatalf("unexpected migrated document: %#v", persisted)
	}
}

func TestSaveProfilesRejectsExternalChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	store := NewAt(path)
	profiles, err := store.LoadProfiles()
	if err != nil {
		t.Fatalf("load profiles: %v", err)
	}
	if err := os.WriteFile(path, []byte("externally changed\n"), 0o600); err != nil {
		t.Fatalf("write external change: %v", err)
	}
	if err := store.SaveProfiles(profiles); err != ErrConflict {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestSaveProfilesRejectsDuplicateNames(t *testing.T) {
	store := NewAt(filepath.Join(t.TempDir(), "profiles.json"))
	err := store.SaveProfiles([]profile.Profile{
		{ID: "one", Name: "Production", Protocol: profile.ProtocolLocal},
		{ID: "two", Name: " production ", Protocol: profile.ProtocolLocal},
	})
	if err == nil {
		t.Fatal("expected duplicate profile names to be rejected")
	}
}
