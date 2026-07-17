package bridge

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/wailsapp/wails/v2/pkg/options"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"shh-h/internal/adapter/profileexchange"
	"shh-h/internal/adapter/textfile"
	filedomain "shh-h/internal/domain/filetransfer"
	"shh-h/internal/domain/profile"
	remotepathdomain "shh-h/internal/domain/remotepath"
	settingsdomain "shh-h/internal/domain/settings"
	snippetdomain "shh-h/internal/domain/snippet"
	sshconnectiondomain "shh-h/internal/domain/sshconnection"
	tunneldomain "shh-h/internal/domain/tunnel"
	workspacedomain "shh-h/internal/domain/workspace"
	"shh-h/internal/port"
	filetransferusecase "shh-h/internal/usecase/filetransfer"
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
	EventTerminalOutput = "shhh:terminal-output"
	EventSessionState   = "shhh:session-state"
	EventCloseRequested = "shhh:close-requested"
	EventTransfer       = "shhh:transfer"
	EventTunnel         = "shhh:tunnel"
	EventSessionLog     = "shhh:session-log"
	leaseTimeout        = 30 * time.Second
	leaseCheckInterval  = 5 * time.Second
)

type ProfileDTO struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	Protocol         profile.Protocol       `json:"protocol"`
	Host             string                 `json:"host"`
	Port             int                    `json:"port"`
	Username         string                 `json:"username"`
	Authentication   profile.Authentication `json:"authentication"`
	IdentityFile     string                 `json:"identityFile"`
	Shell            string                 `json:"shell"`
	Arguments        []string               `json:"arguments"`
	WorkingDirectory string                 `json:"workingDirectory"`
	Environment      map[string]string      `json:"environment"`
	Tags             []string               `json:"tags"`
	Group            string                 `json:"group"`
	Favorite         bool                   `json:"favorite"`
	Endpoint         string                 `json:"endpoint"`
	Connectable      bool                   `json:"connectable"`
}

type ProfileInputDTO struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	Protocol         profile.Protocol       `json:"protocol"`
	Host             string                 `json:"host"`
	Port             int                    `json:"port"`
	Username         string                 `json:"username"`
	Authentication   profile.Authentication `json:"authentication"`
	IdentityFile     string                 `json:"identityFile"`
	Shell            string                 `json:"shell"`
	Arguments        []string               `json:"arguments"`
	WorkingDirectory string                 `json:"workingDirectory"`
	Environment      map[string]string      `json:"environment"`
	Tags             []string               `json:"tags"`
	Group            string                 `json:"group"`
	Favorite         bool                   `json:"favorite"`
}

type ProfileImportDTO struct {
	Cancelled bool         `json:"cancelled"`
	Format    string       `json:"format"`
	Filename  string       `json:"filename"`
	Imported  []ProfileDTO `json:"imported"`
	Warnings  []string     `json:"warnings"`
}

type ProfileExportDTO struct {
	Cancelled bool   `json:"cancelled"`
	Filename  string `json:"filename"`
	Exported  int    `json:"exported"`
}

type SSHHostKeyDTO struct {
	Status      sshconnectiondomain.HostKeyStatus `json:"status"`
	Host        string                            `json:"host"`
	Address     string                            `json:"address"`
	Algorithm   string                            `json:"algorithm"`
	Fingerprint string                            `json:"fingerprint"`
	ChallengeID string                            `json:"challengeId"`
}

type SSHAuthenticationDTO struct {
	Secret       sshconnectiondomain.SecretRequirement `json:"secret"`
	IdentityFile string                                `json:"identityFile"`
}

type SSHCredentialsDTO struct {
	Password   string `json:"password"`
	Passphrase string `json:"passphrase"`
}

type QuickSSHInputDTO struct {
	Host           string                 `json:"host"`
	Port           int                    `json:"port"`
	Username       string                 `json:"username"`
	Authentication profile.Authentication `json:"authentication"`
	IdentityFile   string                 `json:"identityFile"`
}

type QuickSSHProbeDTO struct {
	Profile ProfileDTO    `json:"profile"`
	HostKey SSHHostKeyDTO `json:"hostKey"`
}

type TunnelInputDTO struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	ProfileID       string            `json:"profileId"`
	Kind            tunneldomain.Kind `json:"kind"`
	BindAddress     string            `json:"bindAddress"`
	BindPort        int               `json:"bindPort"`
	DestinationHost string            `json:"destinationHost"`
	DestinationPort int               `json:"destinationPort"`
	AutoStart       bool              `json:"autoStart"`
	Reconnect       bool              `json:"reconnect"`
}

type SnippetInputDTO struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Folder string   `json:"folder"`
	Tags   []string `json:"tags"`
	Body   string   `json:"body"`
}

type SnippetDTO struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Folder    string   `json:"folder"`
	Tags      []string `json:"tags"`
	Body      string   `json:"body"`
	Variables []string `json:"variables"`
	CreatedAt string   `json:"createdAt"`
	UpdatedAt string   `json:"updatedAt"`
}

type SnippetPreviewDTO struct {
	Text      string   `json:"text"`
	Variables []string `json:"variables"`
}

type WorkspaceTabDTO struct {
	ProfileID string `json:"profileId"`
	Title     string `json:"title"`
	Endpoint  string `json:"endpoint"`
}

type WorkspaceLayoutInputDTO struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Tabs      []WorkspaceTabDTO `json:"tabs"`
	ActiveTab int               `json:"activeTab"`
}

type WorkspaceLayoutDTO struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Tabs      []WorkspaceTabDTO `json:"tabs"`
	ActiveTab int               `json:"activeTab"`
	CreatedAt string            `json:"createdAt"`
	UpdatedAt string            `json:"updatedAt"`
}

type RemotePathFavoriteDTO struct {
	ID        string `json:"id"`
	ProfileID string `json:"profileId"`
	Path      string `json:"path"`
	CreatedAt string `json:"createdAt"`
}

