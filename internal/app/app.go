package app

import (
	"context"
	"io/fs"
	"log"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"shh-h/internal/apperror"
	"shh-h/internal/bridge"
	settingsdomain "shh-h/internal/domain/settings"
	"shh-h/internal/terminalbenchmark"
)

const (
	appID                   = "dev.johannes.shhh"
	singleInstanceUnique    = "3a20ab4f-f760-4f88-8105-99b8f347bc99"
	benchmarkInstanceUnique = "57696e64-f39d-4b07-a26c-cdaf26395558"
)

type compositionFactory func() (*runtimeComposition, error)

type application struct {
	desktop           *bridge.Desktop
	desktopController bridge.DesktopController
	compose           compositionFactory

	startOnce       sync.Once
	startErr        error
	startupObserved atomic.Bool

	lifecycleMu sync.Mutex
	runtime     *runtimeComposition
	closed      bool
}

func newApplication(compose compositionFactory) *application {
	desktop, controller := bridge.NewDeferredDesktop()
	return &application{desktop: desktop, desktopController: controller, compose: compose}
}

func Run(assets fs.FS) error {
	application := newApplication(composeRuntime)
	defer func() {
		if err := application.close(context.Background()); err != nil {
			log.Printf("shut down application services: %v", err)
		}
	}()
	return wails.Run(application.options(assets))
}

func (application *application) options(assets fs.FS) *options.App {
	instanceUnique := singleInstanceUnique
	if terminalbenchmark.EnabledInEnvironment() {
		instanceUnique = benchmarkInstanceUnique
	}
	return &options.App{
		Title:                    "shh-h",
		Width:                    settingsdomain.DefaultWindowWidth,
		Height:                   settingsdomain.DefaultWindowHeight,
		MinWidth:                 settingsdomain.MinWindowWidth,
		MinHeight:                settingsdomain.MinWindowHeight,
		BackgroundColour:         options.NewRGB(14, 17, 22),
		AssetServer:              &assetserver.Options{Assets: assets},
		OnStartup:                application.startup,
		OnDomReady:               application.domReady,
		OnBeforeClose:            application.beforeClose,
		OnShutdown:               application.shutdown,
		LogLevelProduction:       logger.ERROR,
		ErrorFormatter:           apperror.Format,
		EnableDefaultContextMenu: false,
		BindingsAllowedOrigins:   "",
		DragAndDrop:              &options.DragAndDrop{DisableWebViewDrop: true},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               instanceUnique,
			OnSecondInstanceLaunch: bridge.SecondInstanceHandler(application.desktop),
		},
		Mac:  &mac.Options{DisableZoom: false},
		Bind: []interface{}{application.desktop},
	}
}

func (application *application) startup(ctx context.Context) {
	// Wails v2 invokes OnStartup before acquiring its Linux single-instance
	// lock, so this hook must remain free of stores, migrations, and runtimes.
	application.startupObserved.Store(true)
	application.desktopController.Prepare(ctx)
}

func (application *application) domReady(ctx context.Context) {
	if err := application.start(ctx); err != nil {
		log.Printf("start application services: %v", err)
	}
	benchmark, _ := application.benchmarkAndLiveTerminals()
	if benchmark != nil {
		if err := benchmark.RecordLifecycleStartup(application.startupObserved.Load()); err != nil {
			log.Printf("record lifecycle startup: %v", err)
		}
	}
	application.desktopController.DomReady(ctx)
	if benchmark != nil {
		if err := benchmark.RecordLifecycleDomReady(); err != nil {
			log.Printf("record lifecycle DOM ready: %v", err)
		}
	}
}

func (application *application) beforeClose(ctx context.Context) bool {
	benchmark, liveBefore := application.benchmarkAndLiveTerminals()
	prevented := application.desktopController.BeforeClose(ctx)
	_, liveAfter := application.benchmarkAndLiveTerminals()
	if benchmark != nil {
		if err := benchmark.RecordLifecycleBeforeClose(prevented, liveBefore, liveAfter); err != nil {
			log.Printf("record lifecycle close attempt: %v", err)
		}
	}
	return prevented
}

func (application *application) start(ctx context.Context) error {
	application.startOnce.Do(func() {
		application.lifecycleMu.Lock()
		defer application.lifecycleMu.Unlock()

		if application.closed {
			application.startErr = startupError(context.Canceled)
			application.desktopController.Fail(application.startErr)
			return
		}
		if application.compose == nil {
			application.startErr = startupError(apperror.New(apperror.CodeInternal, "Application composition is unavailable."))
			application.desktopController.Fail(application.startErr)
			return
		}

		composition, err := application.compose()
		if err != nil {
			application.startErr = startupError(err)
			application.desktopController.Fail(application.startErr)
			return
		}
		if composition == nil {
			application.startErr = startupError(apperror.New(apperror.CodeInternal, "Application composition returned no services."))
			application.desktopController.Fail(application.startErr)
			return
		}
		if err := application.desktopController.Start(ctx, composition.dependencies); err != nil {
			_ = composition.Shutdown()
			application.startErr = startupError(err)
			application.desktopController.Fail(application.startErr)
			return
		}
		application.runtime = composition
	})
	return application.startErr
}

func startupError(err error) error {
	return apperror.Wrap(
		apperror.CodeUnavailable,
		"start application",
		"Application services could not be initialized.",
		err,
	)
}

func (application *application) shutdown(ctx context.Context) {
	benchmark, _ := application.benchmarkAndLiveTerminals()
	err := application.close(ctx)
	if err != nil {
		log.Printf("shut down application services: %v", err)
	}
	if benchmark != nil {
		if recordErr := benchmark.RecordLifecycleShutdown(err == nil); recordErr != nil {
			log.Printf("record lifecycle shutdown: %v", recordErr)
		}
	}
}

func (application *application) benchmarkAndLiveTerminals() (*terminalbenchmark.Service, int) {
	application.lifecycleMu.Lock()
	defer application.lifecycleMu.Unlock()
	if application.runtime == nil {
		return nil, 0
	}
	manager := application.runtime.dependencies.Manager
	if manager == nil {
		return application.runtime.dependencies.Benchmark, 0
	}
	return application.runtime.dependencies.Benchmark, manager.LiveCount()
}

func (application *application) close(ctx context.Context) error {
	application.lifecycleMu.Lock()
	defer application.lifecycleMu.Unlock()
	if application.closed {
		return nil
	}
	application.closed = true
	application.desktopController.Shutdown(ctx)
	return application.runtime.Shutdown()
}
