package gillmprovider

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

const (
	CloudflareWorkersAIBaseURL          = "https://api.cloudflare.com/client/v4/accounts/{CLOUDFLARE_ACCOUNT_ID}/ai/v1"
	CloudflareAIGatewayCompatBaseURL    = "https://gateway.ai.cloudflare.com/v1/{CLOUDFLARE_ACCOUNT_ID}/{CLOUDFLARE_GATEWAY_ID}/compat"
	CloudflareAIGatewayOpenAIBaseURL    = "https://gateway.ai.cloudflare.com/v1/{CLOUDFLARE_ACCOUNT_ID}/{CLOUDFLARE_GATEWAY_ID}/openai"
	CloudflareAIGatewayAnthropicBaseURL = "https://gateway.ai.cloudflare.com/v1/{CLOUDFLARE_ACCOUNT_ID}/{CLOUDFLARE_GATEWAY_ID}/anthropic"
)

var cloudflareBaseURLPlaceholder = regexp.MustCompile(`\{([A-Z_][A-Z0-9_]*)\}`)

func IsCloudflareProvider(provider string) bool {
	return provider == "cloudflare-workers-ai" || provider == "cloudflare-ai-gateway"
}

func hasCloudflareAIGatewayAuthorization(model Model, headers map[string]string) bool {
	if model.Provider != "cloudflare-ai-gateway" {
		return false
	}
	value, ok := headerValueCaseInsensitive(headers, "cf-aig-authorization")
	return ok && strings.TrimSpace(value) != ""
}

// ResolveCloudflareModel materializes account-scoped endpoint placeholders
// from the request-scoped provider environment without mutating the catalog
// model or process environment.
func ResolveCloudflareModel(model Model, env ProviderEnv) Model {
	if len(env) == 0 || !strings.Contains(model.BaseURL, "{") {
		return model
	}
	resolved := model.BaseURL
	for _, name := range []string{"CLOUDFLARE_ACCOUNT_ID", "CLOUDFLARE_GATEWAY_ID"} {
		if value := env[name]; value != "" {
			resolved = strings.ReplaceAll(resolved, "{"+name+"}", value)
		}
	}
	if resolved == model.BaseURL {
		return model
	}
	cloned := cloneModel(model)
	cloned.BaseURL = resolved
	return cloned
}

func ResolveCloudflareBaseURL(model Model) (string, error) {
	baseURL := model.BaseURL
	if !strings.Contains(baseURL, "{") {
		return baseURL, nil
	}
	var missing string
	resolved := cloudflareBaseURLPlaceholder.ReplaceAllStringFunc(baseURL, func(match string) string {
		if missing != "" {
			return match
		}
		groups := cloudflareBaseURLPlaceholder.FindStringSubmatch(match)
		if len(groups) != 2 {
			return match
		}
		value := os.Getenv(groups[1])
		if value == "" {
			missing = groups[1]
			return match
		}
		return value
	})
	if missing != "" {
		return "", fmt.Errorf("%s is required for provider %s but is not set.", missing, model.Provider)
	}
	return resolved, nil
}
