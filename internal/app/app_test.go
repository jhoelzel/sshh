package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"shh-h/internal/apperror"
	"shh-h/internal/bridge"
	"shh-h/internal/terminalbenchmark"
	sessionusecase "shh-h/internal/usecase/session"
)

func TestApplicationDefersCompositionUntilDomReady(t *testing.T) {
	t.Setenv(terminalbenchmark.EnvironmentEnabled, "")
	composeCalls := 0
	closeCalls := 0
	application := newApplication(func() (*runtimeComposition, error) {
		composeCalls++
		return &runtimeComposition{
			dependencies: bridge.Dependencies{Manager: sessionusecase.NewManager(nil)},
			close: func() error {
				closeCalls++
				return nil
			},
		}, nil
	})
	t.Cleanup(func() { _ = application.close(context.Background()) })

	appOptions := application.options(fstest.MapFS{
		"index.html": {Data: []byte("<!doctype html>")},
	})
	if composeCalls != 0 {
		t.Fatalf("building Wails options composed services %d times", composeCalls)
	}
	if appOptions.SingleInstanceLock == nil || appOptions.SingleInstanceLock.UniqueId != singleInstanceUnique {
		t.Fatal("Wails single-instance lock is not configured")
	}
	if appOptions.Width != 1240 || appOptions.Height != 780 || appOptions.MinWidth != 860 || appOptions.MinHeight != 560 {
		t.Fatalf(
			"Wails window contract = %dx%d min %dx%d",
			appOptions.Width, appOptions.Height, appOptions.MinWidth, appOptions.MinHeight,
		)
	}

	appOptions.OnStartup(context.Background())
	if composeCalls != 0 {
		t.Fatalf("OnStartup composed services %d times before the native instance decision", composeCalls)
	}

	ready := make(chan error, 1)
	go func() { ready <- application.desktop.AwaitReady() }()
	select {
	case err := <-ready:
		t.Fatalf("frontend became ready before DOM-ready composition: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	appOptions.OnDomReady(context.Background())
	if composeCalls != 1 {
		t.Fatalf("DOM-ready composition calls = %d, want 1", composeCalls)
	}
	if err := <-ready; err != nil {
		t.Fatalf("await application readiness: %v", err)
	}

	appOptions.OnDomReady(context.Background())
	if composeCalls != 1 {
		t.Fatalf("repeated DOM-ready composition calls = %d, want 1", composeCalls)
	}

	appOptions.OnShutdown(context.Background())
	appOptions.OnShutdown(context.Background())
	if closeCalls != 1 {
		t.Fatalf("runtime close calls = %d, want 1", closeCalls)
	}
}

func TestBenchmarkUsesAnIndependentSingleInstanceLock(t *testing.T) {
	t.Setenv(terminalbenchmark.EnvironmentEnabled, " 1 ")
	application := newApplication(nil)
	appOptions := application.options(fstest.MapFS{
		"index.html": {Data: []byte("<!doctype html>")},
	})
	if appOptions.SingleInstanceLock == nil || appOptions.SingleInstanceLock.UniqueId != benchmarkInstanceUnique {
		t.Fatal("benchmark Wails single-instance lock is not isolated from the product application")
	}
}

func TestApplicationRecordsWailsLifecycleHooks(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test executable: %v", err)
	}
	reportPath := filepath.Join(t.TempDir(), "lifecycle.json")
	benchmark, err := terminalbenchmark.NewServiceWithMode(executable, reportPath, terminalbenchmark.ModeLifecycle)
	if err != nil {
		t.Fatalf("create lifecycle recorder: %v", err)
	}
	application := newApplication(func() (*runtimeComposition, error) {
		return &runtimeComposition{dependencies: bridge.Dependencies{
			Manager:   sessionusecase.NewManager(nil),
			Benchmark: benchmark,
		}}, nil
	})
	t.Cleanup(func() { _ = application.close(context.Background()) })

	appOptions := application.options(fstest.MapFS{
		"index.html": {Data: []byte("<!doctype html>")},
	})
	ctx := context.Background()
	appOptions.OnStartup(ctx)
	appOptions.OnDomReady(ctx)
	if prevented := appOptions.OnBeforeClose(ctx); prevented {
		t.Fatal("empty lifecycle test prevented native close")
	}
	appOptions.OnShutdown(ctx)

	report, err := terminalbenchmark.ReadLifecycleReport(reportPath)
	if err != nil {
		t.Fatalf("read lifecycle hook report: %v", err)
	}
	if !report.StartupObserved || !report.DomReadyObserved || !report.ShutdownCompleted || !report.ShutdownSucceeded {
		t.Fatalf("application lifecycle hooks were not recorded: %#v", report)
	}
	if len(report.CloseAttempts) != 1 || report.CloseAttempts[0].Prevented ||
		report.CloseAttempts[0].LiveTerminalsBefore != 0 || report.CloseAttempts[0].LiveTerminalsAfter != 0 {
		t.Fatalf("empty native close attempt was not recorded: %#v", report.CloseAttempts)
	}
}

func TestApplicationPublishesCompositionFailureOnce(t *testing.T) {
	want := errors.New("load settings")
	composeCalls := 0
	application := newApplication(func() (*runtimeComposition, error) {
		composeCalls++
		return nil, want
	})
	t.Cleanup(func() { _ = application.close(context.Background()) })

	first := application.start(context.Background())
	second := application.start(context.Background())
	if !errors.Is(first, want) || !errors.Is(second, want) {
		t.Fatalf("startup errors = (%v, %v), want wrapped %v", first, second, want)
	}
	if !apperror.IsCode(first, apperror.CodeUnavailable) {
		t.Fatalf("startup error code = %q, want %q", apperror.CodeOf(first), apperror.CodeUnavailable)
	}
	if composeCalls != 1 {
		t.Fatalf("failed composition calls = %d, want 1", composeCalls)
	}
	if readyErr := application.desktop.AwaitReady(); !errors.Is(readyErr, want) {
		t.Fatalf("frontend readiness error = %v, want wrapped %v", readyErr, want)
	}
}

func TestApplicationClosesCompositionRejectedByDesktop(t *testing.T) {
	closeCalls := 0
	application := newApplication(func() (*runtimeComposition, error) {
		return &runtimeComposition{
			dependencies: bridge.Dependencies{},
			close: func() error {
				closeCalls++
				return nil
			},
		}, nil
	})
	t.Cleanup(func() { _ = application.close(context.Background()) })

	err := application.start(context.Background())
	if !apperror.IsCode(err, apperror.CodeUnavailable) {
		t.Fatalf("startup error code = %q, want %q", apperror.CodeOf(err), apperror.CodeUnavailable)
	}
	if closeCalls != 1 {
		t.Fatalf("rejected composition close calls = %d, want 1", closeCalls)
	}
	if readyErr := application.desktop.AwaitReady(); readyErr == nil {
		t.Fatal("frontend readiness succeeded after desktop rejected composition")
	}
}
