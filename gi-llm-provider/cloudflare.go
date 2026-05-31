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
