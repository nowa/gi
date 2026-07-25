package gicodingagent

import (
	"context"
	"errors"
	"os"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

func (h *CLIInteractiveTUIHost) handleLoginSlashCommand(args string) error {
	providerID := strings.TrimSpace(args)
	credentialType := llm.CredentialTypeAPIKey
	if providerID == "" && !h.exitAfterInitial {
		registry := h.modelRegistry()
		if registry != nil {
			selected, selectedType, handled, err := h.selectLoginProvider(registry)
			if err != nil {
				return err
			}
			if !handled {
				return nil
			}
			providerID = selected
			credentialType = llm.CredentialType(selectedType)
		}
	}
	if providerID != "" && !h.exitAfterInitial {
		return h.runInteractiveLogin(providerID, credentialType)
	}
	message := providerLoginHelp()
	if providerID != "" {
		message = formatNoAPIKeyFoundMessage(providerID)
	} else if !h.exitAfterInitial {
		h.addStatus("No API key providers available. Configure ~/.gi/agent/models.json or provider environment variables.")
	}
	h.chat.AddChild(newCLIMarkdownWithOptions(
		"**Login**\n\n"+message,
		gitui.MarkdownOptions{PaddingX: 1, PaddingY: 1},
	))
	h.requestRender(false)
	return nil
}

func (h *CLIInteractiveTUIHost) selectLoginProvider(
	registry *ModelRegistry,
) (providerID string, authType string, handled bool, err error) {
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
			if selectedAuthType == string(llm.CredentialTypeOAuth) {
				h.addStatus("No subscription providers available.")
			} else {
				h.addStatus("No API key providers available.")
			}
			return "", "", false, nil
		}
		selected, providerCancelled, err := h.selectAuthProvider(
			"login",
			registry,
			providers,
		)
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
	selector := NewExtensionSelectorComponent(
		"Select authentication method:",
		[]string{subscriptionLabel, apiKeyLabel},
	)
	completion := newCLIDialogCompletion()
	selector.OnSelect = func(option string) {
		credentialType := llm.CredentialTypeAPIKey
		if option == subscriptionLabel {
			credentialType = llm.CredentialTypeOAuth
		}
		completion.finish(TUIDialogResult{
			Action: "selected",
			Value:  string(credentialType),
		})
	}
	selector.OnCancel = func() {
		completion.finish(TUIDialogResult{Action: "cancelled"})
	}
	completion.installRestore(h.showEditorReplacement(selector, selector))
	result := completion.wait(h.done)
	if result.Action != "selected" {
		return "", true, nil
	}
	return dialogStringValue(result.Value), false, nil
}

func (h *CLIInteractiveTUIHost) runInteractiveLogin(
	providerID string,
	credentialType llm.CredentialType,
) error {
	runtime := h.modelRuntime()
	if runtime == nil {
		h.chat.AddChild(newCLIMarkdownWithOptions(
			"**Login**\n\n"+formatNoAPIKeyFoundMessage(providerID),
			gitui.MarkdownOptions{PaddingX: 1, PaddingY: 1},
		))
		h.requestRender(false)
		return nil
	}
	provider, ok := runtime.GetProvider(providerID)
	if !ok {
		h.addStatus("Failed to login: unknown provider " + providerID)
		return nil
	}
	providerName := firstNonEmptyString(
		strings.TrimSpace(provider.Name),
		providerID,
	)
	return h.showProviderLoginDialog(
		providerID,
		providerName,
		credentialType,
	)
}

func (h *CLIInteractiveTUIHost) showProviderLoginDialog(
	providerID string,
	providerName string,
	credentialType llm.CredentialType,
) error {
	if h == nil || h.ui == nil {
		return errors.New("interactive TUI is not ready")
	}
	runtime := h.modelRuntime()
	if runtime == nil {
		h.addStatus(
			"Failed to login to " + providerName +
				": model runtime is not configured",
		)
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	interaction := newCLIProviderAuthInteraction(
		h,
		providerName,
		cancel,
	)
	if providerID == "amazon-bedrock" &&
		credentialType == llm.CredentialTypeAPIKey {
		interaction.ShowDetails([]string{
			tuiThemeFG(
				"text",
				"You can also use an AWS profile, IAM keys, or role-based credentials.",
			),
			tuiThemeMuted("See:"),
			tuiThemeAccent(
				"  " + giProvidersDocumentationPath(h.interactiveCWD()),
			),
		})
	}
	stopDoneWatch := make(chan struct{})
	go func() {
		select {
		case <-h.done:
			interaction.cancelLogin()
		case <-stopDoneWatch:
		}
	}()

	_, err := runtime.Login(
		ctx,
		providerID,
		credentialType,
		interaction,
	)
	cancelled := interaction.Cancelled() || ctx.Err() != nil
	close(stopDoneWatch)
	interaction.Close()
	cancel()
	if err != nil {
		if cancelled ||
			errors.Is(err, context.Canceled) ||
			strings.Contains(err.Error(), "Login cancelled") {
			return nil
		}
		action := "login to "
		if credentialType == llm.CredentialTypeAPIKey {
			action = "save API key for "
		}
		h.addStatus(
			"Failed to " + action + providerName + ": " + err.Error(),
		)
		return nil
	}
	if credentialType == llm.CredentialTypeAPIKey {
		h.addStatus(
			"Saved API key for " + providerName +
				". Credentials saved to ~/.gi/agent/auth.json",
		)
		return nil
	}
	h.addStatus(
		"Logged in to " + providerName +
			". Credentials saved to ~/.gi/agent/auth.json",
	)
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
