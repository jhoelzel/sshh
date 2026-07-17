package bridge

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v2/pkg/options"

	"shh-h/internal/apperror"
	profiledomain "shh-h/internal/domain/profile"
	remotepathdomain "shh-h/internal/domain/remotepath"
	settingsdomain "shh-h/internal/domain/settings"
	profileusecase "shh-h/internal/usecase/profile"
	remotepathusecase "shh-h/internal/usecase/remotepath"
	sessionusecase "shh-h/internal/usecase/session"
)

type bridgeProfileRepository struct {
	profiles []profiledomain.Profile
}

func (repository *bridgeProfileRepository) LoadProfiles() ([]profiledomain.Profile, error) {
	return repository.profiles, nil
}

func (repository *bridgeProfileRepository) SaveProfiles(profiles []profiledomain.Profile) error {
	repository.profiles = profiles
	return nil
}

type bridgeRemotePathRepository struct {
	favorites []remotepathdomain.Favorite
}

func (repository *bridgeRemotePathRepository) LoadFavorites() ([]remotepathdomain.Favorite, error) {
	return repository.favorites, nil
}

func (repository *bridgeRemotePathRepository) SaveFavorites(favorites []remotepathdomain.Favorite) error {
	repository.favorites = favorites
	return nil
}

func TestAttachFrontendIsIdempotentForSameInstance(t *testing.T) {
	desktop := NewDesktop(sessionusecase.NewManager(nil), nil, nil, nil, nil, nil, nil, nil, nil, nil)

	first, err := desktop.AttachFrontend("frontend-instance")
	if err != nil {
		t.Fatalf("attach frontend: %v", err)
	}
	second, err := desktop.AttachFrontend("frontend-instance")
	if err != nil {
		t.Fatalf("reattach frontend: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("same frontend instance received a new lease: %q != %q", first.ID, second.ID)
	}
	if _, err := time.Parse(time.RFC3339Nano, second.ExpiresAt); err != nil {
		t.Fatalf("lease expiry is not RFC3339: %v", err)
	}
}

func TestAttachFrontendReplacesPreviousInstance(t *testing.T) {
	desktop := NewDesktop(sessionusecase.NewManager(nil), nil, nil, nil, nil, nil, nil, nil, nil, nil)

	first, err := desktop.AttachFrontend("first-instance")
	if err != nil {
		t.Fatalf("attach first frontend: %v", err)
	}
	second, err := desktop.AttachFrontend("second-instance")
	if err != nil {
		t.Fatalf("attach second frontend: %v", err)
	}
	if first.ID == second.ID {
		t.Fatal("replacement frontend reused the previous lease")
	}
	if _, err := desktop.RenewFrontendLease(first.ID); err == nil {
		t.Fatal("expected the replaced lease to be rejected")
	}
	if _, err := desktop.RenewFrontendLease(second.ID); err != nil {
		t.Fatalf("renew active lease: %v", err)
	}
}

func TestAttachFrontendRejectsInvalidNonce(t *testing.T) {
	desktop := NewDesktop(sessionusecase.NewManager(nil), nil, nil, nil, nil, nil, nil, nil, nil, nil)

	if _, err := desktop.AttachFrontend("  "); !apperror.IsCode(err, apperror.CodeInvalidArgument) {
		t.Fatalf("empty frontend nonce error code = %q, want %q", apperror.CodeOf(err), apperror.CodeInvalidArgument)
	}
}

func TestDesktopStartupAndShutdownStopEveryLeaseMonitor(t *testing.T) {
	desktop := NewDesktop(sessionusecase.NewManager(nil), nil, nil, nil, nil, nil, nil, nil, nil, nil)

	for cycle := 0; cycle < 25; cycle++ {
		desktop.startup(context.Background())
		run := currentDesktopLifecycle(t, desktop)

		desktop.shutdown(context.Background())

		select {
		case <-run.done:
		case <-time.After(time.Second):
			t.Fatalf("lease monitor from cycle %d did not stop", cycle)
		}
		desktop.lifecycleMu.Lock()
		active := desktop.lifecycle
		desktop.lifecycleMu.Unlock()
		if active != nil || desktop.context() != nil {
			t.Fatalf("cycle %d retained lifecycle state", cycle)
		}
	}
}

func TestRepeatedStartupStopsThePreviousLeaseMonitor(t *testing.T) {
	desktop := NewDesktop(sessionusecase.NewManager(nil), nil, nil, nil, nil, nil, nil, nil, nil, nil)
	desktop.startup(context.Background())
	first := currentDesktopLifecycle(t, desktop)

	desktop.startup(context.Background())
	second := currentDesktopLifecycle(t, desktop)
	t.Cleanup(func() { desktop.shutdown(context.Background()) })

	if first == second {
		t.Fatal("repeated startup reused lifecycle state")
	}
	select {
	case <-first.done:
	case <-time.After(time.Second):
		t.Fatal("previous lease monitor did not stop")
	}
}

func TestDeferredDesktopWaitsForHostInitialization(t *testing.T) {
	desktop, controller := NewDeferredDesktop()
	ready := make(chan error, 1)
	go func() {
		ready <- desktop.AwaitReady()
	}()

	select {
	case err := <-ready:
		t.Fatalf("deferred desktop became ready before host initialization: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	err := controller.Start(context.Background(), Dependencies{
		Manager: sessionusecase.NewManager(nil),
	})
	if err != nil {
		t.Fatalf("start deferred desktop: %v", err)
	}
	t.Cleanup(func() { desktop.shutdown(context.Background()) })

	select {
	case err := <-ready:
		if err != nil {
			t.Fatalf("await initialized desktop: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("frontend readiness was not released after host initialization")
	}
}

func TestDeferredDesktopReportsHostInitializationFailure(t *testing.T) {
	desktop, controller := NewDeferredDesktop()
	want := errors.New("composition failed")
	controller.Fail(want)

	if err := desktop.AwaitReady(); !errors.Is(err, want) {
		t.Fatalf("startup failure = %v, want %v", err, want)
	}
}

func TestSecondInstanceHandlerActivatesPreparedPrimaryWindow(t *testing.T) {
	desktop, controller := NewDeferredDesktop()
	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("primary"), true)

	activations := 0
	desktop.activateWindow = func(received context.Context) {
		activations++
		if received != ctx {
			t.Fatal("second-instance handler used a different Wails context")
		}
	}

	SecondInstanceHandler(desktop)(options.SecondInstanceData{})
	if activations != 0 {
		t.Fatalf("window activated %d times before Wails context preparation", activations)
	}
	controller.Prepare(ctx)
	if activations != 1 {
		t.Fatalf("queued window activations = %d, want 1", activations)
	}
	SecondInstanceHandler(desktop)(options.SecondInstanceData{})
	if activations != 2 {
		t.Fatalf("total window activations = %d, want 2", activations)
	}
}

func currentDesktopLifecycle(t *testing.T, desktop *Desktop) *desktopLifecycle {
	t.Helper()
	desktop.lifecycleMu.Lock()
	defer desktop.lifecycleMu.Unlock()
	if desktop.lifecycle == nil {
		t.Fatal("desktop lifecycle is not running")
	}
	return desktop.lifecycle
}

func TestTerminalTextFilenameSanitizesUntrustedTitles(t *testing.T) {
	filename := terminalTextFilename(" ../../Production\nShell ")
	if filename != "Production-Shell-selection.txt" {
		t.Fatalf("unexpected filename %q", filename)
	}
	if fallback := terminalTextFilename("///"); fallback != "terminal-selection.txt" {
		t.Fatalf("unexpected fallback filename %q", fallback)
	}
	if long := terminalTextFilename(strings.Repeat("界", 100)); len(long) > 100 {
		t.Fatalf("suggested filename exceeds byte budget: %d", len(long))
	}
}

func TestSettingsDTORoundTripIncludesConnectionNotificationAndTransferPreferences(t *testing.T) {
	settings := settingsdomain.Defaults()
	settings.Connection.ConnectTimeoutSeconds = 25
	settings.Connection.KeepAliveEnabled = false
	settings.Connection.KeepAliveIntervalSeconds = 45
	settings.Connection.KeepAliveMaxFailures = 5
	settings.Notifications.Enabled = true
	settings.Notifications.LongTransferSeconds = 75
	settings.Transfers.Concurrency = 5
	settings.Transfers.CollisionPolicy = "rename"
	settings.Transfers.KeepPartialFiles = true
	if roundTrip := settingsFromDTO(settingsDTO(settings)); roundTrip != settings {
		t.Fatalf("settings DTO changed preferences: %#v", roundTrip)
	}
}

func TestBuildInfoDTOIsAlwaysPopulated(t *testing.T) {
	info := (&Desktop{}).GetBuildInfo()
	if info.Version == "" || info.Commit == "" || info.BuildDate == "" {
		t.Fatalf("build identity contains empty values: %#v", info)
	}
	if info.GoVersion == "" || info.Platform == "" {
		t.Fatalf("runtime identity contains empty values: %#v", info)
	}
}

func TestBoundedDialogTextRemovesControlsAndCapsBytes(t *testing.T) {
	if got := boundedDialogText(" report\nname.csv ", 160); got != "report name.csv" {
		t.Fatalf("unexpected dialog text %q", got)
	}
	if got := boundedDialogText("\n\t", 160); got != "The selected file" {
		t.Fatalf("unexpected empty fallback %q", got)
	}
	if got := boundedDialogText(strings.Repeat("界", 100), 20); len(got) > 20 {
		t.Fatalf("dialog text exceeds byte budget: %d", len(got))
	}
}

func TestRemotePathFavoritesRequireExistingSSHProfile(t *testing.T) {
	profiles, err := profileusecase.NewService(&bridgeProfileRepository{profiles: []profiledomain.Profile{
		{ID: "local", Protocol: profiledomain.ProtocolLocal},
		{ID: "ssh", Protocol: profiledomain.ProtocolSSH},
	}})
	if err != nil {
		t.Fatalf("new profile service: %v", err)
	}
	remotePaths, err := remotepathusecase.NewService(&bridgeRemotePathRepository{})
	if err != nil {
		t.Fatalf("new remote path service: %v", err)
	}
	desktop := NewDesktop(sessionusecase.NewManager(nil), profiles, nil, nil, nil, nil, nil, remotePaths, nil, nil)

	if _, err := desktop.CreateRemotePathFavorite("missing", "/srv/app"); err == nil {
		t.Fatal("expected missing profile rejection")
	}
	if _, err := desktop.CreateRemotePathFavorite("local", "/srv/app"); err == nil {
		t.Fatal("expected local profile rejection")
	}
	created, err := desktop.CreateRemotePathFavorite("ssh", "/srv/app/../logs")
	if err != nil {
		t.Fatalf("create SSH favorite: %v", err)
	}
	if created.ProfileID != "ssh" || created.Path != "/srv/logs" || len(desktop.ListRemotePathFavorites()) != 1 {
		t.Fatalf("unexpected remote path favorite: %#v", created)
	}
}
