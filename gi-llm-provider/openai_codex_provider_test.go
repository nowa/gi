package gillmprovider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestOpenAICodexResponsesProviderStreamsFromHTTP(t *testing.T) {
	token := mockOpenAICodexToken(t, "acc_test")
	var requestPath string
	var authHeader string
	var accountHeader string
	var sessionHeader string
	var payload OpenAICodexResponsesPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		authHeader = r.Header.Get("Authorization")
		accountHeader = r.Header.Get("chatgpt-account-id")
		sessionHeader = r.Header.Get("session-id")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		if r.Header.Get("content-encoding") == "zstd" {
			body, err = decodeOpenAICodexZstd(body)
			if err != nil {
				t.Errorf("decode zstd request: %v", err)
			}
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("content-type", "text/event-stream")
		writeSSE(t, w, `{"type":"response.created","response":{"id":"resp_live","status":"in_progress"}}`)
		writeSSE(t, w, `{"type":"response.output_item.added","item":{"type":"message","id":"msg_1","status":"in_progress"}}`)
		writeSSE(t, w, `{"type":"response.content_part.added","part":{"type":"output_text","text":""}}`)
		writeSSE(t, w, `{"type":"response.output_text.delta","delta":"Hello"}`)
		writeSSE(t, w, `{"type":"response.output_item.done","item":{"type":"message","id":"msg_1","status":"completed","content":[{"type":"output_text","text":"Hello"}]}}`)
		writeSSE(t, w, `{"type":"response.completed","response":{"id":"resp_live","status":"completed","service_tier":"default","usage":{"input_tokens":1000000,"output_tokens":1000000,"total_tokens":2000000,"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":0}}}}`)
	}))
	defer server.Close()

	model := Model{
		ID:               "gpt-5.5",
		Provider:         "openai-codex",
		API:              "openai-codex-responses",
		BaseURL:          server.URL + "/backend-api",
		Reasoning:        true,
		Input:            []string{"text"},
		Cost:             ModelCost{Input: 5, Output: 30},
		ThinkingLevelMap: map[string]*string{"xhigh": ptrString("xhigh")},
	}
	llmContext := Context{
		SystemPrompt: "You are a helpful assistant.",
		Messages:     []Message{UserMessageText("hi")},
		Tools: []Tool{{
			Name:        "lookup",
			Description: "Lookup",
			Parameters:  Object(map[string]Schema{"query": String()}, "query"),
		}},
	}
	provider := NewOpenAICodexResponsesProvider(server.Client())
	stream, err := provider.StreamSimple(model, llmContext, SimpleStreamOptions{
		APIKey:    token,
		SessionID: "session-123",
		Reasoning: "xhigh",
		Transport: "sse",
		Metadata:  map[string]any{"service_tier": "priority"},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := collectAssistantStreamEvents(stream)
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if requestPath != "/backend-api/codex/responses" || authHeader != "Bearer "+token || accountHeader != "acc_test" || sessionHeader != "session-123" {
		t.Fatalf("request path/auth/account/session = %q %q %q %q", requestPath, authHeader, accountHeader, sessionHeader)
	}
	if payload.Model != "gpt-5.5" || !payload.Stream || payload.Store || payload.PromptCacheKey != "session-123" {
		t.Fatalf("payload metadata = %#v", payload)
	}
	if !reflect.DeepEqual(payload.Reasoning, map[string]string{"effort": "xhigh", "summary": "auto"}) || payload.ServiceTier != "priority" {
		t.Fatalf("payload options = %#v", payload)
	}
	if len(payload.Tools) != 1 || payload.Tools[0].Name != "lookup" || payload.Tools[0].Strict != nil {
		t.Fatalf("tools = %#v", payload.Tools)
	}
	if !containsAssistantEvent(events, "start") || !containsAssistantEvent(events, "text_delta") || !containsAssistantEvent(events, "done") {
		t.Fatalf("events = %#v", events)
	}
	if result.ResponseID != "resp_live" || result.Content[0].Text != "Hello" || result.Usage.Cost.Input != 12.5 || result.Usage.Cost.Output != 75 || result.Usage.Cost.Total != 87.5 {
		t.Fatalf("result = %#v", result)
	}
}

func TestOpenAICodexResponsesProviderRetriesRetryableHTTPStatus(t *testing.T) {
	token := mockOpenAICodexToken(t, "acc_test")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.Header().Set("retry-after-ms", "25")
			http.Error(w, `{"error":{"message":"rate limited"}}`, http.StatusTooManyRequests)
			return
		}
		w.Header().Set("content-type", "text/event-stream")
		writeSSE(t, w, `{"type":"response.output_item.added","item":{"type":"message","id":"msg_1","status":"in_progress"}}`)
		writeSSE(t, w, `{"type":"response.content_part.added","part":{"type":"output_text","text":""}}`)
		writeSSE(t, w, `{"type":"response.output_text.delta","delta":"Hello"}`)
		writeSSE(t, w, `{"type":"response.output_item.done","item":{"type":"message","id":"msg_1","status":"completed","content":[{"type":"output_text","text":"Hello"}]}}`)
		writeSSE(t, w, `{"type":"response.completed","response":{"id":"resp_retry","status":"completed"}}`)
	}))
	defer server.Close()

	var delays []time.Duration
	provider := NewOpenAICodexResponsesProvider(server.Client())
	provider.Now = func() time.Time { return time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC) }
	provider.Sleep = func(ctx context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return ctx.Err()
	}
	model := Model{ID: "gpt-5.1-codex", Provider: "openai-codex", API: "openai-codex-responses", BaseURL: server.URL, Input: []string{"text"}}
	stream, err := provider.StreamSimple(model, Context{Messages: []Message{UserMessageText("hi")}}, SimpleStreamOptions{
		APIKey:     token,
		Transport:  "sse",
		MaxRetries: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if requests != 2 || len(delays) != 1 || delays[0] != 25*time.Millisecond {
		t.Fatalf("requests=%d delays=%v", requests, delays)
	}
	if result.ResponseID != "resp_retry" || result.Content[0].Text != "Hello" {
		t.Fatalf("result = %#v", result)
	}
}

func TestOpenAICodexResponsesProviderCompletesWhenSSEBodyStaysOpen(t *testing.T) {
	token := mockOpenAICodexToken(t, "acc_test")
	handlerDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerDone)
		w.Header().Set("content-type", "text/event-stream")
		writeSSE(t, w, `{"type":"response.output_item.added","item":{"type":"message","id":"msg_1","status":"in_progress"}}`)
		writeSSE(t, w, `{"type":"response.content_part.added","part":{"type":"output_text","text":""}}`)
		writeSSE(t, w, `{"type":"response.output_text.delta","delta":"Hello"}`)
		writeSSE(t, w, `{"type":"response.output_item.done","item":{"type":"message","id":"msg_1","status":"completed","content":[{"type":"output_text","text":"Hello"}]}}`)
		writeSSE(t, w, `{"type":"response.completed","response":{"id":"resp_open","status":"completed"}}`)
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer server.Close()

	model := Model{ID: "gpt-5.1-codex", Provider: "openai-codex", API: "openai-codex-responses", BaseURL: server.URL, Input: []string{"text"}}
	stream, err := NewOpenAICodexResponsesProvider(server.Client()).StreamSimple(model, Context{Messages: []Message{UserMessageText("hi")}}, SimpleStreamOptions{
		APIKey:    token,
		Transport: "sse",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := stream.Result(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.ResponseID != "resp_open" || result.StopReason != StopReasonStop || result.Content[0].Text != "Hello" {
		t.Fatalf("result = %#v", result)
	}
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("provider did not close the SSE body after terminal Codex event")
	}
}

func TestOpenAICodexResponsesProviderRejectsSSEWithoutTerminalEvent(t *testing.T) {
	t.Parallel()

	model := Model{
		ID:       "gpt-5.1-codex",
		Provider: "openai-codex",
		API:      "openai-codex-responses",
		Input:    []string{"text"},
	}
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_partial","status":"in_progress"}}`,
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_1","status":"in_progress"}}`,
		`data: {"type":"response.output_text.delta","output_index":0,"delta":"partial"}`,
		"",
	}, "\n\n")
	stream := NewAssistantMessageEventStream()
	streamOpenAICodexResponsesBody(
		model,
		io.NopCloser(strings.NewReader(body)),
		stream,
		"",
		nil,
		nil,
	)
	events := collectAssistantStreamEvents(stream)
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != StopReasonError ||
		result.ErrorMessage != "OpenAI Responses stream ended before a terminal response event" {
		t.Fatalf("result = %#v", result)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "partial" {
		t.Fatalf("partial content was not preserved: %#v", result.Content)
	}
	if len(events) == 0 || events[len(events)-1].Type != "error" ||
		containsAssistantEvent(events, "done") {
		t.Fatalf("events = %#v", events)
	}
}

func TestOpenAICodexResponsesProviderMapsSSETerminalErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{
			name:    "api error",
			payload: `{"type":"error","code":"invalid_request","message":"bad request"}`,
			want:    "Codex error: bad request",
		},
		{
			name:    "response failed",
			payload: `{"type":"response.failed","response":{"error":{"code":"server_error","message":"backend failed"}}}`,
			want:    "backend failed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := Model{
				ID:       "gpt-5.1-codex",
				Provider: "openai-codex",
				API:      "openai-codex-responses",
				Input:    []string{"text"},
			}
			stream := NewAssistantMessageEventStream()
			streamOpenAICodexResponsesBody(
				model,
				io.NopCloser(strings.NewReader("data: "+test.payload+"\n\n")),
				stream,
				"",
				nil,
				nil,
			)
			events := collectAssistantStreamEvents(stream)
			result, err := stream.Result(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if result.StopReason != StopReasonError || result.ErrorMessage != test.want {
				t.Fatalf("result = %#v, want error %q", result, test.want)
			}
			if len(events) != 2 || events[0].Type != "start" ||
				events[1].Type != "error" || events[1].Error.ErrorMessage != test.want {
				t.Fatalf("events = %#v", events)
			}
		})
	}
}

