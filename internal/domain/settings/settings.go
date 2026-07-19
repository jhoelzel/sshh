package settings

import (
	"errors"
	"fmt"
	"math"

	filedomain "shh-h/internal/domain/filetransfer"
)

type FontFamily string
type CursorStyle string
type Theme string
type Workspace string

const (
	FontSystemMono FontFamily = "system-mono"
	FontMenlo      FontFamily = "menlo"
	FontMonaco     FontFamily = "monaco"

	CursorBlock     CursorStyle = "block"
	CursorBar       CursorStyle = "bar"
	CursorUnderline CursorStyle = "underline"

	ThemeSystem Theme = "system"
	ThemeDark   Theme = "dark"
	ThemeLight  Theme = "light"

	WorkspaceTerminals Workspace = "terminals"
	WorkspaceActivity  Workspace = "activity"
	WorkspaceFiles     Workspace = "files"
	WorkspaceTunnels   Workspace = "tunnels"
	WorkspaceSnippets  Workspace = "snippets"
	WorkspaceLayouts   Workspace = "layouts"
	WorkspaceSettings  Workspace = "settings"

	MinSidebarWidth     = 220
	DefaultSidebarWidth = 272
	MaxSidebarWidth     = 420
	MinWindowWidth      = 860
	MinWindowHeight     = 560
	DefaultWindowWidth  = 1240
	DefaultWindowHeight = 780
	MaxWindowSize       = 16_384
	MaxWindowOffset     = 100_000
)

type Terminal struct {
	FontFamily  FontFamily  `json:"fontFamily"`
	FontSize    int         `json:"fontSize"`
	LineHeight  float64     `json:"lineHeight"`
	CursorStyle CursorStyle `json:"cursorStyle"`
	CursorBlink bool        `json:"cursorBlink"`
	Scrollback  int         `json:"scrollback"`
	Bell        bool        `json:"bell"`
}

type Notifications struct {
	Enabled              bool `json:"enabled"`
	TransferCompleted    bool `json:"transferCompleted"`
	UnexpectedDisconnect bool `json:"unexpectedDisconnect"`
	LongTransferSeconds  int  `json:"longTransferSeconds"`
}

type Transfers struct {
	Concurrency      int                        `json:"concurrency"`
	CollisionPolicy  filedomain.CollisionPolicy `json:"collisionPolicy"`
	KeepPartialFiles bool                       `json:"keepPartialFiles"`
}

type Connection struct {
	ConnectTimeoutSeconds    int  `json:"connectTimeoutSeconds"`
	KeepAliveEnabled         bool `json:"keepAliveEnabled"`
	KeepAliveIntervalSeconds int  `json:"keepAliveIntervalSeconds"`
	KeepAliveMaxFailures     int  `json:"keepAliveMaxFailures"`
}

type UI struct {
	Theme        Theme     `json:"theme"`
	SidebarWidth int       `json:"sidebarWidth"`
	Workspace    Workspace `json:"workspace"`
}

// WindowState is backend-owned desktop state. It is intentionally separate
// from UI so frontend settings updates cannot overwrite native geometry.
type WindowState struct {
	X          int  `json:"x"`
	Y          int  `json:"y"`
	Width      int  `json:"width"`
	Height     int  `json:"height"`
	Positioned bool `json:"positioned"`
	Maximized  bool `json:"maximized"`
}

type Settings struct {
	Terminal      Terminal      `json:"terminal"`
	Connection    Connection    `json:"connection"`
	Notifications Notifications `json:"notifications"`
	Transfers     Transfers     `json:"transfers"`
	UI            UI            `json:"ui"`
	Window        WindowState   `json:"window"`
}

func Defaults() Settings {
	return Settings{
		Terminal: Terminal{
			FontFamily: FontSystemMono, FontSize: 13, LineHeight: 1.2,
			CursorStyle: CursorBlock, CursorBlink: true, Scrollback: 10_000, Bell: true,
		},
		Connection: Connection{
			ConnectTimeoutSeconds: 15, KeepAliveEnabled: true,
			KeepAliveIntervalSeconds: 30, KeepAliveMaxFailures: 3,
		},
		Notifications: Notifications{
			Enabled: false, TransferCompleted: true, UnexpectedDisconnect: true,
			LongTransferSeconds: 30,
		},
		Transfers: Transfers{
			Concurrency: 2, CollisionPolicy: filedomain.CollisionAsk, KeepPartialFiles: false,
		},
		UI: UI{Theme: ThemeDark, SidebarWidth: DefaultSidebarWidth, Workspace: WorkspaceTerminals},
		Window: WindowState{
			Width: DefaultWindowWidth, Height: DefaultWindowHeight,
		},
	}
}