type TerminalSettingsDTO struct {
	FontFamily  settingsdomain.FontFamily  `json:"fontFamily"`
	FontSize    int                        `json:"fontSize"`
	LineHeight  float64                    `json:"lineHeight"`
	CursorStyle settingsdomain.CursorStyle `json:"cursorStyle"`
	CursorBlink bool                       `json:"cursorBlink"`
	Scrollback  int                        `json:"scrollback"`
	Bell        bool                       `json:"bell"`
}

type SettingsDTO struct {
	Terminal TerminalSettingsDTO `json:"terminal"`
}

type FrontendLeaseDTO struct {
	ID        string `json:"id"`
	ExpiresAt string `json:"expiresAt"`
}

type TerminalOutputDTO struct {
	LeaseID    string `json:"leaseId"`
	SessionID  string `json:"sessionId"`
	Generation uint64 `json:"generation"`
	Sequence   uint64 `json:"sequence"`
	EndOffset  uint64 `json:"endOffset"`
	ByteCount  int    `json:"byteCount"`
	Payload    string `json:"payload"`
	Final      bool   `json:"final"`
}

type TerminalTextExportDTO struct {
	Cancelled bool   `json:"cancelled"`
	Filename  string `json:"filename"`
	Bytes     int    `json:"bytes"`
}

type frontendLease struct {
	id        string
	nonce     string
	lastSeen  time.Time
	expiresAt time.Time
}

type Desktop struct {
	manager     *sessionusecase.Manager
	profiles    *profileusecase.Service
	remote      *sshconnectionusecase.Service
	files       *filetransferusecase.Manager
	tunnels     *tunnelusecase.Service
	snippets    *snippetusecase.Service
	workspaces  *workspaceusecase.Service
	remotePaths *remotepathusecase.Service
	settings    *settingsusecase.Service

	ctxMu sync.RWMutex
	ctx   context.Context

	leaseMu   sync.Mutex
	lease     *frontendLease
	leaseWake chan struct{}

	allowClose atomic.Bool
}

type eventSink struct {
	desktop *Desktop
}

func NewDesktop(manager *sessionusecase.Manager, profiles *profileusecase.Service, remote *sshconnectionusecase.Service, files *filetransferusecase.Manager, tunnels *tunnelusecase.Service, snippets *snippetusecase.Service, workspaces *workspaceusecase.Service, remotePaths *remotepathusecase.Service, settings *settingsusecase.Service) *Desktop {
	desktop := &Desktop{
		manager: manager, profiles: profiles, remote: remote, files: files, tunnels: tunnels,
		snippets: snippets, workspaces: workspaces, remotePaths: remotePaths, settings: settings,
		leaseWake: make(chan struct{}, 1),
	}
	sink := &eventSink{desktop: desktop}
	manager.SetSink(sink)
	if files != nil {
		files.SetSink(sink)
	}
	if tunnels != nil {
		tunnels.SetSink(sink)
	}
	return desktop
}

func (d *Desktop) Startup(ctx context.Context) {
	d.ctxMu.Lock()
	d.ctx = ctx
	d.ctxMu.Unlock()
	go d.monitorLease(ctx)
}

func (d *Desktop) DomReady(context.Context) {}

func (d *Desktop) BeforeClose(context.Context) bool {
	if d.allowClose.Load() || d.activeResourceCount() == 0 {
		d.manager.Shutdown()
		if d.files != nil {
			d.files.Shutdown()
		}
		if d.tunnels != nil {
			d.tunnels.Shutdown()
		}
		return false
	}
	wailsruntime.EventsEmit(d.context(), EventCloseRequested)
	return true
}

func (d *Desktop) Shutdown(context.Context) {
	d.manager.Shutdown()
	if d.files != nil {
		d.files.Shutdown()
	}
	if d.tunnels != nil {
		d.tunnels.Shutdown()
	}
}

func SecondInstanceHandler(desktop *Desktop) func(options.SecondInstanceData) {
	return desktop.handleSecondInstance
}

func (d *Desktop) handleSecondInstance(options.SecondInstanceData) {
	ctx := d.context()
	if ctx == nil {
		return
	}
	wailsruntime.WindowShow(ctx)
	wailsruntime.WindowUnminimise(ctx)
}

func (d *Desktop) ListProfiles() []ProfileDTO {
	if d.profiles == nil {
		return nil
	}
	profiles := d.profiles.List()
	result := make([]ProfileDTO, 0, len(profiles))
	for _, item := range profiles {
		result = append(result, profileDTO(item))
	}
	return result
}

func (d *Desktop) CreateProfile(input ProfileInputDTO) (ProfileDTO, error) {
	created, err := d.profiles.Create(profileFromInput(input))
	if err != nil {
		return ProfileDTO{}, err
	}
	return profileDTO(created), nil
}

func (d *Desktop) UpdateProfile(input ProfileInputDTO) (ProfileDTO, error) {
	updated, err := d.profiles.Update(profileFromInput(input))
	if err != nil {
		return ProfileDTO{}, err
	}
	return profileDTO(updated), nil
}

func (d *Desktop) DuplicateProfile(profileID string) (ProfileDTO, error) {
	duplicated, err := d.profiles.Duplicate(profileID)
	if err != nil {
		return ProfileDTO{}, err
	}
	return profileDTO(duplicated), nil
}

func (d *Desktop) DeleteProfile(profileID string) error {
	if d.tunnels != nil {
		for _, config := range d.tunnels.List() {
			if config.ProfileID == profileID {
				return fmt.Errorf("profile is used by tunnel %q", config.Name)
			}
		}
	}
	return d.profiles.Delete(profileID)
}

