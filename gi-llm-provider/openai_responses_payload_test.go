package gillmprovider

import "testing"

func TestBuildOpenAIResponsesPayloadReasoningDefaults(t *testing.T) {
	contextValue := Context{SystemPrompt: "sys", Messages: []Message{UserMessageText("hi")}}

	copilot := Model{ID: "gpt-5-mini", Provider: "github-copilot", API: "openai-responses", Reasoning: true, Input: []string{"text"}}
	copilotPayload := BuildOpenAIResponsesPayload(copilot, contextValue, OpenAIResponsesPayloadOptions{})
	if copilotPayload.Reasoning != nil {
		t.Fatalf("copilot reasoning = %#v", copilotPayload.Reasoning)
	}

	openAI := Model{ID: "gpt-5.4", Provider: "openai", API: "openai-responses", Reasoning: true, Input: []string{"text"}, ThinkingLevelMap: map[string]*string{"off": ptrString("none"), "xhigh": ptrString("xhigh")}}
	openAIPayload := BuildOpenAIResponsesPayload(openAI, contextValue, OpenAIResponsesPayloadOptions{})
	if openAIPayload.Reasoning["effort"] != "none" {
		t.Fatalf("openai reasoning = %#v", openAIPayload.Reasoning)
	}

	unsupportedOff := Model{ID: "gpt-5-pro", Provider: "openai", API: "openai-responses", Reasoning: true, Input: []string{"text"}, ThinkingLevelMap: map[string]*string{"off": nil}}
	unsupportedPayload := BuildOpenAIResponsesPayload(unsupportedOff, contextValue, OpenAIResponsesPayloadOptions{})
	if unsupportedPayload.Reasoning != nil {
		t.Fatalf("unsupported off reasoning = %#v", unsupportedPayload.Reasoning)
	}

	requested := BuildOpenAIResponsesPayload(openAI, contextValue, OpenAIResponsesPayloadOptions{ReasoningEffort: "xhigh"})
	if requested.Reasoning["effort"] != "xhigh" || requested.Reasoning["summary"] != "auto" || len(requested.Include) != 1 {
		t.Fatalf("requested reasoning = %#v include=%#v", requested.Reasoning, requested.Include)
	}
}

func TestBuildOpenAIResponsesHeadersCacheAffinity(t *testing.T) {
	model := Model{ID: "gpt-5.4", Provider: "openai", API: "openai-responses", Headers: map[string]string{"x-base": "yes"}}

	headers := BuildOpenAIResponsesHeaders(model, OpenAIResponsesPayloadOptions{SessionID: "session-123"})
	if headers["session_id"] != "session-123" || headers["x-client-request-id"] != "session-123" || headers["x-base"] != "yes" {
		t.Fatalf("headers = %#v", headers)
	}

	noSessionID := model
	noSessionID.Compat.SendSessionIDHeader = ptrBool(false)
	headers = BuildOpenAIResponsesHeaders(noSessionID, OpenAIResponsesPayloadOptions{SessionID: "session-123"})
	if _, ok := headers["session_id"]; ok || headers["x-client-request-id"] != "session-123" {
		t.Fatalf("no session id headers = %#v", headers)
	}

	headers = BuildOpenAIResponsesHeaders(model, OpenAIResponsesPayloadOptions{
		SessionID:      "session-123",
		CacheRetention: "none",
	})
	if _, ok := headers["session_id"]; ok {
		t.Fatalf("cache none headers = %#v", headers)
	}
	if _, ok := headers["x-client-request-id"]; ok {
		t.Fatalf("cache none headers = %#v", headers)
	}

	headers = BuildOpenAIResponsesHeaders(model, OpenAIResponsesPayloadOptions{
		SessionID: "session-123",
		Headers: map[string]string{
			"session_id":          "override-session",
			"x-client-request-id": "override-request",
		},
	})
	if headers["session_id"] != "override-session" || headers["x-client-request-id"] != "override-request" {
		t.Fatalf("override headers = %#v", headers)
	}
}

func TestBuildOpenAIResponsesPayloadCacheRetentionAndTools(t *testing.T) {
	model := Model{ID: "gpt-5.4", Provider: "openai", API: "openai-responses", Reasoning: true, Input: []string{"text"}}
	contextValue := Context{
		Messages: []Message{UserMessageText("hi")},
		Tools: []Tool{{
			Name:        "lookup",
			Description: "Lookup",
			Parameters:  Object(map[string]Schema{"query": String()}, "query"),
		}},
	}

	payload := BuildOpenAIResponsesPayload(model, contextValue, OpenAIResponsesPayloadOptions{SessionID: "session-123", CacheRetention: "long", MaxTokens: 512})
	if payload.PromptCacheKey != "session-123" || payload.PromptCacheRetention != "24h" || payload.MaxOutputTokens != 512 {
		t.Fatalf("payload = %#v", payload)
	}
	if len(payload.Tools) != 1 || payload.Tools[0].Name != "lookup" || payload.Tools[0].Strict == nil || !*payload.Tools[0].Strict {
		t.Fatalf("tools = %#v", payload.Tools)
	}

	noLong := model
	noLong.Compat.SupportsLongCacheRetention = ptrBool(false)
	payload = BuildOpenAIResponsesPayload(noLong, contextValue, OpenAIResponsesPayloadOptions{SessionID: "session-123", CacheRetention: "long"})
	if payload.PromptCacheRetention != "" {
		t.Fatalf("retention = %#v", payload)
	}
}

func TestOpenAIResponsesServiceTierPricing(t *testing.T) {
	model := Model{ID: "gpt-5.5", Cost: ModelCost{Input: 5, Output: 30}}
	usage := ParseOpenAIResponsesUsage(OpenAIResponsesUsage{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
		TotalTokens:  2_000_000,
	}, model)

	ApplyOpenAIResponsesServiceTierPricing(&usage, "priority", model)

	if usage.Cost.Input != 12.5 || usage.Cost.Output != 75 || usage.Cost.Total != 87.5 {
		t.Fatalf("cost = %#v", usage.Cost)
	}

	usage = ParseOpenAIResponsesUsage(OpenAIResponsesUsage{InputTokens: 1_000_000, OutputTokens: 1_000_000, TotalTokens: 2_000_000}, model)
	ApplyOpenAIResponsesServiceTierPricing(&usage, "flex", model)
	if usage.Cost.Input != 2.5 || usage.Cost.Output != 15 || usage.Cost.Total != 17.5 {
		t.Fatalf("flex cost = %#v", usage.Cost)
	}
}
