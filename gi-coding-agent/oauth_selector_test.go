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

func TestLoginAuthSelectorProvidersIncludesProviderOwnedOAuthFlows(t *testing.T) {
	ResetOAuthProviders()
	t.Cleanup(ResetOAuthProviders)

	registry := NewModelRegistry(NewInMemoryAuthStorage(nil), "")
	if _, err := NewModelRuntimeFromRegistry(registry); err != nil {
		t.Fatal(err)
	}
	providers := loginAuthSelectorProviders(registry, "oauth")
	var labels []string
	for _, provider := range providers {
		labels = append(labels, provider.Name)
		if provider.AuthType != "oauth" {
			t.Fatalf("provider %q auth type = %q, want oauth", provider.ID, provider.AuthType)
		}
	}
	want := []string{
		"Anthropic",
		"GitHub Copilot",
		"Kimi For Coding",
		"OpenAI Codex",
		"OpenRouter",
		"Radius",
		"xAI",
	}
	if strings.Join(labels, "\n") != strings.Join(want, "\n") {
		t.Fatalf("subscription providers = %#v, want %#v", labels, want)
	}
}

func TestOAuthSelectorComponentLabelsMixedAuthTypes(t *testing.T) {
	if got := formatAuthSelectorProviderType("oauth"); got != "subscription" {
		t.Fatalf("OAuth label = %q", got)
	}
	if got := formatAuthSelectorProviderType("api_key"); got != "API key" {
		t.Fatalf("API key label = %q", got)
	}

	selector := NewOAuthSelectorComponent(OAuthSelector{
		Mode: "logout",
		Providers: []AuthSelectorProvider{
			{ID: "anthropic", Name: "Anthropic", AuthType: "oauth"},
			{ID: "openai", Name: "OpenAI", AuthType: "api_key"},
		},
	})
	rendered := strings.Join(selector.Render(120), "\n")
	for _, expected := range []string{"[subscription]", "[API key]"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("render missing %q:\n%s", expected, rendered)
		}
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
	for _, expected := range []string{"Select provider to configure:", "→ ", "Alpha"} {
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

func TestExtensionSelectorComponentUsesTUIKeybindingsPiStyle(t *testing.T) {
	previous := gitui.GetKeybindings()
	gitui.SetKeybindings(gitui.NewKeybindingsManager(gitui.KeybindingsConfig{
		"tui.select.up":      []string{"p"},
		"tui.select.down":    []string{"n"},
		"tui.select.confirm": []string{"x"},
		"tui.select.cancel":  []string{"q"},
	}))
	t.Cleanup(func() { gitui.SetKeybindings(previous) })

	selector := NewExtensionSelectorComponent("Select authentication method:", []string{"Use a subscription", "Use an API key"})
	selected := ""
	selector.OnSelect = func(option string) { selected = option }
	cancelled := false
	selector.OnCancel = func() { cancelled = true }

	rendered := strings.Join(selector.Render(120), "\n")
	for _, expected := range []string{"Select authentication method:", "↑↓", "navigate", "x", "select", "q", "cancel"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("render missing %q:\n%s", expected, rendered)
		}
	}

	selector.HandleInput("n")
	selector.HandleInput("x")
	if selected != "Use an API key" {
		t.Fatalf("selected = %q, want Use an API key", selected)
	}
	selector.HandleInput("q")
	if !cancelled {
		t.Fatal("cancel keybinding did not call OnCancel")
	}
}

func TestLoginDialogComponentShowInfoUsesPiStyle(t *testing.T) {
	dialog := NewLoginDialogComponent("Amazon Bedrock setup", "")
	cancelled := false
	dialog.OnCancel = func() { cancelled = true }
	dialog.ShowInfo([]string{
		tuiThemeFG("text", "Amazon Bedrock uses AWS credentials instead of a single API key."),
		tuiThemeMuted("See:"),
		tuiThemeAccent("  /tmp/docs/providers.md"),
	}, true)

	rendered := strings.Join(dialog.Render(120), "\n")
	for _, expected := range []string{
		tuiThemeBorder(strings.Repeat("─", 120)),
		tuiThemeBoldAccent("Amazon Bedrock setup"),
		tuiThemeFG("text", "Amazon Bedrock uses AWS credentials instead of a single API key."),
		tuiThemeAccent("  /tmp/docs/providers.md"),
		"to close",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("render missing %q:\n%s", expected, rendered)
		}
	}
	if strings.Contains(rendered, "to submit") {
		t.Fatalf("info dialog should not render submit hint:\n%s", rendered)
	}
	dialog.HandleInput("\x1b")
	if !cancelled {
		t.Fatal("cancel keybinding did not close info dialog")
	}
}

