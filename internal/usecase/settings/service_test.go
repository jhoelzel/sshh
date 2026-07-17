package settings

import (
	"testing"

	settingsdomain "shh-h/internal/domain/settings"
)

type memoryRepository struct {
	value settingsdomain.Settings
}

func (r *memoryRepository) LoadSettings() (settingsdomain.Settings, error) {
	if r.value == (settingsdomain.Settings{}) {
		r.value = settingsdomain.Defaults()
	}
	return r.value, nil
}

func (r *memoryRepository) SaveSettings(value settingsdomain.Settings) error {
	r.value = value
	return nil
}

func TestServiceUpdateAndReset(t *testing.T) {
	service, err := NewService(&memoryRepository{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	value := service.Get()
	value.Terminal.FontSize = 18
	updated, err := service.Update(value)
	if err != nil || updated.Terminal.FontSize != 18 {
		t.Fatalf("update: value=%#v err=%v", updated, err)
	}
	reset, err := service.Reset()
	if err != nil || reset != settingsdomain.Defaults() {
		t.Fatalf("reset: value=%#v err=%v", reset, err)
	}
}
