package gicodingagent

import (
	"strings"
	"testing"
	"time"

	gitui "github.com/nowa/gi/gi-tui"
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

func TestOAuthSelectorComponentUsesTUIKeybindingsPiStyle(t *testing.T) {
	previous := gitui.GetKeybindings()
	gitui.SetKeybindings(gitui.NewKeybindingsManager(gitui.KeybindingsConfig{
		"tui.select.up":                 []string{"p"},
		"tui.select.down":               []string{"n"},
		"tui.select.confirm":            []string{"x"},
		"tui.select.cancel":             []string{"q"},
		"tui.editor.deleteToLineStart":  []string{"u"},
		"tui.editor.deleteCharBackward": []string{"backspace"},
	}))
	t.Cleanup(func() { gitui.SetKeybindings(previous) })

	selector := NewOAuthSelectorComponent(OAuthSelector{
		Mode: "login",
		Providers: []AuthSelectorProvider{
			{ID: "alpha", Name: "Alpha", AuthType: "api_key"},
			{ID: "beta", Name: "Beta", AuthType: "api_key"},
		},
	})
	selected := ""
	selector.OnSelect = func(providerID string) { selected = providerID }
	cancelled := false
	selector.OnCancel = func() { cancelled = true }

	rendered := strings.Join(selector.Render(120), "\n")
	for _, expected := range []string{"P/N move", "X select", "U clear", "Q cancel"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("render missing %q:\n%s", expected, rendered)
		}
	}

	selector.HandleInput("n")
	selector.HandleInput("x")
	if selected != "beta" {
		t.Fatalf("selected = %q, want beta", selected)
	}

	selector.HandleInput("q")
	if !cancelled {
		t.Fatal("cancel keybinding did not call OnCancel")
	}

	selector.HandleInput("b")
	selector.HandleInput("e")
	rendered = strings.Join(selector.Render(120), "\n")
	if strings.Contains(rendered, "Alpha") || !strings.Contains(rendered, "Beta") {
		t.Fatalf("search render mismatch:\n%s", rendered)
	}
	selector.HandleInput("u")
	rendered = strings.Join(selector.Render(120), "\n")
	if !strings.Contains(rendered, "Alpha") || !strings.Contains(rendered, "Beta") {
		t.Fatalf("clear search render mismatch:\n%s", rendered)
	}
}

func renderOAuthSelector(selector OAuthSelector) string {
	return strings.Join(selector.Render(120), "\n")
}
