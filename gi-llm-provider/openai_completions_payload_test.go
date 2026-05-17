package gillmprovider

import "testing"

func TestBuildOpenAICompletionsPayloadToolChoiceAndStrictMode(t *testing.T) {
	model := Model{
		ID:       "gpt-4o-mini",
		Provider: "openai",
		API:      "openai-completions",
		BaseURL:  "https://api.openai.com/v1",
		Input:    []string{"text"},
	}
	context := Context{
		Messages: []Message{UserMessageText("Call ping")},
		Tools: []Tool{{
			Name:        "ping",
			Description: "Ping tool",
			Parameters:  Object(map[string]Schema{"ok": Boolean()}, "ok"),
		}},
	}

	payload := BuildOpenAICompletionsPayload(model, context, OpenAICompletionsPayloadOptions{ToolChoice: "required"})

	if payload.ToolChoice != "required" {
		t.Fatalf("tool choice = %#v", payload.ToolChoice)
	}
	if len(payload.Tools) != 1 {
		t.Fatalf("tools = %#v", payload.Tools)
	}
	if payload.Tools[0].Function.Strict == nil || *payload.Tools[0].Function.Strict {
		t.Fatalf("strict = %#v", payload.Tools[0].Function.Strict)
	}

	model.Compat.SupportsStrictMode = ptrBool(false)
	payload = BuildOpenAICompletionsPayload(model, context, OpenAICompletionsPayloadOptions{})
	if payload.Tools[0].Function.Strict != nil {
		t.Fatalf("strict should be omitted, got %#v", payload.Tools[0].Function.Strict)
	}
}

func TestBuildOpenAICompletionsPayloadAppliesAnthropicCacheControl(t *testing.T) {
	model := Model{
		ID:        "custom-qwen",
		Name:      "Custom Qwen",
		Provider:  "openrouter",
		API:       "openai-completions",
		BaseURL:   "https://example.com/v1",
		Reasoning: true,
		Input:     []string{"text"},
		Compat:    ModelCompat{CacheControlFormat: "anthropic"},
	}
	context := Context{
		SystemPrompt: "System prompt",
		Messages:     []Message{UserMessageText("Hello")},
		Tools: []Tool{{
			Name:        "read",
			Description: "Read a file",
			Parameters:  Object(map[string]Schema{"path": String()}, "path"),
		}},
	}

	payload := BuildOpenAICompletionsPayload(model, context, OpenAICompletionsPayloadOptions{})

	instruction := payload.Messages[0]
	instructionContent, ok := instruction.Content.([]OpenAIChatContentPart)
	if !ok || len(instructionContent) != 1 || instructionContent[0].CacheControl == nil {
		t.Fatalf("instruction cache control = %#v", instruction.Content)
	}
	if payload.Tools[0].CacheControl == nil {
		t.Fatalf("tool cache control missing: %#v", payload.Tools[0])
	}
	last := payload.Messages[len(payload.Messages)-1]
	lastContent, ok := last.Content.([]OpenAIChatContentPart)
	if !ok || len(lastContent) == 0 || lastContent[0].CacheControl == nil {
		t.Fatalf("last message cache control = %#v", last.Content)
	}

	payload = BuildOpenAICompletionsPayload(model, context, OpenAICompletionsPayloadOptions{CacheRetention: "none"})
	if _, ok := payload.Messages[0].Content.([]OpenAIChatContentPart); ok {
		t.Fatalf("instruction cache marker should be omitted: %#v", payload.Messages[0].Content)
	}
	if payload.Tools[0].CacheControl != nil {
		t.Fatalf("tool cache marker should be omitted: %#v", payload.Tools[0].CacheControl)
	}
}

