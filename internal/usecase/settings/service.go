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

func (s *Service) UpdateUI(value settingsdomain.UI) (settingsdomain.UI, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.current
	next.UI = value
	if err := next.Validate(); err != nil {
		return settingsdomain.UI{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "update UI settings", err.Error(), err,
		)
	}
	if next == s.current {
		return s.current.UI, nil
	}
	if err := s.repo.SaveSettings(next); err != nil {
		return settingsdomain.UI{}, err
	}
	s.current = next
	return next.UI, nil
}

func (s *Service) UpdateWindow(value settingsdomain.WindowState) (settingsdomain.WindowState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.current
	next.Window = value
	if err := next.Validate(); err != nil {
		return settingsdomain.WindowState{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "update window state", err.Error(), err,
		)
	}
	if next == s.current {
		return s.current.Window, nil
	}
	if err := s.repo.SaveSettings(next); err != nil {
		return settingsdomain.WindowState{}, err
	}
	s.current = next
	return next.Window, nil
}

func (s *Service) Reset() (settingsdomain.Settings, error) {
	return s.Update(settingsdomain.Defaults())
}
