package sftpfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"shh-h/internal/domain/filetransfer"
	"shh-h/internal/port"
)

type sshDialer interface {
	DialSSH(context.Context, port.SSHTerminalSpec) (*ssh.Client, error)
}

type Factory struct {
	dialer sshDialer
}

func NewFactory(dialer sshDialer) *Factory {
	return &Factory{dialer: dialer}
}

func (f *Factory) OpenRemoteFilesystem(ctx context.Context, spec port.SSHTerminalSpec) (port.RemoteFilesystem, error) {
	client, err := f.dialer.DialSSH(ctx, spec)
	if err != nil {
		return nil, err
	}
	remote, err := sftp.NewClient(client)
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("start sftp subsystem: %w", err)
	}
	workingDirectory, err := remote.Getwd()
	if err != nil {
		_ = remote.Close()
		_ = client.Close()
		return nil, fmt.Errorf("resolve remote working directory: %w", err)
	}
	return &filesystem{ssh: client, client: remote, workingDirectory: workingDirectory}, nil
}

type filesystem struct {
	ssh              *ssh.Client
	client           *sftp.Client
	workingDirectory string
	closeOnce        sync.Once
	closeErr         error
}

func (f *filesystem) WorkingDirectory() string {
	return f.workingDirectory
}

func (f *filesystem) ReadDirectory(ctx context.Context, remotePath string) ([]filetransfer.Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	remotePath = cleanRemotePath(remotePath, f.workingDirectory)
	items, err := f.client.ReadDir(remotePath)
	if err != nil {
		return nil, fmt.Errorf("read remote directory %q: %w", remotePath, err)
	}
	result := make([]filetransfer.Entry, 0, len(items))
	for _, item := range items {
		result = append(result, filetransfer.Entry{
			Name: item.Name(), Path: path.Join(remotePath, item.Name()),
			Directory: item.IsDir(), Symlink: item.Mode()&os.ModeSymlink != 0,
			Size: item.Size(), Mode: uint32(item.Mode()), ModifiedAt: item.ModTime().UTC().Format(time.RFC3339Nano),
		})
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Directory != result[right].Directory {
			return result[left].Directory
		}
		return result[left].Name < result[right].Name
	})
	return result, nil
}

func (f *filesystem) Stat(ctx context.Context, remotePath string) (filetransfer.Entry, error) {
	if err := ctx.Err(); err != nil {
		return filetransfer.Entry{}, err
	}
	remotePath = cleanRemotePath(remotePath, f.workingDirectory)
	info, err := f.client.Lstat(remotePath)
	if err != nil {
		return filetransfer.Entry{}, fmt.Errorf("inspect remote path %q: %w", remotePath, err)
	}
	return filetransfer.Entry{
		Name: info.Name(), Path: remotePath, Directory: info.IsDir(),
		Symlink: info.Mode()&os.ModeSymlink != 0, Size: info.Size(),
		Mode: uint32(info.Mode()), ModifiedAt: info.ModTime().UTC().Format(time.RFC3339Nano),
	}, nil
}

func (f *filesystem) CreateDirectory(ctx context.Context, remotePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := f.client.Mkdir(cleanRemotePath(remotePath, f.workingDirectory)); err != nil {
		return fmt.Errorf("create remote directory: %w", err)
	}
	return nil
}

func (f *filesystem) Rename(ctx context.Context, source, destination string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	source = cleanRemotePath(source, f.workingDirectory)
	destination = cleanRemotePath(destination, f.workingDirectory)
	if err := f.client.PosixRename(source, destination); err != nil {
		if fallbackErr := f.client.Rename(source, destination); fallbackErr != nil {
			return fmt.Errorf("rename remote path: %w", errors.Join(err, fallbackErr))
		}
	}
	return nil
}

func (f *filesystem) Remove(ctx context.Context, remotePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	remotePath = cleanRemotePath(remotePath, f.workingDirectory)
	info, err := f.client.Lstat(remotePath)
	if err != nil {
		return fmt.Errorf("inspect remote path: %w", err)
	}
	if info.IsDir() {
		if err := f.client.RemoveDirectory(remotePath); err != nil {
			return fmt.Errorf("remove remote directory: %w", err)
		}
		return nil
	}
	if err := f.client.Remove(remotePath); err != nil {
		return fmt.Errorf("remove remote file: %w", err)
	}
	return nil
}

func (f *filesystem) Chmod(ctx context.Context, remotePath string, mode os.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := f.client.Chmod(cleanRemotePath(remotePath, f.workingDirectory), mode.Perm()); err != nil {
		return fmt.Errorf("change remote permissions: %w", err)
	}
	return nil
}

func (f *filesystem) OpenRead(ctx context.Context, remotePath string) (io.ReadCloser, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	remotePath = cleanRemotePath(remotePath, f.workingDirectory)
	info, err := f.client.Stat(remotePath)
	if err != nil {
		return nil, 0, fmt.Errorf("inspect remote file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, 0, errors.New("remote path is not a regular file")
	}
	file, err := f.client.Open(remotePath)
	if err != nil {
		return nil, 0, fmt.Errorf("open remote file: %w", err)
	}
	return file, info.Size(), nil
}

func (f *filesystem) OpenWrite(ctx context.Context, remotePath string) (io.WriteCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := f.client.OpenFile(cleanRemotePath(remotePath, f.workingDirectory), os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return nil, fmt.Errorf("open remote destination: %w", err)
	}
	return file, nil
}

func (f *filesystem) Close() error {
	f.closeOnce.Do(func() {
		f.closeErr = errors.Join(meaningfulCloseError(f.client.Close()), meaningfulCloseError(f.ssh.Close()))
	})
	return f.closeErr
}

func cleanRemotePath(remotePath, workingDirectory string) string {
	if remotePath == "" || remotePath == "." {
		return path.Clean(workingDirectory)
	}
	if !path.IsAbs(remotePath) {
		return path.Join(workingDirectory, remotePath)
	}
	return path.Clean(remotePath)
}

func meaningfulCloseError(err error) error {
	if err == nil || errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

var _ port.RemoteFilesystemFactory = (*Factory)(nil)
var _ port.RemoteFilesystem = (*filesystem)(nil)
