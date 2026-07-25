package gicodingagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	gitui "github.com/nowa/gi/gi-tui"
)

func (h *CLIInteractiveTUIHost) handleNameSlashCommand(args string) error {
	session, err := h.currentAgentSession()
	if err != nil {
		return err
	}
	name := strings.TrimSpace(args)
	if name == "" {
		current := session.SessionManager.GetSessionName()
		if current == "" {
			h.addWarning("Usage: /name <name>")
		} else {
			h.addStatus("Session name: " + current)
		}
		return nil
	}
	if err := session.SetSessionName(name); err != nil {
		return err
	}
	h.updateTerminalTitle()
	h.addStatus("Session name set: " + name)
	return nil
}

func (h *CLIInteractiveTUIHost) handleSessionSlashCommand() error {
	session, err := h.currentAgentSession()
	if err != nil {
		return err
	}
	host, err := h.newRPCSessionHost()
	if err != nil {
		return err
	}
	state := host.GetState()
	details := session.GetSessionStats()
	stats := rpcSessionStatsFromAgentStats(details)
	h.chat.AddChild(gitui.NewSpacer(1))
	h.chat.AddChild(gitui.NewText(renderInteractiveSessionInfo(state, stats, details), 1, 0))
	h.requestRender(false)
	return nil
}

