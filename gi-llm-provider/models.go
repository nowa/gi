package gillmprovider

import "fmt"

func ptrString(value string) *string { return &value }
func ptrBool(value bool) *bool       { return &value }

var (
	modelRegistry      = map[string]map[string]Model{}
	modelProviderOrder []string
	modelOrder         = map[string][]string{}
)

func init() {
	registerPiGeneratedModels()
}

func RegisterModel(model Model) {
	if model.Provider == "" {
		return
	}
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
	modelRegistry = map[string]map[string]Model{}
	modelProviderOrder = nil
	modelOrder = map[string][]string{}
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
	return append([]string(nil), modelProviderOrder...)
}

func GetModels(provider string) []Model {
	models := modelRegistry[provider]
	result := make([]Model, 0, len(models))
	for _, id := range modelOrder[provider] {
		if model, ok := models[id]; ok {
			result = append(result, model)
		}
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
