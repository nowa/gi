package gillmprovider

import "github.com/nowa/gi/gi-llm-provider/internal/eventstream"

type EventStream[T any, R any] struct {
	*eventstream.EventStream[T, R]
}

func NewEventStream[T any, R any](isComplete func(T) bool, extract func(T) R) *EventStream[T, R] {
	return &EventStream[T, R]{
		EventStream: eventstream.NewEventStream(isComplete, extract),
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