func TestLoginDialogComponentShowAuthUsesPiStyle(t *testing.T) {
	dialog := NewLoginDialogComponent("Login to Anthropic (Claude Pro/Max)", "")
	cancelled := false
	dialog.OnCancel = func() { cancelled = true }
	authURL := "https://claude.ai/oauth/authorize?code=true&client_id=test-client"
	dialog.ShowAuth(
		authURL,
		"Complete login in your browser. If the browser is on another machine, paste the final redirect URL here.",
		"Paste redirect URL below, or complete login in browser:",
	)

	rendered := strings.Join(dialog.Render(120), "\n")
	for _, expected := range []string{
		tuiThemeBorder(strings.Repeat("─", 120)),
		tuiThemeBoldAccent("Login to Anthropic (Claude Pro/Max)"),
		tuiThemeAccent(terminalHyperlink(authURL, authURL)),
		tuiThemeDim(terminalHyperlink(authURL, oauthClickHint())),
		tuiThemeWarning("Complete login in your browser. If the browser is on another machine, paste the final redirect URL here."),
		tuiThemeDim("Paste redirect URL below, or complete login in browser:"),
		"to cancel",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("render missing %q:\n%s", expected, rendered)
		}
	}
	if strings.Contains(rendered, "to submit") {
		t.Fatalf("OAuth auth dialog should not render submit hint:\n%s", rendered)
	}
	dialog.HandleInput("\x1b")
	if !cancelled {
		t.Fatal("cancel keybinding did not cancel OAuth dialog")
	}
}

func TestLoginDialogComponentPreservesProviderPromptContext(t *testing.T) {
	dialog := NewLoginDialogComponent("Login to Amazon Bedrock", "")
	submitted := ""
	dialog.OnSubmit = func(value string) {
		submitted = value
	}
	dialog.ShowInfo([]string{"AWS credential provider chain"})
	dialog.ShowPrompt("Enter AWS profile name", "default")

	rendered := strings.Join(dialog.Render(120), "\n")
	for _, expected := range []string{
		"AWS credential provider chain",
		"Enter AWS profile name",
		"e.g., default",
		"to submit",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("render missing %q:\n%s", expected, rendered)
		}
	}
	dialog.HandleInput("engineering")
	dialog.HandleInput("\r")
	if submitted != "engineering" {
		t.Fatalf("submitted = %q", submitted)
	}
	if rendered = strings.Join(dialog.Render(120), "\n"); !strings.Contains(rendered, "> engineering") {
		t.Fatalf("submitted input was not retained:\n%s", rendered)
	}

	dialog.ShowDeviceCode(
		"https://github.com/login/device",
		"USER-CODE",
	)
	dialog.ShowWaiting("Waiting for authentication...")
	rendered = strings.Join(dialog.Render(120), "\n")
	for _, expected := range []string{
		"https://github.com/login/device",
		"Enter code: USER-CODE",
		"Waiting for authentication...",
		"to cancel",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("render missing %q:\n%s", expected, rendered)
		}
	}
	if strings.Contains(rendered, "to submit") {
		t.Fatalf("device-code progress should not render submit hint:\n%s", rendered)
	}
}

func renderOAuthSelector(selector OAuthSelector) string {
	return strings.Join(selector.Render(120), "\n")
}
