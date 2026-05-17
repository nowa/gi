package gillmprovider

import "os"

func apiKeyEnvVars(provider string) []string {
	switch provider {
	case "github-copilot":
		return []string{"COPILOT_GITHUB_TOKEN"}
	case "anthropic":
		return []string{"ANTHROPIC_OAUTH_TOKEN", "ANTHROPIC_API_KEY"}
	case "openai":
		return []string{"OPENAI_API_KEY"}
	case "azure-openai-responses":
		return []string{"AZURE_OPENAI_API_KEY"}
	case "deepseek":
		return []string{"DEEPSEEK_API_KEY"}
	case "google":
		return []string{"GEMINI_API_KEY"}
	case "google-vertex":
		return []string{"GOOGLE_CLOUD_API_KEY"}
	case "groq":
		return []string{"GROQ_API_KEY"}
	case "cerebras":
		return []string{"CEREBRAS_API_KEY"}
	case "xai":
		return []string{"XAI_API_KEY"}
	case "openrouter":
		return []string{"OPENROUTER_API_KEY"}
	case "vercel-ai-gateway":
		return []string{"AI_GATEWAY_API_KEY"}
	case "zai":
		return []string{"ZAI_API_KEY"}
	case "mistral":
		return []string{"MISTRAL_API_KEY"}
	case "minimax":
		return []string{"MINIMAX_API_KEY"}
	case "minimax-cn":
		return []string{"MINIMAX_CN_API_KEY"}
	case "moonshotai", "moonshotai-cn":
		return []string{"MOONSHOT_API_KEY"}
	case "huggingface":
		return []string{"HF_TOKEN"}
	case "fireworks":
		return []string{"FIREWORKS_API_KEY"}
	case "together":
		return []string{"TOGETHER_API_KEY"}
	case "opencode", "opencode-go":
		return []string{"OPENCODE_API_KEY"}
	case "kimi-coding":
		return []string{"KIMI_API_KEY"}
	case "cloudflare-workers-ai", "cloudflare-ai-gateway":
		return []string{"CLOUDFLARE_API_KEY"}
	case "xiaomi":
		return []string{"XIAOMI_API_KEY"}
	case "xiaomi-token-plan-cn":
		return []string{"XIAOMI_TOKEN_PLAN_CN_API_KEY"}
	case "xiaomi-token-plan-ams":
		return []string{"XIAOMI_TOKEN_PLAN_AMS_API_KEY"}
	case "xiaomi-token-plan-sgp":
		return []string{"XIAOMI_TOKEN_PLAN_SGP_API_KEY"}
	default:
		return nil
	}
}

func FindEnvKeys(provider string) []string {
	var found []string
	for _, key := range apiKeyEnvVars(provider) {
		if os.Getenv(key) != "" {
			found = append(found, key)
		}
	}
	if len(found) == 0 {
		return nil
	}
	return found
}

func GetEnvAPIKey(provider string) string {
	keys := FindEnvKeys(provider)
	if len(keys) > 0 {
		return os.Getenv(keys[0])
	}
	if provider == "amazon-bedrock" {
		if os.Getenv("AWS_PROFILE") != "" ||
			(os.Getenv("AWS_ACCESS_KEY_ID") != "" && os.Getenv("AWS_SECRET_ACCESS_KEY") != "") ||
			os.Getenv("AWS_BEARER_TOKEN_BEDROCK") != "" ||
			os.Getenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI") != "" ||
			os.Getenv("AWS_CONTAINER_CREDENTIALS_FULL_URI") != "" ||
			os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE") != "" {
			return "<authenticated>"
		}
	}
	return ""
}
