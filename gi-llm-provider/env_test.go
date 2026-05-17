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