func TestOpenAICodexResponsesProviderFallsBackFromExplicitWebSocketTransport(t *testing.T) {
	token := mockOpenAICodexToken(t, "acc_test")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		writeSSE(t, w, `{"type":"response.output_item.added","item":{"type":"message","id":"msg_1","status":"in_progress"}}`)
		writeSSE(t, w, `{"type":"response.content_part.added","part":{"type":"output_text","text":""}}`)
		writeSSE(t, w, `{"type":"response.output_text.delta","delta":"Hello"}`)
		writeSSE(t, w, `{"type":"response.output_item.done","item":{"type":"message","id":"msg_1","status":"completed","content":[{"type":"output_text","text":"Hello"}]}}`)
		writeSSE(t, w, `{"type":"response.completed","response":{"id":"resp_fallback","status":"completed"}}`)
	}))
	defer server.Close()

	model := Model{ID: "gpt-5.1-codex", Provider: "openai-codex", API: "openai-codex-responses", BaseURL: server.URL, Input: []string{"text"}}
	stream, err := NewOpenAICodexResponsesProvider(server.Client()).StreamSimple(model, Context{Messages: []Message{UserMessageText("hi")}}, SimpleStreamOptions{
		APIKey:    token,
		Transport: "websocket",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != StopReasonStop || result.Content[0].Text != "Hello" {
		t.Fatalf("result = %#v", result)
	}
	if len(result.Diagnostics) != 1 ||
		result.Diagnostics[0].Type != "provider_transport_failure" ||
		result.Diagnostics[0].Details["configuredTransport"] != "websocket" ||
		result.Diagnostics[0].Details["fallbackTransport"] != "sse" {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}