func (d *Desktop) ImportProfiles() (ProfileImportDTO, error) {
	if d.profiles == nil {
		return ProfileImportDTO{}, errors.New("profile service is unavailable")
	}
	filename, err := wailsruntime.OpenFileDialog(d.context(), wailsruntime.OpenDialogOptions{
		Title:           "Import profiles",
		ShowHiddenFiles: true,
		ResolvesAliases: true,
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Profile files", Pattern: "*.json;*.conf;*.config;config;ssh_config"},
			{DisplayName: "All files", Pattern: "*"},
		},
	})
	if err != nil {
		return ProfileImportDTO{}, fmt.Errorf("select profile file: %w", err)
	}
	if filename == "" {
		return ProfileImportDTO{Cancelled: true, Imported: []ProfileDTO{}, Warnings: []string{}}, nil
	}

	data, err := profileexchange.ReadFile(filename)
	if err != nil {
		return ProfileImportDTO{}, err
	}
	parsed, err := profileexchange.Parse(filename, data)
	if err != nil {
		return ProfileImportDTO{}, err
	}
	imported, err := d.profiles.Import(parsed.Profiles)
	if err != nil {
		return ProfileImportDTO{}, err
	}
	result := make([]ProfileDTO, 0, len(imported))
	for _, item := range imported {
		result = append(result, profileDTO(item))
	}
	return ProfileImportDTO{
		Format: string(parsed.Format), Filename: filepath.Base(filename),
		Imported: result, Warnings: append([]string(nil), parsed.Warnings...),
	}, nil
}

func (d *Desktop) ExportProfiles() (ProfileExportDTO, error) {
	if d.profiles == nil {
		return ProfileExportDTO{}, errors.New("profile service is unavailable")
	}
	filename, err := wailsruntime.SaveFileDialog(d.context(), wailsruntime.SaveDialogOptions{
		Title: "Export profiles", DefaultFilename: "shh-h-profiles.json",
		CanCreateDirectories: true, ShowHiddenFiles: true,
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "shh-h profiles", Pattern: "*.json"},
		},
	})
	if err != nil {
		return ProfileExportDTO{}, fmt.Errorf("select profile export: %w", err)
	}
	if filename == "" {
		return ProfileExportDTO{Cancelled: true}, nil
	}

	profiles := d.profiles.List()
	data, err := profileexchange.Encode(profiles)
	if err != nil {
		return ProfileExportDTO{}, err
	}
	if err := profileexchange.WriteFile(filename, data); err != nil {
		return ProfileExportDTO{}, err
	}
	return ProfileExportDTO{Filename: filepath.Base(filename), Exported: len(profiles)}, nil
}

func (d *Desktop) ExportTerminalText(title, text string) (TerminalTextExportDTO, error) {
	if text == "" {
		return TerminalTextExportDTO{}, errors.New("terminal selection is empty")
	}
	data := []byte(text)
	if len(data) > textfile.MaxBytes {
		return TerminalTextExportDTO{}, fmt.Errorf("terminal selection exceeds the %d MiB limit", textfile.MaxBytes/(1<<20))
	}
	filename, err := wailsruntime.SaveFileDialog(d.context(), wailsruntime.SaveDialogOptions{
		Title: "Export terminal selection", DefaultFilename: terminalTextFilename(title),
		CanCreateDirectories: true, ShowHiddenFiles: true,
		Filters: []wailsruntime.FileFilter{{DisplayName: "Text files", Pattern: "*.txt;*.log"}},
	})
	if err != nil {
		return TerminalTextExportDTO{}, fmt.Errorf("select terminal export: %w", err)
	}
	if filename == "" {
		return TerminalTextExportDTO{Cancelled: true}, nil
	}
	if err := textfile.WriteAtomic(filename, data); err != nil {
		return TerminalTextExportDTO{}, err
	}
	return TerminalTextExportDTO{Filename: filepath.Base(filename), Bytes: len(data)}, nil
}

func (d *Desktop) ListRemotePathFavorites() []RemotePathFavoriteDTO {
	if d.remotePaths == nil {
		return []RemotePathFavoriteDTO{}
	}
	favorites := d.remotePaths.List()
	result := make([]RemotePathFavoriteDTO, 0, len(favorites))
	for _, favorite := range favorites {
		result = append(result, remotePathFavoriteDTO(favorite))
	}
	return result
}

func (d *Desktop) CreateRemotePathFavorite(profileID, remotePath string) (RemotePathFavoriteDTO, error) {
	if d.remotePaths == nil || d.profiles == nil {
		return RemotePathFavoriteDTO{}, errors.New("remote path favorites are unavailable")
	}
	selected, exists := d.profiles.Find(strings.TrimSpace(profileID))
	if !exists {
		return RemotePathFavoriteDTO{}, errors.New("profile not found")
	}
	if selected.Protocol != profile.ProtocolSSH {
		return RemotePathFavoriteDTO{}, errors.New("remote path favorites require an SSH profile")
	}
	created, err := d.remotePaths.Create(selected.ID, remotePath)
	if err != nil {
		return RemotePathFavoriteDTO{}, err
	}
	return remotePathFavoriteDTO(created), nil
}

func (d *Desktop) DeleteRemotePathFavorite(favoriteID string) error {
	if d.remotePaths == nil {
		return errors.New("remote path favorites are unavailable")
	}
	return d.remotePaths.Delete(favoriteID)
}

func (d *Desktop) ListTunnels() []tunneldomain.Config {
	if d.tunnels == nil {
		return []tunneldomain.Config{}
	}
	return d.tunnels.List()
}

func (d *Desktop) CreateTunnel(input TunnelInputDTO) (tunneldomain.Config, error) {
	if d.tunnels == nil {
		return tunneldomain.Config{}, errors.New("SSH tunnel support is unavailable")
	}
	return d.tunnels.Create(tunnelFromInput(input))
}

func (d *Desktop) UpdateTunnel(input TunnelInputDTO) (tunneldomain.Config, error) {
	if d.tunnels == nil {
		return tunneldomain.Config{}, errors.New("SSH tunnel support is unavailable")
	}
	return d.tunnels.Update(tunnelFromInput(input))
}

func (d *Desktop) DeleteTunnel(configID string) error {
	if d.tunnels == nil {
		return errors.New("SSH tunnel support is unavailable")
	}
	return d.tunnels.Delete(configID)
}

