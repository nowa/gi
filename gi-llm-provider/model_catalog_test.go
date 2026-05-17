package gillmprovider

import "testing"

func TestFireworksModelCatalog(t *testing.T) {
	model := MustGetModel("fireworks", "accounts/fireworks/models/kimi-k2p6")
	if model.API != "anthropic-messages" || model.Provider != "fireworks" || model.BaseURL != "https://api.fireworks.ai/inference" {
		t.Fatalf("model = %#v", model)
	}
	if !model.Reasoning || !stringSlicesEqual(model.Input, []string{"text", "image"}) || model.ContextWindow != 262000 || model.MaxTokens != 262000 {
		t.Fatalf("model = %#v", model)
	}
	if model.Cost != (ModelCost{Input: 0.95, Output: 4, CacheRead: 0.16, CacheWrite: 0}) {
		t.Fatalf("cost = %#v", model.Cost)
	}
	if model.Compat.SendSessionAffinityHeaders == nil || !*model.Compat.SendSessionAffinityHeaders {
		t.Fatalf("compat = %#v", model.Compat)
	}
	if model.Compat.SupportsEagerToolInputStreaming == nil || *model.Compat.SupportsEagerToolInputStreaming {
		t.Fatalf("compat = %#v", model.Compat)
	}
	if model.Compat.SupportsCacheControlOnTools == nil || *model.Compat.SupportsCacheControlOnTools {
		t.Fatalf("compat = %#v", model.Compat)
	}
	if model.Compat.SupportsLongCacheRetention == nil || *model.Compat.SupportsLongCacheRetention {
		t.Fatalf("compat = %#v", model.Compat)
	}

	router := MustGetModel("fireworks", "accounts/fireworks/routers/kimi-k2p5-turbo")
	if router.API != "anthropic-messages" || router.BaseURL != "https://api.fireworks.ai/inference" || !stringSlicesEqual(router.Input, []string{"text", "image"}) {
		t.Fatalf("router = %#v", router)
	}
}

func TestFireworksAnthropicHeadersAndToolCompat(t *testing.T) {
	model := MustGetModel("fireworks", "accounts/fireworks/models/kimi-k2p6")
	context := Context{
		Messages: []Message{UserMessageText("Use tool")},
		Tools: []Tool{{
			Name:        "lookup",
			Description: "Lookup",
			Parameters:  Object(map[string]Schema{"value": String()}, "value"),
		}},
	}

	headers := BuildAnthropicHeaders(model, context, AnthropicPayloadOptions{SessionID: "fireworks-session-1"})
	if headers["x-session-affinity"] != "fireworks-session-1" {
		t.Fatalf("headers = %#v", headers)
	}
	headers = BuildAnthropicHeaders(model, context, AnthropicPayloadOptions{SessionID: "fireworks-session-2", CacheRetention: "none"})
	if _, ok := headers["x-session-affinity"]; ok {
		t.Fatalf("headers = %#v", headers)
	}

	payload := BuildAnthropicPayload(model, context, AnthropicPayloadOptions{})
	if len(payload.Tools) != 1 {
		t.Fatalf("tools = %#v", payload.Tools)
	}
	if payload.Tools[0].CacheControl != nil || payload.Tools[0].EagerInputStreaming != nil {
		t.Fatalf("fireworks tool compat = %#v", payload.Tools[0])
	}

	anthropic := MustGetModel("anthropic", "claude-opus-4-7")
	payload = BuildAnthropicPayload(anthropic, context, AnthropicPayloadOptions{})
	if payload.Tools[0].CacheControl == nil || payload.Tools[0].EagerInputStreaming == nil || !*payload.Tools[0].EagerInputStreaming {
		t.Fatalf("anthropic tool compat = %#v", payload.Tools[0])
	}
}

