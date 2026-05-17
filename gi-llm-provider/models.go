package gillmprovider

import "fmt"

func ptrString(value string) *string { return &value }
func ptrBool(value bool) *bool       { return &value }

var modelRegistry = map[string]map[string]Model{}

func init() {
	RegisterModel(Model{ID: "gpt-4o-mini", Name: "gpt-4o-mini", Provider: "openai", API: "openai-responses", BaseURL: "https://api.openai.com/v1", Input: []string{"text", "image"}, ContextWindow: 128000, MaxTokens: 16384})
	RegisterModel(Model{ID: "gpt-5-mini", Name: "GPT-5 Mini", Provider: "openai", API: "openai-responses", BaseURL: "https://api.openai.com/v1", Reasoning: true, Input: []string{"text", "image"}, Cost: ModelCost{Input: 0.25, Output: 2, CacheRead: 0.025, CacheWrite: 0}, ContextWindow: 400000, MaxTokens: 128000, ThinkingLevelMap: map[string]*string{"off": nil}})
	RegisterModel(Model{ID: "gpt-5.2-codex", Name: "GPT-5.2 Codex", Provider: "openai", API: "openai-responses", BaseURL: "https://api.openai.com/v1", Reasoning: true, Input: []string{"text", "image"}, Cost: ModelCost{Input: 1.75, Output: 14, CacheRead: 0.175, CacheWrite: 0}, ContextWindow: 400000, MaxTokens: 128000, ThinkingLevelMap: map[string]*string{"off": nil, "xhigh": ptrString("xhigh")}})
	RegisterModel(Model{ID: "gpt-5.3-codex", Name: "GPT-5.3 Codex", Provider: "openai", API: "openai-responses", BaseURL: "https://api.openai.com/v1", Reasoning: true, Input: []string{"text", "image"}, Cost: ModelCost{Input: 1.75, Output: 14, CacheRead: 0.175, CacheWrite: 0}, ContextWindow: 400000, MaxTokens: 128000, ThinkingLevelMap: map[string]*string{"off": ptrString("none"), "xhigh": ptrString("xhigh")}})
	RegisterModel(Model{ID: "gpt-4o-mini", Name: "gpt-4o-mini", Provider: "azure-openai-responses", API: "azure-openai-responses", Input: []string{"text", "image"}, ContextWindow: 128000, MaxTokens: 16384})
	RegisterModel(Model{ID: "gpt-5.2-codex", Name: "GPT-5.2 Codex", Provider: "openai-codex", API: "openai-codex-responses", BaseURL: "https://chatgpt.com/backend-api", Reasoning: true, Input: []string{"text", "image"}, Cost: ModelCost{Input: 1.75, Output: 14, CacheRead: 0.175, CacheWrite: 0}, ContextWindow: 272000, MaxTokens: 128000, ThinkingLevelMap: map[string]*string{"minimal": ptrString("low"), "xhigh": ptrString("xhigh")}})
	RegisterModel(Model{ID: "gpt-5.3-codex", Name: "GPT-5.3 Codex", Provider: "openai-codex", API: "openai-codex-responses", BaseURL: "https://chatgpt.com/backend-api", Reasoning: true, Input: []string{"text", "image"}, Cost: ModelCost{Input: 1.75, Output: 14, CacheRead: 0.175, CacheWrite: 0}, ContextWindow: 272000, MaxTokens: 128000, ThinkingLevelMap: map[string]*string{"minimal": ptrString("low"), "xhigh": ptrString("xhigh")}})
	RegisterModel(Model{ID: "gpt-5.4", Name: "gpt-5.4", Provider: "openai-codex", API: "openai-codex-responses", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 400000, MaxTokens: 128000, ThinkingLevelMap: map[string]*string{"xhigh": ptrString("xhigh")}})
	RegisterModel(Model{ID: "gpt-5.5", Name: "gpt-5.5", Provider: "openai-codex", API: "openai-codex-responses", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 400000, MaxTokens: 128000, ThinkingLevelMap: map[string]*string{"xhigh": ptrString("xhigh")}})
	RegisterModel(Model{ID: "gemini-2.5-flash", Name: "gemini-2.5-flash", Provider: "google", API: "google-generative-ai", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 1048576, MaxTokens: 65536})
	RegisterModel(Model{ID: "gemini-3-flash-preview", Name: "gemini-3-flash-preview", Provider: "google-vertex", API: "google-vertex", BaseURL: "https://{location}-aiplatform.googleapis.com/v1", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 1048576, MaxTokens: 65536})
	RegisterModel(Model{ID: "claude-opus-4-6", Name: "claude-opus-4-6", Provider: "anthropic", API: "anthropic-messages", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 200000, MaxTokens: 32000, ThinkingLevelMap: map[string]*string{"xhigh": ptrString("max")}})
	RegisterModel(Model{ID: "claude-opus-4-7", Name: "claude-opus-4-7", Provider: "anthropic", API: "anthropic-messages", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 200000, MaxTokens: 32000, ThinkingLevelMap: map[string]*string{"xhigh": ptrString("xhigh")}})
	RegisterModel(Model{ID: "claude-haiku-4-5", Name: "claude-haiku-4-5", Provider: "anthropic", API: "anthropic-messages", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 200000, MaxTokens: 64000})
	RegisterModel(Model{ID: "claude-sonnet-4-5", Name: "claude-sonnet-4-5", Provider: "anthropic", API: "anthropic-messages", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 200000, MaxTokens: 32000})
	RegisterModel(Model{ID: "claude-sonnet-4-6", Name: "claude-sonnet-4-6", Provider: "anthropic", API: "anthropic-messages", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 200000, MaxTokens: 32000})
	RegisterModel(Model{ID: "deepseek-v4-flash", Name: "DeepSeek V4 Flash", Provider: "deepseek", API: "openai-completions", BaseURL: "https://api.deepseek.com", Compat: ModelCompat{MaxTokensField: "max_tokens", ThinkingFormat: "deepseek"}, Reasoning: true, Input: []string{"text"}, Cost: ModelCost{Input: 0.14, Output: 0.28, CacheRead: 0.0028, CacheWrite: 0}, ContextWindow: 1000000, MaxTokens: 384000, ThinkingLevelMap: map[string]*string{"minimal": nil, "low": ptrString("high"), "medium": ptrString("high"), "high": ptrString("high"), "xhigh": ptrString("max")}})
	RegisterModel(Model{ID: "deepseek-v4-pro", Name: "DeepSeek V4 Pro", Provider: "deepseek", API: "openai-completions", BaseURL: "https://api.deepseek.com", Compat: ModelCompat{MaxTokensField: "max_tokens", ThinkingFormat: "deepseek"}, Reasoning: true, Input: []string{"text"}, Cost: ModelCost{Input: 0.435, Output: 0.87, CacheRead: 0.003625, CacheWrite: 0}, ContextWindow: 1000000, MaxTokens: 384000, ThinkingLevelMap: map[string]*string{"minimal": nil, "low": ptrString("high"), "medium": ptrString("high"), "high": ptrString("high"), "xhigh": ptrString("max")}})
	RegisterModel(Model{ID: "grok-4.3", Name: "Grok 4.3", Provider: "xai", API: "openai-completions", BaseURL: "https://api.x.ai/v1", Compat: ModelCompat{SupportsStore: ptrBool(false), SupportsDeveloperRole: ptrBool(false), SupportsStrictMode: ptrBool(false), SupportsLongCacheRetention: ptrBool(false), MaxTokensField: "max_tokens", ThinkingFormat: "xai"}, Reasoning: true, Input: []string{"text", "image"}, Cost: ModelCost{Input: 1.25, Output: 2.50, CacheRead: 0.20, CacheWrite: 0}, ContextWindow: 1000000, ThinkingLevelMap: map[string]*string{"off": ptrString("none"), "minimal": nil, "low": ptrString("low"), "medium": ptrString("medium"), "high": ptrString("high"), "xhigh": nil}})
	RegisterModel(Model{ID: "grok-4.3-latest", Name: "Grok 4.3 Latest", Provider: "xai", API: "openai-completions", BaseURL: "https://api.x.ai/v1", Compat: ModelCompat{SupportsStore: ptrBool(false), SupportsDeveloperRole: ptrBool(false), SupportsStrictMode: ptrBool(false), SupportsLongCacheRetention: ptrBool(false), MaxTokensField: "max_tokens", ThinkingFormat: "xai"}, Reasoning: true, Input: []string{"text", "image"}, Cost: ModelCost{Input: 1.25, Output: 2.50, CacheRead: 0.20, CacheWrite: 0}, ContextWindow: 1000000, ThinkingLevelMap: map[string]*string{"off": ptrString("none"), "minimal": nil, "low": ptrString("low"), "medium": ptrString("medium"), "high": ptrString("high"), "xhigh": nil}})
	RegisterModel(Model{ID: "grok-latest", Name: "Grok Latest", Provider: "xai", API: "openai-completions", BaseURL: "https://api.x.ai/v1", Compat: ModelCompat{SupportsStore: ptrBool(false), SupportsDeveloperRole: ptrBool(false), SupportsStrictMode: ptrBool(false), SupportsLongCacheRetention: ptrBool(false), MaxTokensField: "max_tokens", ThinkingFormat: "xai"}, Reasoning: true, Input: []string{"text", "image"}, Cost: ModelCost{Input: 1.25, Output: 2.50, CacheRead: 0.20, CacheWrite: 0}, ContextWindow: 1000000, ThinkingLevelMap: map[string]*string{"off": ptrString("none"), "minimal": nil, "low": ptrString("low"), "medium": ptrString("medium"), "high": ptrString("high"), "xhigh": nil}})
	RegisterModel(Model{ID: "big-pickle", Name: "Big Pickle", Provider: "opencode", API: "openai-completions", BaseURL: "https://opencode.ai/zen/v1", Reasoning: true, Input: []string{"text"}, ContextWindow: 200000, MaxTokens: 128000})
	RegisterModel(Model{ID: "claude-haiku-4-5", Name: "Claude Haiku 4.5", Provider: "opencode", API: "anthropic-messages", BaseURL: "https://opencode.ai/zen", Reasoning: true, Input: []string{"text", "image"}, Cost: ModelCost{Input: 1, Output: 5, CacheRead: 0.1, CacheWrite: 1.25}, ContextWindow: 200000, MaxTokens: 64000})
	RegisterModel(Model{ID: "deepseek-v4-flash", Name: "DeepSeek V4 Flash", Provider: "opencode-go", API: "openai-completions", BaseURL: "https://opencode.ai/zen/go/v1", Compat: ModelCompat{RequiresReasoningContentOnAssistantTurns: ptrBool(true), ThinkingFormat: "deepseek"}, Reasoning: true, Input: []string{"text"}, Cost: ModelCost{Input: 0.14, Output: 0.28, CacheRead: 0.0028, CacheWrite: 0}, ContextWindow: 1000000, MaxTokens: 384000, ThinkingLevelMap: map[string]*string{"minimal": nil, "low": nil, "medium": nil, "high": ptrString("high"), "xhigh": ptrString("max")}})
	RegisterModel(Model{ID: "deepseek/deepseek-v4-flash", Name: "deepseek/deepseek-v4-flash", Provider: "openrouter", API: "openai-completions", Reasoning: true, Input: []string{"text"}, ThinkingLevelMap: map[string]*string{"minimal": nil, "low": nil, "medium": nil, "high": ptrString("high"), "xhigh": ptrString("xhigh")}})
	RegisterModel(Model{ID: "anthropic/claude-opus-4.6", Name: "anthropic/claude-opus-4.6", Provider: "openrouter", API: "openai-completions", Reasoning: true, Input: []string{"text"}, ThinkingLevelMap: map[string]*string{"xhigh": ptrString("xhigh")}})
	RegisterModel(Model{ID: "anthropic/claude-sonnet-4", Name: "Anthropic: Claude Sonnet 4", Provider: "openrouter", API: "openai-completions", BaseURL: "https://openrouter.ai/api/v1", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 1000000, MaxTokens: 64000})
	RegisterModel(Model{ID: "claude-sonnet-4.6", Name: "Claude Sonnet 4.6", Provider: "github-copilot", API: "anthropic-messages", BaseURL: "https://api.individual.githubcopilot.com", Headers: map[string]string{"User-Agent": "GitHubCopilotChat/1.0", "Copilot-Integration-Id": "vscode-chat"}, Compat: ModelCompat{SupportsEagerToolInputStreaming: ptrBool(false)}, Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 200000, MaxTokens: 32000})
	RegisterModel(Model{ID: "deepseek/deepseek-r1", Name: "DeepSeek: R1", Provider: "openrouter", API: "openai-completions", BaseURL: "https://openrouter.ai/api/v1", Reasoning: true, Input: []string{"text"}, ContextWindow: 163840, MaxTokens: 16000})
	RegisterModel(Model{ID: "accounts/fireworks/models/kimi-k2p6", Name: "Kimi K2.6", Provider: "fireworks", API: "anthropic-messages", BaseURL: "https://api.fireworks.ai/inference", Compat: ModelCompat{SendSessionAffinityHeaders: ptrBool(true), SupportsEagerToolInputStreaming: ptrBool(false), SupportsCacheControlOnTools: ptrBool(false), SupportsLongCacheRetention: ptrBool(false)}, Reasoning: true, Input: []string{"text", "image"}, Cost: ModelCost{Input: 0.95, Output: 4, CacheRead: 0.16, CacheWrite: 0}, ContextWindow: 262000, MaxTokens: 262000})
	RegisterModel(Model{ID: "accounts/fireworks/routers/kimi-k2p5-turbo", Name: "Kimi K2.5 Turbo", Provider: "fireworks", API: "anthropic-messages", BaseURL: "https://api.fireworks.ai/inference", Compat: ModelCompat{SendSessionAffinityHeaders: ptrBool(true), SupportsEagerToolInputStreaming: ptrBool(false), SupportsCacheControlOnTools: ptrBool(false), SupportsLongCacheRetention: ptrBool(false)}, Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 262000, MaxTokens: 262000})
	RegisterModel(Model{ID: "openai/gpt-oss-20b", Name: "GPT OSS 20B", Provider: "groq", API: "openai-completions", BaseURL: "https://api.groq.com/openai/v1", Reasoning: true, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 65536})
	RegisterModel(Model{ID: "qwen/qwen3-32b", Name: "Qwen3 32B", Provider: "groq", API: "openai-completions", BaseURL: "https://api.groq.com/openai/v1", Reasoning: true, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 40960, ThinkingLevelMap: map[string]*string{"minimal": nil, "low": nil, "medium": nil, "high": ptrString("default")}})
	RegisterModel(Model{ID: "moonshotai/Kimi-K2.6", Name: "Kimi-K2.6", Provider: "together", API: "openai-completions", BaseURL: "https://api.together.ai/v1", Compat: ModelCompat{SupportsStore: ptrBool(false), SupportsDeveloperRole: ptrBool(false), SupportsReasoningEffort: ptrBool(false), MaxTokensField: "max_tokens", ThinkingFormat: "together", SupportsStrictMode: ptrBool(false), SupportsLongCacheRetention: ptrBool(false)}, Reasoning: true, ThinkingLevelMap: map[string]*string{"minimal": nil, "low": nil, "medium": nil}, Input: []string{"text", "image"}, Cost: ModelCost{Input: 1.2, Output: 4.5, CacheRead: 0.2, CacheWrite: 0}, ContextWindow: 262144, MaxTokens: 131000})
	RegisterModel(Model{ID: "openai/gpt-oss-120b", Name: "GPT OSS 120B", Provider: "together", API: "openai-completions", BaseURL: "https://api.together.ai/v1", Compat: ModelCompat{SupportsStore: ptrBool(false), SupportsDeveloperRole: ptrBool(false), SupportsReasoningEffort: ptrBool(true), MaxTokensField: "max_tokens", ThinkingFormat: "openai", SupportsStrictMode: ptrBool(false), SupportsLongCacheRetention: ptrBool(false)}, Reasoning: true, ThinkingLevelMap: map[string]*string{"off": nil, "minimal": nil}, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 131072})
	RegisterModel(Model{ID: "deepseek-ai/DeepSeek-V4-Pro", Name: "DeepSeek V4 Pro", Provider: "together", API: "openai-completions", BaseURL: "https://api.together.ai/v1", Compat: ModelCompat{SupportsStore: ptrBool(false), SupportsDeveloperRole: ptrBool(false), SupportsReasoningEffort: ptrBool(true), MaxTokensField: "max_tokens", ThinkingFormat: "together", SupportsStrictMode: ptrBool(false), SupportsLongCacheRetention: ptrBool(false)}, Reasoning: true, ThinkingLevelMap: map[string]*string{"minimal": nil, "low": nil, "medium": nil, "high": ptrString("high"), "xhigh": nil}, Input: []string{"text"}, ContextWindow: 256000, MaxTokens: 131000})
	RegisterModel(Model{ID: "MiniMaxAI/MiniMax-M2.7", Name: "MiniMax M2.7", Provider: "together", API: "openai-completions", BaseURL: "https://api.together.ai/v1", Compat: ModelCompat{SupportsStore: ptrBool(false), SupportsDeveloperRole: ptrBool(false), SupportsReasoningEffort: ptrBool(false), MaxTokensField: "max_tokens", SupportsStrictMode: ptrBool(false), SupportsLongCacheRetention: ptrBool(false)}, Reasoning: true, ThinkingLevelMap: map[string]*string{"off": nil, "minimal": nil, "low": nil, "medium": nil}, Input: []string{"text"}, ContextWindow: 200000, MaxTokens: 65536})
	RegisterModel(Model{ID: "glm-4.5-air", Name: "GLM-4.5-Air", Provider: "zai", API: "openai-completions", BaseURL: "https://api.z.ai/api/coding/paas/v4", Compat: ModelCompat{SupportsDeveloperRole: ptrBool(false), ThinkingFormat: "zai"}, Reasoning: true, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 98304})
	RegisterModel(Model{ID: "glm-4.7", Name: "GLM-4.7", Provider: "zai", API: "openai-completions", BaseURL: "https://api.z.ai/api/coding/paas/v4", Compat: ModelCompat{SupportsDeveloperRole: ptrBool(false), ThinkingFormat: "zai", ZAIToolStream: ptrBool(true)}, Reasoning: true, Input: []string{"text"}, ContextWindow: 204800, MaxTokens: 131072})
	RegisterModel(Model{ID: "glm-5-turbo", Name: "GLM-5-Turbo", Provider: "zai", API: "openai-completions", BaseURL: "https://api.z.ai/api/coding/paas/v4", Compat: ModelCompat{SupportsDeveloperRole: ptrBool(false), ThinkingFormat: "zai", ZAIToolStream: ptrBool(true)}, Reasoning: true, Input: []string{"text"}, ContextWindow: 200000, MaxTokens: 131072})
	RegisterModel(Model{ID: "glm-5.1", Name: "GLM-5.1", Provider: "zai", API: "openai-completions", BaseURL: "https://api.z.ai/api/coding/paas/v4", Compat: ModelCompat{SupportsDeveloperRole: ptrBool(false), ThinkingFormat: "zai", ZAIToolStream: ptrBool(true)}, Reasoning: true, Input: []string{"text"}, ContextWindow: 200000, MaxTokens: 131072})
	RegisterModel(Model{ID: "devstral-medium-latest", Name: "Devstral 2 (latest)", Provider: "mistral", API: "mistral-conversations", BaseURL: "https://api.mistral.ai", Input: []string{"text"}, ContextWindow: 262144, MaxTokens: 262144})
	RegisterModel(Model{ID: "magistral-medium-latest", Name: "Magistral Medium (latest)", Provider: "mistral", API: "mistral-conversations", BaseURL: "https://api.mistral.ai", Reasoning: true, Input: []string{"text"}, ContextWindow: 128000, MaxTokens: 16384})
	RegisterModel(Model{ID: "mistral-medium-3.5", Name: "Mistral Medium 3.5", Provider: "mistral", API: "mistral-conversations", BaseURL: "https://api.mistral.ai", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 262144, MaxTokens: 262144})
	RegisterModel(Model{ID: "mistral-small-2603", Name: "Mistral Small 4", Provider: "mistral", API: "mistral-conversations", BaseURL: "https://api.mistral.ai", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 256000, MaxTokens: 256000})
	RegisterModel(Model{ID: "global.anthropic.claude-opus-4-6-v1", Name: "Claude Opus 4.6 (Global)", Provider: "amazon-bedrock", API: "bedrock-converse-stream", BaseURL: "https://bedrock-runtime.us-east-1.amazonaws.com", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 200000, MaxTokens: 32000})
	RegisterModel(Model{ID: "us.anthropic.claude-sonnet-4-5-20250929-v1:0", Name: "Claude Sonnet 4.5", Provider: "amazon-bedrock", API: "bedrock-converse-stream", BaseURL: "https://bedrock-runtime.us-east-1.amazonaws.com", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 200000, MaxTokens: 32000})
	RegisterModel(Model{ID: "us.anthropic.claude-opus-4-7", Name: "Claude Opus 4.7", Provider: "amazon-bedrock", API: "bedrock-converse-stream", BaseURL: "https://bedrock-runtime.us-east-1.amazonaws.com", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 200000, MaxTokens: 64000, ThinkingLevelMap: map[string]*string{"xhigh": ptrString("xhigh")}})
	RegisterModel(Model{ID: "eu.anthropic.claude-sonnet-4-5-20250929-v1:0", Name: "Claude Sonnet 4.5 (EU)", Provider: "amazon-bedrock", API: "bedrock-converse-stream", BaseURL: "https://bedrock-runtime.eu-central-1.amazonaws.com", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 200000, MaxTokens: 64000})
}

