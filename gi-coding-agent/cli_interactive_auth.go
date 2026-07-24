package gicodingagent

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"

	gitui "github.com/nowa/gi/gi-tui"
)

func (h *CLIInteractiveTUIHost) handleLoginSlashCommand(args string) error {
	provider := strings.TrimSpace(args)
	authType := "api_key"
	if provider == "" && !h.exitAfterInitial {
		registry := h.modelRegistry()
		if registry != nil {
			selected, selectedAuthType, handled, err := h.selectLoginProvider(registry)
			if err != nil {
				return err
			}
			if !handled {
				return nil
			}
			provider = selected
			authType = selectedAuthType
		}
	}
	if provider != "" && !h.exitAfterInitial {
		return h.runInteractiveLogin(provider, authType)
	}
	message := providerLoginHelp()
	if provider != "" {
		message = formatNoAPIKeyFoundMessage(provider)
	} else if !h.exitAfterInitial {
		h.addStatus("No API key providers available. Configure ~/.gi/agent/models.json or provider environment variables.")
	}
	h.chat.AddChild(newCLIMarkdownWithOptions("**Login**\n\n"+message, gitui.MarkdownOptions{PaddingX: 1, PaddingY: 1}))
	h.requestRender(false)
	return nil
}

func (h *CLIInteractiveTUIHost) selectLoginProvider(registry *ModelRegistry) (providerID string, authType string, handled bool, err error) {
	for {
		selectedAuthType, cancelled, err := h.selectLoginAuthType()
		if err != nil {
			return "", "", false, err
		}
		if cancelled {
			return "", "", false, nil
		}
		providers := loginAuthSelectorProviders(registry, selectedAuthType)
		if len(providers) == 0 {
			if selectedAuthType == "oauth" {
				h.addStatus("No subscription providers available.")
			} else {
				h.addStatus("No API key providers available.")
			}
			return "", "", false, nil
		}
		selected, providerCancelled, err := h.selectAuthProvider("login", registry, providers)
		if err != nil {
			return "", "", false, err
		}
		if providerCancelled {
			continue
		}
		return selected, selectedAuthType, true, nil
	}
}

func (h *CLIInteractiveTUIHost) selectLoginAuthType() (string, bool, error) {
	if h == nil || h.ui == nil {
		return "", true, errors.New("interactive TUI is not ready")
	}
	const subscriptionLabel = "Use a subscription"
	const apiKeyLabel = "Use an API key"
	selector := NewExtensionSelectorComponent("Select authentication method:", []string{subscriptionLabel, apiKeyLabel})
	completion := newCLIDialogCompletion()
	finish := func(result TUIDialogResult) {
		completion.finish(result)
	}
	selector.OnSelect = func(option string) {
		value := "api_key"
		if option == subscriptionLabel {
			value = "oauth"
		}
		finish(TUIDialogResult{Action: "selected", Value: value})
	}
	selector.OnCancel = func() {
		finish(TUIDialogResult{Action: "cancelled"})
	}
	completion.installRestore(h.showEditorReplacement(selector, selector))
	result := completion.wait(h.done)
	if result.Action != "selected" {
		return "", true, nil
	}
	return dialogStringValue(result.Value), false, nil
}

func (h *CLIInteractiveTUIHost) runInteractiveLogin(providerID, authType string) error {
	registry := h.modelRegistry()
	if registry == nil || registry.authStorage == nil {
		h.chat.AddChild(newCLIMarkdownWithOptions("**Login**\n\n"+formatNoAPIKeyFoundMessage(providerID), gitui.MarkdownOptions{PaddingX: 1, PaddingY: 1}))
		h.requestRender(false)
		return nil
	}
	providerName := registry.GetProviderDisplayName(providerID)
	if authType == "oauth" {
		return h.showOAuthLoginDialog(providerID, providerName)
	}
	if providerID == "amazon-bedrock" {
		h.addBedrockSetupInfo(providerID, providerName)
		return nil
	}
	apiKey, cancelled, err := h.promptForAPIKey(providerName)
	if err != nil {
		return err
	}
	if cancelled {
		return nil
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		h.addStatus("Failed to save API key for " + providerName + ": API key cannot be empty.")
		return nil
	}
	registry.authStorage.Set(providerID, AuthCredential{Type: "api_key", Key: apiKey})
	if err := h.refreshModelRuntimeAfterCredentialChange(
		context.Background(),
		registry,
	); err != nil {
		return err
	}
	h.addStatus("Saved API key for " + providerName + ". Credentials saved to ~/.gi/agent/auth.json")
	return nil
}

