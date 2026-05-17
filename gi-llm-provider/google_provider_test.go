package gillmprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGoogleProviderStreamsFromHTTP(t *testing.T) {
	var requestPath string
	var rawQuery string
	var payload GooglePayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		rawQuery = r.URL.RawQuery
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("content-type", "text/event-stream")
		writeSSE(t, w, `{"responseId":"google-live","candidates":[{"content":{"role":"model","parts":[{"text":"Hello"}]}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":1,"cachedContentTokenCount":1,"totalTokenCount":6}}`)
		writeSSE(t, w, `{"responseId":"google-live","candidates":[{"content":{"role":"model","parts":[{"text":" Gemini"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2,"cachedContentTokenCount":1,"totalTokenCount":7}}`)
	}))
	defer server.Close()

	model := MustGetModel("google", "gemini-2.5-flash")
	model.BaseURL = server.URL + "/v1beta"
	stream, err := NewGoogleProvider(server.Client()).StreamSimple(model, Context{
		SystemPrompt: "Be concise.",
		Messages:     []Message{UserMessageText("hi")},
	}, SimpleStreamOptions{APIKey: "google-key", MaxTokens: 128})
	if err != nil {
		t.Fatal(err)
	}
	events := collectAssistantStreamEvents(stream)
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if requestPath != "/v1beta/models/gemini-2.5-flash:streamGenerateContent" || !strings.Contains(rawQuery, "alt=sse") || !strings.Contains(rawQuery, "key=google-key") {
		t.Fatalf("request path/query = %q %q", requestPath, rawQuery)
	}
	if len(payload.Contents) != 1 || payload.Config.MaxOutputTokens != 128 || payload.Config.SystemInstruction == nil {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.Config.ThinkingConfig == nil || payload.Config.ThinkingConfig.ThinkingBudget == nil || *payload.Config.ThinkingConfig.ThinkingBudget != 0 {
		t.Fatalf("thinking config = %#v", payload.Config.ThinkingConfig)
	}
	if !containsAssistantEvent(events, "start") || !containsAssistantEvent(events, "text_delta") || !containsAssistantEvent(events, "done") {
		t.Fatalf("events = %#v", events)
	}
	if result.ResponseID != "google-live" || result.Content[0].Text != "Hello Gemini" || result.Usage.Input != 4 || result.Usage.CacheRead != 1 || result.Usage.Output != 2 || result.Usage.TotalTokens != 7 {
		t.Fatalf("result = %#v", result)
	}
}

func TestGoogleProviderStreamsThinkingAndToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		writeSSE(t, w, `{"responseId":"google-tool","candidates":[{"content":{"role":"model","parts":[{"text":"reason","thought":true,"thoughtSignature":"AAAA"},{"functionCall":{"name":"lookup","args":{"query":"go"},"id":"call_1"},"thoughtSignature":"BBBB"}]},"finishReason":"STOP"}]}`)
	}))
	defer server.Close()

	model := Model{ID: "gemini-3-pro-preview", Provider: "google", API: "google-generative-ai", BaseURL: server.URL, Reasoning: true, Input: []string{"text"}}
	stream, err := NewGoogleProvider(server.Client()).StreamSimple(model, Context{
		Messages: []Message{UserMessageText("lookup")},
		Tools:    []Tool{{Name: "lookup", Parameters: Object(map[string]Schema{"query": String()}, "query")}},
	}, SimpleStreamOptions{APIKey: "google-key", Reasoning: "medium"})
	if err != nil {
		t.Fatal(err)
	}
	events := collectAssistantStreamEvents(stream)
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !containsAssistantEvent(events, "thinking_delta") || !containsAssistantEvent(events, "toolcall_end") || result.StopReason != StopReasonToolUse {
		t.Fatalf("events=%#v result=%#v", events, result)
	}
	if len(result.Content) != 2 || result.Content[0].Thinking != "reason" || result.Content[0].ThinkingSignature != "AAAA" {
		t.Fatalf("thinking = %#v", result.Content)
	}
	if result.Content[1].ID != "call_1" || result.Content[1].ThoughtSignature != "BBBB" || result.Content[1].Arguments["query"] != "go" {
		t.Fatalf("tool call = %#v", result.Content[1])
	}
}

func TestGoogleProviderHandlesHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	model := MustGetModel("google", "gemini-2.5-flash")
	model.BaseURL = server.URL
	stream, err := NewGoogleProvider(server.Client()).StreamSimple(model, Context{Messages: []Message{UserMessageText("hi")}}, SimpleStreamOptions{APIKey: "google-key"})
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

func TestGooglePayloadIncludesToolsAndEnabledThinking(t *testing.T) {
	model := Model{ID: "gemini-3-flash-preview", Provider: "google", API: "google-generative-ai", Reasoning: true, Input: []string{"text"}}
	payload := BuildGooglePayload(model, Context{
		Messages: []Message{UserMessageText("hi")},
		Tools:    []Tool{{Name: "lookup", Parameters: Object(map[string]Schema{"query": String()}, "query")}},
	}, GooglePayloadOptions{Reasoning: "medium"})
	if len(payload.Tools) != 1 || payload.Config.ThinkingConfig == nil || payload.Config.ThinkingConfig.IncludeThoughts == nil || !*payload.Config.ThinkingConfig.IncludeThoughts || payload.Config.ThinkingConfig.ThinkingLevel != "MEDIUM" {
		t.Fatalf("payload = %#v", payload)
	}
}
