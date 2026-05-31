package gicodingagent

import authwarning "github.com/nowa/gi/gi-coding-agent/internal/authwarning"

const AnthropicSubscriptionAuthWarning = authwarning.AnthropicSubscriptionAuthWarning

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
	if authwarning.ShouldWarn(modelProvider, authwarning.Options{
		Shown:               c.Shown,
		AnthropicExtraUsage: c.Settings.AnthropicExtraUsage,
		HasOAuthCredential: func(provider string) bool {
			if c.AuthStorage == nil {
				return false
			}
			credential, ok := c.AuthStorage.Get(provider)
			return ok && credential.Type == "oauth"
		},
		GetAPIKeyForProvider: c.GetAPIKeyForProvider,
	}) {
		c.Shown = true
		return true
	}
	return false
}

func IsAnthropicSubscriptionAuthKey(apiKey string) bool {
	return authwarning.IsAnthropicSubscriptionAuthKey(apiKey)
}
