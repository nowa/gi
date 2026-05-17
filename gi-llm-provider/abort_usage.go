package gillmprovider

type AbortUsageExpectation string

const (
	AbortUsageFinalOnly   AbortUsageExpectation = "final-only"
	AbortUsageInputOnly   AbortUsageExpectation = "input-only"
	AbortUsageIncremental AbortUsageExpectation = "incremental"
)

func ExpectedAbortUsage(model Model) AbortUsageExpectation {
	switch model.API {
	case "openai-completions", "mistral-conversations", "openai-responses", "azure-openai-responses", "openai-codex-responses":
		return AbortUsageFinalOnly
	}
	switch model.Provider {
	case "zai", "amazon-bedrock", "vercel-ai-gateway", "minimax":
		return AbortUsageFinalOnly
	case "kimi-coding":
		return AbortUsageInputOnly
	default:
		return AbortUsageIncremental
	}
}