func (s Settings) Validate() error {
	switch s.Terminal.FontFamily {
	case FontSystemMono, FontMenlo, FontMonaco:
	default:
		return fmt.Errorf("unsupported terminal font family %q", s.Terminal.FontFamily)
	}
	if s.Terminal.FontSize < 10 || s.Terminal.FontSize > 28 {
		return errors.New("terminal font size must be between 10 and 28")
	}
	if math.IsNaN(s.Terminal.LineHeight) || math.IsInf(s.Terminal.LineHeight, 0) || s.Terminal.LineHeight < 1 || s.Terminal.LineHeight > 2 {
		return errors.New("terminal line height must be between 1 and 2")
	}
	switch s.Terminal.CursorStyle {
	case CursorBlock, CursorBar, CursorUnderline:
	default:
		return fmt.Errorf("unsupported terminal cursor style %q", s.Terminal.CursorStyle)
	}
	if s.Terminal.Scrollback < 1_000 || s.Terminal.Scrollback > 100_000 {
		return errors.New("terminal scrollback must be between 1000 and 100000 lines")
	}
	if s.Connection.ConnectTimeoutSeconds < 3 || s.Connection.ConnectTimeoutSeconds > 120 {
		return errors.New("connection timeout must be between 3 and 120 seconds")
	}
	if s.Connection.KeepAliveIntervalSeconds < 5 || s.Connection.KeepAliveIntervalSeconds > 300 {
		return errors.New("keepalive interval must be between 5 and 300 seconds")
	}
	if s.Connection.KeepAliveMaxFailures < 1 || s.Connection.KeepAliveMaxFailures > 10 {
		return errors.New("keepalive failure threshold must be between 1 and 10")
	}
	if s.Notifications.LongTransferSeconds < 5 || s.Notifications.LongTransferSeconds > 3_600 {
		return errors.New("long transfer notification threshold must be between 5 and 3600 seconds")
	}
	if s.Transfers.Concurrency < filedomain.MinConcurrency || s.Transfers.Concurrency > filedomain.MaxConcurrency {
		return fmt.Errorf("transfer concurrency must be between %d and %d", filedomain.MinConcurrency, filedomain.MaxConcurrency)
	}
	switch s.Transfers.CollisionPolicy {
	case filedomain.CollisionAsk, filedomain.CollisionOverwrite, filedomain.CollisionSkip, filedomain.CollisionRename:
	default:
		return fmt.Errorf("unsupported transfer collision policy %q", s.Transfers.CollisionPolicy)
	}
	if err := s.UI.Validate(); err != nil {
		return err
	}
	if err := s.Window.Validate(); err != nil {
		return err
	}
	return nil
}

func (u UI) Validate() error {
	switch u.Theme {
	case ThemeSystem, ThemeDark, ThemeLight:
	default:
		return fmt.Errorf("unsupported application theme %q", u.Theme)
	}
	if u.SidebarWidth < MinSidebarWidth || u.SidebarWidth > MaxSidebarWidth {
		return fmt.Errorf("sidebar width must be between %d and %d pixels", MinSidebarWidth, MaxSidebarWidth)
	}
	switch u.Workspace {
	case WorkspaceTerminals, WorkspaceActivity, WorkspaceFiles, WorkspaceTunnels,
		WorkspaceSnippets, WorkspaceLayouts, WorkspaceSettings:
	default:
		return fmt.Errorf("unsupported workspace %q", u.Workspace)
	}
	return nil
}

func (w WindowState) Validate() error {
	if w.Width < MinWindowWidth || w.Width > MaxWindowSize {
		return fmt.Errorf("window width must be between %d and %d pixels", MinWindowWidth, MaxWindowSize)
	}
	if w.Height < MinWindowHeight || w.Height > MaxWindowSize {
		return fmt.Errorf("window height must be between %d and %d pixels", MinWindowHeight, MaxWindowSize)
	}
	if w.X < -MaxWindowOffset || w.X > MaxWindowOffset || w.Y < -MaxWindowOffset || w.Y > MaxWindowOffset {
		return fmt.Errorf("window position must stay within %d pixels of the display origin", MaxWindowOffset)
	}
	if !w.Positioned && (w.X != 0 || w.Y != 0) {
		return errors.New("window coordinates require a saved position")
	}
	return nil
}
