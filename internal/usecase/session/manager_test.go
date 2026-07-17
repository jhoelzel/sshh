package session

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"shh-h/internal/domain/profile"
	"shh-h/internal/port"
)

type fakeFactory struct {
	transport *fakeTransport
}

func (f *fakeFactory) Open(context.Context, port.TerminalSpec) (port.TerminalTransport, error) {
	return f.transport, nil
}

type fakeTransport struct {
	reader     *io.PipeReader
	writer     *io.PipeWriter
	inputMu    sync.Mutex
	input      bytes.Buffer
	wait       chan struct{}
	finishOnce sync.Once
}

func newFakeTransport() *fakeTransport {
	reader, writer := io.Pipe()
	return &fakeTransport{reader: reader, writer: writer, wait: make(chan struct{})}
}

func (t *fakeTransport) Read(data []byte) (int, error) {
	return t.reader.Read(data)
}

func (t *fakeTransport) Write(data []byte) (int, error) {
	t.inputMu.Lock()
	defer t.inputMu.Unlock()
	return t.input.Write(data)
}

func (t *fakeTransport) Resize(context.Context, uint16, uint16) error {
	return nil
}

func (t *fakeTransport) Signal(context.Context, port.TerminalSignal) error {
	t.finish()
	return nil
}

func (t *fakeTransport) Wait() (port.ExitStatus, error) {
	<-t.wait
	return port.ExitStatus{}, nil
}

func (t *fakeTransport) Close() error {
	t.finish()
	return nil
}

func (t *fakeTransport) finish() {
	t.finishOnce.Do(func() {
		close(t.wait)
		_ = t.writer.Close()
		_ = t.reader.Close()
	})
}

func (t *fakeTransport) inputBytes() []byte {
	t.inputMu.Lock()
	defer t.inputMu.Unlock()
	return append([]byte(nil), t.input.Bytes()...)
}

type recordingSink struct {
	output chan OutputChunk
	state  chan StateEvent
	logs   chan SessionLogStatus
}

func newRecordingSink() *recordingSink {
	return &recordingSink{
		output: make(chan OutputChunk, 16), state: make(chan StateEvent, 16), logs: make(chan SessionLogStatus, 16),
	}
}

func (s *recordingSink) PublishOutput(chunk OutputChunk) {
	s.output <- chunk
}

func (s *recordingSink) PublishState(event StateEvent) {
	s.state <- event
}

func (s *recordingSink) PublishSessionLog(status SessionLogStatus) {
	s.logs <- status
}

func TestManagerActivatesBeforePublishingAndAcknowledgesCumulatively(t *testing.T) {
	transport := newFakeTransport()
	manager := NewManager(&fakeFactory{transport: transport})
	sink := newRecordingSink()
	manager.SetSink(sink)

	opened, err := manager.OpenLocal(context.Background(), "lease", profile.Profile{
		ID: "local", Name: "Local", Protocol: profile.ProtocolLocal,
	}, 100, 30)
	if err != nil {
		t.Fatalf("open local terminal: %v", err)
	}
	openedEvent := <-sink.state
	if openedEvent.Title != "Local" {
		t.Fatalf("state event omitted the session title: %#v", openedEvent)
	}

	writeDone := make(chan error, 1)
	go func() {
		_, writeErr := transport.writer.Write([]byte("hello"))
		writeDone <- writeErr
	}()
	select {
	case <-sink.output:
		t.Fatal("received output before frontend activation")
	case <-time.After(30 * time.Millisecond):
	}

	if err := manager.Activate("lease", opened.ID, opened.Generation); err != nil {
		t.Fatalf("activate terminal: %v", err)
	}
	chunk := receiveOutput(t, sink.output)
	if string(chunk.Data) != "hello" || chunk.Sequence != 1 || chunk.EndOffset != 5 {
		t.Fatalf("unexpected output chunk: %#v", chunk)
	}
	if err := <-writeDone; err != nil {
		t.Fatalf("write output fixture: %v", err)
	}
	if err := manager.Acknowledge("lease", opened.ID, opened.Generation, chunk.Sequence, chunk.EndOffset); err != nil {
		t.Fatalf("acknowledge output: %v", err)
	}
	if err := manager.Acknowledge("lease", opened.ID, opened.Generation, chunk.Sequence, chunk.EndOffset); err != nil {
		t.Fatalf("repeat cumulative acknowledgement: %v", err)
	}
	if err := manager.Acknowledge("lease", opened.ID, opened.Generation, 99, chunk.EndOffset); err == nil {
		t.Fatal("expected unknown output sequence to fail")
	}

	if err := manager.Close("lease", opened.ID, opened.Generation); err != nil {
		t.Fatalf("close terminal: %v", err)
	}
	if manager.LiveCount() != 0 {
		t.Fatal("expected no live terminal after close")
	}
}

func TestManagerSerializesInputAndRejectsStaleGeneration(t *testing.T) {
	transport := newFakeTransport()
	manager := NewManager(&fakeFactory{transport: transport})
	manager.SetSink(newRecordingSink())
	opened, err := manager.OpenLocal(context.Background(), "lease", profile.Profile{
		ID: "local", Name: "Local", Protocol: profile.ProtocolLocal,
	}, 80, 24)
	if err != nil {
		t.Fatalf("open local terminal: %v", err)
	}
	if err := manager.Activate("lease", opened.ID, opened.Generation); err != nil {
		t.Fatalf("activate terminal: %v", err)
	}

	if err := manager.Write("lease", opened.ID, opened.Generation, 1, []byte("a")); err != nil {
		t.Fatalf("write first input: %v", err)
	}
	if err := manager.Write("lease", opened.ID, opened.Generation, 1, []byte("duplicate")); err != nil {
		t.Fatalf("repeat input should be idempotent: %v", err)
	}
	if err := manager.Write("lease", opened.ID, opened.Generation, 3, []byte("gap")); err == nil {
		t.Fatal("expected an input sequence gap")
	}
	if err := manager.Write("lease", opened.ID, opened.Generation, 2, []byte("b")); err != nil {
		t.Fatalf("write second input: %v", err)
	}
	if got := string(transport.inputBytes()); got != "ab" {
		t.Fatalf("unexpected input stream %q", got)
	}
	if err := manager.Write("lease", opened.ID, opened.Generation+1, 3, []byte("stale")); err == nil {
		t.Fatal("expected stale generation to fail")
	}
	if err := manager.Write("other-lease", opened.ID, opened.Generation, 3, []byte("stale")); err == nil {
		t.Fatal("expected stale lease to fail")
	}

	if err := manager.Close("lease", opened.ID, opened.Generation); err != nil {
		t.Fatalf("close terminal: %v", err)
	}
}

func TestManagerRejectsInvalidSize(t *testing.T) {
	manager := NewManager(&fakeFactory{transport: newFakeTransport()})
	_, err := manager.OpenLocal(context.Background(), "lease", profile.Profile{
		ID: "local", Name: "Local", Protocol: profile.ProtocolLocal,
	}, 0, 24)
	if err == nil {
		t.Fatal("expected invalid size to fail")
	}
}

func receiveOutput(t *testing.T, channel <-chan OutputChunk) OutputChunk {
	t.Helper()
	select {
	case chunk := <-channel:
		return chunk
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for terminal output")
		return OutputChunk{}
	}
}

var _ port.TerminalTransport = (*fakeTransport)(nil)
var _ Sink = (*recordingSink)(nil)
