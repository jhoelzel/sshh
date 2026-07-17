package settings

import (
	"sync"

	"shh-h/internal/apperror"
	settingsdomain "shh-h/internal/domain/settings"
)

type Repository interface {
	LoadSettings() (settingsdomain.Settings, error)
	SaveSettings(settingsdomain.Settings) error
}

type Service struct {
	mu      sync.RWMutex
	repo    Repository
	current settingsdomain.Settings
}

func NewService(repo Repository) (*Service, error) {
	current, err := repo.LoadSettings()
	if err != nil {
		return nil, err
	}
	return &Service{repo: repo, current: current}, nil
}

func (s *Service) Get() settingsdomain.Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

func (s *Service) ConnectionSettings() settingsdomain.Connection {
	return s.Get().Connection
}

func (s *Service) Update(value settingsdomain.Settings) (settingsdomain.Settings, error) {
	if err := value.Validate(); err != nil {
		return settingsdomain.Settings{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "update settings", err.Error(), err,
		)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.repo.SaveSettings(value); err != nil {
		return settingsdomain.Settings{}, err
	}
	s.current = value
	return value, nil
}

func (s *Service) Reset() (settingsdomain.Settings, error) {
	return s.Update(settingsdomain.Defaults())
}
