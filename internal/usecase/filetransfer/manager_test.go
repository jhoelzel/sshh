package filetransfer

import (
	"bytes"
	"context"
	"errors"
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
	mu      sync.Mutex
	files   map[string][]byte
	closed  bool
	readErr error
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
	reader := io.Reader(bytes.NewReader(copyData))
	if f.readErr != nil {
		reader = io.MultiReader(reader, errorReader{err: f.readErr})
	}
	return io.NopCloser(reader), int64(len(copyData)), nil
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

type errorReader struct{ err error }

func (reader errorReader) Read([]byte) (int, error) { return 0, reader.err }

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

	transfer, err := manager.StartDownload("lease", session.ID, "/remote/source.txt", destination, filedomain.CollisionAsk, false)
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

	transfer, err := manager.StartUpload("lease", session.ID, localPath, "/remote/upload.txt", filedomain.CollisionAsk, false)
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

func TestManagerAppliesDownloadCollisionPolicies(t *testing.T) {
	filesystem := newFakeFilesystem()
	filesystem.files["/remote/source.txt"] = []byte("new content")
	manager := NewManager(&fakeFactory{filesystem: filesystem})
	session := openTestSession(t, manager)
	directory := t.TempDir()
	destination := filepath.Join(directory, "source.txt")
	if err := os.WriteFile(destination, []byte("old content"), 0o600); err != nil {
		t.Fatalf("write collision fixture: %v", err)
	}

	if _, err := manager.StartDownload("lease", session.ID, "/remote/source.txt", destination, filedomain.CollisionAsk, false); !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("expected ask collision, got %v", err)
	}
	skipped, err := manager.StartDownload("lease", session.ID, "/remote/source.txt", destination, filedomain.CollisionSkip, false)
	if err != nil || skipped.State != filedomain.TransferSkipped || skipped.Message == "" {
		t.Fatalf("unexpected skipped transfer: transfer=%#v err=%v", skipped, err)
	}
	renamed, err := manager.StartDownload("lease", session.ID, "/remote/source.txt", destination, filedomain.CollisionRename, false)
	if err != nil {
		t.Fatalf("start renamed download: %v", err)
	}
	completed := waitForTransfer(t, manager, renamed.ID)
	if completed.State != filedomain.TransferCompleted || filepath.Base(completed.Destination) != "source (1).txt" {
		t.Fatalf("unexpected renamed download: %#v", completed)
	}
	if old, err := os.ReadFile(destination); err != nil || string(old) != "old content" {
		t.Fatalf("original destination changed: content=%q err=%v", old, err)
	}
}

func TestManagerAppliesUploadRenameAndOverwritePolicies(t *testing.T) {
	filesystem := newFakeFilesystem()
	filesystem.files["/remote/upload.txt"] = []byte("old content")
	manager := NewManager(&fakeFactory{filesystem: filesystem})
	session := openTestSession(t, manager)
	localPath := filepath.Join(t.TempDir(), "upload.txt")
	if err := os.WriteFile(localPath, []byte("new content"), 0o600); err != nil {
		t.Fatalf("write upload fixture: %v", err)
	}

	renamed, err := manager.StartUpload("lease", session.ID, localPath, "/remote/upload.txt", filedomain.CollisionRename, false)
	if err != nil {
		t.Fatalf("start renamed upload: %v", err)
	}
	if completed := waitForTransfer(t, manager, renamed.ID); completed.Destination != "/remote/upload (1).txt" {
		t.Fatalf("unexpected renamed upload: %#v", completed)
	}
	overwritten, err := manager.StartUpload("lease", session.ID, localPath, "/remote/upload.txt", filedomain.CollisionOverwrite, false)
	if err != nil {
		t.Fatalf("start overwrite upload: %v", err)
	}
	if completed := waitForTransfer(t, manager, overwritten.ID); completed.State != filedomain.TransferCompleted {
		t.Fatalf("unexpected overwritten upload: %#v", completed)
	}
	if got := string(filesystem.file("/remote/upload.txt")); got != "new content" {
		t.Fatalf("upload did not replace destination: %q", got)
	}
}