func renderInteractiveSessionInfo(
	state RPCSessionState,
	stats RPCSessionStats,
	details ...AgentSessionStats,
) string {
	label := func(text string) string {
		return tuiThemeDim(text + ":")
	}
	lines := []string{tuiThemeBold("Session Info"), ""}
	if state.SessionName != "" {
		lines = append(lines, label("Name")+" "+state.SessionName)
	}
	sessionFile := firstNonEmptyString(stats.SessionFile, state.SessionFile, "In-memory")
	lines = append(lines,
		label("File")+" "+sessionFile,
		label("ID")+" "+firstNonEmptyString(stats.SessionID, state.SessionID),
	)
	lines = append(lines,
		"",
		tuiThemeBold("Messages"),
		label("User")+" "+formatSessionInfoInt(stats.UserMessages),
		label("Assistant")+" "+formatSessionInfoInt(stats.AssistantMessages),
		label("Tool Calls")+" "+formatSessionInfoInt(stats.ToolCalls),
		label("Tool Results")+" "+formatSessionInfoInt(stats.ToolResults),
		label("Total")+" "+formatSessionInfoInt(stats.TotalMessages),
	)
	lines = append(lines,
		"",
		tuiThemeBold("Tokens"),
	)
	promptTokens := stats.Tokens.Input + stats.Tokens.CacheRead + stats.Tokens.CacheWrite
	lines = append(lines, label("Input")+" "+formatSessionInfoInt(promptTokens))
	if promptTokens > 0 && (stats.Tokens.CacheRead > 0 || stats.Tokens.CacheWrite > 0) {
		hitRate := float64(stats.Tokens.CacheRead) / float64(promptTokens) * 100
		lines = append(
			lines,
			"  "+label("Cached")+" "+
				formatSessionInfoInt(stats.Tokens.CacheRead)+
				" "+tuiThemeDim(fmt.Sprintf("(%.1f%%)", hitRate)),
		)
		written := ""
		if stats.Tokens.CacheWrite > 0 {
			written = " " + tuiThemeDim(
				"("+formatSessionInfoInt(stats.Tokens.CacheWrite)+" written to cache)",
			)
		}
		lines = append(
			lines,
			"  "+label("Uncached")+" "+
				formatSessionInfoInt(stats.Tokens.Input+stats.Tokens.CacheWrite)+
				written,
		)
	}
	lines = append(
		lines,
		label("Output")+" "+formatSessionInfoInt(stats.Tokens.Output),
		label("Total")+" "+formatSessionInfoInt(stats.Tokens.Total),
	)

	var usageBreakdown []UsageCostBreakdownEntry
	var cacheWaste CacheWasteTotals
	if len(details) > 0 {
		usageBreakdown = details[0].UsageBreakdown
		cacheWaste = details[0].CacheWaste
	}
	if stats.Cost > 0 || cacheWaste.MissedTokens > 0 {
		lines = append(lines, "", tuiThemeBold("Cost"), label("Total")+" $"+fmt.Sprintf("%.3f", stats.Cost))
		if len(usageBreakdown) > 1 {
			for _, entry := range usageBreakdown {
				lines = append(
					lines,
					"  "+label(entry.Key)+" $"+fmt.Sprintf("%.3f", entry.Cost)+
						" "+tuiThemeDim("("+formatFooterTokens(entry.Tokens)+" tokens)"),
				)
			}
		}
		if cacheWaste.MissedTokens > 0 {
			missLabel := fmt.Sprintf("%d misses", cacheWaste.MissCount)
			if cacheWaste.MissCount == 1 {
				missLabel = "1 miss"
			}
			detail := formatSessionInfoInt(cacheWaste.MissedTokens) + " tokens, " + missLabel
			value := detail
			if cacheWaste.MissedCost >= 0.0001 {
				value = "$" + fmt.Sprintf("%.3f", cacheWaste.MissedCost) +
					" " + tuiThemeDim("("+detail+")")
			}
			lines = append(lines, label("Cache Re-billed")+" "+value)
		}
	}
	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

func formatSessionInfoInt(value int) string {
	text := strconv.Itoa(value)
	if len(text) <= 3 {
		return text
	}
	var builder strings.Builder
	prefix := len(text) % 3
	if prefix == 0 {
		prefix = 3
	}
	builder.WriteString(text[:prefix])
	for index := prefix; index < len(text); index += 3 {
		builder.WriteByte(',')
		builder.WriteString(text[index : index+3])
	}
	return builder.String()
}

func (h *CLIInteractiveTUIHost) handleNewSlashCommand() error {
	if runtimeHost := h.agentSessionRuntimeHost(); runtimeHost != nil {
		result, err := runtimeHost.NewSession()
		if err != nil {
			return err
		}
		if result.Cancelled {
			h.addStatus("New session cancelled")
			return nil
		}
	} else {
		host, err := h.newRPCSessionHost()
		if err != nil {
			return err
		}
		if _, err := host.handleCommand(context.Background(), RPCCommand{Type: RPCCommandNewSession}); err != nil {
			return err
		}
	}
	h.resetChatState()
	h.addSuccessStatus("✓ New session started")
	return nil
}

func (h *CLIInteractiveTUIHost) addSuccessStatus(text string) *gitui.Text {
	if strings.TrimSpace(text) == "" || h.chat == nil {
		return nil
	}
	h.chat.AddChild(gitui.NewSpacer(1))
	status := gitui.NewText(tuiThemeAccent(text), 1, 1)
	h.chat.AddChild(status)
	h.resetLastStatus()
	h.requestRender(false)
	return status
}

func (h *CLIInteractiveTUIHost) handleExportSlashCommand(path string) error {
	host, err := h.newRPCSessionHost()
	if err != nil {
		return err
	}
	var exported string
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(path)), ".jsonl") {
		exported, err = host.ExportJSONL(path)
	} else {
		exported, err = host.ExportHTML(path)
	}
	if err != nil {
		return fmt.Errorf("Failed to export session: %w", err)
	}
	h.addStatus("Session exported to: " + exported)
	return nil
}

func (h *CLIInteractiveTUIHost) handleShareSlashCommand() error {
	host, err := h.newRPCSessionHost()
	if err != nil {
		return err
	}
	file, err := os.CreateTemp("", "gi-session-*.html")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	defer os.Remove(tempPath)
	if _, err := host.ExportHTML(tempPath); err != nil {
		return fmt.Errorf("Failed to export session: %w", err)
	}
	createGist := h.shareCreateGist
	if createGist == nil {
		createGist = defaultCreateSecretGist
	}
	ctx := context.Background()
	restoreShareLoader := h.showShareLoader(&ctx)
	defer restoreShareLoader()
	gistURL, err := createGist(ctx, tempPath)
	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return err
	}
	gistID, err := gistIDFromShareOutput(gistURL)
	if err != nil {
		return err
	}
	h.addStatus("Share URL: " + shareViewerURL(gistID) + "\nGist: " + strings.TrimSpace(gistURL))
	return nil
}

