package eventstream

import (
	"context"
	"sync"
)

type EventStream[T any, R any] struct {
	events     chan T
	resultCh   chan struct{}
	isComplete func(T) bool
	extract    func(T) R
	mu         sync.Mutex
	ready      *sync.Cond
	queue      []T
	queueHead  int
	done       bool
	result     R
}

func NewEventStream[T any, R any](isComplete func(T) bool, extract func(T) R) *EventStream[T, R] {
	stream := &EventStream[T, R]{
		// Retain the previous delivery buffer so Result-only consumers do not
		// strand the dispatcher for ordinary streams. The producer-facing queue
		// below remains logically unbounded and never inherits this capacity.
		events:     make(chan T, 4096),
		resultCh:   make(chan struct{}),
		isComplete: isComplete,
		extract:    extract,
	}
	stream.ready = sync.NewCond(&stream.mu)
	go stream.dispatch()
	return stream
}

func (s *EventStream[T, R]) Push(event T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return
	}
	s.queue = append(s.queue, event)
	if s.isComplete(event) {
		s.result = s.extract(event)
		s.done = true
		close(s.resultCh)
	}
	s.ready.Signal()
}

func (s *EventStream[T, R]) End(result R) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return
	}
	s.result = result
	s.done = true
	close(s.resultCh)
	s.ready.Signal()
}

func (s *EventStream[T, R]) Events() <-chan T {
	return s.events
}

func (s *EventStream[T, R]) Result(ctx context.Context) (R, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-s.resultCh:
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.result, nil
	case <-ctx.Done():
		var zero R
		return zero, ctx.Err()
	}
}

// dispatch is the sole owner of the public event channel. Producers append to
// an unbounded logical queue under the mutex, matching Pi's synchronous push
// semantics without making provider I/O wait for a consumer. Keeping channel
// closure in one goroutine also removes Push/End send-versus-close races.
func (s *EventStream[T, R]) dispatch() {
	for {
		s.mu.Lock()
		for s.queueHead == len(s.queue) && !s.done {
			s.ready.Wait()
		}
		if s.queueHead < len(s.queue) {
			event := s.queue[s.queueHead]
			var zero T
			s.queue[s.queueHead] = zero
			s.queueHead++
			if s.queueHead == len(s.queue) {
				s.queue = nil
				s.queueHead = 0
			}
			s.mu.Unlock()
			s.events <- event
			continue
		}
		s.mu.Unlock()
		close(s.events)
		return
	}
}
