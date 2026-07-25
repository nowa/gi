package gicodingagent

import (
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

var resolverMockModels = []llm.Model{
	{
		ID:            "claude-sonnet-4-5",
		Name:          "Claude Sonnet 4.5",
		API:           "anthropic-messages",
		Provider:      "anthropic",
		BaseURL:       "https://api.anthropic.com",
		Reasoning:     true,
		Input:         []string{"text", "image"},
		Cost:          llm.ModelCost{Input: 3, Output: 15, CacheRead: 0.3, CacheWrite: 3.75},
		ContextWindow: 200000,
		MaxTokens:     8192,
	},
	{
		ID:            "gpt-4o",
		Name:          "GPT-4o",
		API:           "anthropic-messages",
		Provider:      "openai",
		BaseURL:       "https://api.openai.com",
		Reasoning:     false,
		Input:         []string{"text", "image"},
		Cost:          llm.ModelCost{Input: 5, Output: 15, CacheRead: 0.5, CacheWrite: 5},
		ContextWindow: 128000,
		MaxTokens:     4096,
	},
}

var resolverOpenRouterMockModels = []llm.Model{
	{
		ID:            "qwen/qwen3-coder:exacto",
		Name:          "Qwen3 Coder Exacto",
		API:           "anthropic-messages",
		Provider:      "openrouter",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     true,
		Input:         []string{"text"},
		Cost:          llm.ModelCost{Input: 1, Output: 2, CacheRead: 0.1, CacheWrite: 1},
		ContextWindow: 128000,
		MaxTokens:     8192,
	},
	{
		ID:            "openai/gpt-4o:extended",
		Name:          "GPT-4o Extended",
		API:           "anthropic-messages",
		Provider:      "openrouter",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     false,
		Input:         []string{"text", "image"},
		Cost:          llm.ModelCost{Input: 5, Output: 15, CacheRead: 0.5, CacheWrite: 5},
		ContextWindow: 128000,
		MaxTokens:     4096,
	},
}

var resolverAllModels = append(append([]llm.Model{}, resolverMockModels...), resolverOpenRouterMockModels...)

type resolverTestRegistry struct {
	all       []llm.Model
	available []llm.Model
}

func (r resolverTestRegistry) GetAll() []llm.Model {
	return append([]llm.Model{}, r.all...)
}

func (r resolverTestRegistry) GetAvailable() []llm.Model {
	return append([]llm.Model{}, r.available...)
}

func (r resolverTestRegistry) Find(provider, modelID string) (llm.Model, bool) {
	for _, model := range r.all {
		if model.Provider == provider && model.ID == modelID {
			return model, true
		}
	}
	for _, model := range r.available {
		if model.Provider == provider && model.ID == modelID {
			return model, true
		}
	}
	return llm.Model{}, false
}

func TestParseModelPatternCompatibilityCases(t *testing.T) {
	t.Run("simple patterns without colons", func(t *testing.T) {
		result := ParseModelPattern("claude-sonnet-4-5", resolverAllModels)
		assertResolvedModel(t, result.Model, "anthropic", "claude-sonnet-4-5")
		if result.ThinkingLevel != "" || result.Warning != "" {
			t.Fatalf("thinking/warning = %q / %q", result.ThinkingLevel, result.Warning)
		}

		result = ParseModelPattern("sonnet", resolverAllModels)
		assertResolvedModel(t, result.Model, "anthropic", "claude-sonnet-4-5")
		if result.ThinkingLevel != "" || result.Warning != "" {
			t.Fatalf("thinking/warning = %q / %q", result.ThinkingLevel, result.Warning)
		}

		result = ParseModelPattern("nonexistent", resolverAllModels)
		if result.Model != nil || result.ThinkingLevel != "" || result.Warning != "" {
			t.Fatalf("nonexistent = %#v", result)
		}
	})

	t.Run("patterns with valid thinking levels", func(t *testing.T) {
		result := ParseModelPattern("sonnet:high", resolverAllModels)
		assertResolvedModel(t, result.Model, "anthropic", "claude-sonnet-4-5")
		if result.ThinkingLevel != ThinkingHigh || result.Warning != "" {
			t.Fatalf("thinking/warning = %q / %q", result.ThinkingLevel, result.Warning)
		}

		result = ParseModelPattern("gpt-4o:medium", resolverAllModels)
		assertResolvedModel(t, result.Model, "openai", "gpt-4o")
		if result.ThinkingLevel != ThinkingMedium || result.Warning != "" {
			t.Fatalf("thinking/warning = %q / %q", result.ThinkingLevel, result.Warning)
		}

		for _, level := range []ThinkingLevel{ThinkingOff, ThinkingMinimal, ThinkingLow, ThinkingMedium, ThinkingHigh, ThinkingXHigh, ThinkingMax} {
			result := ParseModelPattern("sonnet:"+string(level), resolverAllModels)
			assertResolvedModel(t, result.Model, "anthropic", "claude-sonnet-4-5")
			if result.ThinkingLevel != level || result.Warning != "" {
				t.Fatalf("level %q -> %#v", level, result)
			}
		}
	})

	t.Run("patterns with invalid thinking levels", func(t *testing.T) {
		result := ParseModelPattern("sonnet:random", resolverAllModels)
		assertResolvedModel(t, result.Model, "anthropic", "claude-sonnet-4-5")
		if result.ThinkingLevel != "" || !strings.Contains(result.Warning, "Invalid thinking level") || !strings.Contains(result.Warning, "random") {
			t.Fatalf("invalid suffix = %#v", result)
		}

		result = ParseModelPattern("gpt-4o:invalid", resolverAllModels)
		assertResolvedModel(t, result.Model, "openai", "gpt-4o")
		if result.ThinkingLevel != "" || !strings.Contains(result.Warning, "Invalid thinking level") {
			t.Fatalf("invalid suffix = %#v", result)
		}
	})

	t.Run("OpenRouter models with colons in IDs", func(t *testing.T) {
		result := ParseModelPattern("qwen/qwen3-coder:exacto", resolverAllModels)
		assertResolvedModel(t, result.Model, "openrouter", "qwen/qwen3-coder:exacto")
		if result.ThinkingLevel != "" || result.Warning != "" {
			t.Fatalf("openrouter exact = %#v", result)
		}

		result = ParseModelPattern("openrouter/qwen/qwen3-coder:exacto", resolverAllModels)
		assertResolvedModel(t, result.Model, "openrouter", "qwen/qwen3-coder:exacto")
		if result.ThinkingLevel != "" || result.Warning != "" {
			t.Fatalf("openrouter provider exact = %#v", result)
		}

		result = ParseModelPattern("qwen/qwen3-coder:exacto:high", resolverAllModels)
		assertResolvedModel(t, result.Model, "openrouter", "qwen/qwen3-coder:exacto")
		if result.ThinkingLevel != ThinkingHigh || result.Warning != "" {
			t.Fatalf("openrouter thinking = %#v", result)
		}

		result = ParseModelPattern("openrouter/qwen/qwen3-coder:exacto:high", resolverAllModels)
		assertResolvedModel(t, result.Model, "openrouter", "qwen/qwen3-coder:exacto")
		if result.ThinkingLevel != ThinkingHigh || result.Warning != "" {
			t.Fatalf("openrouter provider thinking = %#v", result)
		}

		result = ParseModelPattern("openai/gpt-4o:extended", resolverAllModels)
		assertResolvedModel(t, result.Model, "openrouter", "openai/gpt-4o:extended")
		if result.ThinkingLevel != "" || result.Warning != "" {
			t.Fatalf("openrouter openai id = %#v", result)
		}
	})

	t.Run("invalid thinking levels with OpenRouter models", func(t *testing.T) {
		result := ParseModelPattern("qwen/qwen3-coder:exacto:random", resolverAllModels)
		assertResolvedModel(t, result.Model, "openrouter", "qwen/qwen3-coder:exacto")
		if result.ThinkingLevel != "" || !strings.Contains(result.Warning, "Invalid thinking level") || !strings.Contains(result.Warning, "random") {
			t.Fatalf("invalid openrouter suffix = %#v", result)
		}

		result = ParseModelPattern("qwen/qwen3-coder:exacto:high:random", resolverAllModels)
		assertResolvedModel(t, result.Model, "openrouter", "qwen/qwen3-coder:exacto")
		if result.ThinkingLevel != "" || !strings.Contains(result.Warning, "Invalid thinking level") || !strings.Contains(result.Warning, "random") {
			t.Fatalf("nested invalid openrouter suffix = %#v", result)
		}
	})

	t.Run("edge cases", func(t *testing.T) {
		result := ParseModelPattern("", resolverAllModels)
		if result.Model == nil || result.ThinkingLevel != "" {
			t.Fatalf("empty pattern = %#v", result)
		}

		result = ParseModelPattern("sonnet:", resolverAllModels)
		assertResolvedModel(t, result.Model, "anthropic", "claude-sonnet-4-5")
		if !strings.Contains(result.Warning, "Invalid thinking level") {
			t.Fatalf("trailing colon = %#v", result)
		}
	})
}

func TestResolveModelScopePatterns(t *testing.T) {
	registry := resolverTestRegistry{
		all:       resolverAllModels,
		available: resolverAllModels,
	}

	scoped := ResolveModelScope([]string{
		"openai/*",
		"sonnet:high",
		"openrouter/*coder*:low",
		"missing-*",
	}, registry)
	if len(scoped) != 4 {
		t.Fatalf("scoped models = %#v, want 4 matches", scoped)
	}
	assertScopedModel(t, scoped[0], "openai", "gpt-4o", "")
	assertScopedModel(t, scoped[1], "openrouter", "openai/gpt-4o:extended", "")
	assertScopedModel(t, scoped[2], "anthropic", "claude-sonnet-4-5", ThinkingHigh)
	assertScopedModel(t, scoped[3], "openrouter", "qwen/qwen3-coder:exacto", ThinkingLow)

	scoped = ResolveModelScope([]string{"sonnet", "anthropic/claude-sonnet-4-5"}, registry)
	if len(scoped) != 1 {
		t.Fatalf("duplicate scoped models = %#v, want 1", scoped)
	}
	assertScopedModel(t, scoped[0], "anthropic", "claude-sonnet-4-5", "")
}

func TestResolveModelScopeWithDiagnosticsPreservesPiDataFlow(t *testing.T) {
	literalGlobModel := resolverMockModels[0]
	literalGlobModel.Provider = "custom"
	literalGlobModel.ID = "bracketed-model[1m]"
	literalGlobModel.Name = "Bracketed Model"

	registry := &countingResolverRegistry{
		resolverTestRegistry: resolverTestRegistry{
			all:       append(append([]llm.Model{}, resolverAllModels...), literalGlobModel),
			available: append(append([]llm.Model{}, resolverAllModels...), literalGlobModel),
		},
	}
	result := ResolveModelScopeWithDiagnostics([]string{
		"sonnet:high",
		"gpt-4o:invalid",
		"missing-*",
		"custom/bracketed-model[1m]:low",
		"anthropic/claude-sonnet-4-5",
	}, registry)

	if registry.availableCalls != 1 {
		t.Fatalf("GetAvailable calls = %d, want one immutable catalog snapshot", registry.availableCalls)
	}
	if len(result.ScopedModels) != 3 {
		t.Fatalf("scoped models = %#v, want 3", result.ScopedModels)
	}
	assertScopedModel(t, result.ScopedModels[0], "anthropic", "claude-sonnet-4-5", ThinkingHigh)
	assertScopedModel(t, result.ScopedModels[1], "openai", "gpt-4o", "")
	assertScopedModel(t, result.ScopedModels[2], "custom", "bracketed-model[1m]", ThinkingLow)

	if len(result.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want invalid-level and no-match warnings", result.Diagnostics)
	}
	if diagnostic := result.Diagnostics[0]; diagnostic.Type != ModelScopeDiagnosticWarning ||
		diagnostic.Pattern != "gpt-4o:invalid" ||
		!strings.Contains(diagnostic.Message, `Invalid thinking level "invalid"`) {
		t.Fatalf("invalid-level diagnostic = %#v", diagnostic)
	}
	if diagnostic := result.Diagnostics[1]; diagnostic.Type != ModelScopeDiagnosticWarning ||
		diagnostic.Pattern != "missing-*" ||
		diagnostic.Message != `No models match pattern "missing-*"` {
		t.Fatalf("no-match diagnostic = %#v", diagnostic)
	}
}

type countingResolverRegistry struct {
	resolverTestRegistry
	availableCalls int
}

func (r *countingResolverRegistry) GetAvailable() []llm.Model {
	r.availableCalls++
	return r.resolverTestRegistry.GetAvailable()
}

func TestResolveCLIModelCompatibilityCases(t *testing.T) {
	registry := resolverTestRegistry{all: resolverAllModels}

	result := ResolveCLIModel(ResolveCLIModelOptions{CLIModel: "openai/gpt-4o", ModelRegistry: registry})
	if result.Error != "" {
		t.Fatalf("provider/id error = %s", result.Error)
	}
	assertResolvedModel(t, result.Model, "openai", "gpt-4o")

	result = ResolveCLIModel(ResolveCLIModelOptions{CLIProvider: "openai", CLIModel: "4o", ModelRegistry: registry})
	if result.Error != "" {
		t.Fatalf("fuzzy provider error = %s", result.Error)
	}
	assertResolvedModel(t, result.Model, "openai", "gpt-4o")

	result = ResolveCLIModel(ResolveCLIModelOptions{CLIModel: "sonnet:high", ModelRegistry: registry})
	if result.Error != "" {
		t.Fatalf("thinking error = %s", result.Error)
	}
	assertResolvedModel(t, result.Model, "anthropic", "claude-sonnet-4-5")
	if result.ThinkingLevel != ThinkingHigh {
		t.Fatalf("thinking level = %q", result.ThinkingLevel)
	}

	result = ResolveCLIModel(ResolveCLIModelOptions{CLIModel: "openai/gpt-4o:extended", ModelRegistry: registry})
	if result.Error != "" {
		t.Fatalf("openrouter exact error = %s", result.Error)
	}
	assertResolvedModel(t, result.Model, "openrouter", "openai/gpt-4o:extended")

	result = ResolveCLIModel(ResolveCLIModelOptions{CLIProvider: "openai", CLIModel: "gpt-4o:extended", ModelRegistry: registry})
	if result.Error != "" {
		t.Fatalf("custom raw id error = %s", result.Error)
	}
	assertResolvedModel(t, result.Model, "openai", "gpt-4o:extended")

	result = ResolveCLIModel(ResolveCLIModelOptions{CLIProvider: "openrouter", CLIModel: "openrouter/openai/ghost-model", ModelRegistry: registry})
	if result.Error != "" {
		t.Fatalf("custom prefixed id error = %s", result.Error)
	}
	assertResolvedModel(t, result.Model, "openrouter", "openai/ghost-model")

	result = ResolveCLIModel(ResolveCLIModelOptions{CLIProvider: "openai", CLIModel: "gpt-4o", ModelRegistry: resolverTestRegistry{}})
	if result.Model != nil || !strings.Contains(result.Error, "No models available") {
		t.Fatalf("empty registry result = %#v", result)
	}

	zaiModel := llm.Model{ID: "glm-5", Name: "GLM-5", API: "anthropic-messages", Provider: "zai", Reasoning: true, Input: []string{"text"}}
	gatewayModel := llm.Model{ID: "zai/glm-5", Name: "GLM-5", API: "anthropic-messages", Provider: "vercel-ai-gateway", Reasoning: true, Input: []string{"text"}}
	result = ResolveCLIModel(ResolveCLIModelOptions{
		CLIModel:      "zai/glm-5",
		ModelRegistry: resolverTestRegistry{all: append(append([]llm.Model{}, resolverAllModels...), zaiModel, gatewayModel)},
	})
	if result.Error != "" {
		t.Fatalf("provider split preference error = %s", result.Error)
	}
	assertResolvedModel(t, result.Model, "zai", "glm-5")

	result = ResolveCLIModel(ResolveCLIModelOptions{CLIModel: "openrouter/qwen", ModelRegistry: registry})
	if result.Error != "" {
		t.Fatalf("provider-prefixed fuzzy error = %s", result.Error)
	}
	assertResolvedModel(t, result.Model, "openrouter", "qwen/qwen3-coder:exacto")
}

func TestDefaultModelSelectionCompatibilityCases(t *testing.T) {
	if DefaultModelPerProvider["openai"] != "gpt-5.4" || DefaultModelPerProvider["openai-codex"] != "gpt-5.5" {
		t.Fatalf("openai defaults = %#v", DefaultModelPerProvider)
	}
	if DefaultModelPerProvider["zai"] != "glm-5.1" ||
		DefaultModelPerProvider["minimax"] != "MiniMax-M2.7" ||
		DefaultModelPerProvider["minimax-cn"] != "MiniMax-M2.7" ||
		DefaultModelPerProvider["cerebras"] != "zai-glm-4.7" {
		t.Fatalf("provider defaults = %#v", DefaultModelPerProvider)
	}
	if DefaultModelPerProvider["vercel-ai-gateway"] != "zai/glm-5.1" {
		t.Fatalf("ai-gateway default = %q", DefaultModelPerProvider["vercel-ai-gateway"])
	}

	result := FindInitialModel(FindInitialModelOptions{
		CLIProvider:   "openrouter",
		CLIModel:      "openrouter/openai/ghost-model",
		ScopedModels:  []ScopedModel{},
		IsContinuing:  false,
		ModelRegistry: resolverTestRegistry{all: resolverAllModels},
	})
	assertResolvedModel(t, result.Model, "openrouter", "openai/ghost-model")

	aiGatewayModel := llm.Model{
		ID:            "anthropic/claude-opus-4-6",
		Name:          "Claude Opus 4.6",
		API:           "anthropic-messages",
		Provider:      "vercel-ai-gateway",
		BaseURL:       "https://ai-gateway.vercel.sh",
		Reasoning:     true,
		Input:         []string{"text", "image"},
		Cost:          llm.ModelCost{Input: 5, Output: 15, CacheRead: 0.5, CacheWrite: 5},
		ContextWindow: 200000,
		MaxTokens:     8192,
	}
	result = FindInitialModel(FindInitialModelOptions{
		ScopedModels:  []ScopedModel{},
		IsContinuing:  false,
		ModelRegistry: resolverTestRegistry{available: []llm.Model{aiGatewayModel}},
	})
	assertResolvedModel(t, result.Model, "vercel-ai-gateway", "anthropic/claude-opus-4-6")
}

func TestFindInitialModelUsesFirstScopedModelBeforeSettingsDefaultPiParity(t *testing.T) {
	result := FindInitialModel(FindInitialModelOptions{
		ScopedModels: []ScopedModel{
			{Model: resolverMockModels[0], ThinkingLevel: ThinkingHigh},
			{Model: resolverMockModels[1], ThinkingLevel: ThinkingOff},
		},
		DefaultProvider:      resolverMockModels[1].Provider,
		DefaultModelID:       resolverMockModels[1].ID,
		DefaultThinkingLevel: ThinkingLow,
		ModelRegistry: resolverTestRegistry{
			all:       resolverAllModels,
			available: resolverAllModels,
		},
	})

	assertResolvedModel(t, result.Model, resolverMockModels[0].Provider, resolverMockModels[0].ID)
	if result.ThinkingLevel != ThinkingHigh {
		t.Fatalf("thinking level = %q, want first scoped model level %q", result.ThinkingLevel, ThinkingHigh)
	}
}

func assertResolvedModel(t *testing.T, model *llm.Model, provider, id string) {
	t.Helper()
	if model == nil {
		t.Fatalf("model is nil, want %s/%s", provider, id)
	}
	if model.Provider != provider || model.ID != id {
		t.Fatalf("model = %s/%s, want %s/%s", model.Provider, model.ID, provider, id)
	}
}

func assertScopedModel(t *testing.T, scoped ScopedModel, provider, id string, thinking ThinkingLevel) {
	t.Helper()
	if scoped.Model.Provider != provider || scoped.Model.ID != id || scoped.ThinkingLevel != thinking {
		t.Fatalf("scoped model = %s/%s:%s, want %s/%s:%s", scoped.Model.Provider, scoped.Model.ID, scoped.ThinkingLevel, provider, id, thinking)
	}
}