func (d *Desktop) ListSnippets() ([]SnippetDTO, error) {
	if d.snippets == nil {
		return []SnippetDTO{}, nil
	}
	items := d.snippets.List()
	result := make([]SnippetDTO, 0, len(items))
	for _, item := range items {
		converted, err := snippetDTO(item)
		if err != nil {
			return nil, err
		}
		result = append(result, converted)
	}
	return result, nil
}

func (d *Desktop) CreateSnippet(input SnippetInputDTO) (SnippetDTO, error) {
	if d.snippets == nil {
		return SnippetDTO{}, errors.New("snippet support is unavailable")
	}
	created, err := d.snippets.Create(snippetFromInput(input))
	if err != nil {
		return SnippetDTO{}, err
	}
	return snippetDTO(created)
}

func (d *Desktop) UpdateSnippet(input SnippetInputDTO) (SnippetDTO, error) {
	if d.snippets == nil {
		return SnippetDTO{}, errors.New("snippet support is unavailable")
	}
	updated, err := d.snippets.Update(snippetFromInput(input))
	if err != nil {
		return SnippetDTO{}, err
	}
	return snippetDTO(updated)
}

func (d *Desktop) DeleteSnippet(snippetID string) error {
	if d.snippets == nil {
		return errors.New("snippet support is unavailable")
	}
	return d.snippets.Delete(snippetID)
}

func (d *Desktop) RenderSnippet(snippetID string, values map[string]string) (SnippetPreviewDTO, error) {
	if d.snippets == nil {
		return SnippetPreviewDTO{}, errors.New("snippet support is unavailable")
	}
	text, variables, err := d.snippets.Render(snippetID, values)
	if err != nil {
		return SnippetPreviewDTO{}, err
	}
	return SnippetPreviewDTO{Text: text, Variables: variables}, nil
}

func (d *Desktop) ListWorkspaceLayouts() []WorkspaceLayoutDTO {
	if d.workspaces == nil {
		return []WorkspaceLayoutDTO{}
	}
	layouts := d.workspaces.List()
	result := make([]WorkspaceLayoutDTO, 0, len(layouts))
	for _, layout := range layouts {
		result = append(result, workspaceLayoutDTO(layout))
	}
	return result
}

func (d *Desktop) CreateWorkspaceLayout(input WorkspaceLayoutInputDTO) (WorkspaceLayoutDTO, error) {
	if d.workspaces == nil {
		return WorkspaceLayoutDTO{}, errors.New("workspace layout support is unavailable")
	}
	created, err := d.workspaces.Create(workspaceLayoutFromInput(input))
	if err != nil {
		return WorkspaceLayoutDTO{}, err
	}
	return workspaceLayoutDTO(created), nil
}

func (d *Desktop) UpdateWorkspaceLayout(input WorkspaceLayoutInputDTO) (WorkspaceLayoutDTO, error) {
	if d.workspaces == nil {
		return WorkspaceLayoutDTO{}, errors.New("workspace layout support is unavailable")
	}
	updated, err := d.workspaces.Update(workspaceLayoutFromInput(input))
	if err != nil {
		return WorkspaceLayoutDTO{}, err
	}
	return workspaceLayoutDTO(updated), nil
}

func (d *Desktop) DeleteWorkspaceLayout(layoutID string) error {
	if d.workspaces == nil {
		return errors.New("workspace layout support is unavailable")
	}
	return d.workspaces.Delete(layoutID)
}

func (d *Desktop) GetSettings() SettingsDTO {
	if d.settings == nil {
		return settingsDTO(settingsdomain.Defaults())
	}
	return settingsDTO(d.settings.Get())
}

func (d *Desktop) UpdateSettings(input SettingsDTO) (SettingsDTO, error) {
	if d.settings == nil {
		return SettingsDTO{}, errors.New("settings support is unavailable")
	}
	updated, err := d.settings.Update(settingsFromDTO(input))
	if err != nil {
		return SettingsDTO{}, err
	}
	return settingsDTO(updated), nil
}

func (d *Desktop) ResetSettings() (SettingsDTO, error) {
	if d.settings == nil {
		return SettingsDTO{}, errors.New("settings support is unavailable")
	}
	reset, err := d.settings.Reset()
	if err != nil {
		return SettingsDTO{}, err
	}
	return settingsDTO(reset), nil
}

func (d *Desktop) AttachFrontend(instanceNonce string) (FrontendLeaseDTO, error) {
	instanceNonce = strings.TrimSpace(instanceNonce)
	if instanceNonce == "" || len(instanceNonce) > 128 {
		return FrontendLeaseDTO{}, errors.New("invalid frontend instance nonce")
	}
	now := time.Now()
	d.leaseMu.Lock()
	if d.lease != nil && d.lease.nonce == instanceNonce {
		d.lease.lastSeen = now
		d.lease.expiresAt = now.Add(leaseTimeout)
		current := leaseDTO(d.lease)
		d.leaseMu.Unlock()
		notify(d.leaseWake)
		return current, nil
	}
	d.leaseMu.Unlock()

	id, err := randomID()
	if err != nil {
		return FrontendLeaseDTO{}, err
	}

	next := &frontendLease{id: id, nonce: instanceNonce, lastSeen: now, expiresAt: now.Add(leaseTimeout)}
	d.leaseMu.Lock()
	previous := d.lease
	d.lease = next
	d.leaseMu.Unlock()
	notify(d.leaseWake)

	if previous != nil && previous.id != next.id {
		go func() {
			d.manager.CloseLease(previous.id)
			if d.files != nil {
				d.files.CloseLease(previous.id)
			}
			if d.tunnels != nil {
				d.tunnels.CloseLease(previous.id)
			}
		}()
	}
	return leaseDTO(next), nil
}

func (d *Desktop) RenewFrontendLease(leaseID string) (FrontendLeaseDTO, error) {
	return d.touchLease(leaseID)
}

