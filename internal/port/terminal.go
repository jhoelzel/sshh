package port

import (
	"context"
	"io"

	"shh-h/internal/domain/profile"
)

type TerminalSignal string

const (
	SignalHangup    TerminalSignal = "hangup"
	SignalInterrupt TerminalSignal = "interrupt"
	SignalTerminate TerminalSignal = "terminate"
	SignalKill      TerminalSignal = "kill"
)

type TerminalSpec struct {
	Command          string
	Arguments        []string
	WorkingDirectory string
	Environment      []string
	Columns          uint16
	Rows             uint16
}

type ExitStatus struct {
	Code   int
	Signal string
}

type TerminalTransport interface {
	io.Reader
	io.Writer
	Resize(ctx context.Context, columns, rows uint16) error
	Signal(ctx context.Context, signal TerminalSignal) error
	Wait() (ExitStatus, error)
	Close() error
}

type TerminalFactory interface {
	Open(ctx context.Context, spec TerminalSpec) (TerminalTransport, error)
}

type SSHCredentials struct {
	Password   []byte
	Passphrase []byte
}

type SSHTerminalSpec struct {
	ProfileID      string
	Host           string
	Port           int
	Username       string
	Authentication profile.Authentication
	IdentityFile   string
	Credentials    SSHCredentials
	Columns        uint16
	Rows           uint16
}

type SSHTerminalFactory interface {
	OpenSSH(ctx context.Context, spec SSHTerminalSpec) (TerminalTransport, error)
}
