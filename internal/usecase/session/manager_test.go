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
	resizeMu   sync.Mutex
	columns    uint16
	rows       uint16
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

func (t *fakeTransport) Resize(_ context.Context, columns, rows uint16) error {
	t.resizeMu.Lock()
	t.columns = columns
	t.rows = rows
	t.resizeMu.Unlock()
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

func (t *fakeTransport) size() (uint16, uint16) {
	t.resizeMu.Lock()
	defer t.resizeMu.Unlock()
	return t.columns, t.rows
}

type recordingSink struct {
	output chan OutputChunk
	state  chan StateEvent
	logs   chan SessionLogStatus
}

type blockingOutputSink struct {
	outputStarted chan struct{}
	releaseOutput chan struct{}
	state         chan StateEvent
	outputOnce    sync.Once
}

func newBlockingOutputSink() *blockingOutputSink {
	return &blockingOutputSink{
		outputStarted: make(chan struct{}),
		releaseOutput: make(chan struct{}),
		state:         make(chan StateEvent, 16),
	}
}

func (s *blockingOutputSink) PublishOutput(OutputChunk) {
	s.outputOnce.Do(func() { close(s.outputStarted) })
	<-s.releaseOutput
}

func (s *blockingOutputSink) PublishState(event StateEvent) {
	s.state <- event
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
	pressure, err := manager.Diagnostics("lease", opened.ID, opened.Generation)
	if err != nil {
		t.Fatalf("read terminal diagnostics: %v", err)
	}
	if pressure.EmittedBytes != 5 || pressure.UnacknowledgedBytes != 5 || pressure.PendingChunks != 1 ||
		pressure.PeakUnacknowledgedBytes != 5 || pressure.PeakPendingChunks != 1 ||
		pressure.MaximumUnacknowledged != maxUnackedBytes {
		t.Fatalf("unexpected terminal pressure before acknowledgement: %#v", pressure)
	}
	if err := manager.Acknowledge("lease", opened.ID, opened.Generation, chunk.Sequence, chunk.EndOffset); err != nil {
		t.Fatalf("acknowledge output: %v", err)
	}
	pressure, err = manager.Diagnostics("lease", opened.ID, opened.Generation)
	if err != nil {
		t.Fatalf("read acknowledged terminal diagnostics: %v", err)
	}
	if pressure.AcknowledgedSequence != 1 || pressure.AcknowledgedBytes != 5 ||
		pressure.UnacknowledgedBytes != 0 || pressure.PendingChunks != 0 ||
		pressure.PeakUnacknowledgedBytes != 5 || pressure.PeakPendingChunks != 1 {
		t.Fatalf("unexpected terminal pressure after acknowledgement: %#v", pressure)
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

func TestManagerPreservesArbitraryTerminalOutputBytes(t *testing.T) {
	transport := newFakeTransport()
	manager := NewManager(&fakeFactory{transport: transport})
	sink := newRecordingSink()
	manager.SetSink(sink)

	opened, err := manager.OpenLocal(context.Background(), "lease", profile.Profile{
		ID: "local", Name: "Local", Protocol: profile.ProtocolLocal,
	}, 80, 24)
	if err != nil {
		t.Fatalf("open local terminal: %v", err)
	}
	if err := manager.Activate("lease", opened.ID, opened.Generation); err != nil {
		t.Fatalf("activate terminal: %v", err)
	}

	payload := []byte{0xff, 0xfe, 0xe2, 0x28, 0xa1, 0x1b, '[', '3', '8', ';'}
	writeDone := make(chan error, 1)
	go func() {
		_, writeErr := transport.writer.Write(payload)
		writeDone <- writeErr
	}()
	chunk := receiveOutput(t, sink.output)
	if !bytes.Equal(chunk.Data, payload) || len(chunk.Data) != len(payload) {
		t.Fatalf("terminal output bytes changed: %v", chunk.Data)
	}
	waitForCall(t, writeDone, "terminal output fixture")
	if err := manager.Acknowledge("lease", opened.ID, opened.Generation, chunk.Sequence, chunk.EndOffset); err != nil {
		t.Fatalf("acknowledge terminal output: %v", err)
	}
	if err := manager.Close("lease", opened.ID, opened.Generation); err != nil {
		t.Fatalf("close terminal: %v", err)
	}
}

func TestManagerControlAndLifecycleBypassBlockedOutput(t *testing.T) {
	transport := newFakeTransport()
	manager := NewManager(&fakeFactory{transport: transport})
	sink := newBlockingOutputSink()
	manager.SetSink(sink)

	var releaseOnce sync.Once
	releaseOutput := func() {
		releaseOnce.Do(func() { close(sink.releaseOutput) })
	}
	t.Cleanup(func() {
		releaseOutput()
		manager.Shutdown()
	})

	opened, err := manager.OpenLocal(context.Background(), "lease", profile.Profile{
		ID: "local", Name: "Local", Protocol: profile.ProtocolLocal,
	}, 80, 24)
	if err != nil {
		t.Fatalf("open local terminal: %v", err)
	}
	receiveState(t, sink.state, StateStarting)
	if err := manager.Activate("lease", opened.ID, opened.Generation); err != nil {
		t.Fatalf("activate terminal: %v", err)
	}
	receiveState(t, sink.state, StateRunning)

	writeOutputDone := make(chan error, 1)
	go func() {
		_, writeErr := transport.writer.Write([]byte("blocked output"))
		writeOutputDone <- writeErr
	}()
	waitForSignal(t, sink.outputStarted, "blocked terminal output")

	callDone := make(chan error, 1)
	go func() {
		callDone <- manager.Write("lease", opened.ID, opened.Generation, 1, []byte("input"))
	}()
	waitForCall(t, callDone, "terminal input")
	if got := string(transport.inputBytes()); got != "input" {
		t.Fatalf("unexpected input while output was blocked: %q", got)
	}

	go func() {
		callDone <- manager.Resize("lease", opened.ID, opened.Generation, 120, 40)
	}()
	waitForCall(t, callDone, "terminal resize")
	if columns, rows := transport.size(); columns != 120 || rows != 40 {
		t.Fatalf("resize did not reach transport: %dx%d", columns, rows)
	}

	manager.mu.RLock()
	dispatcher := manager.output
	manager.mu.RUnlock()
	go func() {
		callDone <- manager.Close("lease", opened.ID, opened.Generation)
	}()
	receiveState(t, sink.state, StateClosing)
	waitForCall(t, callDone, "terminal close")

	releaseOutput()
	waitForSignal(t, dispatcher.stopped, "output dispatcher shutdown")
	waitForCall(t, writeOutputDone, "terminal output fixture")
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

func receiveState(t *testing.T, channel <-chan StateEvent, expected State) StateEvent {
	t.Helper()
	for {
		select {
		case event := <-channel:
			if event.State == expected {
				return event
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for terminal state %q", expected)
			return StateEvent{}
		}
	}
}

func waitForCall(t *testing.T, done <-chan error, operation string) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("%s failed: %v", operation, err)
		}
	case <-time.After(time.Second):
		t.Fatalf("%s waited behind terminal output", operation)
	}
}

func waitForSignal(t *testing.T, signal <-chan struct{}, operation string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", operation)
	}
}

var _ port.TerminalTransport = (*fakeTransport)(nil)
var _ Sink = (*recordingSink)(nil)
var _ Sink = (*blockingOutputSink)(nil)
