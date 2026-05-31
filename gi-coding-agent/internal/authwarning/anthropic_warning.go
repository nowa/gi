package authwarning

import "strings"

const AnthropicSubscriptionAuthWarning = "Anthropic subscription auth is active. Third-party harness usage draws from extra usage and is billed per token, not your Claude plan limits. Manage extra usage at https://claude.ai/settings/usage."

type Options struct {
	Shown                bool
	AnthropicExtraUsage  *bool
	HasOAuthCredential   func(provider string) bool
	GetAPIKeyForProvider func(provider string) (string, bool)
}

func ShouldWarn(modelProvider string, options Options) bool {
	if options.AnthropicExtraUsage != nil && !*options.AnthropicExtraUsage {
		return false
	}
	if options.Shown || modelProvider != "anthropic" {
		return false
	}
	if options.HasOAuthCredential != nil && options.HasOAuthCredential("anthropic") {
		return true
	}
	if options.GetAPIKeyForProvider == nil {
		return false
	}
	apiKey, ok := options.GetAPIKeyForProvider("anthropic")
	return ok && IsAnthropicSubscriptionAuthKey(apiKey)
}

func IsAnthropicSubscriptionAuthKey(apiKey string) bool {
	return strings.HasPrefix(apiKey, "sk-ant-oat")
}