func TestManagerKeepsDownloadPartialOnlyWhenConfigured(t *testing.T) {
	for _, test := range []struct {
		name string
		keep bool
		want bool
	}{{name: "remove", keep: false, want: false}, {name: "keep", keep: true, want: true}} {
		t.Run(test.name, func(t *testing.T) {
			filesystem := newFakeFilesystem()
			filesystem.files["/remote/source.txt"] = []byte("partial content")
			filesystem.readErr = errors.New("connection lost")
			manager := NewManager(&fakeFactory{filesystem: filesystem})
			session := openTestSession(t, manager)
			destination := filepath.Join(t.TempDir(), "source.txt")
			transfer, err := manager.StartDownload("lease", session.ID, "/remote/source.txt", destination, filedomain.CollisionOverwrite, test.keep)
			if err != nil {
				t.Fatalf("start failed download: %v", err)
			}
			if failed := waitForTransfer(t, manager, transfer.ID); failed.State != filedomain.TransferFailed {
				t.Fatalf("unexpected failed download: %#v", failed)
			}
			partial := filepath.Join(filepath.Dir(destination), "."+filepath.Base(destination)+".shhh-part-"+transfer.ID)
			_, statErr := os.Stat(partial)
			if exists := statErr == nil; exists != test.want {
				t.Fatalf("partial existence=%t, want %t (err=%v)", exists, test.want, statErr)
			}
		})
	}
}

func TestManagerConcurrencyCanIncreaseWhileWorkIsQueued(t *testing.T) {
	manager := NewManager(nil)
	if err := manager.SetConcurrency(1); err != nil {
		t.Fatalf("set initial concurrency: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if !manager.acquireWorker(ctx) {
		t.Fatal("first worker was not acquired")
	}
	second := make(chan bool, 1)
	go func() { second <- manager.acquireWorker(ctx) }()
	select {
	case <-second:
		t.Fatal("second worker bypassed the configured limit")
	case <-time.After(25 * time.Millisecond):
	}
	if err := manager.SetConcurrency(2); err != nil {
		t.Fatalf("increase concurrency: %v", err)
	}
	select {
	case acquired := <-second:
		if !acquired {
			t.Fatal("queued worker was cancelled")
		}
	case <-time.After(time.Second):
		t.Fatal("queued worker did not start after increasing concurrency")
	}
	if err := manager.SetConcurrency(1); err != nil {
		t.Fatalf("lower concurrency: %v", err)
	}
	third := make(chan bool, 1)
	go func() { third <- manager.acquireWorker(ctx) }()
	manager.releaseWorker()
	select {
	case <-third:
		t.Fatal("lowered limit admitted work before active workers drained")
	case <-time.After(25 * time.Millisecond):
	}
	manager.releaseWorker()
	select {
	case acquired := <-third:
		if !acquired {
			t.Fatal("third worker was cancelled")
		}
	case <-time.After(time.Second):
		t.Fatal("lowered limit did not admit work after active workers drained")
	}
	manager.releaseWorker()
	if err := manager.SetConcurrency(filedomain.MaxConcurrency + 1); err == nil {
		t.Fatal("expected invalid concurrency to fail")
	}
}

func TestKeepBothReservesDistinctQueuedDestinations(t *testing.T) {
	manager := NewManager(nil)
	runtime := &runtimeSession{snapshot: filedomain.Session{ID: "files"}}
	destination := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(destination, []byte("existing"), 0o600); err != nil {
		t.Fatalf("write collision fixture: %v", err)
	}
	first, skipped, err := manager.resolveLocalCollision(runtime, destination, filedomain.CollisionRename)
	if err != nil || skipped {
		t.Fatalf("reserve first destination: path=%q skipped=%t err=%v", first, skipped, err)
	}
	otherRuntime := &runtimeSession{snapshot: filedomain.Session{ID: "other-files"}}
	second, skipped, err := manager.resolveLocalCollision(otherRuntime, destination, filedomain.CollisionRename)
	if err != nil || skipped {
		t.Fatalf("reserve second destination: path=%q skipped=%t err=%v", second, skipped, err)
	}
	if filepath.Base(first) != "report (1).txt" || filepath.Base(second) != "report (2).txt" {
		t.Fatalf("queued destinations collided: first=%q second=%q", first, second)
	}
	manager.releaseDestination(filedomain.DirectionDownload, runtime.snapshot.ID, first)
	manager.releaseDestination(filedomain.DirectionDownload, otherRuntime.snapshot.ID, second)
	if _, _, err := manager.resolveLocalCollision(runtime, filepath.Join(t.TempDir(), "new.txt"), "invalid"); err == nil {
		t.Fatal("expected invalid collision policy to fail without an existing file")
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
