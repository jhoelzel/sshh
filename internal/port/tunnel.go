package port

import (
	"context"
	"net"
)

type SSHConnection interface {
	Dial(network, address string) (net.Conn, error)
	Listen(network, address string) (net.Listener, error)
	Wait() error
	Close() error
}

type SSHConnectionFactory interface {
	OpenSSHConnection(context.Context, SSHTerminalSpec) (SSHConnection, error)
}
