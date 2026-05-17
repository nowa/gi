package gillmprovider

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
	done       bool
	result     R
}

func NewEventStream[T any, R any](isComplete func(T) bool, extract func(T) R) *EventStream[T, R] {
	return &EventStream[T, R]{
		events:     make(chan T, 4096),
		resultCh:   make(chan struct{}),
		isComplete: isComplete,
		extract:    extract,
	}
}

func (s *EventStream[T, R]) Push(event T) {
	s.mu.Lock()
	if s.done {
		s.mu.Unlock()
		return
	}
	complete := s.isComplete(event)
	if complete {
		s.result = s.extract(event)
		s.done = true
	}
	s.mu.Unlock()

	s.events <- event
	if complete {
		close(s.events)
		close(s.resultCh)
	}
}

func (s *EventStream[T, R]) End(result R) {
	s.mu.Lock()
	if s.done {
		s.mu.Unlock()
		return
	}
	s.result = result
	s.done = true
	s.mu.Unlock()
	close(s.events)
	close(s.resultCh)
}

func (s *EventStream[T, R]) Events() <-chan T {
	return s.events
}

func (s *EventStream[T, R]) Result(ctx context.Context) (R, error) {
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

type AssistantMessageEventStream = EventStream[AssistantMessageEvent, Message]

func NewAssistantMessageEventStream() *AssistantMessageEventStream {
	return NewEventStream(func(event AssistantMessageEvent) bool {
		return event.Type == "done" || event.Type == "error"
	}, func(event AssistantMessageEvent) Message {
		if event.Type == "done" {
			return event.Message
		}
		return event.Error
	})
}

func CompletedAssistantStream(message Message) *AssistantMessageEventStream {
	stream := NewAssistantMessageEventStream()
	go stream.Push(AssistantMessageEvent{Type: "done", Reason: message.StopReason, Message: message})
	return stream
}

func ErrorAssistantStream(message Message) *AssistantMessageEventStream {
	stream := NewAssistantMessageEventStream()
	go stream.Push(AssistantMessageEvent{Type: "error", Reason: message.StopReason, Error: message})
	return stream
}
