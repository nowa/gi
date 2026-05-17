package gillmprovider

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestBuildOpenAICodexPayloadAndHeaders(t *testing.T) {
	model := Model{
		ID:        "gpt-5.5",
		Name:      "GPT-5.5",
		API:       "openai-codex-responses",
		Provider:  "openai-codex",
		BaseURL:   "https://chatgpt.com/backend-api",
		Reasoning: true,
		Input:     []string{"text"},
		ThinkingLevelMap: map[string]*string{
			"xhigh": ptrString("xhigh"),
		},
	}
	context := Context{
		SystemPrompt: "You are a helpful assistant.",
		Messages:     []Message{UserMessageText("Say hello")},
	}

	payload := BuildOpenAICodexResponsesPayload(model, context, OpenAICodexResponsesPayloadOptions{
		SessionID:       "session-123",
		ReasoningEffort: "xhigh",
		ServiceTier:     "priority",
	})

	if payload.Model != "gpt-5.5" || !payload.Stream || payload.Store {
		t.Fatalf("payload metadata = %#v", payload)
	}
	if payload.Instructions != "You are a helpful assistant." || payload.PromptCacheKey != "session-123" {
		t.Fatalf("payload prompt fields = %#v", payload)
	}
	if !reflect.DeepEqual(payload.Reasoning, map[string]string{"effort": "xhigh", "summary": "auto"}) {
		t.Fatalf("reasoning = %#v", payload.Reasoning)
	}
	if payload.ServiceTier != "priority" || !reflect.DeepEqual(payload.Text, map[string]string{"verbosity": "low"}) {
		t.Fatalf("options = %#v", payload)
	}
	if len(payload.Input) != 1 || payload.Input[0].Role != "user" {
		t.Fatalf("input = %#v", payload.Input)
	}

	token := mockOpenAICodexToken(t, "acc_test")
	headers, err := BuildOpenAICodexSSEHeaders(map[string]string{"x-model": "yes"}, map[string]string{"x-extra": "ok"}, token, "session-123")
	if err != nil {
		t.Fatal(err)
	}
	if headers["Authorization"] != "Bearer "+token || headers["chatgpt-account-id"] != "acc_test" {
		t.Fatalf("auth headers = %#v", headers)
	}
	if headers["OpenAI-Beta"] != "responses=experimental" || headers["accept"] != "text/event-stream" || headers["originator"] != "pi" {
		t.Fatalf("protocol headers = %#v", headers)
	}
	if headers["session_id"] != "session-123" || headers["x-client-request-id"] != "session-123" {
		t.Fatalf("session headers = %#v", headers)
	}
	if _, ok := headers["x-api-key"]; ok {
		t.Fatalf("x-api-key should not be set: %#v", headers)
	}
}

