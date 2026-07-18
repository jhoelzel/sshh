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

const (
	nativeFloodCloseBytes     = 2 * 1024 * 1024
	nativeShortLivedPTYCycles = 100
)

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

type nativeShortLivedSink struct {
	manager  *Manager
	states   chan StateEvent
	ackError chan error
	bytes    atomic.Uint64
}

func newNativeShortLivedSink(manager *Manager) *nativeShortLivedSink {
	return &nativeShortLivedSink{
		manager:  manager,
		states:   make(chan StateEvent, 8),
		ackError: make(chan error, 1),
	}
}

func (sink *nativeShortLivedSink) PublishOutput(chunk OutputChunk) {
	if err := sink.manager.Acknowledge(
		chunk.LeaseID, chunk.SessionID, chunk.Generation, chunk.Sequence, chunk.EndOffset,
	); err != nil {
		select {
		case sink.ackError <- err:
		default:
		}
		return
	}
	sink.bytes.Add(uint64(len(chunk.Data)))
}

func (sink *nativeShortLivedSink) PublishState(event StateEvent) {
	sink.states <- event
}

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

func TestManagerReaps100ShortLivedRealPTYsWithoutResourceLeaks(t *testing.T) {
	if testing.Short() {
		t.Skip("real PTY stress test")
	}

	manager := NewManager(localpty.NewFactory())
	sink := newNativeShortLivedSink(manager)
	manager.SetSink(sink)
	t.Cleanup(manager.Shutdown)

	// Warm up process creation, the runtime poller, and PTY bookkeeping before
	// recording the process-wide resource baseline used for leak detection.
	runNativeShortLivedPTY(t, manager, sink, 0)
	runtime.GC()
	baselineGoroutines := runtime.NumGoroutine()
	baselineDescriptors := openDescriptorCount(t)
	baselineBytes := sink.bytes.Load()

	started := time.Now()
	for cycle := 1; cycle <= nativeShortLivedPTYCycles; cycle++ {
		runNativeShortLivedPTY(t, manager, sink, cycle)
	}
	waitForResourceBaseline(
		t, "short-lived PTY", nativeShortLivedPTYCycles, baselineGoroutines, baselineDescriptors,
	)
	finalGoroutines := runtime.NumGoroutine()
	finalDescriptors := openDescriptorCount(t)
	t.Logf(
		"reaped %d short-lived PTYs in %s; output=%d bytes, goroutines=%d/%d, descriptors=%d/%d",
		nativeShortLivedPTYCycles, time.Since(started), sink.bytes.Load()-baselineBytes,
		finalGoroutines, baselineGoroutines, finalDescriptors, baselineDescriptors,
	)
}

func runNativeShortLivedPTY(t *testing.T, manager *Manager, sink *nativeShortLivedSink, cycle int) {
	t.Helper()

	bytesBefore := sink.bytes.Load()
	opened, err := manager.OpenLocal(context.Background(), "native-short-lived-lease", profile.Profile{
		ID: "native-short-lived", Name: "Native short-lived", Protocol: profile.ProtocolLocal,
		Shell: "/bin/sh", Arguments: []string{"-c", `printf 'ready:%s\n' "$SHHH_SESSION_ID"`},
	}, 80, 24)
	if err != nil {
		t.Fatalf("open short-lived PTY cycle %d: %v", cycle, err)
	}
	if err := manager.Activate(opened.LeaseID, opened.ID, opened.Generation); err != nil {
		t.Fatalf("activate short-lived PTY cycle %d: %v", cycle, err)
	}

	exited := waitForNativeSessionState(t, sink, opened, StateExited, cycle)
	if exited.ExitCode == nil || *exited.ExitCode != 0 || exited.Signal != "" || exited.Message != "" {
		t.Fatalf("short-lived PTY cycle %d exited unexpectedly: %#v", cycle, exited)
	}
	if sink.bytes.Load() <= bytesBefore {
		t.Fatalf("short-lived PTY cycle %d exited without delivering output", cycle)
	}
	if err := manager.Close(opened.LeaseID, opened.ID, opened.Generation); err != nil {
		t.Fatalf("close short-lived PTY cycle %d: %v", cycle, err)
	}
	waitForNativeSessionState(t, sink, opened, StateClosed, cycle)
	if manager.LiveCount() != 0 {
		t.Fatalf("short-lived PTY cycle %d left %d terminal runtimes live", cycle, manager.LiveCount())
	}
	manager.mu.RLock()
	runtimeCount := len(manager.runtimes)
	dispatcherPresent := manager.output != nil
	manager.mu.RUnlock()
	if runtimeCount != 0 || dispatcherPresent {
		t.Fatalf(
			"short-lived PTY cycle %d retained manager resources: runtimes=%d, dispatcher=%t",
			cycle, runtimeCount, dispatcherPresent,
		)
	}
}

func waitForNativeSessionState(
	t *testing.T,
	sink *nativeShortLivedSink,
	opened Session,
	expected State,
	cycle int,
) StateEvent {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()

	for {
		select {
		case err := <-sink.ackError:
			t.Fatalf("acknowledge short-lived PTY cycle %d output: %v", cycle, err)
		case event := <-sink.states:
			if event.SessionID != opened.ID || event.Generation != opened.Generation || event.LeaseID != opened.LeaseID {
				t.Fatalf("short-lived PTY cycle %d received state for another session: %#v", cycle, event)
			}
			if event.State == StateFailed {
				t.Fatalf("short-lived PTY cycle %d failed: %#v", cycle, event)
			}
			if event.State == expected {
				return event
			}
		case <-timer.C:
			t.Fatalf("short-lived PTY cycle %d timed out waiting for state %q", cycle, expected)
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

var (
	_ Sink = (*nativeFloodSink)(nil)
	_ Sink = (*nativeShortLivedSink)(nil)
)
