package gicodingagent

import (
	"strings"
	"testing"
	"time"
)

func TestOAuthSelectorPiCases(t *testing.T) {
	oauthProviderIDs := map[string]bool{"anthropic": true, "github-copilot": true, "custom-oauth": true}
	builtInProviderIDs := map[string]bool{"anthropic": true, "github-copilot": true, "amazon-bedrock": true, "openai": true}

	if !IsAPIKeyLoginProvider("anthropic", oauthProviderIDs, builtInProviderIDs) ||
		builtInProviderDisplayNames["anthropic"] != "Anthropic" ||
		!IsAPIKeyLoginProvider("openai", oauthProviderIDs, builtInProviderIDs) ||
		IsAPIKeyLoginProvider("github-copilot", oauthProviderIDs, builtInProviderIDs) ||
		!IsAPIKeyLoginProvider("amazon-bedrock", oauthProviderIDs, builtInProviderIDs) ||
		IsAPIKeyLoginProvider("custom-oauth", oauthProviderIDs, builtInProviderIDs) ||
		!IsAPIKeyLoginProvider("custom-api", oauthProviderIDs, builtInProviderIDs) {
		t.Fatalf("api key login provider split failed")
	}

	authStorage := NewInMemoryAuthStorage(AuthStorageData{
		"anthropic": {
			Type:    "oauth",
			Access:  "access-token",
			Refresh: "refresh-token",
			Expires: time.Now().Add(time.Minute).UnixMilli(),
		},
	})
	output := renderOAuthSelector(OAuthSelector{
		Mode:        "login",
		AuthStorage: authStorage,
		Providers:   []AuthSelectorProvider{{ID: "anthropic", Name: "Anthropic", AuthType: "api_key"}},
	})
	if !strings.Contains(output, "Anthropic") || !strings.Contains(output, "subscription configured") {
		t.Fatalf("stored oauth output = %q", output)
	}

	t.Setenv("OPENAI_API_KEY", "test-openai-key")
	output = renderOAuthSelector(OAuthSelector{
		Mode:        "login",
		AuthStorage: NewInMemoryAuthStorage(nil),
		Providers:   []AuthSelectorProvider{{ID: "openai", Name: "OpenAI", AuthType: "api_key"}},
	})
	if !strings.Contains(output, "OpenAI") ||
		!strings.Contains(output, "✓ env: OPENAI_API_KEY") ||
		strings.Contains(output, "unconfigured") {
		t.Fatalf("env auth output = %q", output)
	}

	for _, tc := range []struct {
		name   string
		status AuthStatus
		want   string
	}{
		{name: "custom provider environment API key auth from status resolver", status: AuthStatus{Configured: true, Source: "environment", Label: "OLLAMA_API_KEY"}, want: "✓ env: OLLAMA_API_KEY"},
		{name: "models.json API key auth as configured", status: AuthStatus{Configured: true, Source: "models_json_key"}, want: "✓ key in models.json"},
		{name: "models.json command auth as configured", status: AuthStatus{Configured: true, Source: "models_json_command"}, want: "✓ command in models.json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := renderOAuthSelector(OAuthSelector{
				Mode:        "login",
				AuthStorage: NewInMemoryAuthStorage(nil),
				Providers:   []AuthSelectorProvider{{ID: "custom", Name: "custom", AuthType: "api_key"}},
				StatusResolver: func(providerID string) AuthStatus {
					return tc.status
				},
			})
			if !strings.Contains(output, "custom") || !strings.Contains(output, tc.want) || strings.Contains(output, "unconfigured") {
				t.Fatalf("%s output = %q", tc.name, output)
			}
		})
	}
}

func renderOAuthSelector(selector OAuthSelector) string {
	return strings.Join(selector.Render(120), "\n")
}
