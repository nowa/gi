package gillmprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAICompletionsProviderStreamsFromHTTP(t *testing.T) {
	var requestPath string
	var authHeader string
	var payload OpenAICompletionsPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		authHeader = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("content-type", "text/event-stream")
		writeSSE(t, w, `{"id":"chatcmpl-live","choices":[{"delta":{"content":"Hello"}}]}`)
		writeSSE(t, w, `{"id":"chatcmpl-live","choices":[{"delta":{"content":" world"},"finish_reason":"stop"}],"usage":{"prompt_tokens":6,"completion_tokens":2,"prompt_tokens_details":{"cached_tokens":1,"cache_write_tokens":0}}}`)
	}))
	defer server.Close()

	model := Model{ID: "gpt-4o-mini", Provider: "openai", API: "openai-completions", BaseURL: server.URL + "/v1", Input: []string{"text"}}
	stream, err := NewOpenAICompletionsProvider(server.Client()).StreamSimple(model, Context{Messages: []Message{UserMessageText("hi")}}, SimpleStreamOptions{APIKey: "openai-key"})
	if err != nil {
		t.Fatal(err)
	}
	events := collectAssistantStreamEvents(stream)
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if requestPath != "/v1/chat/completions" || authHeader != "Bearer openai-key" {
		t.Fatalf("request path/auth = %q %q", requestPath, authHeader)
	}
	if payload.Model != "gpt-4o-mini" || !payload.Stream || len(payload.Messages) != 1 {
		t.Fatalf("payload = %#v", payload)
	}
	if !containsAssistantEvent(events, "start") || !containsAssistantEvent(events, "text_delta") || !containsAssistantEvent(events, "done") {
		t.Fatalf("events = %#v", events)
	}
	if result.ResponseID != "chatcmpl-live" || result.Content[0].Text != "Hello world" || result.Usage.Input != 5 || result.Usage.CacheRead != 1 || result.Usage.Output != 2 {
		t.Fatalf("result = %#v", result)
	}
}

func TestOpenAICompletionsProviderStreamsToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		writeSSE(t, w, `{"id":"chatcmpl-tool","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"query\""}}]}}]}`)
		writeSSE(t, w, `{"id":"chatcmpl-tool","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"go\"}"}}]},"finish_reason":"tool_calls"}]}`)
	}))
	defer server.Close()

	model := Model{ID: "gpt-4o-mini", Provider: "openai", API: "openai-completions", BaseURL: server.URL, Input: []string{"text"}}
	stream, err := NewOpenAICompletionsProvider(server.Client()).StreamSimple(model, Context{
		Messages: []Message{UserMessageText("lookup")},
		Tools: []Tool{{
			Name:       "lookup",
			Parameters: Object(map[string]Schema{"query": String()}, "query"),
		}},
	}, SimpleStreamOptions{APIKey: "openai-key"})
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
	if len(result.Content) != 1 || result.Content[0].Name != "lookup" || result.Content[0].Arguments["query"] != "go" {
		t.Fatalf("tool call = %#v", result.Content)
	}
}

func TestOpenAICompletionsProviderHandlesHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	model := Model{ID: "gpt-4o-mini", Provider: "openai", API: "openai-completions", BaseURL: server.URL, Input: []string{"text"}}
	stream, err := NewOpenAICompletionsProvider(server.Client()).StreamSimple(model, Context{Messages: []Message{UserMessageText("hi")}}, SimpleStreamOptions{APIKey: "openai-key"})
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

func TestDecodeOpenAIChatCompletionChunk(t *testing.T) {
	chunk, err := DecodeOpenAIChatCompletionChunk([]byte(`{"id":"chatcmpl","choices":[{"delta":{"reasoning_content":"think","tool_calls":[{"index":0,"id":"call_1","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":4,"prompt_tokens_details":{"cached_tokens":1}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if chunk.ID != "chatcmpl" || len(chunk.Choices) != 1 || chunk.Choices[0].Delta.ReasoningContent != "think" || chunk.Choices[0].FinishReason == nil {
		t.Fatalf("chunk = %#v", chunk)
	}
	if chunk.Usage == nil || chunk.Usage.PromptTokens != 3 || chunk.Usage.PromptTokensDetails.CachedTokens != 1 {
		t.Fatalf("usage = %#v", chunk.Usage)
	}
}