func (d *Desktop) OpenLocalTerminal(leaseID, profileID string, columns, rows uint16) (sessionusecase.Session, error) {
	if _, err := d.touchLease(leaseID); err != nil {
		return sessionusecase.Session{}, err
	}
	selected, ok := d.findProfile(profileID)
	if !ok {
		return sessionusecase.Session{}, fmt.Errorf("profile %q not found", profileID)
	}
	session, err := d.manager.OpenLocal(d.context(), leaseID, selected, columns, rows)
	notify(d.leaseWake)
	return session, err
}

func (d *Desktop) ProbeSSHHostKey(leaseID, profileID string) (SSHHostKeyDTO, error) {
	if _, err := d.touchLease(leaseID); err != nil {
		return SSHHostKeyDTO{}, err
	}
	if d.remote == nil {
		return SSHHostKeyDTO{}, errors.New("ssh support is unavailable")
	}
	info, err := d.remote.ProbeHostKey(d.context(), leaseID, profileID)
	if err != nil {
		return SSHHostKeyDTO{}, err
	}
	return sshHostKeyDTO(info), nil
}

func (d *Desktop) ProbeQuickSSHHostKey(leaseID string, input QuickSSHInputDTO) (QuickSSHProbeDTO, error) {
	if _, err := d.touchLease(leaseID); err != nil {
		return QuickSSHProbeDTO{}, err
	}
	if d.remote == nil {
		return QuickSSHProbeDTO{}, errors.New("ssh support is unavailable")
	}
	selected, info, err := d.remote.ProbeQuickHostKey(d.context(), leaseID, quickSSHProfile(input))
	if err != nil {
		return QuickSSHProbeDTO{}, err
	}
	return QuickSSHProbeDTO{Profile: profileDTO(selected), HostKey: sshHostKeyDTO(info)}, nil
}

func (d *Desktop) TrustSSHHostKey(leaseID, challengeID string, permanent bool) error {
	if _, err := d.touchLease(leaseID); err != nil {
		return err
	}
	if d.remote == nil {
		return errors.New("ssh support is unavailable")
	}
	return d.remote.TrustHostKey(leaseID, challengeID, permanent)
}

func (d *Desktop) InspectSSHAuthentication(leaseID, profileID string) (SSHAuthenticationDTO, error) {
	if _, err := d.touchLease(leaseID); err != nil {
		return SSHAuthenticationDTO{}, err
	}
	if d.remote == nil {
		return SSHAuthenticationDTO{}, errors.New("ssh support is unavailable")
	}
	info, err := d.remote.InspectAuthentication(profileID)
	if err != nil {
		return SSHAuthenticationDTO{}, err
	}
	return SSHAuthenticationDTO{Secret: info.Secret, IdentityFile: info.IdentityFile}, nil
}

func (d *Desktop) InspectQuickSSHAuthentication(leaseID string, input QuickSSHInputDTO) (SSHAuthenticationDTO, error) {
	if _, err := d.touchLease(leaseID); err != nil {
		return SSHAuthenticationDTO{}, err
	}
	if d.remote == nil {
		return SSHAuthenticationDTO{}, errors.New("ssh support is unavailable")
	}
	info, err := d.remote.InspectQuickAuthentication(quickSSHProfile(input))
	if err != nil {
		return SSHAuthenticationDTO{}, err
	}
	return SSHAuthenticationDTO{Secret: info.Secret, IdentityFile: info.IdentityFile}, nil
}

func (d *Desktop) OpenSSHTerminal(leaseID, profileID string, columns, rows uint16, credentials SSHCredentialsDTO) (sessionusecase.Session, error) {
	if _, err := d.touchLease(leaseID); err != nil {
		return sessionusecase.Session{}, err
	}
	if d.remote == nil {
		return sessionusecase.Session{}, errors.New("ssh support is unavailable")
	}
	password := []byte(credentials.Password)
	passphrase := []byte(credentials.Passphrase)
	credentials.Password = ""
	credentials.Passphrase = ""
	defer clear(password)
	defer clear(passphrase)
	session, err := d.remote.Open(d.context(), leaseID, profileID, columns, rows, port.SSHCredentials{
		Password: password, Passphrase: passphrase,
	})
	notify(d.leaseWake)
	return session, err
}

func (d *Desktop) OpenQuickSSHTerminal(leaseID string, input QuickSSHInputDTO, columns, rows uint16, credentials SSHCredentialsDTO) (sessionusecase.Session, error) {
	if _, err := d.touchLease(leaseID); err != nil {
		return sessionusecase.Session{}, err
	}
	if d.remote == nil {
		return sessionusecase.Session{}, errors.New("ssh support is unavailable")
	}
	password := []byte(credentials.Password)
	passphrase := []byte(credentials.Passphrase)
	credentials.Password = ""
	credentials.Passphrase = ""
	defer clear(password)
	defer clear(passphrase)
	session, err := d.remote.OpenQuick(d.context(), leaseID, quickSSHProfile(input), columns, rows, port.SSHCredentials{
		Password: password, Passphrase: passphrase,
	})
	notify(d.leaseWake)
	return session, err
}

func (d *Desktop) OpenSFTP(leaseID, profileID string, credentials SSHCredentialsDTO) (filedomain.Session, error) {
	if _, err := d.touchLease(leaseID); err != nil {
		return filedomain.Session{}, err
	}
	if d.remote == nil || d.files == nil {
		return filedomain.Session{}, errors.New("sftp support is unavailable")
	}
	password := []byte(credentials.Password)
	passphrase := []byte(credentials.Passphrase)
	credentials.Password = ""
	credentials.Passphrase = ""
	defer clear(password)
	defer clear(passphrase)
	session, err := d.remote.OpenFiles(d.context(), leaseID, profileID, port.SSHCredentials{
		Password: password, Passphrase: passphrase,
	})
	notify(d.leaseWake)
	return session, err
}

