package gicodingagent

import (
	"context"
	"errors"
	"strings"
)

func (h *CLIInteractiveTUIHost) handleLogoutSlashCommand(args string) error {
	registry := h.modelRegistry()
	if registry == nil || registry.authStorage == nil {
		h.addStatus("No local auth storage is configured")
		return nil
	}
	providerID := strings.TrimSpace(args)
	if providerID != "" {
		if !registry.authStorage.Has(providerID) {
			h.addStatus("No stored credential for " + providerID + ". Environment variables and models.json config are unchanged.")
			return nil
		}
		if err := h.logoutProvider(
			context.Background(),
			registry,
			providerID,
		); err != nil {
			return err
		}
		h.addStatus("Removed stored credential for " + providerID + ". Environment variables and models.json config are unchanged.")
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
	selected, cancelled, err := h.selectAuthProvider(
		"logout",
		registry,
		logoutAuthSelectorProviders(registry),
	)
	if err != nil {
		return err
	}
	if cancelled {
		h.addStatus("Logout cancelled")
		return nil
	}
	providerID = selected
	if providerID == "" {
		return errors.New("invalid provider selection")
	}
	if err := h.logoutProvider(
		context.Background(),
		registry,
		providerID,
	); err != nil {
		return err
	}
	h.addStatus("Removed stored credential for " + providerID + ". Environment variables and models.json config are unchanged.")
	return nil
}

func (h *CLIInteractiveTUIHost) logoutProvider(
	ctx context.Context,
	registry *ModelRegistry,
	providerID string,
) error {
	if runtime := h.modelRuntime(); runtime != nil {
		return runtime.Logout(ctx, providerID)
	}
	if registry == nil || registry.authStorage == nil {
		return errors.New("auth storage is not configured")
	}
	if err := registry.authStorage.DeleteCredential(ctx, providerID); err != nil {
		return err
	}
	registry.Refresh()
	return nil
}

func (h *CLIInteractiveTUIHost) selectAuthProvider(
	mode string,
	registry *ModelRegistry,
	providers []AuthSelectorProvider,
) (string, bool, error) {
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
