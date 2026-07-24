package gillmprovider

import "testing"

func TestEnvironmentAPIKeysDoesNotTreatGenericGitHubTokensAsCopilotCredentials(t *testing.T) {
	t.Setenv("COPILOT_GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "gh-token")
	t.Setenv("GITHUB_TOKEN", "github-token")

	if got := FindEnvKeys("github-copilot"); got != nil {
		t.Fatalf("FindEnvKeys(github-copilot) = %#v, want nil", got)
	}
	if got := GetEnvAPIKey("github-copilot"); got != "" {
		t.Fatalf("GetEnvAPIKey(github-copilot) = %q, want empty", got)
	}
}

func TestEnvironmentAPIKeysResolvesCopilotToken(t *testing.T) {
	t.Setenv("COPILOT_GITHUB_TOKEN", "copilot-token")
	t.Setenv("GH_TOKEN", "gh-token")
	t.Setenv("GITHUB_TOKEN", "github-token")

	got := FindEnvKeys("github-copilot")
	if len(got) != 1 || got[0] != "COPILOT_GITHUB_TOKEN" {
		t.Fatalf("FindEnvKeys(github-copilot) = %#v, want COPILOT_GITHUB_TOKEN", got)
	}
	if got := GetEnvAPIKey("github-copilot"); got != "copilot-token" {
		t.Fatalf("GetEnvAPIKey(github-copilot) = %q, want copilot-token", got)
	}
}

func TestEnvironmentAPIKeysCoverPiV082Providers(t *testing.T) {
	cases := map[string]string{
		"ant-ling":           "ANT_LING_API_KEY",
		"qwen-token-plan":    "QWEN_TOKEN_PLAN_API_KEY",
		"qwen-token-plan-cn": "QWEN_TOKEN_PLAN_CN_API_KEY",
		"radius":             "RADIUS_API_KEY",
		"zai-coding-cn":      "ZAI_CODING_CN_API_KEY",
	}
	for providerID, envVar := range cases {
		t.Run(providerID, func(t *testing.T) {
			t.Setenv(envVar, "test-key")
			if got := FindEnvKeys(providerID); len(got) != 1 || got[0] != envVar {
				t.Fatalf("FindEnvKeys(%s) = %#v, want %s", providerID, got, envVar)
			}
			if got := GetEnvAPIKey(providerID); got != "test-key" {
				t.Fatalf("GetEnvAPIKey(%s) = %q, want test-key", providerID, got)
			}
		})
	}
}
