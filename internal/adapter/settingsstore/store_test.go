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
	loaded.Connection.ConnectTimeoutSeconds = 25
	loaded.UI.Theme = settingsdomain.ThemeLight
	loaded.UI.SidebarWidth = 316
	loaded.UI.Workspace = settingsdomain.WorkspaceActivity
	loaded.Window = settingsdomain.WindowState{
		X: 80, Y: 45, Width: 1360, Height: 840, Positioned: true, Maximized: true,
	}
	if err := store.SaveSettings(loaded); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	reloaded, err := NewAt(path).LoadSettings()
	if err != nil || reloaded != loaded {
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
	fixture := []byte(`{"version":3,"revision":1,"settings":{"terminal":{"fontFamily":"system-mono","fontSize":13,"lineHeight":1.2,"cursorStyle":"block","cursorBlink":true,"scrollback":10000,"bell":true,"extra":1},"notifications":{"enabled":false,"transferCompleted":true,"unexpectedDisconnect":true,"longTransferSeconds":30},"transfers":{"concurrency":2,"collisionPolicy":"ask","keepPartialFiles":false}}}`)
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
	if loaded.Transfers != settingsdomain.Defaults().Transfers {
		t.Fatalf("unexpected migrated transfers: %#v", loaded.Transfers)
	}
	loaded.Notifications.Enabled = true
	if err := store.SaveSettings(loaded); err != nil {
		t.Fatalf("save migrated settings: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migrated settings: %v", err)
	}
	if !bytes.Contains(data, []byte(`"version": 5`)) {
		t.Fatalf("settings were not upgraded to version 5: %s", data)
	}
}

func TestStoreMigratesVersionTwoTransferDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	fixture := []byte(`{"version":2,"revision":9,"settings":{"terminal":{"fontFamily":"system-mono","fontSize":13,"lineHeight":1.2,"cursorStyle":"block","cursorBlink":true,"scrollback":10000,"bell":true},"notifications":{"enabled":true,"transferCompleted":false,"unexpectedDisconnect":true,"longTransferSeconds":45}}}`)
	if err := os.WriteFile(path, fixture, 0o600); err != nil {
		t.Fatalf("write version two fixture: %v", err)
	}
	loaded, err := NewAt(path).LoadSettings()
	if err != nil {
		t.Fatalf("load version two settings: %v", err)
	}
	if loaded.Transfers != settingsdomain.Defaults().Transfers {
		t.Fatalf("unexpected migrated transfers: %#v", loaded.Transfers)
	}
	if loaded.Connection != settingsdomain.Defaults().Connection {
		t.Fatalf("unexpected migrated connection settings: %#v", loaded.Connection)
	}
	if !loaded.Notifications.Enabled || loaded.Notifications.TransferCompleted || loaded.Notifications.LongTransferSeconds != 45 {
		t.Fatalf("version two notification preferences changed: %#v", loaded.Notifications)
	}
}

func TestStoreMigratesVersionThreeConnectionDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	fixture := []byte(`{"version":3,"revision":11,"settings":{"terminal":{"fontFamily":"menlo","fontSize":15,"lineHeight":1.25,"cursorStyle":"bar","cursorBlink":false,"scrollback":20000,"bell":false},"notifications":{"enabled":true,"transferCompleted":true,"unexpectedDisconnect":false,"longTransferSeconds":60},"transfers":{"concurrency":4,"collisionPolicy":"rename","keepPartialFiles":true}}}`)
	if err := os.WriteFile(path, fixture, 0o600); err != nil {
		t.Fatalf("write version three fixture: %v", err)
	}
	loaded, err := NewAt(path).LoadSettings()
	if err != nil {
		t.Fatalf("load version three settings: %v", err)
	}
	if loaded.Connection != settingsdomain.Defaults().Connection {
		t.Fatalf("unexpected migrated connection settings: %#v", loaded.Connection)
	}
	if loaded.Terminal.FontSize != 15 || loaded.Transfers.Concurrency != 4 || !loaded.Notifications.Enabled {
		t.Fatalf("existing version three settings changed: %#v", loaded)
	}
	if loaded.UI != settingsdomain.Defaults().UI || loaded.Window != settingsdomain.Defaults().Window {
		t.Fatalf("version three UI defaults were not added: %#v", loaded)
	}
}

func TestStoreMigratesVersionFourUIAndWindowDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	fixture := []byte(`{"version":4,"revision":13,"settings":{"terminal":{"fontFamily":"monaco","fontSize":14,"lineHeight":1.3,"cursorStyle":"underline","cursorBlink":false,"scrollback":30000,"bell":true},"connection":{"connectTimeoutSeconds":20,"keepAliveEnabled":false,"keepAliveIntervalSeconds":60,"keepAliveMaxFailures":4},"notifications":{"enabled":true,"transferCompleted":false,"unexpectedDisconnect":true,"longTransferSeconds":90},"transfers":{"concurrency":3,"collisionPolicy":"skip","keepPartialFiles":true}}}`)
	if err := os.WriteFile(path, fixture, 0o600); err != nil {
		t.Fatalf("write version four fixture: %v", err)
	}
	loaded, err := NewAt(path).LoadSettings()
	if err != nil {
		t.Fatalf("load version four settings: %v", err)
	}
	if loaded.UI != settingsdomain.Defaults().UI || loaded.Window != settingsdomain.Defaults().Window {
		t.Fatalf("version four UI defaults were not added: %#v", loaded)
	}
	if loaded.Terminal.FontSize != 14 || loaded.Connection.ConnectTimeoutSeconds != 20 || loaded.Transfers.Concurrency != 3 {
		t.Fatalf("version four preferences changed: %#v", loaded)
	}
}