func TestOpenAICodexCacheAffinityE2EContract(t *testing.T) {
	model := MustGetModel("openai-codex", "gpt-5.3-codex")
	sessionID := "0195d6e4-4cf9-7f44-a2d8-f8f7f49ee9d3"
	context := Context{
		SystemPrompt: "You are a helpful assistant. Reply exactly as requested.",
		Messages:     []Message{UserMessageText("Reply with exactly: cache affinity e2e success")},
	}

	payload := BuildOpenAICodexResponsesPayload(model, context, OpenAICodexResponsesPayloadOptions{SessionID: sessionID})
	if payload.PromptCacheKey != sessionID || len(payload.Input) != 1 || payload.Input[0].Role != "user" {
		t.Fatalf("payload = %#v", payload)
	}

	token := mockOpenAICodexToken(t, "acc_test")
	headers, err := BuildOpenAICodexSSEHeaders(model.Headers, nil, token, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if headers["session_id"] != sessionID || headers["x-client-request-id"] != sessionID || headers["accept"] != "text/event-stream" {
		t.Fatalf("headers = %#v", headers)
	}
}

func TestProcessOpenAICodexStreamEventsTextCompletedAndIncomplete(t *testing.T) {
	model := Model{ID: "gpt-5.1-codex", API: "openai-codex-responses", Provider: "openai-codex"}
	completed := AssistantMessage(nil, StopReasonStop, model)
	events := codexHelloEvents("response.completed", "completed", "resp_1")

	emitted := ProcessOpenAICodexStreamEvents(model, &completed, events, OpenAICodexProcessOptions{})

	if completed.ResponseID != "resp_1" || completed.StopReason != StopReasonStop {
		t.Fatalf("completed metadata = %#v", completed)
	}
	if len(completed.Content) != 1 || completed.Content[0].Text != "Hello" {
		t.Fatalf("completed content = %#v", completed.Content)
	}
	if completed.Usage.Input != 5 || completed.Usage.Output != 3 || completed.Usage.TotalTokens != 8 {
		t.Fatalf("completed usage = %#v", completed.Usage)
	}
	if lastEventType(emitted) != "done" {
		t.Fatalf("events = %#v", emitted)
	}

	incomplete := AssistantMessage(nil, StopReasonStop, model)
	ProcessOpenAICodexStreamEvents(model, &incomplete, codexHelloEvents("response.incomplete", "incomplete", "resp_2"), OpenAICodexProcessOptions{})
	if incomplete.StopReason != StopReasonLength || incomplete.Content[0].Text != "Hello" {
		t.Fatalf("incomplete = %#v", incomplete)
	}
}

func TestOpenAICodexServiceTierPricing(t *testing.T) {
	model := Model{
		ID:       "gpt-5.5",
		API:      "openai-codex-responses",
		Provider: "openai-codex",
		Cost:     ModelCost{Input: 5, Output: 30},
	}
	output := AssistantMessage(nil, StopReasonStop, model)
	events := codexHelloEvents("response.completed", "completed", "resp_1")
	events[len(events)-1].Response.ServiceTier = "default"
	events[len(events)-1].Response.Usage.InputTokens = 1_000_000
	events[len(events)-1].Response.Usage.OutputTokens = 1_000_000
	events[len(events)-1].Response.Usage.TotalTokens = 2_000_000

	ProcessOpenAICodexStreamEvents(model, &output, events, OpenAICodexProcessOptions{ServiceTier: "priority"})

	if output.Usage.Cost.Input != 12.5 || output.Usage.Cost.Output != 75 || output.Usage.Cost.Total != 87.5 {
		t.Fatalf("cost = %#v", output.Usage.Cost)
	}
}

func TestOpenAICodexRetryDelay(t *testing.T) {
	now := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		headers map[string]string
		attempt int
		want    time.Duration
	}{
		{"retry-after-ms", map[string]string{"retry-after-ms": "1500"}, 0, 1500 * time.Millisecond},
		{"retry-after seconds", map[string]string{"retry-after": "60"}, 0, 60 * time.Second},
		{"retry-after date", map[string]string{"retry-after": now.Add(45 * time.Second).Format(time.RFC1123)}, 0, 45 * time.Second},
		{"backoff 0", nil, 0, time.Second},
		{"backoff 2", nil, 2, 4 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := OpenAICodexRetryDelay(429, tc.headers, tc.attempt, now); got != tc.want {
				t.Fatalf("delay = %s want %s", got, tc.want)
			}
		})
	}
	if !IsOpenAICodexRetryable(429, "") || !IsOpenAICodexRetryable(400, "upstream connect error") || IsOpenAICodexRetryable(404, "not found") {
		t.Fatal("retryable classification mismatch")
	}
}

func TestOpenAICodexURLResolution(t *testing.T) {
	if got := ResolveOpenAICodexURL(""); got != "https://chatgpt.com/backend-api/codex/responses" {
		t.Fatalf("default url = %q", got)
	}
	if got := ResolveOpenAICodexURL("https://example.test/base/codex"); got != "https://example.test/base/codex/responses" {
		t.Fatalf("codex url = %q", got)
	}
	if got := ResolveOpenAICodexWebSocketURL("https://example.test/base"); got != "wss://example.test/base/codex/responses" {
		t.Fatalf("ws url = %q", got)
	}
}

func codexHelloEvents(terminalType, status, responseID string) []OpenAIResponsesStreamEvent {
	return []OpenAIResponsesStreamEvent{
		{
			Type: "response.output_item.added",
			Item: &OpenAIResponsesOutputItem{Type: "message", ID: "msg_1", Status: "in_progress"},
		},
		{
			Type: "response.content_part.added",
			Part: &OpenAIResponsesOutputContentPart{Type: "output_text"},
		},
		{Type: "response.output_text.delta", Delta: "Hello"},
		{
			Type: "response.output_item.done",
			Item: &OpenAIResponsesOutputItem{
				Type:    "message",
				ID:      "msg_1",
				Status:  "completed",
				Content: []OpenAIResponsesOutputContentPart{{Type: "output_text", Text: "Hello"}},
			},
		},
		{
			Type: terminalType,
			Response: &OpenAIResponsesResponseEvent{
				ID:     responseID,
				Status: status,
				Usage: &OpenAIResponsesUsage{
					InputTokens:  5,
					OutputTokens: 3,
					TotalTokens:  8,
				},
			},
		},
	}
}

func mockOpenAICodexToken(t *testing.T, accountID string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": accountID},
	})
	if err != nil {
		t.Fatal(err)
	}
	return "aaa." + base64.StdEncoding.EncodeToString(payload) + ".bbb"
}

func lastEventType(events []AssistantMessageEvent) string {
	if len(events) == 0 {
		return ""
	}
	return events[len(events)-1].Type
}
