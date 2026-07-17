package settingsstore

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	settingsdomain "shh-h/internal/domain/settings"
)

func TestStoreCreatesDefaultsAndRoundTripsPrivately(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "settings.json")
	store := NewAt(path)
	loaded, err := store.LoadSettings()
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if loaded != settingsdomain.Defaults() {
		t.Fatalf("unexpected defaults: %#v", loaded)
	}
	loaded.Terminal.FontSize = 16
	if err := store.SaveSettings(loaded); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	reloaded, err := NewAt(path).LoadSettings()
	if err != nil || reloaded.Terminal.FontSize != 16 {
		t.Fatalf("reload settings: value=%#v err=%v", reloaded, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("settings permissions: info=%v err=%v", info, err)
	}
}

func TestStoreRejectsExternalChangesAndUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store := NewAt(path)
	value, err := store.LoadSettings()
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if err := os.WriteFile(path, append(data, ' '), 0o600); err != nil {
		t.Fatalf("change settings: %v", err)
	}
	if err := store.SaveSettings(value); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}

	unknownPath := filepath.Join(t.TempDir(), "unknown.json")
	fixture := []byte(`{"version":2,"revision":1,"settings":{"terminal":{"fontFamily":"system-mono","fontSize":13,"lineHeight":1.2,"cursorStyle":"block","cursorBlink":true,"scrollback":10000,"bell":true,"extra":1},"notifications":{"enabled":false,"transferCompleted":true,"unexpectedDisconnect":true,"longTransferSeconds":30}}}`)
	if err := os.WriteFile(unknownPath, fixture, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := NewAt(unknownPath).LoadSettings(); err == nil {
		t.Fatal("expected unknown field rejection")
	}
}

func TestStoreMigratesVersionOneNotificationDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	fixture := []byte(`{"version":1,"revision":7,"settings":{"terminal":{"fontFamily":"system-mono","fontSize":13,"lineHeight":1.2,"cursorStyle":"block","cursorBlink":true,"scrollback":10000,"bell":true}}}`)
	if err := os.WriteFile(path, fixture, 0o600); err != nil {
		t.Fatalf("write version one fixture: %v", err)
	}
	store := NewAt(path)
	loaded, err := store.LoadSettings()
	if err != nil {
		t.Fatalf("load version one settings: %v", err)
	}
	if loaded.Notifications != settingsdomain.Defaults().Notifications {
		t.Fatalf("unexpected migrated notifications: %#v", loaded.Notifications)
	}
	loaded.Notifications.Enabled = true
	if err := store.SaveSettings(loaded); err != nil {
		t.Fatalf("save migrated settings: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migrated settings: %v", err)
	}
	if !bytes.Contains(data, []byte(`"version": 2`)) {
		t.Fatalf("settings were not upgraded to version 2: %s", data)
	}
}
