package gillmprovider

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func collectAssistantEvents(stream *AssistantMessageEventStream) []AssistantMessageEvent {
	var events []AssistantMessageEvent
	for event := range stream.Events() {
		events = append(events, event)
	}
	return events
}

func TestFauxProviderRegistersCustomProviderAndEstimatesUsage(t *testing.T) {
	registration := RegisterFauxProvider()
	defer registration.Unregister()
	registration.SetResponses([]FauxResponseStep{{Message: FauxAssistantText("hello world")}})

	contextValue := Context{SystemPrompt: "Be concise.", Messages: []Message{UserMessageText("hi there")}}
	response, err := Complete(context.Background(), registration.MustModel(), contextValue, StreamOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(response.Content, []ContentPart{Text("hello world")}) {
		t.Fatalf("content = %#v", response.Content)
	}
	if response.Usage.Input <= 0 || response.Usage.Output <= 0 || response.Usage.TotalTokens != response.Usage.Input+response.Usage.Output {
		t.Fatalf("usage = %#v", response.Usage)
	}
	if registration.State.CallCount != 1 {
		t.Fatalf("call count = %d", registration.State.CallCount)
	}
}

func TestFauxProviderHelpersAndMultipleModels(t *testing.T) {
	registration := RegisterFauxProvider(WithFauxModels(
		FauxModelDefinition{ID: "faux-fast", Name: "Faux Fast", Reasoning: false},
		FauxModelDefinition{ID: "faux-thinker", Name: "Faux Thinker", Reasoning: true},
	))
	defer registration.Unregister()
	registration.SetResponses([]FauxResponseStep{
		{Factory: func(_ Context, _ StreamOptions, _ FauxState, model Model) (Message, error) {
			return FauxAssistantText(model.ID + ":false"), nil
		}},
		{Factory: func(_ Context, _ StreamOptions, _ FauxState, model Model) (Message, error) {
			return FauxAssistantText(model.ID + ":true"), nil
		}},
	})
	if len(registration.Models) != 2 || registration.Models[0].ID != "faux-fast" || !registration.Models[1].Reasoning {
		t.Fatalf("models = %#v", registration.Models)
	}
	fast, _ := Complete(context.Background(), registration.MustModel("faux-fast"), Context{Messages: []Message{UserMessageText("hi")}}, StreamOptions{})
	thinker, _ := Complete(context.Background(), registration.MustModel("faux-thinker"), Context{Messages: []Message{UserMessageText("hi")}}, StreamOptions{})
	if fast.Content[0].Text != "faux-fast:false" || thinker.Content[0].Text != "faux-thinker:true" {
		t.Fatalf("responses = %#v / %#v", fast.Content, thinker.Content)
	}
}

func TestFauxProviderRewritesAPIProviderModel(t *testing.T) {
	registration := RegisterFauxProvider(
		WithFauxAPI("faux:test"),
		WithFauxProvider("faux-provider"),
		WithFauxModels(FauxModelDefinition{ID: "faux-model"}),
	)
	defer registration.Unregister()
	registration.SetResponses([]FauxResponseStep{{Message: FauxAssistantText("hello")}})
	response, err := Complete(context.Background(), registration.MustModel(), Context{Messages: []Message{UserMessageText("hi")}}, StreamOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if response.API != "faux:test" || response.Provider != "faux-provider" || response.Model != "faux-model" {
		t.Fatalf("response identity = %#v", response)
	}
}

func TestFauxProviderQueuedResponses(t *testing.T) {
	registration := RegisterFauxProvider()
	defer registration.Unregister()
	registration.SetResponses([]FauxResponseStep{{Message: FauxAssistantText("first")}, {Message: FauxAssistantText("second")}})
	contextValue := Context{Messages: []Message{UserMessageText("hi")}}
	first, _ := Complete(context.Background(), registration.MustModel(), contextValue, StreamOptions{})
	second, _ := Complete(context.Background(), registration.MustModel(), contextValue, StreamOptions{})
	exhausted, _ := Complete(context.Background(), registration.MustModel(), contextValue, StreamOptions{})
	if first.Content[0].Text != "first" || second.Content[0].Text != "second" {
		t.Fatalf("responses = %q / %q", first.Content[0].Text, second.Content[0].Text)
	}
	if exhausted.StopReason != StopReasonError || exhausted.ErrorMessage != "No more faux responses queued" || registration.PendingResponseCount() != 0 || registration.State.CallCount != 3 {
		t.Fatalf("exhausted = %#v pending=%d calls=%d", exhausted, registration.PendingResponseCount(), registration.State.CallCount)
	}

	registration.SetResponses([]FauxResponseStep{{Message: FauxAssistantText("third")}})
	registration.AppendResponses([]FauxResponseStep{{Message: FauxAssistantText("fourth")}})
	if registration.PendingResponseCount() != 2 {
		t.Fatalf("pending = %d", registration.PendingResponseCount())
	}
}

func TestFauxProviderFactoryErrorsBecomeTerminalErrorEvents(t *testing.T) {
	registration := RegisterFauxProvider()
	defer registration.Unregister()
	registration.SetResponses([]FauxResponseStep{{Factory: func(Context, StreamOptions, FauxState, Model) (Message, error) {
		return Message{}, errors.New("boom")
	}}})
	stream, err := Stream(registration.MustModel(), Context{Messages: []Message{UserMessageText("hi")}}, StreamOptions{})
	if err != nil {
		t.Fatal(err)
	}
	events := collectAssistantEvents(stream)
	if len(events) != 1 || events[0].Type != "error" || events[0].Error.ErrorMessage != "boom" {
		t.Fatalf("events = %#v", events)
	}
}

func TestFauxProviderPromptCaching(t *testing.T) {
	registration := RegisterFauxProvider()
	defer registration.Unregister()
	registration.SetResponses([]FauxResponseStep{{Message: FauxAssistantText("first")}, {Message: FauxAssistantText("second")}, {Message: FauxAssistantText("third")}})
	contextValue := Context{Messages: []Message{UserMessageText("hello")}}
	first, _ := Complete(context.Background(), registration.MustModel(), contextValue, StreamOptions{SessionID: "session-1", CacheRetention: "short"})
	if first.Usage.CacheRead != 0 || first.Usage.CacheWrite <= 0 {
		t.Fatalf("first usage = %#v", first.Usage)
	}
	contextValue.Messages = append(contextValue.Messages, first, UserMessageText("follow up"))
	second, _ := Complete(context.Background(), registration.MustModel(), contextValue, StreamOptions{SessionID: "session-1", CacheRetention: "short"})
	if second.Usage.CacheRead <= 0 {
		t.Fatalf("second usage = %#v", second.Usage)
	}
	third, _ := Complete(context.Background(), registration.MustModel(), contextValue, StreamOptions{})
	if third.Usage.CacheRead != 0 || third.Usage.CacheWrite != 0 {
		t.Fatalf("third usage = %#v", third.Usage)
	}
}

func TestFauxProviderStreamsContentEvents(t *testing.T) {
	registration := RegisterFauxProvider()
	defer registration.Unregister()
	registration.SetResponses([]FauxResponseStep{{Message: FauxAssistantMessage([]ContentPart{
		FauxThinking("think"),
		FauxText("answer"),
		FauxToolCall("echo", map[string]any{"text": "hi"}, "tool-1"),
	}, "toolUse")}})
	stream, err := Stream(registration.MustModel(), Context{Messages: []Message{UserMessageText("hi")}}, StreamOptions{})
	if err != nil {
		t.Fatal(err)
	}
	events := collectAssistantEvents(stream)
	types := make([]string, len(events))
	for i, event := range events {
		types[i] = event.Type
	}
	for _, want := range []string{"start", "thinking_start", "thinking_delta", "thinking_end", "text_start", "text_delta", "text_end", "toolcall_start", "toolcall_delta", "toolcall_end", "done"} {
		if !containsString(types, want) {
			t.Fatalf("event types = %#v, missing %s", types, want)
		}
	}
	message, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if message.StopReason != "toolUse" || len(message.Content) != 3 {
		t.Fatalf("message = %#v", message)
	}
}
