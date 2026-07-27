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
	if len(payload.Contents) != 1 || payload.Config.MaxOutputTokens != 128 || payload.SystemInstruction == nil {
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
	if result.StopReason != StopReasonError || result.ErrorMessage != "400: bad request" {
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

func TestBuildGooglePayloadResolvesStrictToolSampling(t *testing.T) {
	strictTool := Tool{
		Name:       "lookup",
		Parameters: Object(map[string]Schema{"query": String()}, "query"),
		ConstrainedSampling: &ConstrainedSamplingConfig{
			Type:   ConstrainedSamplingJSONSchema,
			Strict: ConstrainedSamplingRequire,
		},
	}

	payload, err := BuildGooglePayloadChecked(
		Model{ID: "gemini-3.1-pro-preview", Provider: "google", API: "google-generative-ai"},
		Context{Tools: []Tool{strictTool}},
		GooglePayloadOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if payload.ToolConfig == nil ||
		payload.ToolConfig.FunctionCallingConfig.Mode != GoogleFunctionCallingValidated {
		t.Fatalf("tool config = %#v", payload.ToolConfig)
	}

	_, err = BuildGooglePayloadChecked(
		Model{ID: "gemini-2.5-pro", Provider: "google", API: "google-generative-ai"},
		Context{Tools: []Tool{strictTool}},
		GooglePayloadOptions{},
	)
	if err == nil || !strings.Contains(err.Error(), `tool "lookup" requires JSON-schema constrained sampling`) {
		t.Fatalf("error = %v", err)
	}
}

func TestGoogleProviderRejectsUnsupportedStrictSamplingBeforeRequest(t *testing.T) {
	called := false
	provider := NewGoogleProvider(simpleOptionsHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, nil
	}))
	model := Model{
		ID:       "gemini-2.5-pro",
		Provider: "google",
		API:      "google-generative-ai",
	}
	stream, err := provider.Stream(model, Context{
		Tools: []Tool{{
			Name: "lookup",
			ConstrainedSampling: &ConstrainedSamplingConfig{
				Type:   ConstrainedSamplingJSONSchema,
				Strict: ConstrainedSamplingRequire,
			},
		}},
	}, StreamOptions{APIKey: "google-key"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("HTTP client was called for an invalid strict-sampling request")
	}
	if result.StopReason != StopReasonError ||
		!strings.Contains(result.ErrorMessage, `tool "lookup" requires JSON-schema constrained sampling`) {
		t.Fatalf("result = %#v", result)
	}
}

func TestGooglePayloadSerializesRequestSectionsAtTopLevel(t *testing.T) {
	payload, err := BuildGooglePayloadChecked(
		Model{ID: "gemini-3-flash-preview", Provider: "google", API: "google-generative-ai"},
		Context{
			SystemPrompt: "Be concise.",
			Tools:        []Tool{{Name: "lookup", Parameters: Object(nil)}},
		},
		GooglePayloadOptions{ToolChoice: "any"},
	)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["systemInstruction"] == nil || wire["toolConfig"] == nil {
		t.Fatalf("payload = %s", encoded)
	}
	generationConfig, ok := wire["generationConfig"].(map[string]any)
	if !ok {
		t.Fatalf("generation config = %#v", wire["generationConfig"])
	}
	if generationConfig["systemInstruction"] != nil || generationConfig["toolConfig"] != nil {
		t.Fatalf("nested generation config = %#v", generationConfig)
	}
}