func (h *CLIInteractiveTUIHost) showOAuthLoginDialog(providerID, providerName string) error {
	if h == nil || h.ui == nil {
		return errors.New("interactive TUI is not ready")
	}
	switch providerID {
	case "anthropic":
		return h.showAnthropicOAuthLoginDialog(providerName)
	case "github-copilot":
		return h.showGitHubCopilotOAuthLoginDialog(providerName)
	case "openai-codex":
		return h.showOpenAICodexOAuthLoginDialog(providerName)
	}
	prompt, ok := oauthLoginPromptForProvider(providerID)
	if !ok {
		h.addStatus("Subscription login is not implemented yet for " + providerName)
		return nil
	}
	dialog := NewLoginDialogComponent("Login to "+providerName, "")
	completion := newCLIDialogCompletion()
	finish := func(result TUIDialogResult) {
		completion.finish(result)
	}
	dialog.OnSubmit = func(value string) {
		finish(TUIDialogResult{Action: "submitted", Value: value})
	}
	dialog.OnCancel = func() {
		finish(TUIDialogResult{Action: "cancelled"})
	}
	dialog.ShowAuth(prompt.URL, prompt.Instructions, prompt.ManualPrompt)
	completion.installRestore(h.showEditorReplacement(dialog, dialog))
	result := completion.wait(h.done)
	if result.Action == "submitted" {
		h.addStatus("Subscription login token exchange is not implemented yet for " + providerName + ".")
	}
	return nil
}

func (h *CLIInteractiveTUIHost) showAnthropicOAuthLoginDialog(providerName string) error {
	registry := h.modelRegistry()
	if registry == nil || registry.authStorage == nil {
		h.addStatus("Failed to login to " + providerName + ": auth storage is not configured")
		return nil
	}
	runtime := defaultAnthropicOAuthRuntime
	flow, err := runtime.NewFlow()
	if err != nil {
		h.addStatus("Failed to login to " + providerName + ": " + err.Error())
		return nil
	}
	callbackServer, serverErr := runtime.StartCallbackServer(flow.State)
	if callbackServer != nil {
		defer callbackServer.Close()
	}

	dialog := NewLoginDialogComponent("Login to "+providerName, "")
	resultCh := make(chan TUIDialogResult, 1)
	var closeOnce sync.Once
	var restore func()
	finish := func(result TUIDialogResult) {
		closeOnce.Do(func() {
			resultCh <- result
		})
	}
	dialog.OnSubmit = func(value string) {
		finish(TUIDialogResult{Action: "submitted", Value: value})
	}
	dialog.OnCancel = func() {
		finish(TUIDialogResult{Action: "cancelled"})
	}
	instructions := "Complete login in your browser. If the browser is on another machine, paste the final redirect URL here."
	if serverErr != nil {
		instructions = "Could not start the local callback server. Complete login in your browser, then paste the final redirect URL here."
	}
	dialog.ShowAuth(flow.URL, instructions, "Paste redirect URL below, or complete login in browser:")
	restore = h.showEditorReplacement(dialog, dialog)
	if serverErr != nil {
		h.addStatus("Warning: " + serverErr.Error())
	} else if h.shouldAutoOpenOAuthBrowser() && runtime.OpenBrowser != nil {
		_ = runtime.OpenBrowser(flow.URL)
	}

	var callbackCh <-chan openAICodexOAuthCallbackResult
	if callbackServer != nil {
		callbackCh = callbackServer.Result()
	}
	code := ""
	select {
	case result := <-resultCh:
		if result.Action != "submitted" {
			if restore != nil {
				restore()
			}
			return nil
		}
		parsedCode, parsedState := parseOAuthAuthorizationInput(dialogStringValue(result.Value))
		if parsedState != "" && parsedState != flow.State {
			if restore != nil {
				restore()
			}
			h.addStatus("Failed to login to " + providerName + ": OAuth state mismatch")
			return nil
		}
		code = parsedCode
	case callback := <-callbackCh:
		if callback.Err != nil {
			if restore != nil {
				restore()
			}
			h.addStatus("Failed to login to " + providerName + ": " + callback.Err.Error())
			return nil
		}
		code = callback.Code
	case <-h.done:
		if restore != nil {
			restore()
		}
		return nil
	}
	if strings.TrimSpace(code) == "" {
		if restore != nil {
			restore()
		}
		h.addStatus("Failed to login to " + providerName + ": missing authorization code")
		return nil
	}
	dialog.ShowInfo([]string{tuiThemeMuted("Exchanging authorization code for tokens...")})
	h.requestRender(false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-h.done:
			cancel()
		case <-ctx.Done():
		}
	}()
	credential, err := runtime.ExchangeCode(ctx, code, flow.Verifier, flow.RedirectURI)
	if restore != nil {
		restore()
	}
	if err != nil {
		h.addStatus("Failed to login to " + providerName + ": " + err.Error())
		return nil
	}
	registry.authStorage.Set("anthropic", credential)
	if err := h.refreshModelRuntimeAfterCredentialChange(
		ctx,
		registry,
	); err != nil {
		h.addStatus("Failed to refresh models after login to " + providerName + ": " + err.Error())
		return nil
	}
	h.addStatus("Logged in to " + providerName + ". Credentials saved to ~/.gi/agent/auth.json")
	return nil
}