func (h *CLIInteractiveTUIHost) showShareLoader(ctx *context.Context) func() {
	if h == nil || h.exitAfterInitial || h.ui == nil || h.editorContainer == nil || ctx == nil {
		return func() {}
	}
	loader := NewBorderedLoaderComponent(h.ui, "Creating gist...")
	*ctx = loader.Context()
	var restoreOnce sync.Once
	restore := func() {
		restoreOnce.Do(func() {
			loader.Dispose()
			h.editorContainer.SetChildren([]gitui.Component{h.editor})
			if h.ui != nil && h.editor != nil {
				h.ui.SetFocus(h.editor)
			}
			h.requestRender(true)
		})
	}
	loader.SetOnAbort(func() {
		restore()
		h.addStatus("Share cancelled")
	})
	h.editorContainer.SetChildren([]gitui.Component{loader})
	h.ui.SetFocus(loader)
	h.requestRender(false)
	return restore
}

func (h *CLIInteractiveTUIHost) handleImportSlashCommand(text string) error {
	path, ok := GetPathCommandArgument(strings.TrimSpace(text), "/import")
	if !ok {
		h.addStatus("Error: Usage: /import <path.jsonl>")
		return nil
	}
	if !h.exitAfterInitial {
		result, err := h.RunTUIDialog(TUIDialogRequest{
			Kind:    "confirm",
			Title:   "Import session",
			Message: "Replace current session with " + path + "?",
		})
		if err != nil {
			return err
		}
		if result.Action != "confirmed" {
			h.addStatus("Import cancelled")
			return nil
		}
	}
	return h.importSessionFromJSONL(path)
}

func (h *CLIInteractiveTUIHost) importSessionFromJSONL(path string, cwdOverride ...string) error {
	importer, ok := h.runtimeHost.(interactiveImportRuntimeHost)
	if !ok {
		return errors.New("interactive TUI import requires an import-capable runtime host")
	}
	result, err := importer.ImportFromJsonl(path, cwdOverride...)
	if err == nil {
		if result.Cancelled {
			h.addStatus("Import cancelled")
			return nil
		}
		h.resetChatState()
		h.renderExistingMessages(true)
		h.addStatus("Session imported from: " + path)
		return nil
	}
	var missingCwd MissingSessionCwdError
	if errors.As(err, &missingCwd) && !h.exitAfterInitial {
		result, dialogErr := h.RunTUIDialog(TUIDialogRequest{
			Kind:         "input",
			Title:        "Session CWD missing",
			Message:      "Current session cwd does not exist: " + missingCwd.Issue.SessionCwd,
			DefaultValue: missingCwd.Issue.FallbackCwd,
		})
		if dialogErr != nil {
			return dialogErr
		}
		if result.Action != "submitted" || strings.TrimSpace(dialogStringValue(result.Value)) == "" {
			h.addStatus("Import cancelled")
			return nil
		}
		return h.importSessionFromJSONL(path, dialogStringValue(result.Value))
	}
	var notFound SessionImportFileNotFoundError
	if errors.As(err, &notFound) {
		h.addStatus("Failed to import session: " + notFound.Error())
		return nil
	}
	return err
}

func (h *CLIInteractiveTUIHost) handleResumeSlashCommand(text string) error {
	path, ok := GetPathCommandArgument(strings.TrimSpace(text), "/resume")
	if !ok {
		return h.handleResumeSessionSelector()
	}
	return h.resumeSessionPath(path)
}

