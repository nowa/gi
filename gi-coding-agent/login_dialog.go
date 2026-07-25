package gicodingagent

import (
	"runtime"
	"strings"
	"sync"

	gitui "github.com/nowa/gi/gi-tui"
)

type loginDialogBlockStyle uint8

const (
	loginDialogBlockRaw loginDialogBlockStyle = iota
	loginDialogBlockText
	loginDialogBlockDim
	loginDialogBlockWarning
	loginDialogBlockAccentLink
	loginDialogBlockSpacer
)

type loginDialogBlock struct {
	style loginDialogBlockStyle
	text  string
	url   string
}

type loginDialogHint uint8

const (
	loginDialogHintNone loginDialogHint = iota
	loginDialogHintCancel
	loginDialogHintSubmit
	loginDialogHintClose
)

// LoginDialogComponent is an ordered presentation state machine for one
// provider-owned authentication flow. Protocol state stays in the provider;
// this component retains only the blocks already shown and the current input.
type LoginDialogComponent struct {
	mu           sync.RWMutex
	focus        gitui.FocusState
	title        string
	input        *gitui.Input
	inputVisible bool
	blocks       []loginDialogBlock
	hint         loginDialogHint
	OnSubmit     func(value string)
	OnCancel     func()
}

func NewLoginDialogComponent(title, message string) *LoginDialogComponent {
	input := gitui.NewInput()
	component := &LoginDialogComponent{
		title: strings.TrimSpace(title),
		input: input,
	}
	if message = strings.TrimSpace(message); message != "" {
		component.blocks = append(component.blocks, loginDialogBlock{
			style: loginDialogBlockText,
			text:  message,
		})
		component.inputVisible = true
		component.hint = loginDialogHintSubmit
	}
	input.OnSubmit = func(value string) {
		component.replaceInputWithSubmittedText(value)
		component.mu.RLock()
		onSubmit := component.OnSubmit
		component.mu.RUnlock()
		if onSubmit != nil {
			onSubmit(value)
		}
	}
	input.OnEscape = func() {
		component.mu.RLock()
		onCancel := component.OnCancel
		component.mu.RUnlock()
		if onCancel != nil {
			onCancel()
		}
	}
	return component
}

func (c *LoginDialogComponent) Focused() bool {
	if c == nil {
		return false
	}
	return c.focus.Focused()
}

func (c *LoginDialogComponent) SetFocused(focused bool) {
	if c == nil {
		return
	}
	c.focus.SetFocused(focused)
	if c.input != nil {
		c.input.SetFocused(focused)
	}
}

func (c *LoginDialogComponent) Invalidate() {
	if c != nil && c.input != nil {
		c.input.Invalidate()
	}
}

// ShowDetails replaces the current content with application-owned setup
// details that should remain visible through the next provider prompt.
func (c *LoginDialogComponent) ShowDetails(lines []string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.blocks = []loginDialogBlock{{style: loginDialogBlockSpacer}}
	c.blocks = appendRawLoginDialogLines(c.blocks, lines)
	c.inputVisible = false
	c.hint = loginDialogHintNone
	c.mu.Unlock()
}

// ShowInfo appends provider-owned information and links. showClose is used
// only for terminal informational dialogs that do not continue into a prompt.
func (c *LoginDialogComponent) ShowInfo(
	lines []string,
	showClose ...bool,
) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.blocks = append(c.blocks, loginDialogBlock{
		style: loginDialogBlockSpacer,
	})
	c.blocks = appendRawLoginDialogLines(c.blocks, lines)
	c.inputVisible = false
	c.hint = loginDialogHintNone
	if len(showClose) > 0 && showClose[0] {
		c.hint = loginDialogHintClose
	}
	c.mu.Unlock()
}

