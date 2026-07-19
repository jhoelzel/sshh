package settings

import (
	"testing"

	settingsdomain "shh-h/internal/domain/settings"
)

type memoryRepository struct {
	value settingsdomain.Settings
	saves int
}

func (r *memoryRepository) LoadSettings() (settingsdomain.Settings, error) {
	if r.value == (settingsdomain.Settings{}) {
		r.value = settingsdomain.Defaults()
	}
	return r.value, nil
}

func (r *memoryRepository) SaveSettings(value settingsdomain.Settings) error {
	r.value = value
	r.saves++
	return nil
}

func TestServiceUpdateAndReset(t *testing.T) {
	repository := &memoryRepository{}
	service, err := NewService(repository)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	value := service.Get()
	value.Terminal.FontSize = 18
	updated, err := service.Update(value)
	if err != nil || updated.Terminal.FontSize != 18 {
		t.Fatalf("update: value=%#v err=%v", updated, err)
	}
	if service.ConnectionSettings() != updated.Connection {
		t.Fatalf("connection settings do not match current value: %#v", service.ConnectionSettings())
	}
	ui := updated.UI
	ui.Theme = settingsdomain.ThemeLight
	ui.SidebarWidth = 320
	ui.Workspace = settingsdomain.WorkspaceTunnels
	updatedUI, err := service.UpdateUI(ui)
	if err != nil || updatedUI != ui {
		t.Fatalf("update UI: value=%#v err=%v", updatedUI, err)
	}
	window := updated.Window
	window.X = 40
	window.Y = 25
	window.Width = 1320
	window.Height = 820
	window.Positioned = true
	updatedWindow, err := service.UpdateWindow(window)
	if err != nil || updatedWindow != window {
		t.Fatalf("update window: value=%#v err=%v", updatedWindow, err)
	}
	beforeNoop := repository.saves
	if _, err := service.UpdateWindow(window); err != nil {
		t.Fatalf("repeat window update: %v", err)
	}
	if repository.saves != beforeNoop {
		t.Fatalf("unchanged window state wrote %d additional documents", repository.saves-beforeNoop)
	}
	if current := service.Get(); current.Terminal.FontSize != 18 || current.UI != ui || current.Window != window {
		t.Fatalf("partial updates changed unrelated settings: %#v", current)
	}
	reset, err := service.Reset()
	if err != nil || reset != settingsdomain.Defaults() {
		t.Fatalf("reset: value=%#v err=%v", reset, err)
	}
}
