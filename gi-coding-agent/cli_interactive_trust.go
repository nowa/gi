package gicodingagent

import (
	"fmt"
	"strconv"
)

func (h *CLIInteractiveTUIHost) handleTrustSlashCommand() error {
	settings := h.settingsManager()
	if settings == nil {
		return fmt.Errorf("project trust requires settings")
	}
	cwd := h.interactiveCWD()
	store := NewProjectTrustStore(settings.agentDir)
	saved, err := store.GetEntry(cwd)
	if err != nil {
		return err
	}
	options := GetProjectTrustOptions(cwd, false)
	dialogOptions, defaultValue := projectTrustDialogOptions(options, saved)
	savedLabel := projectTrustSavedDecisionLabel(cwd, saved)
	result, err := h.RunTUIDialog(TUIDialogRequest{
		Kind:         "select",
		Title:        "Project trust",
		Message:      fmt.Sprintf("%s\nSaved decision: %s\nCurrent session: %s", cwd, savedLabel, projectTrustDecisionLabel(settings.IsProjectTrusted())),
		Options:      dialogOptions,
		DefaultValue: defaultValue,
	})
	if err != nil {
		return err
	}
	if result.Action != "selected" {
		h.addStatus("Project trust selection cancelled")
		return nil
	}
	index, err := strconv.Atoi(result.OptionID)
	if err != nil || index < 0 || index >= len(options) {
		return fmt.Errorf("invalid project trust selection")
	}
	selected := options[index]
	if err := store.SetMany(selected.Updates); err != nil {
		return err
	}
	h.addStatus(fmt.Sprintf(
		"Saved trust decision: %s. Restart gi for this to take effect.",
		projectTrustDecisionLabel(selected.Trusted),
	))
	return nil
}

func projectTrustDialogOptions(options []ProjectTrustOption, saved *ProjectTrustStoreEntry) ([]TUIDialogOption, int) {
	dialogOptions := make([]TUIDialogOption, 0, len(options))
	defaultValue := 0
	for index, option := range options {
		if saved != nil && option.SavedPath == saved.Path && option.Trusted == saved.Decision {
			defaultValue = index
		}
		label := option.Label
		if index == defaultValue && saved != nil {
			label += " ✓"
		}
		dialogOptions = append(dialogOptions, TUIDialogOption{
			ID:    strconv.Itoa(index),
			Label: label,
			Value: index,
		})
	}
	return dialogOptions, defaultValue
}

func projectTrustSavedDecisionLabel(cwd string, saved *ProjectTrustStoreEntry) string {
	if saved == nil {
		return "none"
	}
	label := projectTrustDecisionLabel(saved.Decision)
	if normalizeProjectTrustPath(cwd) != saved.Path {
		return fmt.Sprintf("%s (inherited from %s)", label, saved.Path)
	}
	return fmt.Sprintf("%s (%s)", label, saved.Path)
}

func projectTrustDecisionLabel(trusted bool) string {
	if trusted {
		return "trusted"
	}
	return "untrusted"
}