func (h *CLIInteractiveTUIHost) showOpenAICodexOAuthLoginDialog(providerName string) error {
	registry := h.modelRegistry()
	if registry == nil || registry.authStorage == nil {
		h.addStatus("Failed to login to " + providerName + ": auth storage is not configured")
		return nil
	}
	runtime := defaultOpenAICodexOAuthRuntime
	flow, err := runtime.NewFlow()
	if err != nil {
		h.addStatus("Failed to login to " + providerName + ": " + err.Error())
		return nil
	}
	callbackServer, serverErr := runtime.StartCallbackServer(flow.State)
	if callbackServer != nil {
		defer callbackServer.Close()
	}

	dialog := NewLoginDialogComponent("Login to "+providerName, "")
	resultCh := make(chan TUIDialogResult, 1)
	var closeOnce sync.Once
	var restore func()
	finish := func(result TUIDialogResult) {
		closeOnce.Do(func() {
			resultCh <- result
		})
	}
	dialog.OnSubmit = func(value string) {
		finish(TUIDialogResult{Action: "submitted", Value: value})
	}
	dialog.OnCancel = func() {
		finish(TUIDialogResult{Action: "cancelled"})
	}
	instructions := "A browser window should open. Complete login to finish."
	if serverErr != nil {
		instructions = "Could not start the local callback server. Complete login in your browser, then paste the final redirect URL here."
	}
	dialog.ShowAuth(flow.URL, instructions, "Paste redirect URL below, or complete login in browser:")
	restore = h.showEditorReplacement(dialog, dialog)
	if serverErr != nil {
		h.addStatus("Warning: " + serverErr.Error())
	} else if h.shouldAutoOpenOAuthBrowser() && runtime.OpenBrowser != nil {
		_ = runtime.OpenBrowser(flow.URL)
	}

	var callbackCh <-chan openAICodexOAuthCallbackResult
	if callbackServer != nil {
		callbackCh = callbackServer.Result()
	}
	code := ""
	select {
	case result := <-resultCh:
		if result.Action != "submitted" {
			if restore != nil {
				restore()
			}
			return nil
		}
		parsedCode, parsedState := parseOpenAICodexAuthorizationInput(dialogStringValue(result.Value))
		if parsedState != "" && parsedState != flow.State {
			if restore != nil {
				restore()
			}
			h.addStatus("Failed to login to " + providerName + ": state mismatch")
			return nil
		}
		code = parsedCode
	case callback := <-callbackCh:
		if callback.Err != nil {
			if restore != nil {
				restore()
			}
			h.addStatus("Failed to login to " + providerName + ": " + callback.Err.Error())
			return nil
		}
		code = callback.Code
	case <-h.done:
		if restore != nil {
			restore()
		}
		return nil
	}
	if strings.TrimSpace(code) == "" {
		if restore != nil {
			restore()
		}
		h.addStatus("Failed to login to " + providerName + ": missing authorization code")
		return nil
	}
	dialog.ShowInfo([]string{tuiThemeMuted("Completing authentication...")})
	h.requestRender(false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-h.done:
			cancel()
		case <-ctx.Done():
		}
	}()
	credential, err := runtime.ExchangeCode(ctx, code, flow.Verifier)
	if restore != nil {
		restore()
	}
	if err != nil {
		h.addStatus("Failed to login to " + providerName + ": " + err.Error())
		return nil
	}
	registry.authStorage.Set("openai-codex", credential)
	if err := h.refreshModelRuntimeAfterCredentialChange(
		ctx,
		registry,
	); err != nil {
		h.addStatus("Failed to refresh models after login to " + providerName + ": " + err.Error())
		return nil
	}
	h.addStatus("Logged in to " + providerName + ". Credentials saved to ~/.gi/agent/auth.json")
	return nil
}

