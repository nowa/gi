package gicodingagent

import "testing"

func TestAnthropicSubscriptionWarningPiCases(t *testing.T) {
	calls := 0
	checker := AnthropicSubscriptionWarningChecker{
		AuthStorage: NewInMemoryAuthStorage(nil),
		GetAPIKeyForProvider: func(provider string) (string, bool) {
			calls++
			return "sk-ant-oat01-test", true
		},
	}
	if !checker.MaybeWarn("anthropic") || checker.MaybeWarn("anthropic") {
		t.Fatalf("warning should show exactly once")
	}
	if calls != 1 {
		t.Fatalf("api key lookup calls = %d, want 1", calls)
	}

	calls = 0
	checker = AnthropicSubscriptionWarningChecker{
		AuthStorage: NewInMemoryAuthStorage(AuthStorageData{
			"anthropic": {Type: "oauth", Access: "access-token"},
		}),
		GetAPIKeyForProvider: func(provider string) (string, bool) {
			calls++
			return "", false
		},
	}
	if !checker.MaybeWarn("anthropic") {
		t.Fatalf("stored oauth should warn")
	}
	if calls != 0 {
		t.Fatalf("stored oauth should not resolve api key, calls = %d", calls)
	}

	calls = 0
	checker = AnthropicSubscriptionWarningChecker{
		AuthStorage: NewInMemoryAuthStorage(nil),
		GetAPIKeyForProvider: func(provider string) (string, bool) {
			calls++
			return "sk-ant-oat01-test", true
		},
	}
	if checker.MaybeWarn("openai") || calls != 0 {
		t.Fatalf("non-anthropic model should not warn or resolve auth")
	}

	disabled := false
	checker = AnthropicSubscriptionWarningChecker{
		Settings:    AnthropicWarningSettings{AnthropicExtraUsage: &disabled},
		AuthStorage: NewInMemoryAuthStorage(AuthStorageData{"anthropic": {Type: "oauth"}}),
		GetAPIKeyForProvider: func(provider string) (string, bool) {
			t.Fatalf("disabled warning should not resolve auth")
			return "", false
		},
	}
	if checker.MaybeWarn("anthropic") {
		t.Fatalf("disabled warning should not show")
	}
}