func (d *Desktop) ListRemoteFiles(leaseID, sessionID, remotePath string) ([]filedomain.Entry, error) {
	if _, err := d.touchLease(leaseID); err != nil {
		return nil, err
	}
	return d.files.List(leaseID, sessionID, remotePath)
}

func (d *Desktop) CreateRemoteDirectory(leaseID, sessionID, remotePath string) error {
	if _, err := d.touchLease(leaseID); err != nil {
		return err
	}
	return d.files.CreateDirectory(leaseID, sessionID, remotePath)
}

func (d *Desktop) RenameRemotePath(leaseID, sessionID, source, destination string) error {
	if _, err := d.touchLease(leaseID); err != nil {
		return err
	}
	return d.files.Rename(leaseID, sessionID, source, destination)
}

func (d *Desktop) DeleteRemotePath(leaseID, sessionID, remotePath string) error {
	if _, err := d.touchLease(leaseID); err != nil {
		return err
	}
	return d.files.Remove(leaseID, sessionID, remotePath)
}

func (d *Desktop) ChmodRemotePath(leaseID, sessionID, remotePath string, mode uint32) error {
	if _, err := d.touchLease(leaseID); err != nil {
		return err
	}
	return d.files.Chmod(leaseID, sessionID, remotePath, mode)
}

func (d *Desktop) StartDownload(leaseID, sessionID, remotePath string) (filedomain.Transfer, error) {
	if _, err := d.touchLease(leaseID); err != nil {
		return filedomain.Transfer{}, err
	}
	localPath, err := wailsruntime.SaveFileDialog(d.context(), wailsruntime.SaveDialogOptions{
		Title: "Download remote file", DefaultFilename: path.Base(remotePath),
		CanCreateDirectories: true, ShowHiddenFiles: true,
	})
	if err != nil {
		return filedomain.Transfer{}, err
	}
	if localPath == "" {
		return filedomain.Transfer{}, nil
	}
	return d.files.StartDownload(leaseID, sessionID, remotePath, localPath, true)
}

func (d *Desktop) StartUpload(leaseID, sessionID, remoteDirectory string) (filedomain.Transfer, error) {
	if _, err := d.touchLease(leaseID); err != nil {
		return filedomain.Transfer{}, err
	}
	localPath, err := wailsruntime.OpenFileDialog(d.context(), wailsruntime.OpenDialogOptions{
		Title: "Upload file", ShowHiddenFiles: true,
	})
	if err != nil {
		return filedomain.Transfer{}, err
	}
	if localPath == "" {
		return filedomain.Transfer{}, nil
	}
	remotePath := path.Join(remoteDirectory, filepath.Base(localPath))
	return d.files.StartUpload(leaseID, sessionID, localPath, remotePath, false)
}

func (d *Desktop) ListTransfers(leaseID string) ([]filedomain.Transfer, error) {
	if _, err := d.touchLease(leaseID); err != nil {
		return nil, err
	}
	return d.files.Transfers(leaseID), nil
}

func (d *Desktop) CancelTransfer(leaseID, transferID string) error {
	if _, err := d.touchLease(leaseID); err != nil {
		return err
	}
	return d.files.CancelTransfer(leaseID, transferID)
}

func (d *Desktop) CloseSFTP(leaseID, sessionID string) error {
	if _, err := d.touchLease(leaseID); err != nil {
		return err
	}
	err := d.files.CloseSession(leaseID, sessionID)
	notify(d.leaseWake)
	return err
}

func (d *Desktop) StartTunnel(leaseID, configID string, credentials SSHCredentialsDTO) (tunneldomain.Snapshot, error) {
	if _, err := d.touchLease(leaseID); err != nil {
		return tunneldomain.Snapshot{}, err
	}
	if d.tunnels == nil {
		return tunneldomain.Snapshot{}, errors.New("SSH tunnel support is unavailable")
	}
	password := []byte(credentials.Password)
	passphrase := []byte(credentials.Passphrase)
	credentials.Password = ""
	credentials.Passphrase = ""
	defer clear(password)
	defer clear(passphrase)
	snapshot, err := d.tunnels.Start(d.context(), leaseID, configID, port.SSHCredentials{
		Password: password, Passphrase: passphrase,
	})
	notify(d.leaseWake)
	return snapshot, err
}

func (d *Desktop) StopTunnel(leaseID, configID string) error {
	if _, err := d.touchLease(leaseID); err != nil {
		return err
	}
	if d.tunnels == nil {
		return errors.New("SSH tunnel support is unavailable")
	}
	err := d.tunnels.Stop(leaseID, configID)
	notify(d.leaseWake)
	return err
}

func (d *Desktop) ListTunnelStates(leaseID string) ([]tunneldomain.Snapshot, error) {
	if _, err := d.touchLease(leaseID); err != nil {
		return nil, err
	}
	if d.tunnels == nil {
		return []tunneldomain.Snapshot{}, nil
	}
	return d.tunnels.Snapshots(leaseID), nil
}

func (d *Desktop) ActivateTerminal(leaseID, sessionID string, generation uint64) error {
	if _, err := d.touchLease(leaseID); err != nil {
		return err
	}
	return d.manager.Activate(leaseID, sessionID, generation)
}

func (d *Desktop) WriteTerminal(leaseID, sessionID string, generation, inputSequence uint64, payloadBase64 string) error {
	if _, err := d.touchLease(leaseID); err != nil {
		return err
	}
	data, err := base64.StdEncoding.DecodeString(payloadBase64)
	if err != nil {
		return errors.New("invalid terminal input payload")
	}
	return d.manager.Write(leaseID, sessionID, generation, inputSequence, data)
}

func (d *Desktop) ResizeTerminal(leaseID, sessionID string, generation uint64, columns, rows uint16) error {
	if _, err := d.touchLease(leaseID); err != nil {
		return err
	}
	return d.manager.Resize(leaseID, sessionID, generation, columns, rows)
}

