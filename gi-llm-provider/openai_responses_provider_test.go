package gillmprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIResponsesProviderStreamsFromHTTP(t *testing.T) {
	var requestPath string
	var authHeader string
	var payload OpenAIResponsesPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		authHeader = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("content-type", "text/event-stream")
		writeSSE(t, w, `{"type":"response.created","response":{"id":"resp_live","status":"in_progress"}}`)
		writeSSE(t, w, `{"type":"response.output_item.added","item":{"type":"message","id":"msg_1","status":"in_progress"}}`)
		writeSSE(t, w, `{"type":"response.content_part.added","part":{"type":"output_text","text":""}}`)
		writeSSE(t, w, `{"type":"response.output_text.delta","delta":"Hello"}`)
		writeSSE(t, w, `{"type":"response.output_item.done","item":{"type":"message","id":"msg_1","status":"completed","content":[{"type":"output_text","text":"Hello"}]}}`)
		writeSSE(t, w, `{"type":"response.completed","response":{"id":"resp_live","status":"completed","usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7,"input_tokens_details":{"cached_tokens":1,"cache_write_tokens":0}}}}`)
	}))
	defer server.Close()

	model := Model{ID: "gpt-test", Provider: "openai", API: "openai-responses", BaseURL: server.URL + "/v1", Input: []string{"text"}}
	provider := NewOpenAIResponsesProvider(server.Client())
	stream, err := provider.StreamSimple(model, Context{Messages: []Message{UserMessageText("hi")}}, SimpleStreamOptions{APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	events := collectAssistantStreamEvents(stream)
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if requestPath != "/v1/responses" || authHeader != "Bearer test-key" {
		t.Fatalf("request path/auth = %q %q", requestPath, authHeader)
	}
	if payload.Model != "gpt-test" || !payload.Stream || payload.Store {
		t.Fatalf("payload = %#v", payload)
	}
	if !containsAssistantEvent(events, "start") || !containsAssistantEvent(events, "text_delta") || !containsAssistantEvent(events, "done") {
		t.Fatalf("events = %#v", events)
	}
	if result.ResponseID != "resp_live" || result.Content[0].Text != "Hello" || result.Usage.Input != 4 || result.Usage.CacheRead != 1 || result.Usage.TotalTokens != 7 {
		t.Fatalf("result = %#v", result)
	}
}

func TestOpenAIResponsesProviderHandlesHTTPErrorAsAssistantError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	model := Model{ID: "gpt-test", Provider: "openai", API: "openai-responses", BaseURL: server.URL, Input: []string{"text"}}
	stream, err := NewOpenAIResponsesProvider(server.Client()).StreamSimple(model, Context{Messages: []Message{UserMessageText("hi")}}, SimpleStreamOptions{APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != StopReasonError || !strings.Contains(result.ErrorMessage, "HTTP 400") {
		t.Fatalf("result = %#v", result)
	}
}

func TestDecodeOpenAIResponsesSSEEvent(t *testing.T) {
	event, err := DecodeOpenAIResponsesSSEEvent([]byte(`{"type":"response.output_item.added","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup","arguments":"{\"q\":\"x\"}"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if event.Type != "response.output_item.added" || event.Item == nil || event.Item.ID != "fc_1" || event.Item.CallID != "call_1" || event.Item.Name != "lookup" {
		t.Fatalf("event = %#v", event)
	}
}

func writeSSE(t *testing.T, w http.ResponseWriter, data string) {
	t.Helper()
	if _, err := w.Write([]byte("data: " + data + "\n\n")); err != nil {
		t.Fatal(err)
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func collectAssistantStreamEvents(stream *AssistantMessageEventStream) []AssistantMessageEvent {
	var events []AssistantMessageEvent
	for event := range stream.Events() {
		events = append(events, event)
	}
	return events
}

func containsAssistantEvent(events []AssistantMessageEvent, typ string) bool {
	for _, event := range events {
		if event.Type == typ {
			return true
		}
	}
	return false
}
