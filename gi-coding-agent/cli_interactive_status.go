package gicodingagent

import (
	"strings"

	gitui "github.com/nowa/gi/gi-tui"
)

type cliRenderedLinesComponent struct {
	render func(width int) []string
}

func (c cliRenderedLinesComponent) Render(width int) []string {
	if c.render == nil {
		return nil
	}
	return normalizeRenderedLines(c.render(width))
}

func (c cliRenderedLinesComponent) Invalidate() {}

func (h *CLIInteractiveTUIHost) addStatus(text string) *gitui.Text {
	if strings.TrimSpace(text) == "" || h.chat == nil {
		return nil
	}
	h.statusMu.Lock()
	status := h.addStatusLocked(text)
	h.statusMu.Unlock()
	if status != nil {
		h.requestRender(false)
	}
	return status
}

func (h *CLIInteractiveTUIHost) addStatusLocked(text string) *gitui.Text {
	if statusTextCoalescible(text) {
		children := h.chat.Children()
		if len(children) > 1 && h.lastStatusText != nil && h.lastStatusSpacer != nil &&
			children[len(children)-1] == h.lastStatusText &&
			children[len(children)-2] == h.lastStatusSpacer {
			h.lastStatusText.SetText(tuiThemeStatusText(text))
			return h.lastStatusText
		}
	}
	spacer := gitui.NewSpacer(1)
	status := gitui.NewText(tuiThemeStatusText(text), 1, 0)
	h.chat.AddChild(spacer)
	h.chat.AddChild(status)
	if statusTextNeedsTrailingSpacer(text) {
		h.chat.AddChild(gitui.NewSpacer(1))
	}
	if statusTextCoalescible(text) {
		h.lastStatusSpacer = spacer
		h.lastStatusText = status
	} else {
		h.lastStatusSpacer = nil
		h.lastStatusText = nil
	}
	return status
}

func (h *CLIInteractiveTUIHost) addWarning(text string) *gitui.Text {
	if h == nil || h.chat == nil {
		return nil
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if !strings.HasPrefix(text, "Warning:") {
		text = "Warning: " + text
	}
	h.statusMu.Lock()
	h.chat.AddChild(gitui.NewSpacer(1))
	warning := gitui.NewText(tuiThemeWarning(text), 1, 0)
	h.chat.AddChild(warning)
	h.lastStatusSpacer = nil
	h.lastStatusText = nil
	h.statusMu.Unlock()
	h.requestRender(false)
	return warning
}

func tuiThemeStatusText(text string) string {
	switch {
	case strings.HasPrefix(text, "Error:"), strings.HasPrefix(text, "Failed "):
		return tuiThemeError(text)
	case strings.HasPrefix(text, "Warning:"), strings.HasPrefix(text, "models.json error:"):
		return tuiThemeWarning(text)
	default:
		return tuiThemeDim(text)
	}
}

func statusTextCoalescible(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	for _, prefix := range []string{"Error:", "Warning:", "models.json error:", "Failed "} {
		if strings.HasPrefix(text, prefix) {
			return false
		}
	}
	return true
}

func statusTextNeedsTrailingSpacer(text string) bool {
	text = strings.TrimSpace(text)
	return strings.HasPrefix(text, "Error:") || strings.HasPrefix(text, "Failed ")
}

func (h *CLIInteractiveTUIHost) showExtensionError(event ProtocolExtensionError) {
	if h == nil || h.chat == nil {
		return
	}
	path := strings.TrimSpace(event.ExtensionPath)
	if path == "" {
		path = "extension"
	}
	message := strings.TrimSpace(event.Error)
	if message == "" {
		message = "Unknown error"
	}
	h.statusMu.Lock()
	h.chat.AddChild(gitui.NewText(`Extension "`+path+`" error: `+message, 1, 0))
	if stackLines := extensionErrorStackLines(event.Stack); len(stackLines) > 0 {
		h.chat.AddChild(gitui.NewText(strings.Join(stackLines, "\n"), 1, 0))
	}
	h.lastStatusSpacer = nil
	h.lastStatusText = nil
	h.statusMu.Unlock()
	h.requestRender(false)
}

func extensionErrorStackLines(stack string) []string {
	lines := strings.Split(strings.TrimSpace(stack), "\n")
	if len(lines) <= 1 {
		return nil
	}
	result := make([]string, 0, len(lines)-1)
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		result = append(result, "  "+line)
	}
	return result
}

