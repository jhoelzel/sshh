package settings

import (
	"errors"
	"fmt"
	"math"

	filedomain "shh-h/internal/domain/filetransfer"
)

type FontFamily string
type CursorStyle string

const (
	FontSystemMono FontFamily = "system-mono"
	FontMenlo      FontFamily = "menlo"
	FontMonaco     FontFamily = "monaco"

	CursorBlock     CursorStyle = "block"
	CursorBar       CursorStyle = "bar"
	CursorUnderline CursorStyle = "underline"
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

type Settings struct {
	Terminal      Terminal      `json:"terminal"`
	Connection    Connection    `json:"connection"`
	Notifications Notifications `json:"notifications"`
	Transfers     Transfers     `json:"transfers"`
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
	return nil
}
