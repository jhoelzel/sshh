package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"shh-h/internal/apperror"
	"shh-h/internal/domain/profile"
	"shh-h/internal/port"
)

type trackingTerminalFactory struct {
	mu         sync.Mutex
	transports map[string]*fakeTransport
}

func newTrackingTerminalFactory() *trackingTerminalFactory {
	return &trackingTerminalFactory{transports: make(map[string]*fakeTransport)}
}

func (f *trackingTerminalFactory) Open(_ context.Context, spec port.TerminalSpec) (port.TerminalTransport, error) {
	const prefix = "SHHH_SESSION_ID="
	var sessionID string
	for _, entry := range spec.Environment {
		if strings.HasPrefix(entry, prefix) {
			sessionID = strings.TrimPrefix(entry, prefix)
			break
		}
	}
	if sessionID == "" {
		return nil, errors.New("terminal spec omitted its session id")
	}
	transport := newFakeTransport()
	f.mu.Lock()
	f.transports[sessionID] = transport
	f.mu.Unlock()
	return transport, nil
}

func (f *trackingTerminalFactory) transport(sessionID string) (*fakeTransport, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	transport, ok := f.transports[sessionID]
	return transport, ok
}

type lifecycleRaceSink struct {
	outputs chan OutputChunk
}

func (s *lifecycleRaceSink) PublishOutput(chunk OutputChunk) {
	s.outputs <- chunk
}

func (*lifecycleRaceSink) PublishState(StateEvent) {}

func TestManagerConcurrentOpenOutputResizeAndClose(t *testing.T) {
	const (
		sessionCount = 8
		operations   = 32
	)
	factory := newTrackingTerminalFactory()
	manager := newManager(factory, sessionCount)
	sink := &lifecycleRaceSink{outputs: make(chan OutputChunk, sessionCount*(operations+4))}
	manager.SetSink(sink)
	t.Cleanup(manager.Shutdown)

	type openResult struct {
		session Session
		err     error
	}
	openResults := make(chan openResult, sessionCount)
	openStart := make(chan struct{})
	var openCalls sync.WaitGroup
	for index := range sessionCount {
		openCalls.Add(1)
		go func() {
			defer openCalls.Done()
			<-openStart
			opened, err := manager.OpenLocal(context.Background(), "lease", profile.Profile{
				ID: fmt.Sprintf("local-%d", index), Name: "Local", Protocol: profile.ProtocolLocal,
			}, 80, 24)
			openResults <- openResult{session: opened, err: err}
		}()
	}
	close(openStart)
	openCalls.Wait()
	close(openResults)

	opened := make([]Session, 0, sessionCount)
	for result := range openResults {
		if result.err != nil {
			t.Fatalf("open terminal concurrently: %v", result.err)
		}
		opened = append(opened, result.session)
	}
	if len(opened) != sessionCount {
		t.Fatalf("opened %d sessions, want %d", len(opened), sessionCount)
	}
	for _, session := range opened {
		if err := manager.Activate("lease", session.ID, session.Generation); err != nil {
			t.Fatalf("activate terminal %q: %v", session.ID, err)
		}
	}

	var initialWrites sync.WaitGroup
	initialWriteErrors := make(chan error, sessionCount)
	for _, session := range opened {
		transport, ok := factory.transport(session.ID)
		if !ok {
			t.Fatalf("transport for session %q was not recorded", session.ID)
		}
		initialWrites.Add(1)
		go func() {
			defer initialWrites.Done()
			if _, err := transport.writer.Write([]byte("initial output")); err != nil {
				initialWriteErrors <- err
			}
		}()
	}
	initialWrites.Wait()
	close(initialWriteErrors)
	for err := range initialWriteErrors {
		t.Fatalf("publish initial terminal output: %v", err)
	}
	seenOutput := make(map[string]bool, sessionCount)
	for len(seenOutput) < sessionCount {
		select {
		case chunk := <-sink.outputs:
			seenOutput[chunk.SessionID] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("received initial output from %d of %d sessions", len(seenOutput), sessionCount)
		}
	}

	startRace := make(chan struct{})
	var firstOperations sync.WaitGroup
	firstOperations.Add(sessionCount * 3)
	var raceCalls sync.WaitGroup
	raceErrors := make(chan error, sessionCount*4)
	for _, session := range opened {
		transport, _ := factory.transport(session.ID)
		raceCalls.Add(4)
		go func() {
			defer raceCalls.Done()
			<-startRace
			for index := range operations {
				_, err := transport.writer.Write([]byte("output"))
				if index == 0 {
					firstOperations.Done()
				}
				if err != nil {
					if !errors.Is(err, io.ErrClosedPipe) {
						raceErrors <- fmt.Errorf("write fixture output: %w", err)
					}
					return
				}
			}
		}()
		go func() {
			defer raceCalls.Done()
			<-startRace
			for index := range operations {
				err := manager.Write("lease", session.ID, session.Generation, uint64(index+1), []byte("input"))
				if index == 0 {
					firstOperations.Done()
				}
				if err != nil {
					if !expectedLifecycleRaceError(err) {
						raceErrors <- fmt.Errorf("write terminal input: %w", err)
					}
					return
				}
			}
		}()
		go func() {
			defer raceCalls.Done()
			<-startRace
			for index := range operations {
				err := manager.Resize(
					"lease", session.ID, session.Generation,
					uint16(80+index%10), uint16(24+index%5),
				)
				if index == 0 {
					firstOperations.Done()
				}
				if err != nil {
					if !expectedLifecycleRaceError(err) {
						raceErrors <- fmt.Errorf("resize terminal: %w", err)
					}
					return
				}
			}
		}()
		go func() {
			defer raceCalls.Done()
			<-startRace
			firstOperations.Wait()
			if err := manager.Close("lease", session.ID, session.Generation); err != nil {
				raceErrors <- fmt.Errorf("close terminal: %w", err)
			}
		}()
	}
	close(startRace)
	raceCalls.Wait()
	close(raceErrors)
	for err := range raceErrors {
		t.Error(err)
	}

	manager.mu.RLock()
	runtimeCount := len(manager.runtimes)
	openingCount := manager.openingSessions
	dispatcher := manager.output
	manager.mu.RUnlock()
	if manager.LiveCount() != 0 || runtimeCount != 0 || openingCount != 0 || dispatcher != nil {
		t.Fatalf(
			"concurrent lifecycle retained resources: live=%d runtimes=%d opening=%d dispatcher=%t",
			manager.LiveCount(), runtimeCount, openingCount, dispatcher != nil,
		)
	}
}

func expectedLifecycleRaceError(err error) bool {
	switch apperror.CodeOf(err) {
	case apperror.CodeNotFound, apperror.CodeConflict, apperror.CodeUnavailable, apperror.CodeCanceled:
		return true
	default:
		return false
	}
}

var _ port.TerminalFactory = (*trackingTerminalFactory)(nil)
var _ Sink = (*lifecycleRaceSink)(nil)
