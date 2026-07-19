package bridge

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v2/pkg/options"

	"shh-h/internal/apperror"
	profiledomain "shh-h/internal/domain/profile"
	remotepathdomain "shh-h/internal/domain/remotepath"
	settingsdomain "shh-h/internal/domain/settings"
	"shh-h/internal/port"
	"shh-h/internal/terminalbenchmark"
	profileusecase "shh-h/internal/usecase/profile"
	remotepathusecase "shh-h/internal/usecase/remotepath"
	sessionusecase "shh-h/internal/usecase/session"
	settingsusecase "shh-h/internal/usecase/settings"
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

type bridgeSettingsRepository struct {
	settings settingsdomain.Settings
}

func (repository *bridgeSettingsRepository) LoadSettings() (settingsdomain.Settings, error) {
	return repository.settings, nil
}

func (repository *bridgeSettingsRepository) SaveSettings(value settingsdomain.Settings) error {
	repository.settings = value
	return nil
}

type bridgeTerminalFactory struct {
	transport *bridgeTerminalTransport
}

func (factory *bridgeTerminalFactory) Open(context.Context, port.TerminalSpec) (port.TerminalTransport, error) {
	return factory.transport, nil
}

type bridgeTerminalTransport struct {
	mu           sync.Mutex
	input        bytes.Buffer
	columns      uint16
	rows         uint16
	closed       chan struct{}
	closeStarted chan struct{}
	closeGate    <-chan struct{}
	closeOnce    sync.Once
}

func newBridgeTerminalTransport() *bridgeTerminalTransport {
	return &bridgeTerminalTransport{closed: make(chan struct{})}
}

func newGatedBridgeTerminalTransport(closeGate <-chan struct{}) *bridgeTerminalTransport {
	return &bridgeTerminalTransport{
		closed: make(chan struct{}), closeStarted: make(chan struct{}), closeGate: closeGate,
	}
}

func (transport *bridgeTerminalTransport) Read([]byte) (int, error) {
	<-transport.closed
	return 0, io.EOF
}

func (transport *bridgeTerminalTransport) Write(data []byte) (int, error) {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	return transport.input.Write(data)
}

func (transport *bridgeTerminalTransport) Resize(_ context.Context, columns, rows uint16) error {
	transport.mu.Lock()
	transport.columns = columns
	transport.rows = rows
	transport.mu.Unlock()
	return nil
}

func (transport *bridgeTerminalTransport) Signal(context.Context, port.TerminalSignal) error {
	transport.finish()
	return nil
}

func (transport *bridgeTerminalTransport) Wait() (port.ExitStatus, error) {
	<-transport.closed
	return port.ExitStatus{}, nil
}

func (transport *bridgeTerminalTransport) Close() error {
	transport.finish()
	return nil
}

func (transport *bridgeTerminalTransport) finish() {
	transport.closeOnce.Do(func() {
		if transport.closeStarted != nil {
			close(transport.closeStarted)
		}
		if transport.closeGate != nil {
			<-transport.closeGate
		}
		close(transport.closed)
	})
}

