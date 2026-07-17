package app

import (
	"io/fs"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"shh-h/internal/adapter/configstore"
	"shh-h/internal/adapter/localpty"
	"shh-h/internal/adapter/remotepathstore"
	"shh-h/internal/adapter/sessionlog"
	"shh-h/internal/adapter/settingsstore"
	"shh-h/internal/adapter/sftpfs"
	"shh-h/internal/adapter/snippetstore"
	"shh-h/internal/adapter/sshterminal"
	"shh-h/internal/adapter/sshtrust"
	"shh-h/internal/adapter/tunnelstore"
	"shh-h/internal/adapter/wailsnotification"
	"shh-h/internal/adapter/workspacestore"
	"shh-h/internal/bridge"
	filetransferusecase "shh-h/internal/usecase/filetransfer"
	notificationusecase "shh-h/internal/usecase/notification"
	profileusecase "shh-h/internal/usecase/profile"
	remotepathusecase "shh-h/internal/usecase/remotepath"
	sessionusecase "shh-h/internal/usecase/session"
	settingsusecase "shh-h/internal/usecase/settings"
	snippetusecase "shh-h/internal/usecase/snippet"
	sshconnectionusecase "shh-h/internal/usecase/sshconnection"
	tunnelusecase "shh-h/internal/usecase/tunnel"
	workspaceusecase "shh-h/internal/usecase/workspace"
)

const (
	appID                = "dev.johannes.shhh"
	singleInstanceUnique = "3a20ab4f-f760-4f88-8105-99b8f347bc99"
)

func Run(assets fs.FS) error {
	store, err := configstore.New(appID)
	if err != nil {
		return err
	}
	profiles, err := profileusecase.NewService(store)
	if err != nil {
		return err
	}
	settingsRepository, err := settingsstore.New(appID)
	if err != nil {
		return err
	}
	settingsService, err := settingsusecase.NewService(settingsRepository)
	if err != nil {
		return err
	}
	notifications, err := notificationusecase.NewService(wailsnotification.New(), settingsService)
	if err != nil {
		return err
	}

	manager := sessionusecase.NewManager(localpty.NewFactory())
	logFactory, err := sessionlog.New(appID)
	if err != nil {
		return err
	}
	manager.SetLogFactory(logFactory)
	trust, err := sshtrust.New(appID)
	if err != nil {
		return err
	}
	sshFactory := sshterminal.NewFactory(trust)
	manager.SetSSHFactory(sshFactory)
	files := filetransferusecase.NewManager(sftpfs.NewFactory(sshFactory))
	tunnelRepository, err := tunnelstore.New(appID)
	if err != nil {
		return err
	}
	tunnels, err := tunnelusecase.NewService(tunnelRepository, profiles, sshFactory)
	if err != nil {
		return err
	}
	snippetRepository, err := snippetstore.New(appID)
	if err != nil {
		return err
	}
	snippets, err := snippetusecase.NewService(snippetRepository)
	if err != nil {
		return err
	}
	workspaceRepository, err := workspacestore.New(appID)
	if err != nil {
		return err
	}
	workspaces, err := workspaceusecase.NewService(workspaceRepository)
	if err != nil {
		return err
	}
	remotePathRepository, err := remotepathstore.New(appID)
	if err != nil {
		return err
	}
	remotePaths, err := remotepathusecase.NewService(remotePathRepository)
	if err != nil {
		return err
	}
	remote := sshconnectionusecase.NewService(profiles, manager, files, trust, sshFactory)
	desktop := bridge.NewDesktop(manager, profiles, remote, files, tunnels, snippets, workspaces, remotePaths, notifications, settingsService)

	return wails.Run(&options.App{
		Title:                    "shh-h",
		Width:                    1240,
		Height:                   780,
		MinWidth:                 860,
		MinHeight:                560,
		BackgroundColour:         options.NewRGB(14, 17, 22),
		AssetServer:              &assetserver.Options{Assets: assets},
		OnStartup:                desktop.Startup,
		OnDomReady:               desktop.DomReady,
		OnBeforeClose:            desktop.BeforeClose,
		OnShutdown:               desktop.Shutdown,
		LogLevelProduction:       logger.ERROR,
		EnableDefaultContextMenu: false,
		BindingsAllowedOrigins:   "",
		DragAndDrop:              &options.DragAndDrop{DisableWebViewDrop: true},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               singleInstanceUnique,
			OnSecondInstanceLaunch: bridge.SecondInstanceHandler(desktop),
		},
		Mac:  &mac.Options{DisableZoom: false},
		Bind: []interface{}{desktop},
	})
}
