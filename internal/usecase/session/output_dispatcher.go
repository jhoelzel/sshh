package session

import (
	"context"
	"sync"
)

const outputDispatchCapacity = 256

type outputDispatchRequest struct {
	ctx    context.Context
	chunk  OutputChunk
	result chan error
}

type outputDispatcher struct {
	emit      func(OutputChunk) error
	incoming  chan outputDispatchRequest
	stopping  chan struct{}
	stopped   chan struct{}
	closeOnce sync.Once
}

func newOutputDispatcher(emit func(OutputChunk) error) *outputDispatcher {
	dispatcher := &outputDispatcher{
		emit:     emit,
		incoming: make(chan outputDispatchRequest, outputDispatchCapacity),
		stopping: make(chan struct{}),
		stopped:  make(chan struct{}),
	}
	go dispatcher.run()
	return dispatcher
}

func (d *outputDispatcher) Dispatch(ctx context.Context, chunk OutputChunk) error {
	request := outputDispatchRequest{ctx: ctx, chunk: chunk, result: make(chan error, 1)}
	select {
	case d.incoming <- request:
	case <-ctx.Done():
		return ctx.Err()
	case <-d.stopping:
		return context.Canceled
	}

	select {
	case err := <-request.result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-d.stopping:
		return context.Canceled
	}
}

func (d *outputDispatcher) Close() {
	// Sink delivery is external and cannot be canceled; control shutdown must not wait behind it.
	d.closeOnce.Do(func() { close(d.stopping) })
}

func (d *outputDispatcher) run() {
	defer close(d.stopped)
	queues := make(map[string][]outputDispatchRequest)
	order := make([]string, 0)

	enqueue := func(request outputDispatchRequest) {
		key := request.chunk.SessionID
		if len(queues[key]) == 0 {
			order = append(order, key)
		}
		queues[key] = append(queues[key], request)
	}
	failQueued := func() {
		for _, queue := range queues {
			for _, request := range queue {
				request.result <- context.Canceled
			}
		}
	}

	for {
		select {
		case <-d.stopping:
			failQueued()
			return
		default:
		}

		if len(order) == 0 {
			select {
			case request := <-d.incoming:
				enqueue(request)
			case <-d.stopping:
				failQueued()
				return
			}
		}

	drain:
		for range outputDispatchCapacity {
			select {
			case request := <-d.incoming:
				enqueue(request)
			case <-d.stopping:
				failQueued()
				return
			default:
				break drain
			}
		}

		key := order[0]
		order = order[1:]
		queue := queues[key]
		request := queue[0]
		if len(queue) == 1 {
			delete(queues, key)
		} else {
			queues[key] = queue[1:]
			order = append(order, key)
		}

		select {
		case <-request.ctx.Done():
			request.result <- request.ctx.Err()
		default:
			request.result <- d.emit(request.chunk)
		}
	}
}
