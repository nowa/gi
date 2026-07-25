package attribution

import (
	"net/url"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
)

const (
	openRouterHost            = "openrouter.ai"
	nvidiaNIMHost             = "integrate.api.nvidia.com"
	cloudflareAPIHost         = "api.cloudflare.com"
	cloudflareAIGatewayHost   = "gateway.ai.cloudflare.com"
	openCodeHost              = "opencode.ai"
	openRouterReferer         = "https://github.com/nowa/gi"
	openRouterTitle           = "gi"
	nvidiaBillingInvokeOrigin = "Gi"
	cloudflareUserAgent       = "gi-coding-agent"
	openCodeClient            = "gi"
)

// Context is the immutable application state needed to derive provider
// attribution for one request.
type Context struct {
	InstallTelemetryEnabled bool
	SessionID               string
}

func GetAttributionHeaders(model llm.Model, installTelemetryEnabled bool) map[string]string {
	if !installTelemetryEnabled {
		return nil
	}
	if isOpenRouterModel(model) {
		return map[string]string{
			"HTTP-Referer":            openRouterReferer,
			"X-OpenRouter-Title":      openRouterTitle,
			"X-OpenRouter-Categories": "cli-agent",
		}
	}
	if isNVIDIANIMModel(model) {
		return map[string]string{
			"X-BILLING-INVOKE-ORIGIN": nvidiaBillingInvokeOrigin,
		}
	}
	if isCloudflareModel(model) {
		return map[string]string{"User-Agent": cloudflareUserAgent}
	}
	return nil
}

func MergeProviderHeaders(
	model llm.Model,
	context Context,
	headerSources ...map[string]string,
) map[string]string {
	headers := map[string]string{}
	for key, value := range getSessionHeaders(model, context.SessionID) {
		headers[key] = value
	}
	for key, value := range GetAttributionHeaders(
		model,
		context.InstallTelemetryEnabled,
	) {
		headers[key] = value
	}
	for _, source := range headerSources {
		for key, value := range source {
			headers[key] = value
		}
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

func BuildSDKStreamHeaders(
	model llm.Model,
	installTelemetryEnabled bool,
	providerHeaders map[string]string,
	requestHeaders map[string]string,
) map[string]string {
	return MergeProviderHeaders(
		model,
		Context{InstallTelemetryEnabled: installTelemetryEnabled},
		providerHeaders,
		requestHeaders,
	)
}

func matchesHost(baseURL string, expectedHost string) bool {
	parsed, err := url.Parse(baseURL)
	return err == nil &&
		strings.EqualFold(parsed.Hostname(), expectedHost)
}

func isOpenRouterModel(model llm.Model) bool {
	return model.Provider == "openrouter" ||
		strings.Contains(model.BaseURL, openRouterHost)
}

func isNVIDIANIMModel(model llm.Model) bool {
	return model.Provider == "nvidia" ||
		matchesHost(model.BaseURL, nvidiaNIMHost)
}

func isCloudflareModel(model llm.Model) bool {
	return model.Provider == "cloudflare-workers-ai" ||
		model.Provider == "cloudflare-ai-gateway" ||
		matchesHost(model.BaseURL, cloudflareAPIHost) ||
		matchesHost(model.BaseURL, cloudflareAIGatewayHost)
}

func getSessionHeaders(
	model llm.Model,
	sessionID string,
) map[string]string {
	if sessionID == "" {
		return nil
	}
	if model.Provider != "opencode" &&
		model.Provider != "opencode-go" &&
		!matchesHost(model.BaseURL, openCodeHost) {
		return nil
	}
	return map[string]string{
		"x-opencode-session": sessionID,
		"x-opencode-client":  openCodeClient,
	}
}
