package gillmprovider

import (
	"context"
	"encoding/json"
	"errors"
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

func TestOpenAIResponsesProviderHooksPayloadAndResponseStatus(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("content-type", "text/event-stream")
		w.Header().Set("x-hook", "seen")
		w.WriteHeader(http.StatusCreated)
		writeSSE(t, w, `{"type":"response.created","response":{"id":"resp_hook","status":"in_progress"}}`)
		writeSSE(t, w, `{"type":"response.output_item.added","item":{"type":"message","id":"msg_1","status":"in_progress"}}`)
		writeSSE(t, w, `{"type":"response.content_part.added","part":{"type":"output_text","text":""}}`)
		writeSSE(t, w, `{"type":"response.output_text.delta","delta":"Hooked"}`)
		writeSSE(t, w, `{"type":"response.output_item.done","item":{"type":"message","id":"msg_1","status":"completed","content":[{"type":"output_text","text":"Hooked"}]}}`)
		writeSSE(t, w, `{"type":"response.completed","response":{"id":"resp_hook","status":"completed","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`)
	}))
	defer server.Close()

	model := Model{ID: "gpt-test", Provider: "openai", API: "openai-responses", BaseURL: server.URL + "/v1", Input: []string{"text"}}
	payloadHookCalled := false
	statusHookCalled := false
	stream, err := NewOpenAIResponsesProvider(server.Client()).StreamSimple(model, Context{Messages: []Message{UserMessageText("hi")}}, SimpleStreamOptions{
		APIKey: "test-key",
		OnPayload: func(payload any, hookModel Model) (any, bool, error) {
			payloadHookCalled = true
			if hookModel.ID != model.ID {
				t.Fatalf("payload hook model = %#v, want %#v", hookModel, model)
			}
			original, ok := payload.(OpenAIResponsesPayload)
			if !ok || original.Model != "gpt-test" {
				t.Fatalf("payload hook input = %#v", payload)
			}
			return map[string]any{
				"model":  "hooked-model",
				"stream": true,
				"store":  false,
				"input":  []any{"hooked input"},
			}, true, nil
		},
		OnResponseStatus: func(status int, headers map[string]string, hookModel Model) error {
			statusHookCalled = true
			if hookModel.ID != model.ID {
				t.Fatalf("status hook model = %#v, want %#v", hookModel, model)
			}
			if status != http.StatusCreated || headers["X-Hook"] != "seen" {
				t.Fatalf("status hook status/headers = %d %#v", status, headers)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if !payloadHookCalled || !statusHookCalled {
		t.Fatalf("hooks called payload=%v status=%v", payloadHookCalled, statusHookCalled)
	}
	if requestBody["model"] != "hooked-model" || requestBody["stream"] != true || requestBody["store"] != false {
		t.Fatalf("request body = %#v", requestBody)
	}
	if result.ResponseID != "resp_hook" || result.Content[0].Text != "Hooked" {
		t.Fatalf("result = %#v", result)
	}
}

func TestOpenAIResponsesProviderHookErrorsBecomeAssistantErrors(t *testing.T) {
	t.Run("payload", func(t *testing.T) {
		hookErr := errors.New("payload hook failed")
		model := Model{ID: "gpt-test", Provider: "openai", API: "openai-responses", Input: []string{"text"}}
		stream, err := NewOpenAIResponsesProvider(nil).StreamSimple(model, Context{Messages: []Message{UserMessageText("hi")}}, SimpleStreamOptions{
			APIKey: "test-key",
			OnPayload: func(any, Model) (any, bool, error) {
				return nil, false, hookErr
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := stream.Result(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if result.StopReason != StopReasonError || !strings.Contains(result.ErrorMessage, hookErr.Error()) {
			t.Fatalf("result = %#v", result)
		}
	})

	t.Run("response status", func(t *testing.T) {
		hookErr := errors.New("response hook failed")
		requests := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			w.Header().Set("content-type", "text/event-stream")
			writeSSE(t, w, `{"type":"response.created","response":{"id":"resp_hook_error","status":"in_progress"}}`)
		}))
		defer server.Close()

		model := Model{ID: "gpt-test", Provider: "openai", API: "openai-responses", BaseURL: server.URL, Input: []string{"text"}}
		stream, err := NewOpenAIResponsesProvider(server.Client()).StreamSimple(model, Context{Messages: []Message{UserMessageText("hi")}}, SimpleStreamOptions{
			APIKey: "test-key",
			OnResponseStatus: func(int, map[string]string, Model) error {
				return hookErr
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := stream.Result(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if requests != 1 {
			t.Fatalf("requests = %d, want 1", requests)
		}
		if result.StopReason != StopReasonError || !strings.Contains(result.ErrorMessage, hookErr.Error()) {
			t.Fatalf("result = %#v", result)
		}
	})
}

func TestOpenAIResponsesProviderClampsUnsupportedReasoning(t *testing.T) {
	var sawRequest bool
	var payload OpenAIResponsesPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawRequest = true
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("content-type", "text/event-stream")
		writeSSE(t, w, `{"type":"response.created","response":{"id":"resp_clamp","status":"in_progress"}}`)
		writeSSE(t, w, `{"type":"response.output_item.added","item":{"type":"message","id":"msg_1","status":"in_progress"}}`)
		writeSSE(t, w, `{"type":"response.content_part.added","part":{"type":"output_text","text":""}}`)
		writeSSE(t, w, `{"type":"response.output_text.delta","delta":"OK"}`)
		writeSSE(t, w, `{"type":"response.output_item.done","item":{"type":"message","id":"msg_1","status":"completed","content":[{"type":"output_text","text":"OK"}]}}`)
		writeSSE(t, w, `{"type":"response.completed","response":{"id":"resp_clamp","status":"completed","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`)
	}))
	defer server.Close()

	model := Model{ID: "gpt-4o-mini", Provider: "openai", API: "openai-responses", BaseURL: server.URL + "/v1", Reasoning: false, Input: []string{"text"}}
	stream, err := NewOpenAIResponsesProvider(server.Client()).StreamSimple(model, Context{Messages: []Message{UserMessageText("hi")}}, SimpleStreamOptions{
		APIKey:    "test-key",
		Reasoning: "medium",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if !sawRequest {
		t.Fatal("provider returned before sending request")
	}
	if payload.Reasoning != nil {
		t.Fatalf("reasoning = %#v, want omitted for non-reasoning model", payload.Reasoning)
	}
	if result.StopReason == StopReasonError || result.Content[0].Text != "OK" {
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
