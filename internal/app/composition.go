package app

import (
	"sync"

	"shh-h/internal/adapter/configstore"
	"shh-h/internal/adapter/localpty"
	"shh-h/internal/adapter/remotepathstore"
	"shh-h/internal/adapter/sessionlog"
	"shh-h/internal/adapter/settingsstore"
	"shh-h/internal/adapter/sftpfs"
	"shh-h/internal/adapter/snippetstore"
	"shh-h/internal/adapter/sshclient"
	"shh-h/internal/adapter/sshterminal"
	"shh-h/internal/adapter/sshtrust"
	"shh-h/internal/adapter/transferstore"
	"shh-h/internal/adapter/tunnelstore"
	"shh-h/internal/adapter/wailsnotification"
	"shh-h/internal/adapter/workspacestore"
	"shh-h/internal/bridge"
	"shh-h/internal/terminalbenchmark"
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

type runtimeComposition struct {
	dependencies bridge.Dependencies
	close        func() error

	closeOnce sync.Once
	closeErr  error
}

func (composition *runtimeComposition) Shutdown() error {
	if composition == nil {
		return nil
	}
	composition.closeOnce.Do(func() {
		if composition.close != nil {
			composition.closeErr = composition.close()
		}
	})
	return composition.closeErr
}

func composeRuntime() (_ *runtimeComposition, resultErr error) {
	benchmark, err := terminalbenchmark.NewServiceFromEnvironment()
	if err != nil {
		return nil, err
	}
	store, err := configstore.New(appID)
	if err != nil {
		return nil, err
	}
	profiles, err := profileusecase.NewService(store)
	if err != nil {
		return nil, err
	}
	settingsRepository, err := settingsstore.New(appID)
	if err != nil {
		return nil, err
	}
	settingsService, err := settingsusecase.NewService(settingsRepository)
	if err != nil {
		return nil, err
	}
	notifications, err := notificationusecase.NewService(wailsnotification.New(), settingsService)
	if err != nil {
		return nil, err
	}

	manager := sessionusecase.NewManager(localpty.NewFactory())
	logFactory, err := sessionlog.New(appID)
	if err != nil {
		return nil, err
	}
	manager.SetLogFactory(logFactory)
	trust, err := sshtrust.New(appID, settingsService)
	if err != nil {
		return nil, err
	}
	sshDialer := sshterminal.NewDialer(trust)
	sshClients := sshclient.NewPool(sshDialer, settingsService)
	defer func() {
		if resultErr != nil {
			_ = sshClients.Shutdown()
		}
	}()
	sshFactory := sshterminal.NewFactory(sshClients)
	manager.SetSSHFactory(sshFactory)
	transferRepository, err := transferstore.New(appID)
	if err != nil {
		return nil, err
	}
	files, err := filetransferusecase.NewManagerWithResumeRepository(sftpfs.NewFactory(sshClients), transferRepository)
	if err != nil {
		return nil, err
	}
	if err := files.SetConcurrency(settingsService.Get().Transfers.Concurrency); err != nil {
		return nil, err
	}
	tunnelRepository, err := tunnelstore.New(appID)
	if err != nil {
		return nil, err
	}
	tunnels, err := tunnelusecase.NewService(tunnelRepository, profiles, sshClients)
	if err != nil {
		return nil, err
	}
	snippetRepository, err := snippetstore.New(appID)
	if err != nil {
		return nil, err
	}
	snippets, err := snippetusecase.NewService(snippetRepository)
	if err != nil {
		return nil, err
	}
	workspaceRepository, err := workspacestore.New(appID)
	if err != nil {
		return nil, err
	}
	workspaces, err := workspaceusecase.NewService(workspaceRepository)
	if err != nil {
		return nil, err
	}
	remotePathRepository, err := remotepathstore.New(appID)
	if err != nil {
		return nil, err
	}
	remotePaths, err := remotepathusecase.NewService(remotePathRepository)
	if err != nil {
		return nil, err
	}
	remote := sshconnectionusecase.NewService(profiles, manager, files, trust, sshDialer)

	return &runtimeComposition{
		dependencies: bridge.Dependencies{
			Manager: manager, Profiles: profiles, Remote: remote, Files: files, Tunnels: tunnels,
			Snippets: snippets, Workspaces: workspaces, RemotePaths: remotePaths,
			Notifications: notifications, Settings: settingsService,
			Benchmark: benchmark,
		},
		close: sshClients.Shutdown,
	}, nil
}
