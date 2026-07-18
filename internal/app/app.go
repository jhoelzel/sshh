package app

import (
	"context"
	"io/fs"
	"log"
	"sync"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"shh-h/internal/apperror"
	"shh-h/internal/bridge"
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

	startOnce sync.Once
	startErr  error

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
		Width:                    1240,
		Height:                   780,
		MinWidth:                 860,
		MinHeight:                560,
		BackgroundColour:         options.NewRGB(14, 17, 22),
		AssetServer:              &assetserver.Options{Assets: assets},
		OnStartup:                application.startup,
		OnDomReady:               application.domReady,
		OnBeforeClose:            application.desktopController.BeforeClose,
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
	application.desktopController.Prepare(ctx)
}

func (application *application) domReady(ctx context.Context) {
	if err := application.start(ctx); err != nil {
		log.Printf("start application services: %v", err)
	}
	application.desktopController.DomReady(ctx)
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
	if err := application.close(ctx); err != nil {
		log.Printf("shut down application services: %v", err)
	}
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