// ShowAuth starts a new authorization-page presentation. The optional manual
// prompt is retained for callers that want to present both steps atomically.
func (c *LoginDialogComponent) ShowAuth(
	url string,
	instructions string,
	manualPrompt string,
) {
	if c == nil {
		return
	}
	url = strings.TrimSpace(url)
	instructions = strings.TrimSpace(instructions)
	manualPrompt = strings.TrimSpace(manualPrompt)

	c.mu.Lock()
	c.blocks = []loginDialogBlock{{style: loginDialogBlockSpacer}}
	if url != "" {
		c.blocks = append(c.blocks,
			loginDialogBlock{
				style: loginDialogBlockAccentLink,
				text:  url,
				url:   url,
			},
			loginDialogBlock{
				style: loginDialogBlockDim,
				text:  terminalHyperlink(url, oauthClickHint()),
			},
		)
	}
	if instructions != "" {
		c.blocks = append(c.blocks,
			loginDialogBlock{style: loginDialogBlockSpacer},
			loginDialogBlock{
				style: loginDialogBlockWarning,
				text:  instructions,
			},
		)
	}
	c.inputVisible = false
	c.hint = loginDialogHintNone
	if manualPrompt != "" {
		c.blocks = append(c.blocks,
			loginDialogBlock{style: loginDialogBlockSpacer},
			loginDialogBlock{
				style: loginDialogBlockDim,
				text:  manualPrompt,
			},
		)
		c.inputVisible = true
		c.hint = loginDialogHintCancel
	}
	input := c.input
	c.mu.Unlock()
	if input != nil {
		input.SetText("")
	}
}

// ShowDeviceCode starts a provider device-code presentation.
func (c *LoginDialogComponent) ShowDeviceCode(
	verificationURI string,
	userCode string,
) {
	instructions := "Waiting for authentication..."
	if userCode = strings.TrimSpace(userCode); userCode != "" {
		instructions = "Enter code: " + userCode
	}
	c.ShowAuth(verificationURI, instructions, "")
}

// ShowPrompt appends one provider-owned text or secret prompt.
func (c *LoginDialogComponent) ShowPrompt(
	message string,
	placeholder string,
) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.blocks = append(c.blocks,
		loginDialogBlock{style: loginDialogBlockSpacer},
		loginDialogBlock{
			style: loginDialogBlockText,
			text:  strings.TrimSpace(message),
		},
	)
	if placeholder = strings.TrimSpace(placeholder); placeholder != "" {
		c.blocks = append(c.blocks, loginDialogBlock{
			style: loginDialogBlockDim,
			text:  "e.g., " + placeholder,
		})
	}
	c.inputVisible = true
	c.hint = loginDialogHintSubmit
	input := c.input
	c.mu.Unlock()
	if input != nil {
		input.SetText("")
	}
}

// ShowManualInput appends the manual callback input to the authorization URL
// and instructions already displayed.
func (c *LoginDialogComponent) ShowManualInput(message string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.blocks = append(c.blocks,
		loginDialogBlock{style: loginDialogBlockSpacer},
		loginDialogBlock{
			style: loginDialogBlockDim,
			text:  strings.TrimSpace(message),
		},
	)
	c.inputVisible = true
	c.hint = loginDialogHintCancel
	input := c.input
	c.mu.Unlock()
	if input != nil {
		input.SetText("")
	}
}

// ShowWaiting appends a cancellable polling state.
func (c *LoginDialogComponent) ShowWaiting(message string) {
	if c == nil || strings.TrimSpace(message) == "" {
		return
	}
	c.mu.Lock()
	c.blocks = append(c.blocks,
		loginDialogBlock{style: loginDialogBlockSpacer},
		loginDialogBlock{
			style: loginDialogBlockDim,
			text:  strings.TrimSpace(message),
		},
	)
	c.inputVisible = false
	c.hint = loginDialogHintCancel
	c.mu.Unlock()
}

// ShowProgress appends one non-terminal provider status.
func (c *LoginDialogComponent) ShowProgress(message string) {
	if c == nil || strings.TrimSpace(message) == "" {
		return
	}
	c.mu.Lock()
	c.blocks = append(c.blocks, loginDialogBlock{
		style: loginDialogBlockDim,
		text:  strings.TrimSpace(message),
	})
	c.mu.Unlock()
}

