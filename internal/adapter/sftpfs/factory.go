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

	"shh-h/internal/adapter/sshclient"
	"shh-h/internal/domain/filetransfer"
	"shh-h/internal/port"
)

type clientAcquirer interface {
	Acquire(context.Context, port.SSHTerminalSpec) (*sshclient.Lease, error)
}

type Factory struct {
	clients clientAcquirer
}

func NewFactory(clients clientAcquirer) *Factory {
	return &Factory{clients: clients}
}

func (f *Factory) OpenRemoteFilesystem(ctx context.Context, spec port.SSHTerminalSpec) (port.RemoteFilesystem, error) {
	if f.clients == nil {
		return nil, errors.New("SSH client pool is unavailable")
	}
	lease, err := f.clients.Acquire(ctx, spec)
	if err != nil {
		return nil, err
	}
	client := lease.Client()
	if client == nil {
		_ = lease.Close()
		return nil, errors.New("SSH client is unavailable")
	}
	remote, err := sftp.NewClient(client)
	if err != nil {
		_ = lease.Close()
		return nil, fmt.Errorf("start sftp subsystem: %w", err)
	}
	workingDirectory, err := remote.Getwd()
	if err != nil {
		_ = remote.Close()
		_ = lease.Close()
		return nil, fmt.Errorf("resolve remote working directory: %w", err)
	}
	return &filesystem{lease: lease, client: remote, workingDirectory: workingDirectory}, nil
}

type filesystem struct {
	lease            *sshclient.Lease
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

func (f *filesystem) OpenRead(ctx context.Context, remotePath string, offset int64) (io.ReadCloser, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	if offset < 0 {
		return nil, 0, errors.New("remote read offset cannot be negative")
	}
	remotePath = cleanRemotePath(remotePath, f.workingDirectory)
	info, err := f.client.Stat(remotePath)
	if err != nil {
		return nil, 0, fmt.Errorf("inspect remote file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, 0, errors.New("remote path is not a regular file")
	}
	if offset > info.Size() {
		return nil, 0, errors.New("remote read offset exceeds file size")
	}
	file, err := f.client.Open(remotePath)
	if err != nil {
		return nil, 0, fmt.Errorf("open remote file: %w", err)
	}
	if offset > 0 {
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			_ = file.Close()
			return nil, 0, fmt.Errorf("seek remote source: %w", err)
		}
	}
	return file, info.Size(), nil
}

func (f *filesystem) OpenWrite(ctx context.Context, remotePath string, offset int64) (io.WriteCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if offset < 0 {
		return nil, errors.New("remote write offset cannot be negative")
	}
	flags := os.O_WRONLY
	if offset == 0 {
		flags |= os.O_CREATE | os.O_TRUNC
	}
	file, err := f.client.OpenFile(cleanRemotePath(remotePath, f.workingDirectory), flags)
	if err != nil {
		return nil, fmt.Errorf("open remote destination: %w", err)
	}
	if offset > 0 {
		info, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("inspect remote destination offset: %w", err)
		}
		if info.Size() != offset {
			_ = file.Close()
			return nil, fmt.Errorf("remote destination size %d does not match resume offset %d", info.Size(), offset)
		}
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("seek remote destination: %w", err)
		}
	}
	return file, nil
}

func (f *filesystem) Close() error {
	f.closeOnce.Do(func() {
		f.closeErr = errors.Join(meaningfulCloseError(f.client.Close()), meaningfulCloseError(f.lease.Close()))
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
