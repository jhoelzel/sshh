package session

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"shh-h/internal/domain/profile"
	"shh-h/internal/port"
)

type memoryLogFactory struct {
	mu     sync.Mutex
	opened *memoryLog
}

func (f *memoryLogFactory) Open(port.SessionLogSpec) (port.SessionLog, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opened = &memoryLog{path: "/private/logs/session.log"}
	return f.opened, nil
}

type memoryLog struct {
	mu     sync.Mutex
	data   bytes.Buffer
	path   string
	closed bool
}

func (l *memoryLog) Write(data []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.data.Write(data)
}

func (l *memoryLog) Path() string { return l.path }

func (l *memoryLog) BytesWritten() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return int64(l.data.Len())
}

func (l *memoryLog) Close() error {
	l.mu.Lock()
	l.closed = true
	l.mu.Unlock()
	return nil
}

func TestSessionLoggingCapturesOutputAndStopsWithRuntime(t *testing.T) {
	transport := newFakeTransport()
	manager := NewManager(&fakeFactory{transport: transport})
	logs := &memoryLogFactory{}
	sink := newRecordingSink()
	manager.SetLogFactory(logs)
	manager.SetSink(sink)
	opened, err := manager.OpenLocal(context.Background(), "lease", profile.Profile{
		ID: "local", Name: "Local", Protocol: profile.ProtocolLocal,
	}, 80, 24)
	if err != nil {
		t.Fatalf("open terminal: %v", err)
	}
	<-sink.state
	if err := manager.Activate("lease", opened.ID, opened.Generation); err != nil {
		t.Fatalf("activate terminal: %v", err)
	}
	if _, err := manager.StartLogging("lease", opened.ID, opened.Generation, false); err != nil {
		t.Fatalf("start logging: %v", err)
	}
	started := receiveLogStatus(t, sink.logs)
	if !started.Active || started.Path == "" {
		t.Fatalf("unexpected started status: %#v", started)
	}

	writeDone := make(chan error, 1)
	go func() {
		_, writeErr := transport.writer.Write([]byte("logged output\n"))
		writeDone <- writeErr
	}()
	chunk := receiveOutput(t, sink.output)
	if err := manager.Acknowledge("lease", opened.ID, opened.Generation, chunk.Sequence, chunk.EndOffset); err != nil {
		t.Fatalf("acknowledge output: %v", err)
	}
	if err := <-writeDone; err != nil {
		t.Fatalf("write output: %v", err)
	}
	if err := manager.Close("lease", opened.ID, opened.Generation); err != nil {
		t.Fatalf("close terminal: %v", err)
	}
	stopped := receiveLogStatus(t, sink.logs)
	if stopped.Active || stopped.BytesWritten != int64(len("logged output\n")) {
		t.Fatalf("unexpected stopped status: %#v", stopped)
	}
	logs.mu.Lock()
	logged := logs.opened
	logs.mu.Unlock()
	logged.mu.Lock()
	defer logged.mu.Unlock()
	if !logged.closed || logged.data.String() != "logged output\n" {
		t.Fatalf("unexpected log state: closed=%t data=%q", logged.closed, logged.data.String())
	}
}

func TestSessionLoggingRejectsInactiveAndDuplicateStart(t *testing.T) {
	transport := newFakeTransport()
	manager := NewManager(&fakeFactory{transport: transport})
	manager.SetLogFactory(&memoryLogFactory{})
	manager.SetSink(newRecordingSink())
	opened, err := manager.OpenLocal(context.Background(), "lease", profile.Profile{
		ID: "local", Name: "Local", Protocol: profile.ProtocolLocal,
	}, 80, 24)
	if err != nil {
		t.Fatalf("open terminal: %v", err)
	}
	if _, err := manager.StartLogging("lease", opened.ID, opened.Generation, false); err == nil {
		t.Fatal("expected starting session to reject logging")
	}
	if err := manager.Activate("lease", opened.ID, opened.Generation); err != nil {
		t.Fatalf("activate terminal: %v", err)
	}
	if _, err := manager.StartLogging("lease", opened.ID, opened.Generation, false); err != nil {
		t.Fatalf("start logging: %v", err)
	}
	if _, err := manager.StartLogging("lease", opened.ID, opened.Generation, false); err == nil {
		t.Fatal("expected duplicate logging start to fail")
	}
	manager.Shutdown()
}

func TestSessionExitCannotRaceWithLoggingStart(t *testing.T) {
	transport := newFakeTransport()
	manager := NewManager(&fakeFactory{transport: transport})
	logs := &memoryLogFactory{}
	sink := newRecordingSink()
	manager.SetLogFactory(logs)
	manager.SetSink(sink)
	opened, err := manager.OpenLocal(context.Background(), "lease", profile.Profile{
		ID: "local", Name: "Local", Protocol: profile.ProtocolLocal,
	}, 80, 24)
	if err != nil {
		t.Fatalf("open terminal: %v", err)
	}
	<-sink.state
	if err := manager.Activate("lease", opened.ID, opened.Generation); err != nil {
		t.Fatalf("activate terminal: %v", err)
	}
	<-sink.state
	transport.finish()
	for {
		event := <-sink.state
		if event.State == StateExited {
			break
		}
	}
	if _, err := manager.StartLogging("lease", opened.ID, opened.Generation, false); err == nil {
		t.Fatal("expected exited session to reject logging")
	}
	status, err := manager.LoggingStatus("lease", opened.ID, opened.Generation)
	if err != nil {
		t.Fatalf("logging status: %v", err)
	}
	if status.Active {
		t.Fatalf("logging remained active after exit: %#v", status)
	}
	manager.Shutdown()
}

func receiveLogStatus(t *testing.T, channel <-chan SessionLogStatus) SessionLogStatus {
	t.Helper()
	select {
	case status := <-channel:
		return status
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for session log status")
		return SessionLogStatus{}
	}
}

var _ port.SessionLogFactory = (*memoryLogFactory)(nil)
var _ port.SessionLog = (*memoryLog)(nil)
var _ LogSink = (*recordingSink)(nil)
