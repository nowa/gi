package gicodingagent

import (
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func GetAttributionHeaders(model llm.Model, installTelemetryEnabled bool) map[string]string {
	if !installTelemetryEnabled {
		return nil
	}
	if model.Provider == "openrouter" || strings.Contains(model.BaseURL, "openrouter.ai") {
		return map[string]string{
			"HTTP-Referer":            "https://pi.dev",
			"X-OpenRouter-Title":      "pi",
			"X-OpenRouter-Categories": "cli-agent",
		}
	}
	if model.Provider == "cloudflare-workers-ai" ||
		model.Provider == "cloudflare-ai-gateway" ||
		strings.Contains(model.BaseURL, "api.cloudflare.com") ||
		strings.Contains(model.BaseURL, "gateway.ai.cloudflare.com") {
		return map[string]string{"User-Agent": "pi-coding-agent"}
	}
	return nil
}

func BuildSDKStreamHeaders(model llm.Model, installTelemetryEnabled bool, providerHeaders, requestHeaders map[string]string) map[string]string {
	headers := map[string]string{}
	for key, value := range GetAttributionHeaders(model, installTelemetryEnabled) {
		headers[key] = value
	}
	for key, value := range providerHeaders {
		headers[key] = value
	}
	for key, value := range requestHeaders {
		headers[key] = value
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}