func RegisterModel(model Model) {
	if model.Provider == "" {
		return
	}
	if modelRegistry[model.Provider] == nil {
		modelRegistry[model.Provider] = map[string]Model{}
	}
	modelRegistry[model.Provider][model.ID] = model
}

func GetModel(provider, modelID string) (Model, bool) {
	if models, ok := modelRegistry[provider]; ok {
		if model, ok := models[modelID]; ok {
			return model, true
		}
	}
	return Model{
		ID:            modelID,
		Name:          modelID,
		Provider:      provider,
		API:           defaultAPIForProvider(provider),
		Input:         []string{"text"},
		ContextWindow: 0,
		MaxTokens:     0,
	}, false
}

func MustGetModel(provider, modelID string) Model {
	model, _ := GetModel(provider, modelID)
	return model
}

func GetProviders() []string {
	providers := make([]string, 0, len(modelRegistry))
	for provider := range modelRegistry {
		providers = append(providers, provider)
	}
	return providers
}

func GetModels(provider string) []Model {
	models := modelRegistry[provider]
	result := make([]Model, 0, len(models))
	for _, model := range models {
		result = append(result, model)
	}
	return result
}

func CalculateCost(model Model, usage Usage) UsageCost {
	usage.Cost.Input = model.Cost.Input / 1_000_000 * float64(usage.Input)
	usage.Cost.Output = model.Cost.Output / 1_000_000 * float64(usage.Output)
	usage.Cost.CacheRead = model.Cost.CacheRead / 1_000_000 * float64(usage.CacheRead)
	usage.Cost.CacheWrite = model.Cost.CacheWrite / 1_000_000 * float64(usage.CacheWrite)
	usage.Cost.Total = usage.Cost.Input + usage.Cost.Output + usage.Cost.CacheRead + usage.Cost.CacheWrite
	return usage.Cost
}

