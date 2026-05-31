package giagentcore

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestStreamProxyReconstructsPiProxyEvents(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/stream" {
			t.Fatalf("path = %q, want /api/stream", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		writeProxySSE(t, w, ProxyAssistantMessageEvent{Type: "start"})
		writeProxySSE(t, w, ProxyAssistantMessageEvent{Type: "text_start", ContentIndex: 0})
		writeProxySSE(t, w, ProxyAssistantMessageEvent{Type: "text_delta", ContentIndex: 0, Delta: "hello"})
		writeProxySSE(t, w, ProxyAssistantMessageEvent{Type: "text_end", ContentIndex: 0, ContentSignature: "text-sig"})
		writeProxySSE(t, w, ProxyAssistantMessageEvent{Type: "thinking_start", ContentIndex: 1})
		writeProxySSE(t, w, ProxyAssistantMessageEvent{Type: "thinking_delta", ContentIndex: 1, Delta: "think"})
		writeProxySSE(t, w, ProxyAssistantMessageEvent{Type: "thinking_end", ContentIndex: 1, ContentSignature: "thinking-sig"})
		writeProxySSE(t, w, ProxyAssistantMessageEvent{Type: "toolcall_start", ContentIndex: 2, ID: "call_1", ToolName: "read"})
		writeProxySSE(t, w, ProxyAssistantMessageEvent{Type: "toolcall_delta", ContentIndex: 2, Delta: `{"path":"README`})
		writeProxySSE(t, w, ProxyAssistantMessageEvent{Type: "toolcall_delta", ContentIndex: 2, Delta: `.md"}`})
		writeProxySSE(t, w, ProxyAssistantMessageEvent{Type: "toolcall_end", ContentIndex: 2})
		writeProxySSE(t, w, ProxyAssistantMessageEvent{Type: "done", Reason: "toolUse", Usage: llm.Usage{Input: 10, Output: 3, TotalTokens: 13}})
	}))
	defer server.Close()

	model := llm.Model{ID: "proxy-model", Provider: "proxy-provider", API: "openai-responses"}
	stream := StreamProxy(model, llm.Context{Messages: []llm.Message{llm.UserMessageText("hi")}}, ProxyStreamOptions{
		SimpleStreamOptions: llm.SimpleStreamOptions{
			Reasoning:       "medium",
			MaxTokens:       123,
			APIKey:          "must-not-be-sent",
			SessionID:       "session-1",
			ThinkingBudgets: map[string]int{"medium": 1024},
			Metadata:        map[string]any{"trace": "abc"},
		},
		AuthToken:  "token",
		ProxyURL:   server.URL,
		HTTPClient: server.Client(),
	})

	events := collectProxyEvents(t, stream)
	types := make([]string, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	wantTypes := []string{
		"start",
		"text_start", "text_delta", "text_end",
		"thinking_start", "thinking_delta", "thinking_end",
		"toolcall_start", "toolcall_delta", "toolcall_delta", "toolcall_end",
		"done",
	}
	if !reflect.DeepEqual(types, wantTypes) {
		t.Fatalf("event types = %#v, want %#v", types, wantTypes)
	}

	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatalf("result: %v", err)
	}
	if result.StopReason != "toolUse" || result.API != model.API || result.Provider != model.Provider || result.Model != model.ID {
		t.Fatalf("result metadata = %#v", result)
	}
	if len(result.Content) != 3 {
		t.Fatalf("content = %#v", result.Content)
	}
	if result.Content[0].Text != "hello" || result.Content[0].TextSignature != "text-sig" {
		t.Fatalf("text content = %#v", result.Content[0])
	}
	if result.Content[1].Thinking != "think" || result.Content[1].ThinkingSignature != "thinking-sig" {
		t.Fatalf("thinking content = %#v", result.Content[1])
	}
	if result.Content[2].Name != "read" || result.Content[2].ID != "call_1" || result.Content[2].Arguments["path"] != "README.md" {
		t.Fatalf("tool content = %#v", result.Content[2])
	}

	options := requestBody["options"].(map[string]any)
	if _, ok := options["apiKey"]; ok {
		t.Fatalf("proxy request options leaked apiKey: %#v", options)
	}
	if options["reasoning"] != "medium" || options["sessionId"] != "session-1" {
		t.Fatalf("proxy options = %#v", options)
	}
}

func TestStreamProxyEmitsPiStyleErrorMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"not authorized"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	model := llm.Model{ID: "proxy-model", Provider: "proxy-provider", API: "openai-responses"}
	stream := StreamProxy(model, llm.Context{}, ProxyStreamOptions{
		AuthToken:  "bad-token",
		ProxyURL:   server.URL,
		HTTPClient: server.Client(),
	})
	events := collectProxyEvents(t, stream)
	if len(events) != 1 || events[0].Type != "error" {
		t.Fatalf("events = %#v", events)
	}
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatalf("result: %v", err)
	}
	if result.StopReason != llm.StopReasonError || !strings.Contains(result.ErrorMessage, "Proxy error") {
		t.Fatalf("result = %#v", result)
	}
}

func writeProxySSE(t *testing.T, w http.ResponseWriter, event ProxyAssistantMessageEvent) {
	t.Helper()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	if _, err := w.Write([]byte("data: " + string(data) + "\n\n")); err != nil {
		t.Fatalf("write event: %v", err)
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func collectProxyEvents(t *testing.T, stream *llm.AssistantMessageEventStream) []llm.AssistantMessageEvent {
	t.Helper()
	var events []llm.AssistantMessageEvent
	deadline := time.After(5 * time.Second)
	for {
		select {
		case event, ok := <-stream.Events():
			if !ok {
				return events
			}
			events = append(events, event)
		case <-deadline:
			t.Fatal("timed out waiting for proxy events")
		}
	}
}
