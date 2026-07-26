package gillmprovider

const (
	contextSafetyTokens       = 4096
	minimumMaxTokens          = 1
	minimumThinkingOutputSize = 1024
	minimalThinkingBudget     = 1024
	lowThinkingBudget         = 2048
	mediumThinkingBudget      = 8192
	highThinkingBudget        = 16384
)

// ThinkingTokenAllocation keeps the total output cap and the reasoning-token
// portion together so provider adapters cannot accidentally update one without
// the other.
type ThinkingTokenAllocation struct {
	MaxTokens      int
	ThinkingBudget int
}

// ClampMaxTokensToContext caps an output-token budget at the estimated
// remaining context while retaining at least one output token.
func ClampMaxTokensToContext(model Model, context Context, maxTokens int) int {
	requested := max(maxTokens, minimumMaxTokens)
	if model.ContextWindow <= 0 {
		return requested
	}
	available := model.ContextWindow - EstimateContextTokens(context).Tokens - contextSafetyTokens
	return min(requested, max(available, minimumMaxTokens))
}

func prepareSimpleStreamOptions(
	model Model,
	context Context,
	options SimpleStreamOptions,
) StreamOptions {
	prepared := cloneStreamOptions(options)
	maxTokens := prepared.MaxTokens
	if maxTokens == 0 {
		maxTokens = model.MaxTokens
	}
	prepared.MaxTokens = ClampMaxTokensToContext(model, context, maxTokens)
	return prepared
}

// AdjustMaxTokensForThinking fits a provider's reasoning budget inside the
// model output cap. The returned allocation can then be constrained by the
// request's remaining context.
func AdjustMaxTokensForThinking(
	baseMaxTokens int,
	modelMaxTokens int,
	reasoningLevel string,
	customBudgets map[string]int,
) ThinkingTokenAllocation {
	level := normalizeThinkingBudgetLevel(reasoningLevel)
	thinkingBudget := defaultThinkingBudget(level)
	if customBudget, ok := customBudgets[level]; ok {
		thinkingBudget = customBudget
	}
	thinkingBudget = max(thinkingBudget, 0)

	if baseMaxTokens <= 0 && modelMaxTokens > 0 {
		baseMaxTokens = modelMaxTokens
	}
	maxTokens := baseMaxTokens + thinkingBudget
	if modelMaxTokens > 0 {
		maxTokens = min(maxTokens, modelMaxTokens)
	}
	maxTokens = max(maxTokens, minimumMaxTokens)
	if maxTokens <= thinkingBudget {
		thinkingBudget = max(0, maxTokens-minimumThinkingOutputSize)
	}
	return ThinkingTokenAllocation{
		MaxTokens:      maxTokens,
		ThinkingBudget: thinkingBudget,
	}
}

func clampThinkingTokenAllocationToContext(
	model Model,
	context Context,
	allocation ThinkingTokenAllocation,
) ThinkingTokenAllocation {
	allocation.MaxTokens = ClampMaxTokensToContext(model, context, allocation.MaxTokens)
	allocation.ThinkingBudget = min(
		allocation.ThinkingBudget,
		max(0, allocation.MaxTokens-minimumThinkingOutputSize),
	)
	return allocation
}

func normalizeThinkingBudgetLevel(level string) string {
	switch level {
	case "xhigh", "max":
		return "high"
	default:
		return level
	}
}

func defaultThinkingBudget(level string) int {
	switch level {
	case "low":
		return lowThinkingBudget
	case "medium":
		return mediumThinkingBudget
	case "high":
		return highThinkingBudget
	default:
		return minimalThinkingBudget
	}
}
