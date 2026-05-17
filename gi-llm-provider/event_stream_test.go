package gillmprovider

import (
	"context"
	"testing"
)

func TestAssistantMessageEventStreamResult(t *testing.T) {
	model := MustGetModel("openai", "gpt-4o-mini")
	message := AssistantMessage([]ContentPart{Text("ok")}, StopReasonStop, model)
	stream := NewAssistantMessageEventStream()
	stream.Push(AssistantMessageEvent{Type: "done", Reason: "stop", Message: message})

	got, err := stream.Result(context.Background())
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	if got.Content[0].Text != "ok" {
		t.Fatalf("Result text = %q, want ok", got.Content[0].Text)
	}
}

func TestCompleteReturnsAbortedMessageOnContextCancellation(t *testing.T) {
	registration := RegisterFauxProvider()
	defer registration.Unregister()
	registration.SetResponses([]FauxResponseStep{{Message: FauxAssistantText("should not run")}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	message, err := Complete(ctx, registration.MustModel(), Context{Messages: []Message{UserMessageText("hi")}}, StreamOptions{})

	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if message.StopReason != StopReasonAborted || message.ErrorMessage == "" {
		t.Fatalf("message = %#v", message)
	}
	if registration.State.CallCount != 0 {
		t.Fatalf("provider should not be called after pre-abort, calls=%d", registration.State.CallCount)
	}
}

func TestStreamSimpleReturnsAbortedStreamOnOptionsContextCancellation(t *testing.T) {
	registration := RegisterFauxProvider()
	defer registration.Unregister()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stream, err := StreamSimple(registration.MustModel(), Context{Messages: []Message{UserMessageText("hi")}}, SimpleStreamOptions{Context: ctx})
	if err != nil {
		t.Fatalf("StreamSimple() error = %v", err)
	}
	message, err := stream.Result(context.Background())
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	if message.StopReason != StopReasonAborted {
		t.Fatalf("message = %#v", message)
	}
}