func (h *CLIInteractiveTUIHost) handleResumeSessionSelector() error {
	if h == nil {
		return errors.New("interactive TUI host is not ready")
	}
	if h.exitAfterInitial {
		h.addStatus("Usage: /resume <path.jsonl>")
		return nil
	}
	session, err := h.currentAgentSession()
	if err != nil {
		return err
	}
	if session == nil || session.SessionManager == nil {
		return errors.New("resume requires an active session")
	}
	manager := session.SessionManager
	if h.ui == nil {
		if len(ListSessions(manager.GetCWD(), manager.GetSessionDir())) == 0 {
			h.addStatus("No sessions to resume")
			return nil
		}
		options := sessionResumeDialogOptions(manager)
		result, err := h.RunTUIDialog(TUIDialogRequest{Kind: "select", Title: "Resume Session", Options: options})
		if err != nil {
			return err
		}
		if result.Action != "selected" {
			h.addStatus("Resume cancelled")
			return nil
		}
		return h.resumeSessionPath(dialogStringValue(result.Value))
	}

	completion := newCLIDialogCompletion()
	finish := func(result TUIDialogResult) {
		completion.finish(result)
	}
	selector := NewLoadingSessionSelectorComponent(
		func(progress SessionListProgress) ([]SessionInfo, error) {
			return ListSessions(manager.GetCWD(), manager.GetSessionDir(), progress), nil
		},
		func(progress SessionListProgress) ([]SessionInfo, error) {
			return ListAllSessions(filepath.Dir(manager.GetSessionDir()), progress), nil
		},
		SessionSelectorOptions{
			ShowRenameHint:     true,
			CurrentSessionPath: manager.GetSessionFile(),
			Keybindings:        h.effectiveKeybindings(),
			RequestRender:      func() { h.requestRender(false) },
			OnSelect: func(path string) {
				finish(TUIDialogResult{Action: "selected", Value: path})
			},
			OnCancel: func() {
				finish(TUIDialogResult{Action: "cancelled"})
			},
			OnError: func(message string) {
				h.addStatus(message)
			},
			RenameSession: func(path, name string) error {
				name = strings.TrimSpace(name)
				if name == "" {
					return nil
				}
				resumeManager, err := OpenSessionManager(path, manager.GetSessionDir())
				if err != nil {
					return err
				}
				resumeManager.AppendSessionInfo(name)
				return nil
			},
			DeleteSession: func(path string) error {
				if err := os.Remove(path); err != nil {
					h.addStatus("Delete session failed: " + err.Error())
					return err
				}
				h.requestRender(false)
				return nil
			},
		},
	)
	completion.installRestore(h.showEditorReplacement(selector, selector))
	result := completion.wait(h.done)
	if result.Action != "selected" {
		return nil
	}
	path := dialogStringValue(result.Value)
	if strings.TrimSpace(path) == "" {
		return nil
	}
	return h.resumeSessionPath(path)
}

func sessionResumeDialogOptions(manager *SessionManager) []TUIDialogOption {
	if manager == nil {
		return nil
	}
	sessions := ListSessions(manager.GetCWD(), manager.GetSessionDir())
	options := make([]TUIDialogOption, 0, len(sessions))
	for _, session := range sessions {
		if strings.TrimSpace(session.Path) == "" {
			continue
		}
		options = append(options, TUIDialogOption{
			ID:          session.Path,
			Label:       sessionResumeDialogLabel(session),
			Description: sessionResumeDialogDescription(session),
			Value:       session.Path,
		})
	}
	return options
}

func sessionResumeDialogLabel(session SessionInfo) string {
	label := firstNonEmptyString(strings.TrimSpace(session.Name), strings.TrimSpace(session.FirstMessage), strings.TrimSpace(session.ID), filepath.Base(session.Path))
	if label == "" {
		label = "Session"
	}
	return truncateDialogLabel(label)
}

func sessionResumeDialogDescription(session SessionInfo) string {
	parts := []string{}
	if session.MessageCount > 0 {
		parts = append(parts, fmt.Sprintf("%d messages", session.MessageCount))
	}
	if session.CWD != "" {
		parts = append(parts, filepath.Base(session.CWD))
	}
	if len(parts) == 0 {
		return filepath.Base(session.Path)
	}
	return strings.Join(parts, " · ")
}

func (h *CLIInteractiveTUIHost) resumeSessionPath(path string) error {
	if runtimeHost := h.agentSessionRuntimeHost(); runtimeHost != nil {
		result, err := runtimeHost.SwitchSession(path)
		if err != nil {
			return err
		}
		if result.Cancelled {
			h.addStatus("Resume cancelled")
			return nil
		}
	} else {
		host, err := h.newRPCSessionHost()
		if err != nil {
			return err
		}
		if _, err := host.SwitchSession(path); err != nil {
			return err
		}
	}
	h.resetChatState()
	h.renderExistingMessages(true)
	h.addStatus("Session resumed from: " + path)
	return nil
}

func (h *CLIInteractiveTUIHost) handleCopySlashCommand() error {
	host, err := h.newRPCSessionHost()
	if err != nil {
		return err
	}
	text := host.GetLastAssistantText()
	if text == nil || strings.TrimSpace(*text) == "" {
		h.addStatus("Error: No agent messages to copy yet.")
		return nil
	}
	copyFn := h.clipboardCopy
	if copyFn == nil {
		copyFn = func(text string) error {
			return CopyToClipboard(text, ClipboardCopyOptions{})
		}
	}
	if err := copyFn(*text); err != nil {
		h.addStatus("Error: " + err.Error())
		return nil
	}
	h.addStatus("Copied last agent message to clipboard")
	return nil
}