func (h *CLIInteractiveTUIHost) shouldAutoOpenOAuthBrowser() bool {
	if h == nil {
		return false
	}
	if os.Getenv("GI_OAUTH_NO_BROWSER") == "1" {
		return false
	}
	if _, ok := h.terminal.(*gitui.VirtualTerminal); ok {
		return false
	}
	return true
}

func (h *CLIInteractiveTUIHost) showGitHubCopilotOAuthLoginDialog(providerName string) error {
	registry := h.modelRegistry()
	if registry == nil || registry.authStorage == nil {
		h.addStatus("Failed to login to " + providerName + ": auth storage is not configured")
		return nil
	}
	enterpriseDomain, cancelled, err := h.promptForGitHubCopilotEnterpriseDomain(providerName)
	if err != nil {
		return err
	}
	if cancelled {
		return nil
	}
	domain := githubCopilotDefaultDomain
	if enterpriseDomain != "" {
		domain = enterpriseDomain
	}
	runtime := defaultGitHubCopilotOAuthRuntime
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-h.done:
			cancel()
		case <-ctx.Done():
		}
	}()
	device, err := runtime.StartDeviceFlow(ctx, domain)
	if err != nil {
		h.addStatus("Failed to login to " + providerName + ": " + err.Error())
		return nil
	}

	dialog := NewLoginDialogComponent("Login to "+providerName, "")
	resultCh := make(chan TUIDialogResult, 1)
	var closeOnce sync.Once
	var restore func()
	finish := func(result TUIDialogResult) {
		closeOnce.Do(func() {
			resultCh <- result
		})
	}
	dialog.OnCancel = func() {
		cancel()
		finish(TUIDialogResult{Action: "cancelled"})
	}
	dialog.ShowAuth(device.VerificationURI, "Enter code: "+device.UserCode, "")
	restore = h.showEditorReplacement(dialog, dialog)
	if h.shouldAutoOpenOAuthBrowser() && runtime.OpenBrowser != nil {
		_ = runtime.OpenBrowser(device.VerificationURI)
	}

	type pollResult struct {
		token string
		err   error
	}
	pollCh := make(chan pollResult, 1)
	go func() {
		token, pollErr := runtime.PollAccessToken(ctx, domain, device)
		pollCh <- pollResult{token: token, err: pollErr}
	}()
	var githubAccessToken string
	select {
	case result := <-resultCh:
		if result.Action != "submitted" {
			if restore != nil {
				restore()
			}
			return nil
		}
	case result := <-pollCh:
		if result.err != nil {
			if restore != nil {
				restore()
			}
			if result.err.Error() != "Login cancelled" {
				h.addStatus("Failed to login to " + providerName + ": " + result.err.Error())
			}
			return nil
		}
		githubAccessToken = result.token
	case <-h.done:
		if restore != nil {
			restore()
		}
		return nil
	}
	if githubAccessToken == "" {
		if restore != nil {
			restore()
		}
		h.addStatus("Failed to login to " + providerName + ": missing GitHub access token")
		return nil
	}
	dialog.ShowInfo([]string{tuiThemeMuted("Configuring GitHub Copilot...")})
	h.requestRender(false)
	credential, err := runtime.RefreshToken(ctx, githubAccessToken, enterpriseDomain)
	if err == nil && runtime.EnableModels != nil {
		_ = runtime.EnableModels(ctx, credential)
	}
	if restore != nil {
		restore()
	}
	if err != nil {
		h.addStatus("Failed to login to " + providerName + ": " + err.Error())
		return nil
	}
	registry.authStorage.Set("github-copilot", credential)
	if err := h.refreshModelRuntimeAfterCredentialChange(
		ctx,
		registry,
	); err != nil {
		h.addStatus("Failed to refresh models after login to " + providerName + ": " + err.Error())
		return nil
	}
	h.addStatus("Logged in to " + providerName + ". Credentials saved to ~/.gi/agent/auth.json")
	return nil
}

