package gillmprovider

import (
	"fmt"
	"sync"
)

func ptrString(value string) *string { return &value }
func ptrBool(value bool) *bool       { return &value }
func ptrInt(value int) *int          { return &value }

var (
	modelRegistryMu    sync.RWMutex
	modelRegistry      = map[string]map[string]Model{}
	modelProviderOrder []string
	modelOrder         = map[string][]string{}
)

func init() {
	registerPiGeneratedModels()
	captureBuiltinCatalog()
}

func RegisterModel(model Model) {
	if model.Provider == "" {
		return
	}
	model = cloneModel(model)
	modelRegistryMu.Lock()
	defer modelRegistryMu.Unlock()
	if modelRegistry[model.Provider] == nil {
		modelRegistry[model.Provider] = map[string]Model{}
		modelProviderOrder = append(modelProviderOrder, model.Provider)
	}
	if _, exists := modelRegistry[model.Provider][model.ID]; !exists {
		modelOrder[model.Provider] = append(modelOrder[model.Provider], model.ID)
	}
	modelRegistry[model.Provider][model.ID] = model
}

func resetModelRegistry() {
	modelRegistryMu.Lock()
	defer modelRegistryMu.Unlock()
	modelRegistry = map[string]map[string]Model{}
	modelProviderOrder = nil
	modelOrder = map[string][]string{}
}

func GetModel(provider, modelID string) (Model, bool) {
	modelRegistryMu.RLock()
	if models, ok := modelRegistry[provider]; ok {
		if model, ok := models[modelID]; ok {
			modelRegistryMu.RUnlock()
			return cloneModel(model), true
		}
	}
	modelRegistryMu.RUnlock()
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
	modelRegistryMu.RLock()
	providers := append([]string(nil), modelProviderOrder...)
	modelRegistryMu.RUnlock()
	return providers
}

func GetModels(provider string) []Model {
	modelRegistryMu.RLock()
	models := modelRegistry[provider]
	result := make([]Model, 0, len(models))
	for _, id := range modelOrder[provider] {
		if model, ok := models[id]; ok {
			result = append(result, cloneModel(model))
		}
	}
	modelRegistryMu.RUnlock()
	return result
}

// HasAPI reports whether a dynamically selected model uses the requested API.
func HasAPI(model Model, api string) bool {
	return model.API == api
}

func CalculateCost(model Model, usage Usage) UsageCost {
	rates := model.Cost
	inputTokens := usage.Input + usage.CacheRead + usage.CacheWrite
	matchedThreshold := -1
	for _, tier := range model.Cost.Tiers {
		if inputTokens > tier.InputTokensAbove && tier.InputTokensAbove > matchedThreshold {
			rates.Input = tier.Input
			rates.Output = tier.Output
			rates.CacheRead = tier.CacheRead
			rates.CacheWrite = tier.CacheWrite
			matchedThreshold = tier.InputTokensAbove
		}
	}

	longCacheWrite := usage.CacheWrite1h
	shortCacheWrite := usage.CacheWrite - longCacheWrite
	usage.Cost.Input = rates.Input / 1_000_000 * float64(usage.Input)
	usage.Cost.Output = rates.Output / 1_000_000 * float64(usage.Output)
	usage.Cost.CacheRead = rates.CacheRead / 1_000_000 * float64(usage.CacheRead)
	usage.Cost.CacheWrite = (rates.CacheWrite*float64(shortCacheWrite) +
		rates.Input*2*float64(longCacheWrite)) / 1_000_000
	usage.Cost.Total = usage.Cost.Input + usage.Cost.Output + usage.Cost.CacheRead + usage.Cost.CacheWrite
	return usage.Cost
}

func GetSupportedThinkingLevels(model Model) []string {
	if !model.Reasoning {
		return []string{"off"}
	}
	all := []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}
	result := make([]string, 0, len(all))
	for _, level := range all {
		mapped, hasMapping := model.ThinkingLevelMap[level]
		if hasMapping && mapped == nil {
			continue
		}
		if (level == "xhigh" || level == "max") && !hasMapping {
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
	order := []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}
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
	case "radius":
		return piMessagesAPI
	default:
		return "openai-completions"
	}
}