func TestTogetherModelCatalog(t *testing.T) {
	model := MustGetModel("together", "moonshotai/Kimi-K2.6")
	if model.API != "openai-completions" || model.Provider != "together" || model.BaseURL != "https://api.together.ai/v1" {
		t.Fatalf("model = %#v", model)
	}
	if !model.Reasoning || !stringSlicesEqual(model.Input, []string{"text", "image"}) || model.ContextWindow != 262144 || model.MaxTokens != 131000 {
		t.Fatalf("model = %#v", model)
	}
	if model.Cost != (ModelCost{Input: 1.2, Output: 4.5, CacheRead: 0.2, CacheWrite: 0}) {
		t.Fatalf("cost = %#v", model.Cost)
	}
	if !thinkingMapHasNil(model.ThinkingLevelMap, "minimal", "low", "medium") {
		t.Fatalf("thinking map = %#v", model.ThinkingLevelMap)
	}
	compat := ResolveOpenAICompletionsCompat(model)
	if compat.SupportsStore || compat.SupportsDeveloperRole || compat.SupportsReasoningEffort || compat.SupportsStrictMode || compat.SupportsLongCacheRetention || compat.MaxTokensField != "max_tokens" || compat.ThinkingFormat != "together" {
		t.Fatalf("compat = %#v", compat)
	}
}

func TestTogetherReasoningControls(t *testing.T) {
	gptOSS := MustGetModel("together", "openai/gpt-oss-120b")
	if !thinkingMapHasNil(gptOSS.ThinkingLevelMap, "off", "minimal") {
		t.Fatalf("gpt oss thinking map = %#v", gptOSS.ThinkingLevelMap)
	}
	gptCompat := ResolveOpenAICompletionsCompat(gptOSS)
	if !gptCompat.SupportsReasoningEffort || gptCompat.ThinkingFormat != "openai" {
		t.Fatalf("gpt oss compat = %#v", gptCompat)
	}

	deepSeek := MustGetModel("together", "deepseek-ai/DeepSeek-V4-Pro")
	if !thinkingMapHasNil(deepSeek.ThinkingLevelMap, "minimal", "low", "medium", "xhigh") || *deepSeek.ThinkingLevelMap["high"] != "high" {
		t.Fatalf("deepseek thinking map = %#v", deepSeek.ThinkingLevelMap)
	}
	deepSeekCompat := ResolveOpenAICompletionsCompat(deepSeek)
	if !deepSeekCompat.SupportsReasoningEffort || deepSeekCompat.ThinkingFormat != "together" {
		t.Fatalf("deepseek compat = %#v", deepSeekCompat)
	}

	minimax := MustGetModel("together", "MiniMaxAI/MiniMax-M2.7")
	if !thinkingMapHasNil(minimax.ThinkingLevelMap, "off", "minimal", "low", "medium") {
		t.Fatalf("minimax thinking map = %#v", minimax.ThinkingLevelMap)
	}
	minimaxCompat := ResolveOpenAICompletionsCompat(minimax)
	if minimaxCompat.ThinkingFormat != "together" || minimaxCompat.SupportsReasoningEffort {
		t.Fatalf("minimax compat = %#v", minimaxCompat)
	}
	if minimax.Compat.ThinkingFormat != "" {
		t.Fatalf("explicit minimax thinking format should be unset: %#v", minimax.Compat)
	}
}

