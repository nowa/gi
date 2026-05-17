package gillmprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicMessagesProviderStreamsFromHTTP(t *testing.T) {
	var requestPath string
	var apiKeyHeader string
	var versionHeader string
	var payload AnthropicPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		apiKeyHeader = r.Header.Get("x-api-key")
		versionHeader = r.Header.Get("anthropic-version")
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("content-type", "text/event-stream")
		writeNamedSSE(t, w, "message_start", `{"type":"message_start","message":{"id":"msg_live","usage":{"input_tokens":12,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`)
		writeNamedSSE(t, w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		writeNamedSSE(t, w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`)
		writeNamedSSE(t, w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		writeNamedSSE(t, w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":12,"output_tokens":5,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`)
		writeNamedSSE(t, w, "message_stop", `{"type":"message_stop"}`)
	}))
	defer server.Close()

	model := Model{ID: "claude-test", Provider: "anthropic", API: "anthropic-messages", BaseURL: server.URL + "/v1", Input: []string{"text"}, MaxTokens: 1024}
	stream, err := NewAnthropicMessagesProvider(server.Client()).StreamSimple(model, Context{Messages: []Message{UserMessageText("hi")}}, SimpleStreamOptions{APIKey: "anthropic-key"})
	if err != nil {
		t.Fatal(err)
	}
	events := collectAssistantStreamEvents(stream)
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if requestPath != "/v1/messages" || apiKeyHeader != "anthropic-key" || versionHeader != "2023-06-01" {
		t.Fatalf("request path/key/version = %q %q %q", requestPath, apiKeyHeader, versionHeader)
	}
	if payload.Model != "claude-test" || !payload.Stream || payload.MaxTokens == 0 {
		t.Fatalf("payload = %#v", payload)
	}
	if !containsAssistantEvent(events, "start") || !containsAssistantEvent(events, "done") {
		t.Fatalf("events = %#v", events)
	}
	if result.ResponseID != "msg_live" || result.Content[0].Text != "Hello" || result.Usage.Input != 12 || result.Usage.Output != 5 {
		t.Fatalf("result = %#v", result)
	}
}

func TestAnthropicMessagesProviderUsesCopilotAuthorizationHeaders(t *testing.T) {
	var authHeader string
	var apiKeyHeader string
	var initiator string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		apiKeyHeader = r.Header.Get("x-api-key")
		initiator = r.Header.Get("X-Initiator")
		w.Header().Set("content-type", "text/event-stream")
		writeNamedSSE(t, w, "message_stop", `{"type":"message_stop"}`)
	}))
	defer server.Close()

	model := MustGetModel("github-copilot", "claude-sonnet-4.6")
	model.BaseURL = server.URL
	stream, err := NewAnthropicMessagesProvider(server.Client()).StreamSimple(model, Context{Messages: []Message{UserMessageText("hi")}}, SimpleStreamOptions{APIKey: "copilot-token"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Result(context.Background()); err != nil {
		t.Fatal(err)
	}
	if authHeader != "Bearer copilot-token" || apiKeyHeader != "" || initiator != "user" {
		t.Fatalf("headers auth=%q apiKey=%q initiator=%q", authHeader, apiKeyHeader, initiator)
	}
}

func writeNamedSSE(t *testing.T, w http.ResponseWriter, eventName, data string) {
	t.Helper()
	if eventName != "" {
		if _, err := w.Write([]byte("event: " + eventName + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := w.Write([]byte("data: " + data + "\n\n")); err != nil {
		t.Fatal(err)
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}
