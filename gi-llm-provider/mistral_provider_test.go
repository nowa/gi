package gillmprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMistralProviderStreamsFromHTTP(t *testing.T) {
	var requestPath string
	var authHeader string
	var affinityHeader string
	var payload MistralPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		authHeader = r.Header.Get("Authorization")
		affinityHeader = r.Header.Get("x-affinity")
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("content-type", "text/event-stream")
		writeSSE(t, w, `{"data":{"id":"mistral-live","choices":[{"delta":{"content":"Hello"}}]}}`)
		writeSSE(t, w, `{"data":{"id":"mistral-live","choices":[{"delta":{"content":[{"type":"text","text":" Mistral"}]},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}}`)
	}))
	defer server.Close()

	model := MustGetModel("mistral", "mistral-small-2603")
	model.BaseURL = server.URL
	stream, err := NewMistralProvider(server.Client()).StreamSimple(model, Context{Messages: []Message{UserMessageText("hi")}}, SimpleStreamOptions{
		APIKey:    "mistral-key",
		SessionID: "session-mistral",
		Reasoning: "medium",
	})
	if err != nil {
		t.Fatal(err)
	}
	events := collectAssistantStreamEvents(stream)
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if requestPath != "/v1/chat/completions" || authHeader != "Bearer mistral-key" || affinityHeader != "session-mistral" {
		t.Fatalf("request path/auth/affinity = %q %q %q", requestPath, authHeader, affinityHeader)
	}
	if payload.Model != model.ID || !payload.Stream || payload.ReasoningEffort != "high" {
		t.Fatalf("payload = %#v", payload)
	}
	if !containsAssistantEvent(events, "start") || !containsAssistantEvent(events, "text_delta") || !containsAssistantEvent(events, "done") {
		t.Fatalf("events = %#v", events)
	}
	if result.ResponseID != "mistral-live" || result.Content[0].Text != "Hello Mistral" || result.Usage.Input != 5 || result.Usage.Output != 3 || result.Usage.TotalTokens != 8 {
		t.Fatalf("result = %#v", result)
	}
}

func TestMistralProviderStreamsToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		writeSSE(t, w, `{"data":{"id":"mistral-tool","choices":[{"delta":{"tool_calls":[{"index":0,"id":"abc123def","type":"function","function":{"name":"lookup","arguments":"{\"query\""}}]}}]}}`)
		writeSSE(t, w, `{"data":{"id":"mistral-tool","choices":[{"delta":{"tool_calls":[{"index":0,"id":"abc123def","type":"function","function":{"arguments":":\"go\"}"}}]},"finish_reason":"tool_calls"}]}}`)
	}))
	defer server.Close()

	model := MustGetModel("mistral", "devstral-medium-latest")
	model.BaseURL = server.URL
	stream, err := NewMistralProvider(server.Client()).StreamSimple(model, Context{
		Messages: []Message{UserMessageText("lookup")},
		Tools:    []Tool{{Name: "lookup", Parameters: Object(map[string]Schema{"query": String()}, "query")}},
	}, SimpleStreamOptions{APIKey: "mistral-key"})
	if err != nil {
		t.Fatal(err)
	}
	events := collectAssistantStreamEvents(stream)
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !containsAssistantEvent(events, "toolcall_delta") || result.StopReason != StopReasonToolUse {
		t.Fatalf("events=%#v result=%#v", events, result)
	}
	if len(result.Content) != 1 || result.Content[0].ID != "abc123def" || result.Content[0].Arguments["query"] != "go" {
		t.Fatalf("tool call = %#v", result.Content)
	}
}

func TestMistralProviderHandlesHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	model := MustGetModel("mistral", "devstral-medium-latest")
	model.BaseURL = server.URL
	stream, err := NewMistralProvider(server.Client()).StreamSimple(model, Context{Messages: []Message{UserMessageText("hi")}}, SimpleStreamOptions{APIKey: "mistral-key"})
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

func TestDecodeMistralCompletionChunk(t *testing.T) {
	chunk, err := DecodeMistralCompletionChunk([]byte(`{"data":{"id":"mistral","choices":[{"delta":{"content":[{"type":"thinking","thinking":[{"type":"text","text":"think"}]}]},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if chunk.ID != "mistral" || len(chunk.Choices) != 1 || chunk.Choices[0].FinishReason != "stop" || chunk.Usage.TotalTokens != 3 {
		t.Fatalf("chunk = %#v", chunk)
	}
}
