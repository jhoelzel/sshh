package notification

import (
	"context"
	"errors"
	"fmt"
	pathpkg "path"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	filedomain "shh-h/internal/domain/filetransfer"
	notificationdomain "shh-h/internal/domain/notification"
	settingsdomain "shh-h/internal/domain/settings"
)

type Gateway interface {
	Initialize(context.Context) error
	Cleanup()
	Available() bool
	Authorized() (bool, error)
	RequestAuthorization() (bool, error)
	Send(notificationdomain.Message) error
}

type SettingsProvider interface {
	Get() settingsdomain.Settings
}

type Status struct {
	Available  bool
	Authorized bool
	Message    string
}

type Service struct {
	mu       sync.RWMutex
	gateway  Gateway
	settings SettingsProvider
	started  bool
	startErr error
}

func NewService(gateway Gateway, settings SettingsProvider) (*Service, error) {
	if gateway == nil {
		return nil, errors.New("notification gateway is required")
	}
	if settings == nil {
		return nil, errors.New("notification settings are required")
	}
	return &Service{gateway: gateway, settings: settings}, nil
}

func (service *Service) Startup(ctx context.Context) {
	err := service.gateway.Initialize(ctx)
	service.mu.Lock()
	service.started = err == nil
	service.startErr = err
	service.mu.Unlock()
}

func (service *Service) Shutdown() {
	service.mu.Lock()
	started := service.started
	service.started = false
	service.mu.Unlock()
	if started {
		service.gateway.Cleanup()
	}
}

func (service *Service) Status() Status {
	started, startErr := service.startState()
	if !started {
		message := "Notifications are unavailable"
		if startErr != nil {
			message = startErr.Error()
		}
		return Status{Message: message}
	}
	if !service.gateway.Available() {
		return Status{Message: "Notifications are unavailable on this system"}
	}
	authorized, err := service.gateway.Authorized()
	if err != nil {
		return Status{Available: true, Message: err.Error()}
	}
	message := "Permission is required"
	if authorized {
		message = "System notifications are allowed"
	}
	return Status{Available: true, Authorized: authorized, Message: message}
}

func (service *Service) RequestAuthorization() (Status, error) {
	started, startErr := service.startState()
	if !started {
		if startErr != nil {
			return Status{}, startErr
		}
		return Status{}, errors.New("notifications are unavailable")
	}
	if !service.gateway.Available() {
		return Status{}, errors.New("notifications are unavailable on this system")
	}
	authorized, err := service.gateway.RequestAuthorization()
	if err != nil {
		return Status{Available: true, Message: err.Error()}, err
	}
	message := "Notification permission was denied"
	if authorized {
		message = "System notifications are allowed"
	}
	return Status{Available: true, Authorized: authorized, Message: message}, nil
}

func (service *Service) SendTest() error {
	if !service.settings.Get().Notifications.Enabled {
		return errors.New("enable notifications in application settings first")
	}
	return service.deliver(notificationdomain.Message{
		ID: "shh-h-notification-test", Title: "shh-h notifications",
		Body: "System notifications are configured correctly.",
	}, true)
}

func (service *Service) TransferCompleted(transfer filedomain.Transfer) error {
	preferences := service.settings.Get().Notifications
	if !preferences.Enabled || !preferences.TransferCompleted || transfer.State != filedomain.TransferCompleted {
		return nil
	}
	startedAt, err := time.Parse(time.RFC3339Nano, transfer.StartedAt)
	if err != nil {
		return fmt.Errorf("parse transfer start time: %w", err)
	}
	finishedAt, err := time.Parse(time.RFC3339Nano, transfer.FinishedAt)
	if err != nil {
		return fmt.Errorf("parse transfer finish time: %w", err)
	}
	duration := finishedAt.Sub(startedAt)
	if duration < 0 {
		return errors.New("transfer finish time precedes start time")
	}
	if duration < time.Duration(preferences.LongTransferSeconds)*time.Second {
		return nil
	}
	direction := "Download"
	filename := transfer.Source
	if transfer.Direction == filedomain.DirectionUpload {
		direction = "Upload"
		filename = transfer.Destination
	}
	filename = pathpkg.Base(strings.ReplaceAll(filename, "\\", "/"))
	return service.deliver(notificationdomain.Message{
		ID: "transfer-" + transfer.ID, Title: "Transfer completed", Subtitle: direction,
		Body: fmt.Sprintf("%s finished in %s.", cleanText(filename, 180), formatDuration(duration)),
	}, false)
}

func (service *Service) UnexpectedDisconnect(sessionID, title, message string) error {
	preferences := service.settings.Get().Notifications
	if !preferences.Enabled || !preferences.UnexpectedDisconnect {
		return nil
	}
	body := "The terminal session ended unexpectedly."
	if cleaned := cleanText(message, 360); cleaned != "" {
		body = cleaned
	}
	return service.deliver(notificationdomain.Message{
		ID: "session-" + cleanIdentifier(sessionID), Title: "Terminal disconnected",
		Subtitle: cleanText(title, 120), Body: body,
	}, false)
}

func (service *Service) deliver(message notificationdomain.Message, requireAuthorization bool) error {
	started, startErr := service.startState()
	if !started {
		if startErr != nil {
			return startErr
		}
		return errors.New("notifications are unavailable")
	}
	authorized, err := service.gateway.Authorized()
	if err != nil {
		return err
	}
	if !authorized {
		if requireAuthorization {
			return errors.New("notification permission is required")
		}
		return nil
	}
	return service.gateway.Send(message)
}

func (service *Service) startState() (bool, error) {
	service.mu.RLock()
	defer service.mu.RUnlock()
	return service.started, service.startErr
}

func cleanIdentifier(value string) string {
	value = cleanText(value, 120)
	var result strings.Builder
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_' {
			result.WriteRune(character)
		} else {
			result.WriteByte('-')
		}
	}
	if result.Len() == 0 {
		return "unknown"
	}
	return result.String()
}

func cleanText(value string, maxBytes int) string {
	value = strings.TrimSpace(value)
	var result strings.Builder
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			character = ' '
		}
		if result.Len()+utf8.RuneLen(character) > maxBytes {
			break
		}
		result.WriteRune(character)
	}
	return strings.Join(strings.Fields(result.String()), " ")
}

func formatDuration(duration time.Duration) string {
	duration = duration.Round(time.Second)
	if duration < time.Minute {
		return fmt.Sprintf("%ds", int(duration.Seconds()))
	}
	minutes := int(duration / time.Minute)
	seconds := int((duration % time.Minute) / time.Second)
	if seconds == 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dm %ds", minutes, seconds)
}