func (h *CLIInteractiveTUIHost) handleCompactSlashCommand(args string) error {
	session, err := h.currentAgentSession()
	if err != nil {
		return err
	}
	messageCount := 0
	for _, entry := range session.SessionManager.GetEntries() {
		if entry.Type == "message" {
			messageCount++
		}
	}
	if messageCount < 2 {
		h.addWarning("Nothing to compact (no messages yet)")
		return nil
	}
	if _, err := session.Compact(strings.TrimSpace(args)); err != nil {
		if isCompactionCancelledError(err) {
			return nil
		}
		return err
	}
	return nil
}

func (h *CLIInteractiveTUIHost) handleCloneSlashCommand() error {
	session, err := h.currentAgentSession()
	if err != nil {
		return err
	}
	leafID := session.SessionManager.GetLeafID()
	if leafID == nil || strings.TrimSpace(*leafID) == "" {
		if runtimeHost := h.agentSessionRuntimeHost(); runtimeHost != nil {
			result, err := runtimeHost.NewSession()
			if err != nil {
				return err
			}
			if result.Cancelled {
				h.addStatus("Clone cancelled")
				return nil
			}
		} else {
			newManager, err := CreateSessionManager(session.SessionManager.GetCWD(), session.SessionManager.GetSessionDir())
			if err != nil {
				return err
			}
			newSession, err := cloneAgentSessionWithManager(session, newManager)
			if err != nil {
				return err
			}
			if owner, ok := h.runtimeHost.(*agentSessionPrintModeHost); ok {
				owner.session = newSession
			} else {
				return errors.New("clone requires a replaceable agent session host")
			}
		}
		h.resetChatState()
		h.renderExistingMessages(true)
		h.addStatus("Cloned to new session")
		return nil
	}
	if runtimeHost := h.agentSessionRuntimeHost(); runtimeHost != nil {
		result, err := runtimeHost.Fork(*leafID, AgentSessionRuntimeForkOptions{Position: "at"})
		if err != nil {
			return err
		}
		if result.Cancelled {
			h.addStatus("Clone cancelled")
			return nil
		}
	} else {
		result, err := session.ForkAt(*leafID)
		if err != nil {
			return err
		}
		if result.Session == nil {
			return errors.New("clone did not produce a session")
		}
		if owner, ok := h.runtimeHost.(*agentSessionPrintModeHost); ok {
			owner.session = result.Session
		} else {
			return errors.New("clone requires a replaceable agent session host")
		}
	}
	h.resetChatState()
	h.renderExistingMessages(true)
	h.addStatus("Cloned to new session")
	return nil
}

func (h *CLIInteractiveTUIHost) handleForkSlashCommand(args string) error {
	session, err := h.currentAgentSession()
	if err != nil {
		return err
	}
	entryID := strings.TrimSpace(args)
	if entryID == "" {
		if h.exitAfterInitial {
			h.addStatus("Usage: /fork <entry-id>")
			return nil
		}
		messages := session.GetUserMessagesForForking()
		if len(messages) == 0 {
			h.addStatus("No messages to fork from")
			return nil
		}
		selectedID, cancelled, err := h.selectForkUserMessage(messages)
		if err != nil {
			return err
		}
		if cancelled {
			h.addStatus("Fork cancelled")
			return nil
		}
		entryID = selectedID
	}
	var result AgentSessionForkResult
	if runtimeHost := h.agentSessionRuntimeHost(); runtimeHost != nil {
		result, err = runtimeHost.Fork(entryID, AgentSessionRuntimeForkOptions{Position: "before"})
	} else {
		result, err = session.Fork(entryID)
		if err == nil && result.Session != nil {
			if owner, ok := h.runtimeHost.(*agentSessionPrintModeHost); ok {
				owner.session = result.Session
			}
		}
	}
	if err != nil {
		return err
	}
	if result.Cancelled {
		h.addStatus("Fork cancelled")
		return nil
	}
	h.resetChatState()
	h.renderExistingMessages(true)
	if strings.TrimSpace(result.SelectedText) != "" && h.editor != nil {
		h.editor.SetText(result.SelectedText)
	}
	h.addStatus("Forked to new session")
	return nil
}

