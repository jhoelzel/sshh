package port

import (
	"context"
	"io"
	"os"

	"shh-h/internal/domain/filetransfer"
)

type RemoteFilesystem interface {
	WorkingDirectory() string
	ReadDirectory(context.Context, string) ([]filetransfer.Entry, error)
	Stat(context.Context, string) (filetransfer.Entry, error)
	CreateDirectory(context.Context, string) error
	Rename(context.Context, string, string) error
	Remove(context.Context, string) error
	Chmod(context.Context, string, os.FileMode) error
	OpenRead(context.Context, string) (io.ReadCloser, int64, error)
	OpenWrite(context.Context, string) (io.WriteCloser, error)
	Close() error
}

type RemoteFilesystemFactory interface {
	OpenRemoteFilesystem(context.Context, SSHTerminalSpec) (RemoteFilesystem, error)
}