func TestOfficialGrokAndDeepSeekModelCatalog(t *testing.T) {
	t.Setenv("XAI_API_KEY", "test-xai-key")
	t.Setenv("DEEPSEEK_API_KEY", "test-deepseek-key")

	if got := FindEnvKeys("xai"); len(got) != 1 || got[0] != "XAI_API_KEY" || GetEnvAPIKey("xai") != "test-xai-key" {
		t.Fatalf("xai env keys = %#v key=%q", got, GetEnvAPIKey("xai"))
	}
	if got := FindEnvKeys("deepseek"); len(got) != 1 || got[0] != "DEEPSEEK_API_KEY" || GetEnvAPIKey("deepseek") != "test-deepseek-key" {
		t.Fatalf("deepseek env keys = %#v key=%q", got, GetEnvAPIKey("deepseek"))
	}

	grok := MustGetModel("xai", "grok-4.3")
	if grok.API != "openai-completions" || grok.Provider != "xai" || grok.BaseURL != "https://api.x.ai/v1" {
		t.Fatalf("grok model = %#v", grok)
	}
	if !grok.Reasoning || !stringSlicesEqual(grok.Input, []string{"text", "image"}) || grok.ContextWindow != 1000000 {
		t.Fatalf("grok model = %#v", grok)
	}
	if grok.Cost != (ModelCost{Input: 1.25, Output: 2.50, CacheRead: 0.20, CacheWrite: 0}) {
		t.Fatalf("grok cost = %#v", grok.Cost)
	}
	if !thinkingMapHas(grok.ThinkingLevelMap, map[string]string{"off": "none", "low": "low", "medium": "medium", "high": "high"}) || !thinkingMapHasNil(grok.ThinkingLevelMap, "xhigh") {
		t.Fatalf("grok thinking map = %#v", grok.ThinkingLevelMap)
	}
	if got := GetSupportedThinkingLevels(grok); !stringSlicesEqual(got, []string{"off", "low", "medium", "high"}) {
		t.Fatalf("grok levels = %#v", got)
	}
	for _, alias := range []string{"grok-4.3-latest", "grok-latest"} {
		model := MustGetModel("xai", alias)
		if model.BaseURL != grok.BaseURL || model.API != grok.API || model.ContextWindow != grok.ContextWindow {
			t.Fatalf("grok alias %s = %#v", alias, model)
		}
	}
	grokCompat := ResolveOpenAICompletionsCompat(grok)
	if grokCompat.SupportsStore || grokCompat.SupportsDeveloperRole || !grokCompat.SupportsReasoningEffort || grokCompat.SupportsStrictMode || grokCompat.SupportsLongCacheRetention || grokCompat.MaxTokensField != "max_tokens" || grokCompat.ThinkingFormat != "xai" {
		t.Fatalf("grok compat = %#v", grokCompat)
	}

	deepseek := MustGetModel("deepseek", "deepseek-v4-flash")
	if deepseek.API != "openai-completions" || deepseek.Provider != "deepseek" || deepseek.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("deepseek model = %#v", deepseek)
	}
	if !deepseek.Reasoning || !stringSlicesEqual(deepseek.Input, []string{"text"}) || deepseek.ContextWindow != 1000000 || deepseek.MaxTokens != 384000 {
		t.Fatalf("deepseek model = %#v", deepseek)
	}
	if deepseek.Cost != (ModelCost{Input: 0.14, Output: 0.28, CacheRead: 0.0028, CacheWrite: 0}) {
		t.Fatalf("deepseek cost = %#v", deepseek.Cost)
	}
	if !thinkingMapHas(deepseek.ThinkingLevelMap, map[string]string{"low": "high", "medium": "high", "high": "high", "xhigh": "max"}) || !thinkingMapHasNil(deepseek.ThinkingLevelMap, "minimal") {
		t.Fatalf("deepseek thinking map = %#v", deepseek.ThinkingLevelMap)
	}
	pro := MustGetModel("deepseek", "deepseek-v4-pro")
	if pro.BaseURL != deepseek.BaseURL || pro.MaxTokens != 384000 || pro.Cost != (ModelCost{Input: 0.435, Output: 0.87, CacheRead: 0.003625, CacheWrite: 0}) {
		t.Fatalf("deepseek pro = %#v", pro)
	}
	deepseekCompat := ResolveOpenAICompletionsCompat(deepseek)
	if deepseekCompat.SupportsStore || deepseekCompat.SupportsDeveloperRole || !deepseekCompat.SupportsReasoningEffort || deepseekCompat.SupportsStrictMode || deepseekCompat.SupportsLongCacheRetention || !deepseekCompat.RequiresReasoningContentOnAssistant || deepseekCompat.MaxTokensField != "max_tokens" || deepseekCompat.ThinkingFormat != "deepseek" {
		t.Fatalf("deepseek compat = %#v", deepseekCompat)
	}
}

