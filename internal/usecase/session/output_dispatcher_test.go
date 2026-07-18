package session

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestOutputDispatcherFairlySchedulesNoisySessions(t *testing.T) {
	const floodBytes = 10 * 1024 * 1024
	const floodChunks = floodBytes / maxOutputChunk

	firstOutputStarted := make(chan struct{})
	releaseFirstOutput := make(chan struct{})
	emitted := make(chan OutputChunk, floodChunks+1)
	var firstOutput sync.Once
	var releaseFirst sync.Once
	release := func() {
		releaseFirst.Do(func() { close(releaseFirstOutput) })
	}
	dispatcher := newOutputDispatcher(func(chunk OutputChunk) error {
		firstOutput.Do(func() {
			close(firstOutputStarted)
			<-releaseFirstOutput
		})
		emitted <- chunk
		return nil
	})
	t.Cleanup(func() {
		release()
		dispatcher.Close()
		select {
		case <-dispatcher.stopped:
		case <-time.After(time.Second):
			t.Error("output dispatcher did not stop")
		}
	})

	payload := make([]byte, maxOutputChunk)
	floodRequests := make([]outputDispatchRequest, floodChunks)
	for index := range floodRequests {
		floodRequests[index] = outputDispatchRequest{
			ctx: context.Background(),
			chunk: OutputChunk{
				SessionID: "noisy", Sequence: uint64(index + 1), Data: payload,
			},
			result: make(chan error, 1),
		}
	}
	dispatcher.incoming <- floodRequests[0]
	waitForSignal(t, firstOutputStarted, "first noisy output")
	for _, request := range floodRequests[1:] {
		dispatcher.incoming <- request
	}

	quietDone := make(chan error, 1)
	dispatcher.incoming <- outputDispatchRequest{
		ctx: context.Background(),
		chunk: OutputChunk{
			SessionID: "quiet", Sequence: 1, Data: []byte("ready"),
		},
		result: quietDone,
	}
	release()

	first := receiveDispatchedOutput(t, emitted)
	second := receiveDispatchedOutput(t, emitted)
	third := receiveDispatchedOutput(t, emitted)
	if first.SessionID != "noisy" || second.SessionID != "noisy" || third.SessionID != "quiet" {
		t.Fatalf("quiet session was not scheduled fairly: %q, %q, %q", first.SessionID, second.SessionID, third.SessionID)
	}
	waitForCall(t, quietDone, "quiet terminal output")

	for _, request := range floodRequests {
		waitForCall(t, request.result, "noisy terminal output")
	}
}

func receiveDispatchedOutput(t *testing.T, output <-chan OutputChunk) OutputChunk {
	t.Helper()
	select {
	case chunk := <-output:
		return chunk
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for dispatched terminal output")
		return OutputChunk{}
	}
}