func (h *CLIInteractiveTUIHost) showLoaderLocked() {
	if h.ui == nil || h.statusContainer == nil || !h.workingVisible {
		return
	}
	if h.activeStatusIndicatorKind() != "" {
		return
	}
	h.showStatusIndicator(NewWorkingStatusIndicator(
		h.ui,
		h.workingMessageLocked(),
		cloneWorkingIndicatorOptions(h.workingIndicator),
	))
}

func (h *CLIInteractiveTUIHost) showStatusIndicator(indicator transientStatusIndicator) bool {
	if h == nil || indicator == nil {
		return false
	}
	h.statusMu.Lock()
	previous := h.activeStatusIndicator
	h.activeStatusIndicator = nil
	if h.statusContainer == nil {
		h.statusMu.Unlock()
		indicator.Dispose()
		if previous != nil {
			previous.Dispose()
		}
		return false
	}
	h.activeStatusIndicator = indicator
	h.statusContainer.Clear()
	h.statusContainer.AddChild(indicator)
	h.statusMu.Unlock()
	if previous != nil {
		previous.Dispose()
	}
	return true
}

func (h *CLIInteractiveTUIHost) clearStatusIndicator(kinds ...StatusIndicatorKind) bool {
	if h == nil {
		return false
	}
	var expected StatusIndicatorKind
	if len(kinds) > 0 {
		expected = kinds[0]
	}

	h.statusMu.Lock()
	indicator := h.activeStatusIndicator
	if expected != "" && (indicator == nil || indicator.StatusKind() != expected) {
		h.statusMu.Unlock()
		return false
	}
	h.activeStatusIndicator = nil
	if h.statusContainer != nil {
		h.statusContainer.Clear()
		if indicator != nil && h.ui != nil && h.ui.GetClearOnShrink() {
			h.statusContainer.AddChild(&h.idleStatus)
		}
	}
	h.statusMu.Unlock()
	if indicator != nil {
		indicator.Dispose()
		return true
	}
	return false
}

func (h *CLIInteractiveTUIHost) activeStatusIndicatorKind() StatusIndicatorKind {
	if h == nil {
		return ""
	}
	h.statusMu.Lock()
	defer h.statusMu.Unlock()
	if h.activeStatusIndicator == nil {
		return ""
	}
	return h.activeStatusIndicator.StatusKind()
}

func (h *CLIInteractiveTUIHost) workingStatusIndicator() *WorkingStatusIndicator {
	if h == nil {
		return nil
	}
	h.statusMu.Lock()
	defer h.statusMu.Unlock()
	indicator, _ := h.activeStatusIndicator.(*WorkingStatusIndicator)
	return indicator
}

func (h *CLIInteractiveTUIHost) showCompactionLoader(reason CompactionStatusReason) {
	if h == nil {
		return
	}
	if !h.showStatusIndicator(NewCompactionStatusIndicator(h.ui, reason)) {
		h.addStatus(compactionStatusMessage(reason))
		return
	}
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) clearCompactionLoader() {
	if h != nil && h.clearStatusIndicator(StatusIndicatorKindCompaction) {
		h.requestRender(false)
	}
}

func (h *CLIInteractiveTUIHost) resetLastStatus() {
	if h == nil {
		return
	}
	h.statusMu.Lock()
	h.lastStatusSpacer = nil
	h.lastStatusText = nil
	h.statusMu.Unlock()
}

func (h *CLIInteractiveTUIHost) workingMessageLocked() string {
	if strings.TrimSpace(h.workingMessage) != "" {
		return h.workingMessage
	}
	return "Working..."
}

func (h *CLIInteractiveTUIHost) workingIndicatorOptionsLocked() gitui.LoaderIndicatorOptions {
	options := gitui.LoaderIndicatorOptions{
		SpinnerColor: tuiThemeAccent,
		MessageColor: tuiThemeMuted,
	}
	if h.workingIndicator == nil {
		return options
	}
	options.Frames = cloneOptionalStringSlice(h.workingIndicator.Frames)
	options.IntervalMs = h.workingIndicator.IntervalMs
	return options
}

func cloneOptionalStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string{}, values...)
}

func cloneWorkingIndicatorOptions(options *TUIWorkingIndicatorOptions) *TUIWorkingIndicatorOptions {
	if options == nil {
		return nil
	}
	return &TUIWorkingIndicatorOptions{
		Frames:     cloneOptionalStringSlice(options.Frames),
		IntervalMs: options.IntervalMs,
	}
}

func (h *CLIInteractiveTUIHost) clearLoader() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clearLoaderLocked()
	if h.editor != nil {
		h.editor.DisableSubmit = false
	}
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) clearLoaderLocked() {
	h.clearStatusIndicator(StatusIndicatorKindWorking)
}
