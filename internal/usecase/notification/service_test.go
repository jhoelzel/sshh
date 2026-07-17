package notification

import (
	"context"
	"errors"
	"testing"

	filedomain "shh-h/internal/domain/filetransfer"
	notificationdomain "shh-h/internal/domain/notification"
	settingsdomain "shh-h/internal/domain/settings"
)

type fakeGateway struct {
	available  bool
	authorized bool
	startErr   error
	sent       []notificationdomain.Message
}

func (gateway *fakeGateway) Initialize(context.Context) error { return gateway.startErr }
func (gateway *fakeGateway) Cleanup()                         {}
func (gateway *fakeGateway) Available() bool                  { return gateway.available }
func (gateway *fakeGateway) Authorized() (bool, error)        { return gateway.authorized, nil }
func (gateway *fakeGateway) RequestAuthorization() (bool, error) {
	gateway.authorized = true
	return true, nil
}
func (gateway *fakeGateway) Send(message notificationdomain.Message) error {
	gateway.sent = append(gateway.sent, message)
	return nil
}

type fakeSettings struct {
	value settingsdomain.Settings
}

func (settings *fakeSettings) Get() settingsdomain.Settings { return settings.value }

func TestLongTransferNotificationUsesOnlyBasename(t *testing.T) {
	gateway := &fakeGateway{available: true, authorized: true}
	settings := &fakeSettings{value: settingsdomain.Defaults()}
	settings.value.Notifications.Enabled = true
	service, err := NewService(gateway, settings)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	service.Startup(context.Background())
	transfer := filedomain.Transfer{
		ID: "transfer-one", Direction: filedomain.DirectionDownload, State: filedomain.TransferCompleted,
		Source: "/private/customer/report.csv", StartedAt: "2026-07-17T12:00:00Z", FinishedAt: "2026-07-17T12:00:45Z",
	}
	if err := service.TransferCompleted(transfer); err != nil {
		t.Fatalf("notify transfer: %v", err)
	}
	if len(gateway.sent) != 1 || gateway.sent[0].Body != "report.csv finished in 45s." {
		t.Fatalf("unexpected notification: %#v", gateway.sent)
	}
	if err := service.TransferCompleted(filedomain.Transfer{
		ID: "short", Direction: filedomain.DirectionDownload, State: filedomain.TransferCompleted,
		Source: "/tmp/short.txt", StartedAt: "2026-07-17T12:00:00Z", FinishedAt: "2026-07-17T12:00:05Z",
	}); err != nil {
		t.Fatalf("short transfer: %v", err)
	}
	if len(gateway.sent) != 1 {
		t.Fatal("short transfer produced a notification")
	}
	if err := service.TransferCompleted(filedomain.Transfer{
		ID: "invalid-time", Direction: filedomain.DirectionUpload, State: filedomain.TransferCompleted,
		Destination: "C:\\private\\upload.txt", StartedAt: "2026-07-17T12:00:45Z", FinishedAt: "2026-07-17T12:00:00Z",
	}); err == nil {
		t.Fatal("expected backwards transfer timestamps to fail")
	}
}

func TestUnexpectedDisconnectHonorsSettingsAndSanitizesText(t *testing.T) {
	gateway := &fakeGateway{available: true, authorized: true}
	settings := &fakeSettings{value: settingsdomain.Defaults()}
	service, err := NewService(gateway, settings)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	service.Startup(context.Background())
	if err := service.UnexpectedDisconnect("session one", "Production\nSSH", "connection\nlost"); err != nil {
		t.Fatalf("disabled notification: %v", err)
	}
	if len(gateway.sent) != 0 {
		t.Fatal("disabled notifications sent a message")
	}
	settings.value.Notifications.Enabled = true
	if err := service.UnexpectedDisconnect("session one", "Production\nSSH", "connection\nlost"); err != nil {
		t.Fatalf("notify disconnect: %v", err)
	}
	if len(gateway.sent) != 1 || gateway.sent[0].ID != "session-session-one" || gateway.sent[0].Subtitle != "Production SSH" || gateway.sent[0].Body != "connection lost" {
		t.Fatalf("unexpected disconnect notification: %#v", gateway.sent)
	}
}

func TestPermissionStatusAndTestNotification(t *testing.T) {
	gateway := &fakeGateway{available: true}
	settings := &fakeSettings{value: settingsdomain.Defaults()}
	settings.value.Notifications.Enabled = true
	service, err := NewService(gateway, settings)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	service.Startup(context.Background())
	if status := service.Status(); !status.Available || status.Authorized {
		t.Fatalf("unexpected initial status: %#v", status)
	}
	status, err := service.RequestAuthorization()
	if err != nil || !status.Authorized {
		t.Fatalf("request authorization: status=%#v err=%v", status, err)
	}
	if err := service.SendTest(); err != nil {
		t.Fatalf("send test: %v", err)
	}
	if len(gateway.sent) != 1 || gateway.sent[0].ID != "shh-h-notification-test" {
		t.Fatalf("unexpected test notification: %#v", gateway.sent)
	}
	gateway.authorized = false
	if err := service.SendTest(); err == nil {
		t.Fatal("expected test notification without permission to fail")
	}
}

func TestStartupFailureIsReportedWithoutPanicking(t *testing.T) {
	gateway := &fakeGateway{startErr: errors.New("unsupported")}
	service, err := NewService(gateway, &fakeSettings{value: settingsdomain.Defaults()})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	service.Startup(context.Background())
	status := service.Status()
	if status.Available || status.Message != "unsupported" {
		t.Fatalf("unexpected failed status: %#v", status)
	}
}