func (h *CLIInteractiveTUIHost) promptForGitHubCopilotEnterpriseDomain(providerName string) (string, bool, error) {
	if h == nil || h.ui == nil {
		return "", true, errors.New("interactive TUI is not ready")
	}
	dialog := NewLoginDialogComponent("Login to "+providerName, "GitHub Enterprise URL/domain (blank for github.com)")
	completion := newCLIDialogCompletion()
	finish := func(result TUIDialogResult) {
		completion.finish(result)
	}
	dialog.OnSubmit = func(value string) {
		finish(TUIDialogResult{Action: "submitted", Value: value})
	}
	dialog.OnCancel = func() {
		finish(TUIDialogResult{Action: "cancelled"})
	}
	completion.installRestore(h.showEditorReplacement(dialog, dialog))
	result := completion.wait(h.done)
	if result.Action != "submitted" {
		return "", true, nil
	}
	value := dialogStringValue(result.Value)
	domain, ok := normalizeGitHubCopilotDomain(value)
	if !ok {
		h.addStatus("Failed to login to " + providerName + ": invalid GitHub Enterprise URL/domain")
		return "", true, nil
	}
	return domain, false, nil
}

func (h *CLIInteractiveTUIHost) promptForAPIKey(providerName string) (string, bool, error) {
	if h == nil || h.ui == nil {
		return "", true, errors.New("interactive TUI is not ready")
	}
	dialog := NewLoginDialogComponent("Login to "+providerName, "Enter API key:")
	completion := newCLIDialogCompletion()
	finish := func(result TUIDialogResult) {
		completion.finish(result)
	}
	dialog.OnSubmit = func(value string) {
		finish(TUIDialogResult{Action: "submitted", Value: value})
	}
	dialog.OnCancel = func() {
		finish(TUIDialogResult{Action: "cancelled"})
	}
	completion.installRestore(h.showEditorReplacement(dialog, dialog))
	result := completion.wait(h.done)
	if result.Action != "submitted" {
		return "", true, nil
	}
	return dialogStringValue(result.Value), false, nil
}