func (d *Desktop) AcknowledgeTerminalOutput(leaseID, sessionID string, generation, throughSequence, bytesConsumed uint64) error {
	if _, err := d.touchLease(leaseID); err != nil {
		return err
	}
	return d.manager.Acknowledge(leaseID, sessionID, generation, throughSequence, bytesConsumed)
}

func (d *Desktop) StartSessionLogging(leaseID, sessionID string, generation uint64, timestampLines bool) (sessionusecase.SessionLogStatus, error) {
	if _, err := d.touchLease(leaseID); err != nil {
		return sessionusecase.SessionLogStatus{}, err
	}
	return d.manager.StartLogging(leaseID, sessionID, generation, timestampLines)
}

func (d *Desktop) StopSessionLogging(leaseID, sessionID string, generation uint64) (sessionusecase.SessionLogStatus, error) {
	if _, err := d.touchLease(leaseID); err != nil {
		return sessionusecase.SessionLogStatus{}, err
	}
	return d.manager.StopLogging(leaseID, sessionID, generation)
}

func (d *Desktop) SessionLoggingStatus(leaseID, sessionID string, generation uint64) (sessionusecase.SessionLogStatus, error) {
	if _, err := d.touchLease(leaseID); err != nil {
		return sessionusecase.SessionLogStatus{}, err
	}
	return d.manager.LoggingStatus(leaseID, sessionID, generation)
}

func (d *Desktop) CloseTerminal(leaseID, sessionID string, generation uint64) error {
	if _, err := d.touchLease(leaseID); err != nil {
		return err
	}
	err := d.manager.Close(leaseID, sessionID, generation)
	notify(d.leaseWake)
	return err
}

func (d *Desktop) ConfirmApplicationClose(leaseID string) error {
	if _, err := d.touchLease(leaseID); err != nil {
		return err
	}
	d.manager.Shutdown()
	if d.files != nil {
		d.files.Shutdown()
	}
	if d.tunnels != nil {
		d.tunnels.Shutdown()
	}
	d.allowClose.Store(true)
	wailsruntime.Quit(d.context())
	return nil
}

func (s *eventSink) PublishOutput(chunk sessionusecase.OutputChunk) {
	d := s.desktop
	wailsruntime.EventsEmit(d.context(), EventTerminalOutput, TerminalOutputDTO{
		LeaseID: chunk.LeaseID, SessionID: chunk.SessionID, Generation: chunk.Generation,
		Sequence: chunk.Sequence, EndOffset: chunk.EndOffset, ByteCount: len(chunk.Data),
		Payload: base64.StdEncoding.EncodeToString(chunk.Data), Final: chunk.Final,
	})
}

func (s *eventSink) PublishState(event sessionusecase.StateEvent) {
	d := s.desktop
	wailsruntime.EventsEmit(d.context(), EventSessionState, event)
	notify(d.leaseWake)
}

func (s *eventSink) PublishSessionLog(status sessionusecase.SessionLogStatus) {
	wailsruntime.EventsEmit(s.desktop.context(), EventSessionLog, status)
}

func (s *eventSink) PublishTransfer(transfer filedomain.Transfer) {
	wailsruntime.EventsEmit(s.desktop.context(), EventTransfer, transfer)
	notify(s.desktop.leaseWake)
}

func (s *eventSink) PublishTunnel(snapshot tunneldomain.Snapshot) {
	wailsruntime.EventsEmit(s.desktop.context(), EventTunnel, snapshot)
	notify(s.desktop.leaseWake)
}

func (d *Desktop) findProfile(id string) (profile.Profile, bool) {
	if d.profiles == nil {
		return profile.Profile{}, false
	}
	return d.profiles.Find(id)
}

func profileDTO(item profile.Profile) ProfileDTO {
	arguments := append([]string{}, item.Arguments...)
	tags := append([]string{}, item.Tags...)
	environment := make(map[string]string, len(item.Environment))
	for key, value := range item.Environment {
		environment[key] = value
	}
	return ProfileDTO{
		ID: item.ID, Name: item.Name, Protocol: item.Protocol,
		Host: item.Host, Port: item.Port, Username: item.Username,
		Authentication: item.Authentication, IdentityFile: item.IdentityFile,
		Shell: item.Shell, Arguments: arguments,
		WorkingDirectory: item.WorkingDirectory, Environment: environment,
		Tags: tags, Group: item.Group, Favorite: item.Favorite,
		Endpoint: item.Endpoint(), Connectable: item.Protocol == profile.ProtocolLocal || item.Protocol == profile.ProtocolSSH,
	}
}

func sshHostKeyDTO(info sshconnectiondomain.HostKeyInfo) SSHHostKeyDTO {
	return SSHHostKeyDTO{
		Status: info.Status, Host: info.Host, Address: info.Address,
		Algorithm: info.Algorithm, Fingerprint: info.Fingerprint, ChallengeID: info.ChallengeID,
	}
}

func profileFromInput(input ProfileInputDTO) profile.Profile {
	return profile.Profile{
		ID: input.ID, Name: input.Name, Protocol: input.Protocol,
		Host: input.Host, Port: input.Port, Username: input.Username,
		Authentication: input.Authentication, IdentityFile: input.IdentityFile,
		Shell: input.Shell, Arguments: append([]string(nil), input.Arguments...),
		WorkingDirectory: input.WorkingDirectory, Environment: input.Environment,
		Tags: append([]string(nil), input.Tags...), Group: input.Group, Favorite: input.Favorite,
	}
}

func quickSSHProfile(input QuickSSHInputDTO) profile.Profile {
	return profile.Profile{
		Protocol: profile.ProtocolSSH, Host: input.Host, Port: input.Port,
		Username: input.Username, Authentication: input.Authentication, IdentityFile: input.IdentityFile,
	}
}