func (c *LoginDialogComponent) replaceInputWithSubmittedText(value string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.inputVisible {
		c.blocks = append(c.blocks, loginDialogBlock{
			style: loginDialogBlockRaw,
			text:  "> " + value,
		})
	}
	c.inputVisible = false
	c.hint = loginDialogHintNone
	c.mu.Unlock()
}

func (c *LoginDialogComponent) Render(width int) []string {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	title := firstNonEmptyString(c.title, "Login")
	input := c.input
	inputVisible := c.inputVisible
	blocks := append([]loginDialogBlock(nil), c.blocks...)
	hint := c.hint
	c.mu.RUnlock()

	width = max(24, width)
	lines := []string{
		selectorDynamicBorder(width),
		selectorTextLine(tuiThemeBoldAccent(title), width),
	}
	for _, block := range blocks {
		lines = appendLoginDialogBlock(lines, block, width)
	}
	if inputVisible && input != nil {
		lines = append(lines, input.Render(width)...)
	}
	if hintText := loginDialogHintText(hint); hintText != "" {
		if hint == loginDialogHintClose {
			lines = append(lines, "")
		}
		lines = append(lines, selectorTextLine(hintText, width))
	}
	lines = append(lines, selectorDynamicBorder(width))
	return lines
}

func (c *LoginDialogComponent) HandleInput(input string) {
	if c == nil {
		return
	}
	c.mu.RLock()
	onCancel := c.OnCancel
	inputVisible := c.inputVisible
	inputComponent := c.input
	c.mu.RUnlock()
	if gitui.GetKeybindings().Matches(input, "tui.select.cancel") ||
		input == "\x03" {
		if onCancel != nil {
			onCancel()
		}
		return
	}
	if inputVisible && inputComponent != nil {
		inputComponent.HandleInput(input)
	}
}

func appendRawLoginDialogLines(
	blocks []loginDialogBlock,
	lines []string,
) []loginDialogBlock {
	for _, line := range lines {
		blocks = append(blocks, loginDialogBlock{
			style: loginDialogBlockRaw,
			text:  line,
		})
	}
	return blocks
}

func appendLoginDialogBlock(
	lines []string,
	block loginDialogBlock,
	width int,
) []string {
	if block.style == loginDialogBlockSpacer {
		return append(lines, "")
	}
	text := block.text
	switch block.style {
	case loginDialogBlockText:
		text = tuiThemeFG("text", text)
	case loginDialogBlockDim:
		text = tuiThemeDim(text)
	case loginDialogBlockWarning:
		text = tuiThemeWarning(text)
	case loginDialogBlockAccentLink:
		text = tuiThemeAccent(terminalHyperlink(block.url, text))
	}
	return append(lines, selectorTextLines(text, width)...)
}

func loginDialogHintText(hint loginDialogHint) string {
	cancel := selectorCancelKeyHint()
	confirm := firstNonEmptyString(
		formatHotkeyKeys(
			gitui.GetKeybindings().GetKeys("tui.select.confirm"),
			false,
		),
		"enter",
	)
	switch hint {
	case loginDialogHintCancel:
		return "(" + tuiThemeKeyHint(cancel, "to cancel") + ")"
	case loginDialogHintSubmit:
		return "(" +
			tuiThemeKeyHint(cancel, "to cancel,") + " " +
			tuiThemeKeyHint(confirm, "to submit") +
			")"
	case loginDialogHintClose:
		return "(" + tuiThemeKeyHint(cancel, "to close") + ")"
	default:
		return ""
	}
}

func terminalHyperlink(url, label string) string {
	if strings.TrimSpace(url) == "" {
		return label
	}
	return "\x1b]8;;" + url + "\x07" + label + "\x1b]8;;\x07"
}

func oauthClickHint() string {
	if runtime.GOOS == "darwin" {
		return "Cmd+click to open"
	}
	return "Ctrl+click to open"
}
