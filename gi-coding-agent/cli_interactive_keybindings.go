package gicodingagent

import (
	"strings"
	"time"

	gitui "github.com/nowa/gi/gi-tui"
)

func (h *CLIInteractiveTUIHost) handleProtocolShortcutKey(data string, keybindings KeybindingsConfig) bool {
	if h == nil || data == "" {
		return false
	}
	session := h.agentSession()
	if session == nil || session.ExtensionRuntime == nil {
		return false
	}
	shortcuts := session.ExtensionRuntime.Shortcuts(keybindings).Shortcuts
	if len(shortcuts) == 0 {
		return false
	}
	for key, shortcut := range shortcuts {
		if !gitui.MatchesKey(data, key) {
			continue
		}
		if shortcut.Handler != nil {
			go func(registration ProtocolShortcutRegistration) {
				if err := registration.Handler(); err != nil {
					h.addStatus("Error: Shortcut handler error: " + err.Error())
				}
			}(shortcut)
		}
		return true
	}
	return false
}

func (h *CLIInteractiveTUIHost) handleClearKey() {
	if h == nil {
		return
	}
	now := time.Now()
	if !h.lastClearKeyTime.IsZero() && now.Sub(h.lastClearKeyTime) < 500*time.Millisecond {
		h.Stop()
		return
	}
	h.lastClearKeyTime = now
	h.setActiveEditorText("")
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) handleEscapeKey() bool {
	if h == nil {
		return false
	}
	if h.activeEditorShowingAutocomplete() {
		return false
	}
	session := h.agentSession()
	if session != nil && session.IsBashRunning() {
		session.AbortBash()
		h.addStatus("Bash command cancelled")
		return true
	}
	if session != nil && session.IsRetrying() {
		session.AbortRetry()
		h.clearRetryStatus()
		h.addStatus("Retry cancelled")
		return true
	}
	if session != nil && session.IsBranchSummaryRunning() {
		session.AbortBranchSummary()
		return true
	}
	if session != nil && session.IsCompacting() {
		session.AbortCompaction()
		h.clearCompactionLoader()
		h.addStatus("Compaction cancelled")
		return true
	}
	if session != nil && session.IsStreaming() {
		h.restoreQueuedMessagesToEditor(false)
		_ = session.Abort()
		return true
	}
	if strings.HasPrefix(strings.TrimSpace(h.activeEditorText()), "!") {
		h.setActiveEditorText("")
		h.requestRender(false)
		return true
	}
	if !h.editorIsEmpty() {
		return false
	}
	action := "tree"
	if settings := h.settingsManager(); settings != nil {
		action = settings.GetDoubleEscapeAction()
	}
	if action == "none" {
		return true
	}
	now := time.Now()
	if h.lastEscapeKeyTime.IsZero() || now.Sub(h.lastEscapeKeyTime) >= 500*time.Millisecond {
		h.lastEscapeKeyTime = now
		return true
	}
	h.lastEscapeKeyTime = time.Time{}
	go h.runDoubleEscapeAction(action)
	return true
}

func (h *CLIInteractiveTUIHost) editorIsEmpty() bool {
	if h == nil {
		return true
	}
	return strings.TrimSpace(h.activeEditorText()) == ""
}

func (h *CLIInteractiveTUIHost) runDoubleEscapeAction(action string) {
	var err error
	switch action {
	case "fork":
		err = h.handleForkSlashCommand("")
	default:
		err = h.handleTreeSlashCommand("")
	}
	if err != nil {
		h.addStatus("Error: " + err.Error())
	}
}

func (h *CLIInteractiveTUIHost) handleAppActionKey(data string, keybindings KeybindingsConfig) bool {
	if h == nil {
		return false
	}
	if h.focusedDefaultEditor() && matchesKeybindingAction(data, keybindings, "app.clipboard.pasteImage") {
		go h.handleClipboardPaste()
		return true
	}
	if h.activeEditorShowingAutocomplete() {
		return false
	}
	switch {
	case matchesKeybindingAction(data, keybindings, "app.thinking.cycle"):
		go h.cycleThinkingLevelFromKey()
		return true
	case matchesKeybindingAction(data, keybindings, "app.model.cycleForward"):
		go h.cycleModelFromKey("forward")
		return true
	case matchesKeybindingAction(data, keybindings, "app.model.cycleBackward"):
		go h.cycleModelFromKey("backward")
		return true
	case matchesKeybindingAction(data, keybindings, "app.model.select"):
		go func() {
			if err := h.handleModelSlashCommand(""); err != nil {
				h.addStatus("Error: " + err.Error())
			}
		}()
		return true
	case matchesKeybindingAction(data, keybindings, "app.tools.expand"):
		h.toggleToolOutputExpansion()
		return true
	case matchesKeybindingAction(data, keybindings, "app.thinking.toggle"):
		go h.toggleThinkingBlockVisibility()
		return true
	case matchesKeybindingAction(data, keybindings, "app.editor.external"):
		if !h.focusedDefaultEditor() {
			return false
		}
		go h.openExternalEditor()
		return true
	case matchesKeybindingAction(data, keybindings, "app.message.copy"):
		if !h.focusedDefaultEditor() {
			return false
		}
		go func() {
			if err := h.handleCopySlashCommand(); err != nil {
				h.addStatus("Error: " + err.Error())
			}
		}()
		return true
	case matchesKeybindingAction(data, keybindings, "app.suspend"):
		go h.handleSuspend()
		return true
	case matchesKeybindingAction(data, keybindings, "app.session.new"):
		go h.runSessionActionKey("new")
		return true
	case matchesKeybindingAction(data, keybindings, "app.session.tree"):
		go h.runSessionActionKey("tree")
		return true
	case matchesKeybindingAction(data, keybindings, "app.session.fork"):
		go h.runSessionActionKey("fork")
		return true
	case matchesKeybindingAction(data, keybindings, "app.session.resume"):
		go h.runSessionActionKey("resume")
		return true
	default:
		return false
	}
}