func (transport *bridgeTerminalTransport) snapshot() ([]byte, uint16, uint16) {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	return append([]byte(nil), transport.input.Bytes()...), transport.columns, transport.rows
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

func TestAttachFrontendReplacesPreviousInstanceAndReapsItsRuntime(t *testing.T) {
	allowClose := make(chan struct{})
	transport := newGatedBridgeTerminalTransport(allowClose)
	manager := sessionusecase.NewManager(&bridgeTerminalFactory{transport: transport})
	desktop := NewDesktop(manager, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	manager.SetSink(nil)
	var releaseOnce sync.Once
	releaseClose := func() { releaseOnce.Do(func() { close(allowClose) }) }
	t.Cleanup(func() {
		releaseClose()
		manager.Shutdown()
	})

	first, err := desktop.AttachFrontend("first-instance")
	if err != nil {
		t.Fatalf("attach first frontend: %v", err)
	}
	opened, err := manager.OpenLocal(context.Background(), first.ID, profiledomain.Profile{
		ID: "local", Name: "Local", Protocol: profiledomain.ProtocolLocal,
	}, 80, 24)
	if err != nil {
		t.Fatalf("open terminal for first frontend: %v", err)
	}
	if err := manager.Activate(first.ID, opened.ID, opened.Generation); err != nil {
		t.Fatalf("activate terminal for first frontend: %v", err)
	}

	type attachResult struct {
		lease FrontendLeaseDTO
		err   error
	}
	attached := make(chan attachResult, 1)
	go func() {
		lease, attachErr := desktop.AttachFrontend("second-instance")
		attached <- attachResult{lease: lease, err: attachErr}
	}()
	select {
	case <-transport.closeStarted:
	case <-time.After(time.Second):
		t.Fatal("replacement attachment did not begin old-lease cleanup")
	}
	select {
	case result := <-attached:
		t.Fatalf("replacement attachment returned before cleanup was released: %#v", result)
	case <-time.After(30 * time.Millisecond):
	}
	releaseClose()

	var result attachResult
	select {
	case result = <-attached:
	case <-time.After(time.Second):
		t.Fatal("replacement attachment did not return after old-lease cleanup")
	}
	if result.err != nil {
		t.Fatalf("attach second frontend: %v", result.err)
	}
	second := result.lease
	if first.ID == second.ID {
		t.Fatal("replacement frontend reused the previous lease")
	}
	select {
	case <-transport.closed:
	default:
		t.Fatal("replacement attachment returned before the old-lease transport closed")
	}
	if manager.LiveCount() != 0 {
		t.Fatalf("replacement attachment left %d old-lease terminals running", manager.LiveCount())
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

func TestTerminalBenchmarkIsDisabledByDefault(t *testing.T) {
	desktop := NewDesktop(sessionusecase.NewManager(nil), nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if config := desktop.GetTerminalBenchmarkConfig(); config.Enabled {
		t.Fatalf("ordinary desktop exposed benchmark mode: %#v", config)
	}
	lease, err := desktop.AttachFrontend("benchmark-disabled")
	if err != nil {
		t.Fatalf("attach frontend: %v", err)
	}
	if _, err := desktop.OpenTerminalBenchmark(lease.ID, 80, 24); !apperror.IsCode(err, apperror.CodeUnavailable) {
		t.Fatalf("disabled benchmark error code = %q, want %q", apperror.CodeOf(err), apperror.CodeUnavailable)
	}
	if err := desktop.RecordTerminalBenchmarkProgress(lease.ID, "running", 1); !apperror.IsCode(err, apperror.CodeUnavailable) {
		t.Fatalf("disabled benchmark progress error code = %q, want %q", apperror.CodeOf(err), apperror.CodeUnavailable)
	}
}

func TestGuardedTerminalBenchmarkOpensReportsAndQuits(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test executable: %v", err)
	}
	benchmark, err := terminalbenchmark.NewService(executable, filepath.Join(t.TempDir(), "result.json"))
	if err != nil {
		t.Fatalf("create terminal benchmark: %v", err)
	}
	transport := newBridgeTerminalTransport()
	manager := sessionusecase.NewManager(&bridgeTerminalFactory{transport: transport})
	desktop, controller := NewDeferredDesktop()
	if err := controller.Start(context.Background(), Dependencies{Manager: manager, Benchmark: benchmark}); err != nil {
		t.Fatalf("start benchmark desktop: %v", err)
	}
	manager.SetSink(nil)
	t.Cleanup(func() { controller.Shutdown(context.Background()) })
	quit := make(chan struct{}, 1)
	desktop.quitApplication = func(context.Context) { quit <- struct{}{} }

	config := desktop.GetTerminalBenchmarkConfig()
	if !config.Enabled || config.Mode != terminalbenchmark.ModeBurst ||
		config.PayloadBytes != terminalbenchmark.PayloadBytes || config.ProcessID != os.Getpid() {
		t.Fatalf("unexpected terminal benchmark config: %#v", config)
	}
	lease, err := desktop.AttachFrontend("benchmark-enabled")
	if err != nil {
		t.Fatalf("attach frontend: %v", err)
	}
	opened, err := desktop.OpenTerminalBenchmark(lease.ID, 80, 24)
	if err != nil {
		t.Fatalf("open terminal benchmark: %v", err)
	}
	if err := desktop.ActivateTerminal(lease.ID, opened.ID, opened.Generation); err != nil {
		t.Fatalf("activate terminal benchmark: %v", err)
	}
	diagnostics, err := desktop.GetTerminalDiagnostics(lease.ID, opened.ID, opened.Generation)
	if err != nil || diagnostics.SessionID != opened.ID {
		t.Fatalf("terminal diagnostics = %#v, %v", diagnostics, err)
	}
	if err := desktop.CloseTerminal(lease.ID, opened.ID, opened.Generation); err != nil {
		t.Fatalf("close terminal benchmark: %v", err)
	}

	now := time.Now().UTC()
	samples := make([]float64, terminalbenchmark.MinimumSamples)
	report := terminalbenchmark.Report{
		SchemaVersion: terminalbenchmark.SchemaVersion,
		StartedAt:     now.Format(time.RFC3339Nano), FinishedAt: now.Add(time.Second).Format(time.RFC3339Nano),
		PayloadBytes:         terminalbenchmark.PayloadBytes,
		IdleEchoMilliseconds: samples, FloodEchoMilliseconds: samples, ResizeMilliseconds: samples,
		Controller: terminalbenchmark.ControllerDiagnostics{MaximumPendingBytes: terminalbenchmark.MaximumQueueBytes},
		Backend:    terminalbenchmark.BackendDiagnostics{MaximumUnacknowledged: terminalbenchmark.MaximumQueueBytes},
	}
	completed, err := desktop.CompleteTerminalBenchmark(lease.ID, report)
	if err != nil {
		t.Fatalf("complete terminal benchmark: %v", err)
	}
	if completed.Passed || len(completed.Failures) == 0 {
		t.Fatal("empty fake transport unexpectedly passed the terminal benchmark")
	}
	select {
	case <-quit:
	case <-time.After(time.Second):
		t.Fatal("completed terminal benchmark did not request application quit")
	}
}

func TestGuardedTerminalSoakReportsAndQuits(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test executable: %v", err)
	}
	benchmark, err := terminalbenchmark.NewServiceWithMode(
		executable, filepath.Join(t.TempDir(), "soak.json"), terminalbenchmark.ModeSoak,
	)
	if err != nil {
		t.Fatalf("create terminal soak: %v", err)
	}
	desktop, controller := NewDeferredDesktop()
	if err := controller.Start(context.Background(), Dependencies{
		Manager:   sessionusecase.NewManager(&bridgeTerminalFactory{transport: newBridgeTerminalTransport()}),
		Benchmark: benchmark,
	}); err != nil {
		t.Fatalf("start terminal soak desktop: %v", err)
	}
	t.Cleanup(func() { controller.Shutdown(context.Background()) })
	quit := make(chan struct{}, 1)
	desktop.quitApplication = func(context.Context) { quit <- struct{}{} }
	lease, err := desktop.AttachFrontend("soak-enabled")
	if err != nil {
		t.Fatalf("attach frontend: %v", err)
	}

	now := time.Now().UTC()
	completed, err := desktop.CompleteTerminalSoak(lease.ID, terminalbenchmark.SoakReport{
		SchemaVersion: terminalbenchmark.SoakSchemaVersion,
		StartedAt:     now.Format(time.RFC3339Nano), FinishedAt: now.Add(time.Second).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("complete terminal soak: %v", err)
	}
	if completed.Passed || len(completed.Failures) == 0 {
		t.Fatal("empty soak report unexpectedly passed")
	}
	select {
	case <-quit:
	case <-time.After(time.Second):
		t.Fatal("completed terminal soak did not request application quit")
	}
}

func TestInvalidTerminalSoakReportStillQuitsGuardedApplication(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test executable: %v", err)
	}
	benchmark, err := terminalbenchmark.NewServiceWithMode(
		executable, filepath.Join(t.TempDir(), "invalid-soak.json"), terminalbenchmark.ModeSoak,
	)
	if err != nil {
		t.Fatalf("create terminal soak: %v", err)
	}
	desktop, controller := NewDeferredDesktop()
	if err := controller.Start(context.Background(), Dependencies{
		Manager:   sessionusecase.NewManager(&bridgeTerminalFactory{transport: newBridgeTerminalTransport()}),
		Benchmark: benchmark,
	}); err != nil {
		t.Fatalf("start terminal soak desktop: %v", err)
	}
	t.Cleanup(func() { controller.Shutdown(context.Background()) })
	quit := make(chan struct{}, 1)
	desktop.quitApplication = func(context.Context) { quit <- struct{}{} }
	lease, err := desktop.AttachFrontend("invalid-soak")
	if err != nil {
		t.Fatalf("attach frontend: %v", err)
	}

	if _, err := desktop.CompleteTerminalSoak(lease.ID, terminalbenchmark.SoakReport{}); !apperror.IsCode(err, apperror.CodeInvalidArgument) {
		t.Fatalf("invalid soak report error code = %q, want %q", apperror.CodeOf(err), apperror.CodeInvalidArgument)
	}
	select {
	case <-quit:
	case <-time.After(time.Second):
		t.Fatal("invalid terminal soak report did not request guarded application quit")
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

func TestSettingsDTORoundTripIncludesExposedPreferencesAndPreservesNativeWindowState(t *testing.T) {
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
	settings.UI.Theme = settingsdomain.ThemeLight
	settings.UI.SidebarWidth = 336
	settings.UI.Workspace = settingsdomain.WorkspaceActivity
	settings.Window = settingsdomain.WindowState{
		X: 90, Y: 60, Width: 1420, Height: 900, Positioned: true, Maximized: true,
	}
	if roundTrip := settingsFromDTO(settingsDTO(settings), settings); roundTrip != settings {
		t.Fatalf("settings DTO changed preferences: %#v", roundTrip)
	}

	current := settings
	current.UI.SidebarWidth = 288
	current.UI.Workspace = settingsdomain.WorkspaceTunnels
	current.Window = settingsdomain.Defaults().Window
	roundTrip := settingsFromDTO(settingsDTO(settings), current)
	if roundTrip.UI.Theme != settingsdomain.ThemeLight || roundTrip.UI.SidebarWidth != 288 || roundTrip.UI.Workspace != settingsdomain.WorkspaceTunnels {
		t.Fatalf("settings DTO overwrote runtime-owned UI preferences: %#v", roundTrip.UI)
	}
	if roundTrip.Window != current.Window {
		t.Fatalf("settings DTO exposed native window state: %#v", roundTrip.Window)
	}
}

func TestDesktopRestoresCapturesAndValidatesNativeWindowState(t *testing.T) {
	initial := settingsdomain.Defaults()
	initial.Window = settingsdomain.WindowState{
		X: 70, Y: 45, Width: 1380, Height: 860, Positioned: true, Maximized: true,
	}
	repository := &bridgeSettingsRepository{settings: initial}
	service, err := settingsusecase.NewService(repository)
	if err != nil {
		t.Fatalf("new settings service: %v", err)
	}
	desktop := newDeferredDesktop()
	desktop.settings = service

	var restoredSize [2]int
	var restoredPosition [2]int
	unmaximiseCalls := 0
	maximiseCalls := 0
	normal := true
	maximised := false
	desktop.window = windowRuntime{
		setSize: func(_ context.Context, width, height int) {
			restoredSize = [2]int{width, height}
		},
		getSize: func(context.Context) (int, int) { return 1510, 940 },
		setPosition: func(_ context.Context, x, y int) {
			restoredPosition = [2]int{x, y}
		},
		getPosition: func(context.Context) (int, int) { return -120, 85 },
		maximise:    func(context.Context) { maximiseCalls++ },
		unmaximise:  func(context.Context) { unmaximiseCalls++ },
		isMaximised: func(context.Context) bool { return maximised },
		isNormal:    func(context.Context) bool { return normal },
	}

	desktop.domReady(context.Background())
	if restoredSize != [2]int{1380, 860} || restoredPosition != [2]int{70, 45} {
		t.Fatalf("restored geometry = size %v position %v", restoredSize, restoredPosition)
	}
	if unmaximiseCalls != 1 || maximiseCalls != 1 {
		t.Fatalf("restore state calls = unmaximise %d maximise %d", unmaximiseCalls, maximiseCalls)
	}

	if err := desktop.captureWindowState(context.Background()); err != nil {
		t.Fatalf("capture normal window: %v", err)
	}
	wantNormal := settingsdomain.WindowState{
		X: -120, Y: 85, Width: 1510, Height: 940, Positioned: true,
	}
	if current := service.Get().Window; current != wantNormal {
		t.Fatalf("captured normal window = %#v, want %#v", current, wantNormal)
	}

	normal = false
	maximised = true
	if err := desktop.captureWindowState(context.Background()); err != nil {
		t.Fatalf("capture maximized window: %v", err)
	}
	wantMaximised := wantNormal
	wantMaximised.Maximized = true
	if current := service.Get().Window; current != wantMaximised {
		t.Fatalf("maximized capture changed normal bounds: %#v", current)
	}

	normal = true
	desktop.window.getSize = func(context.Context) (int, int) { return 100, 100 }
	if err := desktop.captureWindowState(context.Background()); !apperror.IsCode(err, apperror.CodeInvalidArgument) {
		t.Fatalf("invalid native geometry error = %v", err)
	}
	if current := service.Get().Window; current != wantMaximised {
		t.Fatalf("invalid native geometry replaced durable state: %#v", current)
	}
}

func TestDesktopUpdatesOnlyNonSensitiveUIPreferences(t *testing.T) {
	initial := settingsdomain.Defaults()
	initial.UI.Theme = settingsdomain.ThemeLight
	initial.Window = settingsdomain.WindowState{
		X: 25, Y: 30, Width: 1300, Height: 800, Positioned: true,
	}
	service, err := settingsusecase.NewService(&bridgeSettingsRepository{settings: initial})
	if err != nil {
		t.Fatalf("new settings service: %v", err)
	}
	desktop := newDeferredDesktop()
	desktop.settings = service

	updated, err := desktop.UpdateUIPreferences(UIPreferencesInputDTO{
		SidebarWidth: 318,
		Workspace:    settingsdomain.WorkspaceLayouts,
	})
	if err != nil {
		t.Fatalf("update UI preferences: %v", err)
	}
	if updated.Theme != settingsdomain.ThemeLight || updated.SidebarWidth != 318 || updated.Workspace != settingsdomain.WorkspaceLayouts {
		t.Fatalf("unexpected UI preferences: %#v", updated)
	}
	current := service.Get()
	if current.Window != initial.Window || current.Terminal != initial.Terminal || current.Connection != initial.Connection {
		t.Fatalf("UI update changed unrelated settings: %#v", current)
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

func TestTerminalOutputDTOFramesArbitraryBytes(t *testing.T) {
	payload := []byte{0xff, 0xfe, 0xe2, 0x28, 0xa1, 0x1b, '[', '3', '8', ';'}
	dto := terminalOutputDTO(sessionusecase.OutputChunk{
		LeaseID: "lease", SessionID: "session", Generation: 2,
		Sequence: 7, EndOffset: 42, Data: payload, Final: true,
	})

	decoded, err := base64.StdEncoding.DecodeString(dto.Payload)
	if err != nil {
		t.Fatalf("decode terminal output payload: %v", err)
	}
	if !bytes.Equal(decoded, payload) {
		t.Fatalf("terminal output bytes changed: %v", decoded)
	}
	if dto.LeaseID != "lease" || dto.SessionID != "session" || dto.Generation != 2 ||
		dto.Sequence != 7 || dto.EndOffset != 42 || dto.ByteCount != len(payload) || !dto.Final {
		t.Fatalf("terminal output metadata changed: %#v", dto)
	}
}

func TestTerminalCommandsPreserveInterleavedInputAndFinalResize(t *testing.T) {
	transport := newBridgeTerminalTransport()
	manager := sessionusecase.NewManager(&bridgeTerminalFactory{transport: transport})
	desktop := NewDesktop(manager, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	manager.SetSink(nil)
	t.Cleanup(manager.Shutdown)

	lease, err := desktop.AttachFrontend("terminal-command-test")
	if err != nil {
		t.Fatalf("attach frontend: %v", err)
	}
	opened, err := manager.OpenLocal(context.Background(), lease.ID, profiledomain.Profile{
		ID: "local", Name: "Local", Protocol: profiledomain.ProtocolLocal,
	}, 80, 24)
	if err != nil {
		t.Fatalf("open terminal: %v", err)
	}
	if err := desktop.ActivateTerminal(lease.ID, opened.ID, opened.Generation); err != nil {
		t.Fatalf("activate terminal: %v", err)
	}

	inputs := [][]byte{
		[]byte("lambda: \xce\xbb"),
		{0x1b, 0x5b, 0x4d, 0, 0xff},
		[]byte("pasted line\r"),
	}
	for index, input := range inputs {
		if err := desktop.WriteTerminal(
			lease.ID, opened.ID, opened.Generation, uint64(index+1),
			base64.StdEncoding.EncodeToString(input),
		); err != nil {
			t.Fatalf("write terminal input %d: %v", index+1, err)
		}
	}
	if err := desktop.WriteTerminal(lease.ID, opened.ID, opened.Generation, 4, "!!!!"); !apperror.IsCode(err, apperror.CodeInvalidArgument) {
		t.Fatalf("malformed terminal input error = %v", err)
	}
	if err := desktop.WriteTerminal(
		lease.ID, opened.ID, opened.Generation, 4, base64.StdEncoding.EncodeToString([]byte{'\r'}),
	); err != nil {
		t.Fatalf("write after malformed payload: %v", err)
	}

	if err := desktop.ResizeTerminal(lease.ID, opened.ID, opened.Generation, 81, 25); err != nil {
		t.Fatalf("resize terminal: %v", err)
	}
	if err := desktop.ResizeTerminal(lease.ID, opened.ID, opened.Generation, 120, 40); err != nil {
		t.Fatalf("resize terminal to final dimensions: %v", err)
	}

	want := bytes.Join(append(inputs, []byte{'\r'}), nil)
	input, columns, rows := transport.snapshot()
	if !bytes.Equal(input, want) {
		t.Fatalf("terminal input bytes changed: %v", input)
	}
	if columns != 120 || rows != 40 {
		t.Fatalf("final terminal size = %dx%d, want 120x40", columns, rows)
	}
	if err := desktop.CloseTerminal(lease.ID, opened.ID, opened.Generation); err != nil {
		t.Fatalf("close terminal: %v", err)
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

var _ port.TerminalFactory = (*bridgeTerminalFactory)(nil)
var _ port.TerminalTransport = (*bridgeTerminalTransport)(nil)
