package gillmprovider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnthropicMessagesProviderStreamsFromHTTP(t *testing.T) {
	var requestPath string
	var apiKeyHeader string
	var versionHeader string
	var extraHeader string
	var payload AnthropicPayload
	var rawPayload map[string]any
	var payloadHookCalled bool
	var statusHookCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		apiKeyHeader = r.Header.Get("x-api-key")
		versionHeader = r.Header.Get("anthropic-version")
		extraHeader = r.Header.Get("X-Test-Header")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if err := json.Unmarshal(body, &rawPayload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("X-Test-Response", "ok")
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
	stream, err := NewAnthropicMessagesProvider(server.Client()).StreamSimple(model, Context{Messages: []Message{UserMessageText("hi")}}, SimpleStreamOptions{
		APIKey:  "anthropic-key",
		Headers: map[string]string{"X-Test-Header": "extra"},
		OnPayload: func(payload any, hookModel Model) (any, bool, error) {
			payloadHookCalled = true
			next := payload.(AnthropicPayload)
			next.Metadata = map[string]any{"user_id": "hook-user"}
			return next, true, nil
		},
		OnResponseStatus: func(status int, headers map[string]string, hookModel Model) error {
			statusHookCalled = true
			if status != http.StatusOK || headers["X-Test-Response"] != "ok" {
				t.Fatalf("response hook status=%d headers=%#v", status, headers)
			}
			return nil
		},
	})
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
	if extraHeader != "extra" || !payloadHookCalled || !statusHookCalled {
		t.Fatalf("hooks/headers extra=%q payloadHook=%v statusHook=%v", extraHeader, payloadHookCalled, statusHookCalled)
	}
	if payload.Model != "claude-test" || !payload.Stream || payload.MaxTokens == 0 {
		t.Fatalf("payload = %#v", payload)
	}
	if rawPayload["model"] != "claude-test" || rawPayload["max_tokens"] == nil || rawPayload["stream"] != true {
		t.Fatalf("raw payload uses wrong JSON shape: %#v", rawPayload)
	}
	if _, ok := rawPayload["Model"]; ok {
		t.Fatalf("raw payload still contains Go field names: %#v", rawPayload)
	}
	if metadata, ok := rawPayload["metadata"].(map[string]any); !ok || metadata["user_id"] != "hook-user" {
		t.Fatalf("metadata = %#v", rawPayload["metadata"])
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

func TestAnthropicMessagesProviderUsesOAuthAuthorizationHeadersPiStyle(t *testing.T) {
	var authHeader string
	var apiKeyHeader string
	var versionHeader string
	var betaHeader string
	var userAgent string
	var appHeader string
	var payload AnthropicPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		apiKeyHeader = r.Header.Get("x-api-key")
		versionHeader = r.Header.Get("anthropic-version")
		betaHeader = r.Header.Get("anthropic-beta")
		userAgent = r.Header.Get("User-Agent")
		appHeader = r.Header.Get("x-app")
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("content-type", "text/event-stream")
		writeNamedSSE(t, w, "message_stop", `{"type":"message_stop"}`)
	}))
	defer server.Close()

	model := MustGetModel("anthropic", "claude-sonnet-4-5")
	model.BaseURL = server.URL + "/v1"
	contextValue := Context{
		SystemPrompt: "Use tools.",
		Messages:     []Message{UserMessageText("Add todo")},
		Tools: []Tool{{
			Name:        "todowrite",
			Description: "Write todo",
			Parameters:  Object(map[string]Schema{"task": String()}, "task"),
		}},
	}
	stream, err := NewAnthropicMessagesProvider(server.Client()).StreamSimple(model, contextValue, SimpleStreamOptions{
		APIKey:    "sk-ant-oat-test-token",
		Reasoning: "high",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Result(context.Background()); err != nil {
		t.Fatal(err)
	}

	if authHeader != "Bearer sk-ant-oat-test-token" || apiKeyHeader != "" || versionHeader != "2023-06-01" {
		t.Fatalf("headers auth=%q apiKey=%q version=%q", authHeader, apiKeyHeader, versionHeader)
	}
	for _, required := range []string{"claude-code-20250219", "oauth-2025-04-20", interleavedThinkingBeta} {
		if !strings.Contains(betaHeader, required) {
			t.Fatalf("anthropic-beta = %q, missing %q", betaHeader, required)
		}
	}
	if userAgent != "claude-cli/2.1.75" || appHeader != "cli" {
		t.Fatalf("oauth identity headers userAgent=%q x-app=%q", userAgent, appHeader)
	}
	if len(payload.System) != 2 || payload.System[0].Text != "You are Claude Code, Anthropic's official CLI for Claude." || payload.System[1].Text != "Use tools." {
		t.Fatalf("system = %#v", payload.System)
	}
	if len(payload.Tools) != 1 || payload.Tools[0].Name != "TodoWrite" {
		t.Fatalf("tools = %#v", payload.Tools)
	}
	if payload.Thinking["type"] != "enabled" || payload.Thinking["budget_tokens"] != float64(16384) {
		t.Fatalf("thinking = %#v", payload.Thinking)
	}
}

func TestAnthropicMessagesEndpointPiBaseURLParity(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{name: "default anthropic", want: "https://api.anthropic.com/v1/messages"},
		{name: "pi anthropic base", baseURL: "https://api.anthropic.com", want: "https://api.anthropic.com/v1/messages"},
		{name: "explicit v1", baseURL: "https://api.anthropic.com/v1", want: "https://api.anthropic.com/v1/messages"},
		{name: "already messages", baseURL: "https://proxy.example/v1/messages", want: "https://proxy.example/v1/messages"},
		{name: "provider compatible base", baseURL: "https://api.fireworks.ai/inference", want: "https://api.fireworks.ai/inference/messages"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := anthropicMessagesEndpoint(tc.baseURL); got != tc.want {
				t.Fatalf("endpoint = %q, want %q", got, tc.want)
			}
		})
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