func (h *CLIInteractiveTUIHost) runSessionActionKey(action string) {
	var err error
	switch action {
	case "new":
		err = h.handleNewSlashCommand()
	case "tree":
		err = h.handleTreeSlashCommand("")
	case "fork":
		err = h.handleForkSlashCommand("")
	case "resume":
		err = h.handleResumeSlashCommand("/resume")
	default:
		return
	}
	if err != nil {
		h.addStatus("Error: " + err.Error())
	}
}

func (h *CLIInteractiveTUIHost) handleSuspend() {
	if h == nil || h.ui == nil {
		return
	}
	mode := &InteractiveMode{
		UI: cliInteractiveSuspendUI{ui: h.ui},
		ShowStatus: func(text string) {
			h.addStatus(text)
		},
		Suspend: h.suspend,
	}
	if err := mode.HandleCtrlZ(); err != nil {
		h.addStatus("Error: " + err.Error())
	}
}

func isShiftTabKey(data string) bool {
	event := gitui.ParseKey(data)
	return event.Shift && (event.Key == gitui.KeyTab || event.Key == gitui.KeyBacktab)
}

func (h *CLIInteractiveTUIHost) handleMessageQueueKey(data string, keybindings KeybindingsConfig) bool {
	if h == nil {
		return false
	}
	if h.activeEditorShowingAutocomplete() {
		return false
	}
	switch {
	case matchesKeybindingAction(data, keybindings, "app.message.followUp"):
		go h.submitEditorTextAs("follow-up")
		return true
	case matchesKeybindingAction(data, keybindings, "app.message.dequeue"):
		h.restoreQueuedMessagesToEditor(true)
		return true
	case matchesKeybindingAction(data, keybindings, "tui.input.submit") && h.agentSessionStreaming():
		go h.submitEditorTextAs("steering")
		return true
	default:
		return false
	}
}

func (h *CLIInteractiveTUIHost) submitEditorTextAs(kind string) {
	if h == nil {
		return
	}
	editor, ok := h.activeEditorComponent()
	if !ok {
		return
	}
	text := strings.TrimSpace(h.activeEditorText())
	if text == "" {
		return
	}
	if history, ok := editor.(gitui.EditorHistoryComponent); ok {
		history.AddToHistory(text)
	}
	editor.SetText("")
	h.requestRender(false)
	var err error
	if kind == "follow-up" {
		err = h.submitFollowUpPrompt(text, nil)
	} else {
		err = h.submitPrompt(text, nil)
	}
	if err != nil {
		h.addStatus("Error: " + err.Error())
	}
}

func (h *CLIInteractiveTUIHost) reloadKeybindings() {
	if h == nil {
		return
	}
	keybindings := DefaultProtocolKeybindings()
	if settings := h.settingsManager(); settings != nil && settings.agentDir != "" {
		keybindings = NewKeybindingsManager(settings.agentDir).GetEffectiveConfig()
	}
	h.keybindings = keybindings
	if h.startupHeader != nil {
		h.startupHeader.SetKeybindings(keybindings)
	}
	if !h.tuiKeybindingsInstalled {
		h.previousTUIKeybindings = gitui.GetKeybindings()
		h.tuiKeybindingsInstalled = true
	}
	gitui.SetKeybindings(gitui.NewKeybindingsManager(tuiKeybindingsFromProtocol(keybindings)))
}

func (h *CLIInteractiveTUIHost) effectiveKeybindings() KeybindingsConfig {
	if h != nil && h.keybindings != nil {
		return h.keybindings
	}
	return DefaultProtocolKeybindings()
}

func tuiKeybindingsFromProtocol(keybindings KeybindingsConfig) gitui.KeybindingsConfig {
	result := gitui.KeybindingsConfig{}
	for action, value := range keybindings {
		if !strings.HasPrefix(action, "tui.") {
			continue
		}
		keys := keybindingValueKeys(value)
		if len(keys) > 0 {
			result[action] = keys
		}
	}
	return result
}

func matchesKeybindingAction(data string, keybindings KeybindingsConfig, action string) bool {
	if data == "" {
		return false
	}
	if keybindings == nil {
		keybindings = DefaultProtocolKeybindings()
	}
	for _, key := range keybindingValueKeys(keybindings[action]) {
		if strings.EqualFold(key, "shift+tab") && isShiftTabKey(data) {
			return true
		}
		if gitui.MatchesKey(data, key) {
			return true
		}
	}
	return false
}