func TestOpenCodeZenModelCatalog(t *testing.T) {
	t.Setenv("OPENCODE_API_KEY", "test-opencode-key")
	if got := FindEnvKeys("opencode"); len(got) != 1 || got[0] != "OPENCODE_API_KEY" || GetEnvAPIKey("opencode") != "test-opencode-key" {
		t.Fatalf("opencode env keys = %#v key=%q", got, GetEnvAPIKey("opencode"))
	}
	if got := FindEnvKeys("opencode-go"); len(got) != 1 || got[0] != "OPENCODE_API_KEY" || GetEnvAPIKey("opencode-go") != "test-opencode-key" {
		t.Fatalf("opencode-go env keys = %#v key=%q", got, GetEnvAPIKey("opencode-go"))
	}

	zen := MustGetModel("opencode", "big-pickle")
	if zen.API != "openai-completions" || zen.BaseURL != "https://opencode.ai/zen/v1" || !zen.Reasoning || zen.ContextWindow != 200000 {
		t.Fatalf("opencode zen model = %#v", zen)
	}

	haiku := MustGetModel("opencode", "claude-haiku-4-5")
	if haiku.API != "anthropic-messages" || haiku.BaseURL != "https://opencode.ai/zen" || !stringSlicesEqual(haiku.Input, []string{"text", "image"}) || haiku.Cost.Input != 1 {
		t.Fatalf("opencode anthropic model = %#v", haiku)
	}

	goModel := MustGetModel("opencode-go", "deepseek-v4-flash")
	if goModel.API != "openai-completions" || goModel.BaseURL != "https://opencode.ai/zen/go/v1" || goModel.ContextWindow != 1000000 || goModel.MaxTokens != 384000 {
		t.Fatalf("opencode-go model = %#v", goModel)
	}
	if goModel.Compat.RequiresReasoningContentOnAssistantTurns == nil || !*goModel.Compat.RequiresReasoningContentOnAssistantTurns || goModel.Compat.ThinkingFormat != "deepseek" {
		t.Fatalf("opencode-go compat = %#v", goModel.Compat)
	}
	if !thinkingMapHas(goModel.ThinkingLevelMap, map[string]string{"high": "high", "xhigh": "max"}) {
		t.Fatalf("opencode-go thinking map = %#v", goModel.ThinkingLevelMap)
	}
}

func TestFireworksAndTogetherEnvKeys(t *testing.T) {
	t.Setenv("FIREWORKS_API_KEY", "test-fireworks-key")
	t.Setenv("TOGETHER_API_KEY", "test-together-key")
	if got := FindEnvKeys("fireworks"); len(got) != 1 || got[0] != "FIREWORKS_API_KEY" || GetEnvAPIKey("fireworks") != "test-fireworks-key" {
		t.Fatalf("fireworks env keys = %#v key=%q", got, GetEnvAPIKey("fireworks"))
	}
	if got := FindEnvKeys("together"); len(got) != 1 || got[0] != "TOGETHER_API_KEY" || GetEnvAPIKey("together") != "test-together-key" {
		t.Fatalf("together env keys = %#v key=%q", got, GetEnvAPIKey("together"))
	}
}

func TestBedrockModelCatalog(t *testing.T) {
	models := GetModels("amazon-bedrock")
	if len(models) == 0 {
		t.Fatal("expected Bedrock models")
	}

	ids := map[string]Model{}
	for _, model := range models {
		ids[model.ID] = model
		if model.API != "bedrock-converse-stream" || model.BaseURL == "" {
			t.Fatalf("bedrock model = %#v", model)
		}
		if model.ContextWindow <= 0 || model.MaxTokens <= 0 {
			t.Fatalf("bedrock context = %#v", model)
		}
	}

	for _, id := range []string{
		"global.anthropic.claude-opus-4-6-v1",
		"us.anthropic.claude-sonnet-4-5-20250929-v1:0",
		"eu.anthropic.claude-sonnet-4-5-20250929-v1:0",
	} {
		if _, ok := ids[id]; !ok {
			t.Fatalf("missing Bedrock model %q in %#v", id, ids)
		}
	}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func thinkingMapHasNil(values map[string]*string, keys ...string) bool {
	for _, key := range keys {
		value, ok := values[key]
		if !ok || value != nil {
			return false
		}
	}
	return true
}

func thinkingMapHas(values map[string]*string, want map[string]string) bool {
	for key, wantValue := range want {
		value, ok := values[key]
		if !ok || value == nil || *value != wantValue {
			return false
		}
	}
	return true
}