func (h *CLIInteractiveTUIHost) addBedrockSetupInfo(providerID, providerName string) {
	title := "Amazon Bedrock setup"
	if providerName == "" {
		providerName = providerID
	}
	dialog := NewLoginDialogComponent(title, "")
	dialog.ShowInfo([]string{
		tuiThemeFG("text", "Amazon Bedrock uses AWS credentials instead of a single API key."),
		tuiThemeFG("text", "Configure an AWS profile, IAM keys, bearer token, or role-based credentials."),
		tuiThemeMuted("See:"),
		tuiThemeAccent("  " + giProvidersDocumentationPath(h.interactiveCWD())),
	})
	replacement := &cliEditorReplacementLifecycle{}
	dialog.OnCancel = func() {
		replacement.close()
	}
	replacement.install(h.showEditorReplacement(dialog, dialog))
}

func (h *CLIInteractiveTUIHost) handleLogoutSlashCommand(args string) error {
	registry := h.modelRegistry()
	if registry == nil || registry.authStorage == nil {
		h.addStatus("No local auth storage is configured")
		return nil
	}
	provider := strings.TrimSpace(args)
	if provider != "" {
		if !registry.authStorage.Has(provider) {
			h.addStatus("No stored credential for " + provider + ". Environment variables and models.json config are unchanged.")
			return nil
		}
		registry.authStorage.Remove(provider)
		if err := h.refreshModelRuntimeAfterCredentialChange(
			context.Background(),
			registry,
		); err != nil {
			return err
		}
		h.addStatus("Removed stored credential for " + provider + ". Environment variables and models.json config are unchanged.")
		return nil
	}
	providers := registry.authStorage.List()
	if len(providers) == 0 {
		h.addStatus("No stored credentials to remove. /logout only removes credentials saved by /login; environment variables and models.json config are unchanged.")
		return nil
	}
	if h.exitAfterInitial {
		h.addStatus("Usage: /logout <provider>. Stored providers: " + strings.Join(providers, ", "))
		return nil
	}
	selected, cancelled, err := h.selectAuthProvider("logout", registry, logoutAuthSelectorProviders(registry))
	if err != nil {
		return err
	}
	if cancelled {
		h.addStatus("Logout cancelled")
		return nil
	}
	provider = selected
	if provider == "" {
		return errors.New("invalid provider selection")
	}
	registry.authStorage.Remove(provider)
	if err := h.refreshModelRuntimeAfterCredentialChange(
		context.Background(),
		registry,
	); err != nil {
		return err
	}
	h.addStatus("Removed stored credential for " + provider + ". Environment variables and models.json config are unchanged.")
	return nil
}

func (h *CLIInteractiveTUIHost) refreshModelRuntimeAfterCredentialChange(
	ctx context.Context,
	registry *ModelRegistry,
) error {
	if runtime := h.modelRuntime(); runtime != nil {
		_, err := runtime.Refresh(
			ctx,
			ModelRegistryRefreshOptions{},
		)
		return err
	}
	if registry != nil {
		registry.Refresh()
	}
	return nil
}

func (h *CLIInteractiveTUIHost) selectAuthProvider(mode string, registry *ModelRegistry, providers []AuthSelectorProvider) (string, bool, error) {
	if h == nil || h.ui == nil {
		return "", true, errors.New("interactive TUI is not ready")
	}
	if len(providers) == 0 {
		return "", true, nil
	}
	var authStorage *AuthStorage
	var resolver AuthStatusResolver
	if registry != nil {
		authStorage = registry.authStorage
		resolver = registry.GetProviderAuthStatus
	}
	selector := NewOAuthSelectorComponent(OAuthSelector{
		Mode:           mode,
		AuthStorage:    authStorage,
		Providers:      providers,
		StatusResolver: resolver,
	})
	completion := newCLIDialogCompletion()
	finish := func(result TUIDialogResult) {
		completion.finish(result)
	}
	selector.OnSelect = func(providerID string) {
		finish(TUIDialogResult{Action: "selected", Value: providerID})
	}
	selector.OnCancel = func() {
		finish(TUIDialogResult{Action: "cancelled"})
	}
	completion.installRestore(h.showEditorReplacement(selector, selector))
	result := completion.wait(h.done)
	if result.Action != "selected" {
		return "", true, nil
	}
	return dialogStringValue(result.Value), false, nil
}