func (h *CLIInteractiveTUIHost) selectForkUserMessage(messages []AgentSessionForkMessage) (string, bool, error) {
	if h == nil || h.ui == nil {
		return "", true, errors.New("interactive TUI is not ready")
	}
	selector := NewUserMessageSelectorComponent(messages, "")
	completion := newCLIDialogCompletion()
	finish := func(result TUIDialogResult) {
		completion.finish(result)
	}
	selector.OnSelect = func(entryID string) {
		finish(TUIDialogResult{Action: "selected", Value: entryID})
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

func (h *CLIInteractiveTUIHost) handleTreeSlashCommand(args string) error {
	session, err := h.currentAgentSession()
	if err != nil {
		return err
	}
	entryID := strings.TrimSpace(args)
	initialSelectedID := entryID
	for {
		selectedFromTree := false
		if entryID == "" {
			if h.exitAfterInitial {
				h.addStatus("Usage: /tree <entry-id>")
				return nil
			}
			selectedID, cancelled, err := h.selectTreeEntry(session, initialSelectedID)
			if err != nil {
				return err
			}
			if cancelled {
				return nil
			}
			entryID = selectedID
			selectedFromTree = true
		}
		if h.treeEntryIsCurrentLeaf(session, entryID) {
			h.addStatus("Already at this point")
			return nil
		}
		options, cancelled, err := h.selectTreeNavigationOptions()
		if err != nil {
			return err
		}
		if cancelled {
			if selectedFromTree && !h.exitAfterInitial {
				initialSelectedID = entryID
				entryID = ""
				continue
			}
			h.addStatus("Tree switch cancelled")
			return nil
		}
		if options.Summarize {
			h.showStatusIndicator(NewBranchSummaryStatusIndicator(h.ui))
			h.requestRender(false)
		}
		result, err := session.NavigateTree(entryID, options)
		if options.Summarize {
			h.clearStatusIndicator(StatusIndicatorKindBranchSummary)
			h.requestRender(false)
		}
		if err != nil {
			return err
		}
		if result.Aborted {
			h.addStatus("Branch summarization cancelled")
			if selectedFromTree && !h.exitAfterInitial {
				initialSelectedID = entryID
				entryID = ""
				continue
			}
			return nil
		}
		if result.Cancelled {
			h.addStatus("Navigation cancelled")
			return nil
		}
		h.resetChatState()
		h.renderExistingMessages(true)
		if strings.TrimSpace(result.EditorText) != "" && h.editor != nil {
			h.editor.SetText(result.EditorText)
		}
		h.addStatus("Navigated to selected point")
		return nil
	}
}

func (h *CLIInteractiveTUIHost) treeEntryIsCurrentLeaf(session *AgentSession, entryID string) bool {
	if session == nil || session.SessionManager == nil || strings.TrimSpace(entryID) == "" {
		return false
	}
	leafID := session.SessionManager.GetLeafID()
	return leafID != nil && *leafID == entryID
}

func (h *CLIInteractiveTUIHost) selectTreeNavigationOptions() (AgentSessionNavigateTreeOptions, bool, error) {
	if h == nil || h.exitAfterInitial {
		return AgentSessionNavigateTreeOptions{}, false, nil
	}
	if settings := h.settingsManager(); settings != nil && settings.GetBranchSummarySkipPrompt() {
		return AgentSessionNavigateTreeOptions{}, false, nil
	}
	for {
		result, err := h.RunTUIDialog(TUIDialogRequest{
			Kind:    "select",
			Title:   "Summarize branch?",
			Message: "Choose whether to summarize the branch before navigating.",
			Options: []TUIDialogOption{
				{ID: "none", Label: "No summary", Value: "none"},
				{ID: "summary", Label: "Summarize", Value: "summary"},
				{ID: "custom", Label: "Summarize with custom prompt", Value: "custom"},
			},
			DefaultValue: "none",
		})
		if err != nil {
			return AgentSessionNavigateTreeOptions{}, false, err
		}
		if result.Action != "selected" {
			return AgentSessionNavigateTreeOptions{}, true, nil
		}
		switch firstNonEmptyString(result.OptionID, dialogStringValue(result.Value)) {
		case "none":
			return AgentSessionNavigateTreeOptions{}, false, nil
		case "summary":
			return AgentSessionNavigateTreeOptions{Summarize: true}, false, nil
		case "custom":
			editorResult, err := h.RunTUIDialog(TUIDialogRequest{
				Kind:  "editor",
				Title: "Custom summarization instructions",
			})
			if err != nil {
				return AgentSessionNavigateTreeOptions{}, false, err
			}
			if editorResult.Action != "submitted" {
				continue
			}
			return AgentSessionNavigateTreeOptions{
				Summarize:          true,
				CustomInstructions: dialogStringValue(editorResult.Value),
			}, false, nil
		default:
			return AgentSessionNavigateTreeOptions{}, false, nil
		}
	}
}

func (h *CLIInteractiveTUIHost) selectTreeEntry(session *AgentSession, initialSelectedID ...string) (string, bool, error) {
	if h == nil || h.ui == nil {
		return "", true, errors.New("interactive TUI is not ready")
	}
	if session == nil || session.SessionManager == nil {
		return "", true, errors.New("interactive TUI session tree is not ready")
	}
	roots := session.SessionManager.GetTree()
	currentLeafID := ""
	if leafID := session.SessionManager.GetLeafID(); leafID != nil {
		currentLeafID = *leafID
	}
	selector := NewTreeSelectorComponent(roots, currentLeafID, TreeSelectorOptions{Keybindings: h.effectiveKeybindings()})
	if len(initialSelectedID) > 0 && strings.TrimSpace(initialSelectedID[0]) != "" {
		selector.selectedID = strings.TrimSpace(initialSelectedID[0])
		selector.rebuild()
	}
	selector.SetFilter(h.treeSelectorInitialFilter())
	if len(roots) > 0 && (selector.GetTreeList() == nil || selector.GetTreeList().GetSelectedNode() == nil) {
		h.addStatus("No tree entries to switch to")
		return "", true, nil
	}
	completion := newCLIDialogCompletion()
	finish := func(result TUIDialogResult) {
		completion.finish(result)
	}
	selector.OnSelect = func(entryID string) {
		finish(TUIDialogResult{Action: "selected", Value: entryID})
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

func (h *CLIInteractiveTUIHost) treeSelectorInitialFilter() TreeSelectorFilter {
	settings := h.settingsManager()
	if settings == nil {
		return TreeSelectorDefaultFilter
	}
	switch settings.GetTreeFilterMode() {
	case "no-tools":
		return TreeSelectorNoToolsFilter
	case "user-only":
		return TreeSelectorUserFilter
	case "labeled-only":
		return TreeSelectorLabelFilter
	case "all":
		return TreeSelectorAllFilter
	default:
		return TreeSelectorDefaultFilter
	}
}

func treeDialogOptions(session *AgentSession) []TUIDialogOption {
	if session == nil || session.SessionManager == nil {
		return nil
	}
	var options []TUIDialogOption
	var walk func(nodes []*SessionTreeNode, depth int)
	walk = func(nodes []*SessionTreeNode, depth int) {
		for _, node := range nodes {
			if node == nil {
				continue
			}
			label := treeDialogLabel(node.Entry)
			if label != "" {
				options = append(options, TUIDialogOption{
					ID:          node.Entry.ID,
					Label:       strings.Repeat("  ", depth) + label,
					Description: node.Entry.ID,
					Value:       node.Entry.ID,
				})
			}
			walk(node.Children, depth+1)
		}
	}
	walk(session.SessionManager.GetTree(), 0)
	return options
}

func treeDialogLabel(entry FileEntry) string {
	switch entry.Type {
	case "message":
		text := strings.TrimSpace(sessionMessageText(entry.Message))
		role := string(sessionMessageRole(entry.Message))
		if text == "" {
			return ""
		}
		return role + ": " + truncateDialogLabel(text)
	case "custom_message":
		return "custom: " + firstNonEmptyString(entry.CustomType, entry.ID)
	case "branch_summary":
		return "summary: " + truncateDialogLabel(entry.Summary)
	default:
		return ""
	}
}

func truncateDialogLabel(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) <= 72 {
		return text
	}
	return string(runes[:69]) + "..."
}
