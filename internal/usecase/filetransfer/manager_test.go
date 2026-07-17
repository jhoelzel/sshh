package filetransfer

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	filedomain "shh-h/internal/domain/filetransfer"
	"shh-h/internal/domain/profile"
	"shh-h/internal/port"
)

type fakeFactory struct {
	filesystem *fakeFilesystem
}

func (f *fakeFactory) OpenRemoteFilesystem(context.Context, port.SSHTerminalSpec) (port.RemoteFilesystem, error) {
	return f.filesystem, nil
}

type fakeFilesystem struct {
	mu     sync.Mutex
	files  map[string][]byte
	closed bool
}

func newFakeFilesystem() *fakeFilesystem {
	return &fakeFilesystem{files: make(map[string][]byte)}
}

func (f *fakeFilesystem) WorkingDirectory() string { return "/home/test" }

func (f *fakeFilesystem) ReadDirectory(context.Context, string) ([]filedomain.Entry, error) {
	return nil, nil
}

func (f *fakeFilesystem) Stat(_ context.Context, path string) (filedomain.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, exists := f.files[path]
	if !exists {
		return filedomain.Entry{}, os.ErrNotExist
	}
	return filedomain.Entry{Name: filepath.Base(path), Path: path, Size: int64(len(data))}, nil
}

func (f *fakeFilesystem) CreateDirectory(context.Context, string) error { return nil }

func (f *fakeFilesystem) Rename(_ context.Context, source, destination string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, exists := f.files[source]
	if !exists {
		return os.ErrNotExist
	}
	f.files[destination] = data
	delete(f.files, source)
	return nil
}

func (f *fakeFilesystem) Remove(_ context.Context, path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.files, path)
	return nil
}

func (f *fakeFilesystem) Chmod(context.Context, string, os.FileMode) error { return nil }

func (f *fakeFilesystem) OpenRead(_ context.Context, path string) (io.ReadCloser, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, exists := f.files[path]
	if !exists {
		return nil, 0, os.ErrNotExist
	}
	copyData := append([]byte(nil), data...)
	return io.NopCloser(bytes.NewReader(copyData)), int64(len(copyData)), nil
}

func (f *fakeFilesystem) OpenWrite(_ context.Context, path string) (io.WriteCloser, error) {
	return &fakeRemoteWriter{filesystem: f, path: path}, nil
}

func (f *fakeFilesystem) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeFilesystem) file(path string) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]byte(nil), f.files[path]...)
}

type fakeRemoteWriter struct {
	filesystem *fakeFilesystem
	path       string
	buffer     bytes.Buffer
	once       sync.Once
}

func (w *fakeRemoteWriter) Write(data []byte) (int, error) { return w.buffer.Write(data) }

func (w *fakeRemoteWriter) Close() error {
	w.once.Do(func() {
		w.filesystem.mu.Lock()
		w.filesystem.files[w.path] = append([]byte(nil), w.buffer.Bytes()...)
		w.filesystem.mu.Unlock()
	})
	return nil
}

func TestManagerStreamsDownloadThroughPartialFile(t *testing.T) {
	filesystem := newFakeFilesystem()
	filesystem.files["/remote/source.txt"] = []byte("downloaded content")
	manager := NewManager(&fakeFactory{filesystem: filesystem})
	session := openTestSession(t, manager)
	destination := filepath.Join(t.TempDir(), "source.txt")

	transfer, err := manager.StartDownload("lease", session.ID, "/remote/source.txt", destination, false)
	if err != nil {
		t.Fatalf("start download: %v", err)
	}
	completed := waitForTransfer(t, manager, transfer.ID)
	if completed.State != filedomain.TransferCompleted || completed.Bytes != int64(len("downloaded content")) {
		t.Fatalf("unexpected download result: %#v", completed)
	}
	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(data) != "downloaded content" {
		t.Fatalf("unexpected downloaded content %q", data)
	}
}

func TestManagerStreamsUploadThroughRemotePartialFile(t *testing.T) {
	filesystem := newFakeFilesystem()
	manager := NewManager(&fakeFactory{filesystem: filesystem})
	session := openTestSession(t, manager)
	localPath := filepath.Join(t.TempDir(), "upload.txt")
	if err := os.WriteFile(localPath, []byte("uploaded content"), 0o600); err != nil {
		t.Fatalf("write upload fixture: %v", err)
	}

	transfer, err := manager.StartUpload("lease", session.ID, localPath, "/remote/upload.txt", false)
	if err != nil {
		t.Fatalf("start upload: %v", err)
	}
	completed := waitForTransfer(t, manager, transfer.ID)
	if completed.State != filedomain.TransferCompleted {
		t.Fatalf("unexpected upload result: %#v", completed)
	}
	if got := string(filesystem.file("/remote/upload.txt")); got != "uploaded content" {
		t.Fatalf("unexpected uploaded content %q", got)
	}
}

func TestManagerRejectsStaleLeaseAndClosesFilesystem(t *testing.T) {
	filesystem := newFakeFilesystem()
	manager := NewManager(&fakeFactory{filesystem: filesystem})
	session := openTestSession(t, manager)
	if _, err := manager.List("other", session.ID, "."); err == nil {
		t.Fatal("expected stale lease to be rejected")
	}
	manager.CloseLease("lease")
	if manager.LiveCount() != 0 {
		t.Fatal("expected no live sftp sessions")
	}
	if !filesystem.closed {
		t.Fatal("expected remote filesystem to close")
	}
}

func openTestSession(t *testing.T, manager *Manager) filedomain.Session {
	t.Helper()
	session, err := manager.Open(context.Background(), "lease", profile.Profile{
		ID: "remote", Name: "Remote", Protocol: profile.ProtocolSSH,
		Host: "example.com", Port: 22, Authentication: profile.AuthenticationPassword,
	}, port.SSHCredentials{Password: []byte("secret")})
	if err != nil {
		t.Fatalf("open file session: %v", err)
	}
	return session
}

func waitForTransfer(t *testing.T, manager *Manager, id string) filedomain.Transfer {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, transfer := range manager.Transfers("lease") {
			if transfer.ID == id && transfer.State != filedomain.TransferQueued && transfer.State != filedomain.TransferRunning {
				return transfer
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for transfer")
	return filedomain.Transfer{}
}

var _ port.RemoteFilesystem = (*fakeFilesystem)(nil)
var _ port.RemoteFilesystemFactory = (*fakeFactory)(nil)