func tunnelFromInput(input TunnelInputDTO) tunneldomain.Config {
	return tunneldomain.Config{
		ID: input.ID, Name: input.Name, ProfileID: input.ProfileID, Kind: input.Kind,
		BindAddress: input.BindAddress, BindPort: input.BindPort,
		DestinationHost: input.DestinationHost, DestinationPort: input.DestinationPort,
		AutoStart: input.AutoStart, Reconnect: input.Reconnect,
	}
}

func snippetFromInput(input SnippetInputDTO) snippetdomain.Snippet {
	return snippetdomain.Snippet{
		ID: input.ID, Name: input.Name, Folder: input.Folder,
		Tags: append([]string(nil), input.Tags...), Body: input.Body,
	}
}

func snippetDTO(item snippetdomain.Snippet) (SnippetDTO, error) {
	variables, err := snippetdomain.Variables(item.Body)
	if err != nil {
		return SnippetDTO{}, err
	}
	return SnippetDTO{
		ID: item.ID, Name: item.Name, Folder: item.Folder,
		Tags: append([]string{}, item.Tags...), Body: item.Body,
		Variables: variables,
		CreatedAt: item.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt: item.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}, nil
}

func workspaceLayoutFromInput(input WorkspaceLayoutInputDTO) workspacedomain.Layout {
	tabs := make([]workspacedomain.Tab, len(input.Tabs))
	for index, tab := range input.Tabs {
		tabs[index] = workspacedomain.Tab{ProfileID: tab.ProfileID, Title: tab.Title, Endpoint: tab.Endpoint}
	}
	return workspacedomain.Layout{ID: input.ID, Name: input.Name, Tabs: tabs, ActiveTab: input.ActiveTab}
}

func workspaceLayoutDTO(layout workspacedomain.Layout) WorkspaceLayoutDTO {
	tabs := make([]WorkspaceTabDTO, len(layout.Tabs))
	for index, tab := range layout.Tabs {
		tabs[index] = WorkspaceTabDTO{ProfileID: tab.ProfileID, Title: tab.Title, Endpoint: tab.Endpoint}
	}
	return WorkspaceLayoutDTO{
		ID: layout.ID, Name: layout.Name, Tabs: tabs, ActiveTab: layout.ActiveTab,
		CreatedAt: layout.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt: layout.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func settingsDTO(value settingsdomain.Settings) SettingsDTO {
	return SettingsDTO{Terminal: TerminalSettingsDTO{
		FontFamily: value.Terminal.FontFamily, FontSize: value.Terminal.FontSize,
		LineHeight: value.Terminal.LineHeight, CursorStyle: value.Terminal.CursorStyle,
		CursorBlink: value.Terminal.CursorBlink, Scrollback: value.Terminal.Scrollback,
		Bell: value.Terminal.Bell,
	}}
}

func settingsFromDTO(value SettingsDTO) settingsdomain.Settings {
	return settingsdomain.Settings{Terminal: settingsdomain.Terminal{
		FontFamily: value.Terminal.FontFamily, FontSize: value.Terminal.FontSize,
		LineHeight: value.Terminal.LineHeight, CursorStyle: value.Terminal.CursorStyle,
		CursorBlink: value.Terminal.CursorBlink, Scrollback: value.Terminal.Scrollback,
		Bell: value.Terminal.Bell,
	}}
}

func (d *Desktop) touchLease(id string) (FrontendLeaseDTO, error) {
	d.leaseMu.Lock()
	defer d.leaseMu.Unlock()
	if d.lease == nil || d.lease.id != id {
		return FrontendLeaseDTO{}, errors.New("frontend lease is missing or stale")
	}
	now := time.Now()
	d.lease.lastSeen = now
	d.lease.expiresAt = now.Add(leaseTimeout)
	notify(d.leaseWake)
	return leaseDTO(d.lease), nil
}

func (d *Desktop) monitorLease(ctx context.Context) {
	timer := time.NewTimer(leaseCheckInterval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.leaseWake:
		case <-timer.C:
		}

		if d.activeResourceCount() > 0 {
			d.leaseMu.Lock()
			if d.lease != nil && time.Now().After(d.lease.expiresAt) {
				expired := d.lease.id
				d.lease = nil
				d.leaseMu.Unlock()
				d.manager.CloseLease(expired)
				if d.files != nil {
					d.files.CloseLease(expired)
				}
				if d.tunnels != nil {
					d.tunnels.CloseLease(expired)
				}
			} else {
				d.leaseMu.Unlock()
			}
		}
		timer.Reset(leaseCheckInterval)
	}
}

func (d *Desktop) activeResourceCount() int {
	count := d.manager.LiveCount()
	if d.files != nil {
		count += d.files.LiveCount()
	}
	if d.tunnels != nil {
		count += d.tunnels.LiveCount()
	}
	return count
}

func (d *Desktop) context() context.Context {
	d.ctxMu.RLock()
	defer d.ctxMu.RUnlock()
	return d.ctx
}

func randomID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate frontend lease: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}

func leaseDTO(lease *frontendLease) FrontendLeaseDTO {
	return FrontendLeaseDTO{ID: lease.id, ExpiresAt: lease.expiresAt.UTC().Format(time.RFC3339Nano)}
}

func terminalTextFilename(title string) string {
	title = strings.TrimSpace(title)
	var name strings.Builder
	name.Grow(80)
	for _, character := range title {
		if !unicode.IsLetter(character) && !unicode.IsNumber(character) && !strings.ContainsRune(" ._-", character) {
			character = '-'
		}
		if name.Len()+utf8.RuneLen(character) > 80 {
			break
		}
		name.WriteRune(character)
	}
	base := strings.Trim(name.String(), " ._-")
	if base == "" {
		base = "terminal"
	}
	return base + "-selection.txt"
}

func remotePathFavoriteDTO(favorite remotepathdomain.Favorite) RemotePathFavoriteDTO {
	return RemotePathFavoriteDTO{
		ID: favorite.ID, ProfileID: favorite.ProfileID, Path: favorite.Path,
		CreatedAt: favorite.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func notify(channel chan struct{}) {
	select {
	case channel <- struct{}{}:
	default:
	}
}
