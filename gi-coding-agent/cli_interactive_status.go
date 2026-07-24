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
	if h.ui == nil || h.statusContainer == nil || h.loader != nil || !h.workingVisible {
		return
	}
	options := h.workingIndicatorOptionsLocked()
	options.TUI = h.ui
	h.statusContainer.Clear()
	h.loader = gitui.NewLoader(h.workingMessageLocked(), options)
	h.statusContainer.AddChild(h.loader)
}

func (h *CLIInteractiveTUIHost) showStatusLoader(message string) *gitui.Loader {
	if h == nil {
		return nil
	}
	h.statusMu.Lock()
	loader := h.showStatusLoaderLocked(message)
	h.statusMu.Unlock()
	if loader != nil {
		h.requestRender(false)
	}
	return loader
}

func (h *CLIInteractiveTUIHost) showStatusLoaderLocked(message string) *gitui.Loader {
	if h == nil || h.ui == nil || h.statusContainer == nil || strings.TrimSpace(message) == "" {
		return nil
	}
	options := gitui.LoaderIndicatorOptions{
		TUI:          h.ui,
		SpinnerColor: tuiThemeAccent,
		MessageColor: tuiThemeMuted,
	}
	loader := gitui.NewLoader(message, options)
	h.statusContainer.Clear()
	h.statusContainer.AddChild(loader)
	return loader
}

func (h *CLIInteractiveTUIHost) showCompactionLoader(message string) {
	if h == nil {
		return
	}
	h.statusMu.Lock()
	previous := h.compactionLoader
	h.compactionLoader = h.showStatusLoaderLocked(message)
	visible := h.compactionLoader != nil
	h.compactionVisible.Store(visible)
	h.statusMu.Unlock()
	if previous != nil {
		previous.Stop()
	}
	if !visible {
		h.addStatus(message)
		return
	}
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) clearCompactionLoader() {
	if h == nil {
		return
	}
	h.statusMu.Lock()
	loader := h.compactionLoader
	h.compactionLoader = nil
	h.compactionVisible.Store(false)
	if loader != nil && h.statusContainer != nil {
		h.statusContainer.Clear()
	}
	h.statusMu.Unlock()
	if loader == nil {
		return
	}
	loader.Stop()
	h.requestRender(false)
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
	if h.loader != nil {
		h.loader.Stop()
		if h.statusContainer != nil {
			h.statusContainer.Clear()
		} else if h.chat != nil {
			h.chat.RemoveChild(h.loader)
		}
		h.loader = nil
		return
	}
	if h.statusContainer != nil {
		h.statusContainer.Clear()
	}
}