func TestBuildOpenAICompletionsPayloadPromptCacheRetention(t *testing.T) {
	model := Model{
		ID:       "gpt-4o-mini",
		Provider: "openai",
		API:      "openai-completions",
		BaseURL:  "https://api.openai.com/v1",
		Input:    []string{"text"},
	}
	context := Context{SystemPrompt: "sys", Messages: []Message{UserMessageText("hi")}}

	payload := BuildOpenAICompletionsPayload(model, context, OpenAICompletionsPayloadOptions{SessionID: "session-123"})
	if payload.PromptCacheKey != "session-123" || payload.PromptCacheRetention != "" {
		t.Fatalf("payload = %#v", payload)
	}

	payload = BuildOpenAICompletionsPayload(model, context, OpenAICompletionsPayloadOptions{CacheRetention: "long", SessionID: "session-456"})
	if payload.PromptCacheKey != "session-456" || payload.PromptCacheRetention != "24h" {
		t.Fatalf("payload = %#v", payload)
	}

	payload = BuildOpenAICompletionsPayload(model, context, OpenAICompletionsPayloadOptions{CacheRetention: "none", SessionID: "session-789"})
	if payload.PromptCacheKey != "" || payload.PromptCacheRetention != "" {
		t.Fatalf("payload = %#v", payload)
	}

	proxy := model
	proxy.BaseURL = "https://proxy.example.com/v1"
	proxy.Compat.SupportsLongCacheRetention = ptrBool(false)
	payload = BuildOpenAICompletionsPayload(proxy, context, OpenAICompletionsPayloadOptions{CacheRetention: "long", SessionID: "session-proxy"})
	if payload.PromptCacheKey != "" || payload.PromptCacheRetention != "" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestBuildOpenAICompletionsHeadersSessionAffinity(t *testing.T) {
	model := Model{
		ID:       "gpt-4o-mini",
		Provider: "openai",
		API:      "openai-completions",
		BaseURL:  "https://proxy.example.com/v1",
		Compat:   ModelCompat{SendSessionAffinityHeaders: ptrBool(true)},
	}

	headers := BuildOpenAICompletionsHeaders(model, OpenAICompletionsPayloadOptions{SessionID: "session-affinity"})
	if headers["session_id"] != "session-affinity" || headers["x-client-request-id"] != "session-affinity" || headers["x-session-affinity"] != "session-affinity" {
		t.Fatalf("headers = %#v", headers)
	}

	headers = BuildOpenAICompletionsHeaders(model, OpenAICompletionsPayloadOptions{CacheRetention: "none", SessionID: "session-affinity"})
	if len(headers) != 0 {
		t.Fatalf("headers = %#v", headers)
	}

	headers = BuildOpenAICompletionsHeaders(model, OpenAICompletionsPayloadOptions{
		SessionID: "session-affinity",
		Headers: map[string]string{
			"session_id":          "override-session",
			"x-client-request-id": "override-request",
			"x-session-affinity":  "override-affinity",
		},
	})
	if headers["session_id"] != "override-session" || headers["x-client-request-id"] != "override-request" || headers["x-session-affinity"] != "override-affinity" {
		t.Fatalf("headers = %#v", headers)
	}
}

func TestBuildPayloadUsesPICacheRetentionEnv(t *testing.T) {
	t.Setenv("PI_CACHE_RETENTION", "long")
	model := Model{ID: "gpt-4o-mini", Provider: "openai", API: "openai-completions", BaseURL: "https://api.openai.com/v1", Input: []string{"text"}}

	payload := BuildOpenAICompletionsPayload(model, Context{Messages: []Message{UserMessageText("hi")}}, OpenAICompletionsPayloadOptions{SessionID: "session-env"})

	if payload.PromptCacheKey != "session-env" || payload.PromptCacheRetention != "24h" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestConvertOpenAICompletionsMessagesThinkingAsText(t *testing.T) {
	model := Model{ID: "repro-model", Provider: "repro-provider", API: "openai-completions", Reasoning: true, Input: []string{"text"}}
	context := Context{Messages: []Message{
		UserMessageText("hello"),
		AssistantMessage([]ContentPart{
			Thinking("internal reasoning"),
			Text("visible answer"),
		}, StopReasonStop, model),
		UserMessageText("continue"),
	}}

	messages := ConvertOpenAICompletionsMessages(model, context, OpenAICompletionsCompat{RequiresThinkingAsText: true})

	if len(messages) != 3 {
		t.Fatalf("messages = %#v", messages)
	}
	content, ok := messages[1].Content.([]OpenAIChatContentPart)
	if !ok || len(content) != 2 || content[0].Text != "internal reasoning" || content[1].Text != "visible answer" {
		t.Fatalf("assistant content = %#v", messages[1].Content)
	}
}

func TestBuildOpenAICompletionsPayloadReasoningAndZAIToolStream(t *testing.T) {
	tests := []struct {
		name            string
		model           Model
		reasoning       string
		wantEffort      string
		wantReasoning   map[string]any
		wantToolStream  bool
		wantEnableThink bool
	}{
		{
			name:       "groq qwen maps medium to default effort",
			model:      MustGetModel("groq", "qwen/qwen3-32b"),
			reasoning:  "medium",
			wantEffort: "default",
		},
		{
			name:       "groq gpt oss keeps medium effort",
			model:      MustGetModel("groq", "openai/gpt-oss-20b"),
			reasoning:  "medium",
			wantEffort: "medium",
		},
		{
			name:          "openrouter uses nested reasoning object",
			model:         MustGetModel("openrouter", "deepseek/deepseek-r1"),
			reasoning:     "high",
			wantReasoning: map[string]any{"effort": "high"},
		},
		{
			name:            "zai enables tool stream and thinking",
			model:           MustGetModel("zai", "glm-5.1"),
			reasoning:       "medium",
			wantToolStream:  true,
			wantEnableThink: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			context := Context{Messages: []Message{UserMessageText("Hi")}}
			if tc.wantToolStream {
				context.Tools = []Tool{{Name: "ping", Parameters: Object(map[string]Schema{"ok": Boolean()}, "ok")}}
			}
			payload := BuildOpenAICompletionsPayload(tc.model, context, OpenAICompletionsPayloadOptions{Reasoning: tc.reasoning})
			if payload.ReasoningEffort != tc.wantEffort {
				t.Fatalf("reasoning effort = %q", payload.ReasoningEffort)
			}
			if tc.wantReasoning != nil && payload.Reasoning["effort"] != tc.wantReasoning["effort"] {
				t.Fatalf("reasoning = %#v", payload.Reasoning)
			}
			if tc.wantToolStream && (payload.ToolStream == nil || !*payload.ToolStream) {
				t.Fatalf("tool stream = %#v", payload.ToolStream)
			}
			if tc.wantEnableThink && (payload.EnableThinking == nil || !*payload.EnableThinking) {
				t.Fatalf("enable thinking = %#v", payload.EnableThinking)
			}
		})
	}
}

func TestBuildOpenAICompletionsPayloadOfficialGrokAndDeepSeek(t *testing.T) {
	context := Context{
		SystemPrompt: "You are concise.",
		Messages:     []Message{UserMessageText("Hi")},
		Tools: []Tool{{
			Name:        "lookup",
			Description: "Lookup",
			Parameters:  Object(map[string]Schema{"query": String()}, "query"),
		}},
	}

	grok := MustGetModel("xai", "grok-4.3")
	payload := BuildOpenAICompletionsPayload(grok, context, OpenAICompletionsPayloadOptions{
		MaxTokens:      4096,
		Reasoning:      "high",
		CacheRetention: "long",
		SessionID:      "grok-session",
	})
	if payload.MaxTokens != 4096 || payload.MaxCompletionTokens != 0 {
		t.Fatalf("grok max tokens = %#v", payload)
	}
	if payload.ReasoningEffort != "high" || payload.Reasoning != nil || payload.Thinking != nil {
		t.Fatalf("grok reasoning payload = %#v", payload)
	}
	if payload.Store != nil || payload.PromptCacheKey != "" || payload.PromptCacheRetention != "" {
		t.Fatalf("grok unsupported fields = %#v", payload)
	}
	if len(payload.Messages) == 0 || payload.Messages[0].Role != "system" {
		t.Fatalf("grok messages = %#v", payload.Messages)
	}
	if len(payload.Tools) != 1 || payload.Tools[0].Function.Strict != nil {
		t.Fatalf("grok tools = %#v", payload.Tools)
	}

	payload = BuildOpenAICompletionsPayload(grok, Context{Messages: []Message{UserMessageText("Hi")}}, OpenAICompletionsPayloadOptions{Reasoning: "off"})
	if payload.ReasoningEffort != "none" {
		t.Fatalf("grok off reasoning payload = %#v", payload)
	}

	deepseek := MustGetModel("deepseek", "deepseek-v4-flash")
	payload = BuildOpenAICompletionsPayload(deepseek, context, OpenAICompletionsPayloadOptions{
		MaxTokens:      8192,
		Reasoning:      "medium",
		CacheRetention: "long",
		SessionID:      "deepseek-session",
	})
	if payload.MaxTokens != 8192 || payload.MaxCompletionTokens != 0 {
		t.Fatalf("deepseek max tokens = %#v", payload)
	}
	if payload.ReasoningEffort != "high" || payload.Thinking["type"] != "enabled" || payload.Reasoning != nil {
		t.Fatalf("deepseek reasoning payload = %#v", payload)
	}
	if payload.Store != nil || payload.PromptCacheKey != "" || payload.PromptCacheRetention != "" {
		t.Fatalf("deepseek unsupported fields = %#v", payload)
	}
	if len(payload.Tools) != 1 || payload.Tools[0].Function.Strict != nil {
		t.Fatalf("deepseek tools = %#v", payload.Tools)
	}

	payload = BuildOpenAICompletionsPayload(deepseek, Context{Messages: []Message{UserMessageText("Hi")}}, OpenAICompletionsPayloadOptions{Reasoning: "off"})
	if payload.ReasoningEffort != "" || payload.Thinking["type"] != "disabled" {
		t.Fatalf("deepseek off reasoning payload = %#v", payload)
	}
}

func TestOpenAICompletionsResolvedZAIToolStreamCompat(t *testing.T) {
	want := map[string]bool{
		"glm-5.1":     true,
		"glm-4.7":     true,
		"glm-5-turbo": true,
		"glm-4.5-air": false,
	}
	for id, expected := range want {
		model := MustGetModel("zai", id)
		if got := ResolveOpenAICompletionsCompat(model).ZAIToolStream; got != expected {
			t.Fatalf("%s zai tool stream = %v, want %v", id, got, expected)
		}
	}
}