func GetSupportedThinkingLevels(model Model) []string {
	if !model.Reasoning {
		return []string{"off"}
	}
	all := []string{"off", "minimal", "low", "medium", "high", "xhigh"}
	result := make([]string, 0, len(all))
	for _, level := range all {
		mapped, hasMapping := model.ThinkingLevelMap[level]
		if hasMapping && mapped == nil {
			continue
		}
		if level == "xhigh" && !hasMapping {
			continue
		}
		result = append(result, level)
	}
	return result
}

func ClampThinkingLevel(model Model, level string) string {
	levels := GetSupportedThinkingLevels(model)
	for _, candidate := range levels {
		if candidate == level {
			return level
		}
	}
	order := []string{"off", "minimal", "low", "medium", "high", "xhigh"}
	index := -1
	for i, candidate := range order {
		if candidate == level {
			index = i
			break
		}
	}
	if index == -1 {
		return levels[0]
	}
	for i := index; i < len(order); i++ {
		if containsString(levels, order[i]) {
			return order[i]
		}
	}
	for i := index - 1; i >= 0; i-- {
		if containsString(levels, order[i]) {
			return order[i]
		}
	}
	return levels[0]
}

func ValidateThinkingLevelSupported(model Model, level string) error {
	if level == "" {
		return nil
	}
	for _, supported := range GetSupportedThinkingLevels(model) {
		if supported == level {
			return nil
		}
	}
	return fmt.Errorf("thinking level %q is not supported by model %s", level, model.ID)
}

func ModelsAreEqual(a, b *Model) bool {
	return a != nil && b != nil && a.ID == b.ID && a.Provider == b.Provider
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func defaultAPIForProvider(provider string) string {
	switch provider {
	case "anthropic":
		return "anthropic-messages"
	case "google":
		return "google-generative-ai"
	case "google-vertex":
		return "google-vertex"
	case "openai-codex":
		return "openai-codex-responses"
	case "azure-openai-responses":
		return "azure-openai-responses"
	case "mistral":
		return "mistral-conversations"
	default:
		return "openai-completions"
	}
}
