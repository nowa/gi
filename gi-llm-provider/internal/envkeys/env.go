package envkeys

import "os"

const (
	AnthropicAuthTokenEnv  = "ANTHROPIC_AUTH_TOKEN"
	AnthropicOAuthTokenEnv = "ANTHROPIC_OAUTH_TOKEN"
	AnthropicAPIKeyEnv     = "ANTHROPIC_API_KEY"
)

// providerEnvKeys keeps status-facing environment discovery separate from
// values that can safely be passed to an SDK as an API key. Most providers use
// the same ordered list for both. Anthropic's bearer token is intentionally
// discoverable but must remain a request header.
type providerEnvKeys struct {
	discovery []string
	apiKeys   []string
}

func envKeysForProvider(provider string) providerEnvKeys {
	if provider == "anthropic" {
		return providerEnvKeys{
			discovery: []string{
				AnthropicAuthTokenEnv,
				AnthropicOAuthTokenEnv,
				AnthropicAPIKeyEnv,
			},
			apiKeys: []string{
				AnthropicOAuthTokenEnv,
				AnthropicAPIKeyEnv,
			},
		}
	}

	var apiKeys []string
	switch provider {
	case "ant-ling":
		apiKeys = []string{"ANT_LING_API_KEY"}
	case "github-copilot":
		apiKeys = []string{"COPILOT_GITHUB_TOKEN"}
	case "openai":
		apiKeys = []string{"OPENAI_API_KEY"}
	case "azure-openai-responses":
		apiKeys = []string{"AZURE_OPENAI_API_KEY"}
	case "nvidia":
		apiKeys = []string{"NVIDIA_API_KEY"}
	case "deepseek":
		apiKeys = []string{"DEEPSEEK_API_KEY"}
	case "google":
		apiKeys = []string{"GEMINI_API_KEY"}
	case "google-vertex":
		apiKeys = []string{"GOOGLE_CLOUD_API_KEY"}
	case "groq":
		apiKeys = []string{"GROQ_API_KEY"}
	case "cerebras":
		apiKeys = []string{"CEREBRAS_API_KEY"}
	case "xai":
		apiKeys = []string{"XAI_API_KEY"}
	case "openrouter":
		apiKeys = []string{"OPENROUTER_API_KEY"}
	case "vercel-ai-gateway":
		apiKeys = []string{"AI_GATEWAY_API_KEY"}
	case "zai":
		apiKeys = []string{"ZAI_API_KEY"}
	case "zai-coding-cn":
		apiKeys = []string{"ZAI_CODING_CN_API_KEY"}
	case "mistral":
		apiKeys = []string{"MISTRAL_API_KEY"}
	case "minimax":
		apiKeys = []string{"MINIMAX_API_KEY"}
	case "minimax-cn":
		apiKeys = []string{"MINIMAX_CN_API_KEY"}
	case "moonshotai", "moonshotai-cn":
		apiKeys = []string{"MOONSHOT_API_KEY"}
	case "huggingface":
		apiKeys = []string{"HF_TOKEN"}
	case "fireworks":
		apiKeys = []string{"FIREWORKS_API_KEY"}
	case "together":
		apiKeys = []string{"TOGETHER_API_KEY"}
	case "opencode", "opencode-go":
		apiKeys = []string{"OPENCODE_API_KEY"}
	case "kimi-coding":
		apiKeys = []string{"KIMI_API_KEY"}
	case "qwen-token-plan":
		apiKeys = []string{"QWEN_TOKEN_PLAN_API_KEY"}
	case "qwen-token-plan-cn":
		apiKeys = []string{"QWEN_TOKEN_PLAN_CN_API_KEY"}
	case "radius":
		apiKeys = []string{"RADIUS_API_KEY"}
	case "cloudflare-workers-ai", "cloudflare-ai-gateway":
		apiKeys = []string{"CLOUDFLARE_API_KEY"}
	case "xiaomi":
		apiKeys = []string{"XIAOMI_API_KEY"}
	case "xiaomi-token-plan-cn":
		apiKeys = []string{"XIAOMI_TOKEN_PLAN_CN_API_KEY"}
	case "xiaomi-token-plan-ams":
		apiKeys = []string{"XIAOMI_TOKEN_PLAN_AMS_API_KEY"}
	case "xiaomi-token-plan-sgp":
		apiKeys = []string{"XIAOMI_TOKEN_PLAN_SGP_API_KEY"}
	}
	return providerEnvKeys{discovery: apiKeys, apiKeys: apiKeys}
}

func FindEnvKeys(provider string) []string {
	return FindEnvKeysWithLookup(provider, os.Getenv)
}

// FindEnvKeysWithLookup returns configured API-key variable names through the
// supplied environment lookup.
func FindEnvKeysWithLookup(provider string, lookup func(string) string) []string {
	if lookup == nil {
		lookup = os.Getenv
	}
	var found []string
	for _, key := range envKeysForProvider(provider).discovery {
		if lookup(key) != "" {
			found = append(found, key)
		}
	}
	if len(found) == 0 {
		return nil
	}
	return found
}

func GetEnvAPIKey(provider string) string {
	return ResolveAPIKey(provider, os.Getenv)
}

// ResolveAPIKey resolves provider authentication through the supplied lookup.
// It also recognizes ambient Bedrock credential sources.
func ResolveAPIKey(provider string, lookup func(string) string) string {
	if lookup == nil {
		lookup = os.Getenv
	}
	for _, key := range envKeysForProvider(provider).apiKeys {
		if value := lookup(key); value != "" {
			return value
		}
	}
	if provider == "amazon-bedrock" {
		if lookup("AWS_PROFILE") != "" ||
			(lookup("AWS_ACCESS_KEY_ID") != "" && lookup("AWS_SECRET_ACCESS_KEY") != "") ||
			lookup("AWS_BEARER_TOKEN_BEDROCK") != "" ||
			lookup("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI") != "" ||
			lookup("AWS_CONTAINER_CREDENTIALS_FULL_URI") != "" ||
			lookup("AWS_WEB_IDENTITY_TOKEN_FILE") != "" {
			return "<authenticated>"
		}
	}
	return ""
}
