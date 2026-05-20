package gicodingagent

import "strings"

const AnthropicSubscriptionAuthWarning = "Anthropic subscription auth is active. Third-party harness usage draws from extra usage and is billed per token, not your Claude plan limits. Manage extra usage at https://claude.ai/settings/usage."

type AnthropicWarningSettings struct {
	AnthropicExtraUsage *bool
}

type AnthropicSubscriptionWarningChecker struct {
	Shown                bool
	Settings             AnthropicWarningSettings
	AuthStorage          *AuthStorage
	GetAPIKeyForProvider func(provider string) (string, bool)
}

func (c *AnthropicSubscriptionWarningChecker) MaybeWarn(modelProvider string) bool {
	if c.Settings.AnthropicExtraUsage != nil && !*c.Settings.AnthropicExtraUsage {
		return false
	}
	if c.Shown || modelProvider != "anthropic" {
		return false
	}
	if c.AuthStorage != nil {
		if credential, ok := c.AuthStorage.Get("anthropic"); ok && credential.Type == "oauth" {
			c.Shown = true
			return true
		}
	}
	if c.GetAPIKeyForProvider == nil {
		return false
	}
	apiKey, ok := c.GetAPIKeyForProvider("anthropic")
	if !ok || !IsAnthropicSubscriptionAuthKey(apiKey) {
		return false
	}
	c.Shown = true
	return true
}

func IsAnthropicSubscriptionAuthKey(apiKey string) bool {
	return strings.HasPrefix(apiKey, "sk-ant-oat")
}
