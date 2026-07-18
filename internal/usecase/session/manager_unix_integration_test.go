//go:build darwin || linux

package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"shh-h/internal/adapter/localpty"
	"shh-h/internal/domain/profile"
)

const nativeFloodCloseBytes = 2 * 1024 * 1024

type nativeFloodSink struct {
	manager   *Manager
	threshold uint64
	reached   chan struct{}
	reachOnce sync.Once
	bytes     atomic.Uint64
	ackError  chan error
}

func newNativeFloodSink(threshold uint64) *nativeFloodSink {
	return &nativeFloodSink{
		threshold: threshold,
		reached:   make(chan struct{}),
		ackError:  make(chan error, 1),
	}
}

func (sink *nativeFloodSink) PublishOutput(chunk OutputChunk) {
	if err := sink.manager.Acknowledge(
		chunk.LeaseID, chunk.SessionID, chunk.Generation, chunk.Sequence, chunk.EndOffset,
	); err != nil {
		select {
		case sink.ackError <- err:
		default:
		}
		return
	}
	if sink.bytes.Add(uint64(len(chunk.Data))) >= sink.threshold {
		sink.reachOnce.Do(func() { close(sink.reached) })
	}
}

func (*nativeFloodSink) PublishState(StateEvent) {}

func TestManagerClosesRealPTYFloodWithoutResourceLeaks(t *testing.T) {
	if testing.Short() {
		t.Skip("real PTY stress test")
	}

	// Warm up os/exec, the runtime poller, and PTY bookkeeping before recording
	// the process-wide baselines used for leak detection.
	runNativeFloodClose(t, "tab", maxOutputChunk)
	runtime.GC()
	baselineGoroutines := runtime.NumGoroutine()
	baselineDescriptors := openDescriptorCount(t)

	for _, closeKind := range []string{"tab", "window"} {
		for cycle := 0; cycle < 2; cycle++ {
			runNativeFloodClose(t, closeKind, nativeFloodCloseBytes)
			waitForResourceBaseline(t, closeKind, cycle, baselineGoroutines, baselineDescriptors)
		}
	}
}

func runNativeFloodClose(t *testing.T, closeKind string, threshold uint64) {
	t.Helper()

	pidPath := filepath.Join(t.TempDir(), "descendant.pid")
	script := `
trap '' HUP
/bin/sh -c 'trap "" HUP; printf "%s\n" "$$" > "$1"; while :; do printf "0123456789abcdef0123456789abcdef\n"; done' sh "$1" &
wait
`
	manager := NewManager(localpty.NewFactory())
	sink := newNativeFloodSink(threshold)
	sink.manager = manager
	manager.SetSink(sink)
	closed := false
	t.Cleanup(func() {
		if !closed {
			manager.Shutdown()
		}
	})

	opened, err := manager.OpenLocal(context.Background(), "native-stress-lease", profile.Profile{
		ID: "native-stress", Name: "Native stress", Protocol: profile.ProtocolLocal,
		Shell: "/bin/sh", Arguments: []string{"-c", script, "shhh-stress", pidPath},
	}, 100, 30)
	if err != nil {
		t.Fatalf("open %s-close PTY stress session: %v", closeKind, err)
	}
	if err := manager.Activate(opened.LeaseID, opened.ID, opened.Generation); err != nil {
		t.Fatalf("activate %s-close PTY stress session: %v", closeKind, err)
	}

	select {
	case <-sink.reached:
	case err := <-sink.ackError:
		t.Fatalf("acknowledge %s-close PTY stress output: %v", closeKind, err)
	case <-time.After(5 * time.Second):
		t.Fatalf("%s-close PTY stress session produced %d of %d bytes", closeKind, sink.bytes.Load(), threshold)
	}
	descendantPID := readProcessID(t, pidPath)

	started := time.Now()
	switch closeKind {
	case "tab":
		if err := manager.Close(opened.LeaseID, opened.ID, opened.Generation); err != nil {
			t.Fatalf("close flooded PTY tab: %v", err)
		}
	case "window":
		manager.Shutdown()
	default:
		t.Fatalf("unsupported stress close kind %q", closeKind)
	}
	closed = true
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("%s-close PTY cleanup exceeded budget: %s", closeKind, elapsed)
	}
	if manager.LiveCount() != 0 {
		t.Fatalf("%s-close left %d terminal runtimes live", closeKind, manager.LiveCount())
	}
	waitForProcessExit(t, descendantPID, closeKind)
}

func readProcessID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
			if parseErr != nil || pid <= 0 {
				t.Fatalf("parse stress descendant PID %q: %v", data, parseErr)
			}
			return pid
		}
		if !errors.Is(err, os.ErrNotExist) || time.Now().After(deadline) {
			t.Fatalf("read stress descendant PID: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForProcessExit(t *testing.T, pid int, closeKind string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if err != nil && !errors.Is(err, syscall.EPERM) {
			t.Fatalf("inspect %s-close descendant %d: %v", closeKind, pid, err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s-close descendant process %d survived cleanup", closeKind, pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitForResourceBaseline(t *testing.T, closeKind string, cycle, goroutines, descriptors int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		runtime.GC()
		currentGoroutines := runtime.NumGoroutine()
		currentDescriptors, err := countOpenDescriptors()
		if err != nil {
			t.Fatalf("count descriptors after %s-close cycle %d: %v", closeKind, cycle, err)
		}
		if currentGoroutines <= goroutines && currentDescriptors <= descriptors {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf(
				"resources after %s-close cycle %d did not return to baseline: goroutines %d/%d, descriptors %d/%d",
				closeKind, cycle, currentGoroutines, goroutines, currentDescriptors, descriptors,
			)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func openDescriptorCount(t *testing.T) int {
	t.Helper()
	count, err := countOpenDescriptors()
	if err != nil {
		t.Fatalf("count open descriptors: %v", err)
	}
	return count
}

func countOpenDescriptors() (int, error) {
	path := "/proc/self/fd"
	if runtime.GOOS == "darwin" {
		path = "/dev/fd"
	}
	directory, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	names, readErr := directory.Readdirnames(-1)
	closeErr := directory.Close()
	if readErr != nil {
		return 0, fmt.Errorf("read %s: %w", path, readErr)
	}
	if closeErr != nil {
		return 0, fmt.Errorf("close %s: %w", path, closeErr)
	}
	return len(names), nil
}

var _ Sink = (*nativeFloodSink)(nil)
