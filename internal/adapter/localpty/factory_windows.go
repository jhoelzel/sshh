//go:build windows

package localpty

import (
	"context"
	"errors"

	"shh-h/internal/port"
)

func (f *Factory) Open(context.Context, port.TerminalSpec) (port.TerminalTransport, error) {
	return nil, errors.New("ConPTY support is scheduled for milestone M2")
}
