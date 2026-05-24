package gicodingagent

import (
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"

	gitui "github.com/nowa/gi/gi-tui"
)

type cliTUIDialogComponent struct {
	mu                    sync.RWMutex
	focus                 gitui.FocusState
	title                 string
	message               string
	input                 *gitui.Input
	searchInput           *gitui.Input
	editor                *gitui.Editor
	editorSubmit          func(string)
	tui                   *gitui.TUI
	list                  *gitui.SelectList
	footer                string
	footerKind            string
	keybindings           KeybindingsConfig
	onCancel              func()
	onToggleToolsExpanded func()
}

type cliSelectDialogOptions struct {
	OnSelectionChange func(TUIDialogOption)
}

func newCLIInputDialog(title, message, placeholder, defaultText string, onSubmit func(string), onCancel func()) *cliTUIDialogComponent {
	input := gitui.NewInput(placeholder)
	if defaultText != "" {
		input.SetText(defaultText)
	}
	input.OnSubmit = onSubmit
	input.OnEscape = onCancel
	return &cliTUIDialogComponent{
		title:       firstNonEmptyString(title, "Input"),
		message:     message,
		input:       input,
		footerKind:  "input",
		keybindings: DefaultProtocolKeybindings(),
		onCancel:    onCancel,
	}
}

func newCLIEditorDialog(tui *gitui.TUI, title, message, defaultText string, onSubmit func(string), onCancel func()) *cliTUIDialogComponent {
	editor := gitui.NewEditor(
		tuiThemeEditor(),
		gitui.EditorOptions{PaddingX: 1, MaxVisibleLines: 8},
	)
	if defaultText != "" {
		editor.SetText(defaultText)
	}
	editor.SetOnSubmit(onSubmit)
	return &cliTUIDialogComponent{
		title:        firstNonEmptyString(title, "Editor"),
		message:      message,
		editor:       editor,
		editorSubmit: onSubmit,
		tui:          tui,
		footerKind:   "editor",
		keybindings:  DefaultProtocolKeybindings(),
		onCancel:     onCancel,
	}
}

func newCLISelectDialog(title, message string, options []TUIDialogOption, defaultIndex int, onSelect func(TUIDialogOption), onCancel func()) *cliTUIDialogComponent {
	return newCLISelectDialogWithOptions(title, message, options, defaultIndex, onSelect, onCancel, cliSelectDialogOptions{})
}

func newCLISelectDialogWithOptions(title, message string, options []TUIDialogOption, defaultIndex int, onSelect func(TUIDialogOption), onCancel func(), dialogOptions cliSelectDialogOptions) *cliTUIDialogComponent {
	items := make([]gitui.SelectItem, 0, len(options))
	byKey := make(map[string]TUIDialogOption, len(options))
	for idx, option := range options {
		key := dialogOptionKey(option, idx)
		label := firstNonEmptyString(option.Label, option.ID, key)
		items = append(items, gitui.SelectItem{Value: key, Label: label, Description: option.Description})
		byKey[key] = option
	}
	list := gitui.NewSelectList(items, 8, tuiThemeSelectList())
	searchInput := gitui.NewInput()
	if defaultIndex >= 0 {
		list.SetSelectedIndex(defaultIndex)
	}
	list.OnSelect = func(item gitui.SelectItem) {
		option, ok := byKey[item.Value]
		if !ok {
			option = TUIDialogOption{ID: item.Value, Label: item.Label, Value: item.Value}
		}
		onSelect(option)
	}
	list.OnCancel = onCancel
	if dialogOptions.OnSelectionChange != nil {
		list.OnSelectionChange = func(item gitui.SelectItem) {
			option, ok := byKey[item.Value]
			if !ok {
				option = TUIDialogOption{ID: item.Value, Label: item.Label, Value: item.Value}
			}
			dialogOptions.OnSelectionChange(option)
		}
	}
	return &cliTUIDialogComponent{
		title:       firstNonEmptyString(title, "Select"),
		message:     message,
		searchInput: searchInput,
		list:        list,
		footerKind:  "select",
		keybindings: DefaultProtocolKeybindings(),
		onCancel:    onCancel,
	}
}

func (c *cliTUIDialogComponent) Focused() bool {
	if c == nil {
		return false
	}
	return c.focus.Focused()
}

func (c *cliTUIDialogComponent) SetFocused(focused bool) {
	if c == nil {
		return
	}
	c.focus.SetFocused(focused)
	if c.input != nil {
		c.input.SetFocused(focused)
	}
	if c.searchInput != nil {
		c.searchInput.SetFocused(focused)
	}
	if c.editor != nil {
		c.editor.SetFocused(focused)
	}
}

func (c *cliTUIDialogComponent) Invalidate() {
	if c == nil {
		return
	}
	if c.input != nil {
		c.input.Invalidate()
	}
	if c.searchInput != nil {
		c.searchInput.Invalidate()
	}
	if c.editor != nil {
		c.editor.Invalidate()
	}
	if c.list != nil {
		c.list.Invalidate()
	}
}

func (c *cliTUIDialogComponent) Title() string {
	if c == nil {
		return ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.title
}

func (c *cliTUIDialogComponent) SetTitle(title string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.title = title
}

func (c *cliTUIDialogComponent) Render(width int) []string {
	if c == nil {
		return nil
	}
	width = max(24, width)
	lines := []string{dialogBorder(width), ""}
	title := c.Title()
	if strings.TrimSpace(title) != "" {
		styleTitle := tuiThemeAccent(title)
		if c.list != nil {
			styleTitle = tuiThemeBoldAccent(title)
		}
		lines = append(lines, gitui.NewText(styleTitle, 1, 0).Render(width)...)
		lines = append(lines, "")
	}
	if strings.TrimSpace(c.message) != "" {
		lines = append(lines, gitui.NewText(tuiThemeFG("text", c.message), 1, 0).Render(width)...)
		lines = append(lines, "")
	}
	if c.input != nil {
		lines = append(lines, c.input.Render(width)...)
	}
	if c.searchInput != nil {
		lines = append(lines, c.searchInput.Render(width)...)
	}
	if c.editor != nil {
		lines = append(lines, c.editor.Render(width)...)
	}
	if c.list != nil {
		lines = append(lines, c.list.Render(width)...)
	}
	lines = append(lines, "")
	if footer := c.footerText(); strings.TrimSpace(footer) != "" {
		lines = append(lines, gitui.NewText(footer, 1, 0).Render(width)...)
		lines = append(lines, "")
	}
	lines = append(lines, dialogBorder(width))
	return lines
}

func (c *cliTUIDialogComponent) HandleInput(data string) {
	if c == nil {
		return
	}
	if (matchesKeybindingAction(data, c.keybindings, "app.tools.expand") || gitui.MatchesKey(data, "ctrl+o")) && c.onToggleToolsExpanded != nil {
		c.onToggleToolsExpanded()
		return
	}
	if c.input != nil {
		c.input.HandleInput(data)
		return
	}
	if c.editor != nil {
		if gitui.GetKeybindings().Matches(data, "tui.select.cancel") {
			if c.onCancel != nil {
				c.onCancel()
			}
			return
		}
		if c.isExternalEditorInput(data) {
			c.openExternalEditor()
			return
		}
		c.editor.HandleInput(data)
		return
	}
	if c.list != nil {
		kb := gitui.GetKeybindings()
		switch data {
		case "j":
			c.list.HandleInput("\x1b[B")
		case "k":
			c.list.HandleInput("\x1b[A")
		default:
			if kb.Matches(data, "tui.select.up") ||
				kb.Matches(data, "tui.select.down") ||
				kb.Matches(data, "tui.select.pageUp") ||
				kb.Matches(data, "tui.select.pageDown") ||
				kb.Matches(data, "tui.select.confirm") ||
				kb.Matches(data, "tui.select.cancel") {
				c.list.HandleInput(data)
				return
			}
			if c.searchInput != nil {
				sanitized := strings.ReplaceAll(data, " ", "")
				if sanitized != "" {
					c.searchInput.HandleInput(sanitized)
					c.list.SetFilter(c.searchInput.GetValue())
				}
				return
			}
			c.list.HandleInput(data)
		}
		return
	}
	if gitui.MatchesKey(data, "escape") && c.onCancel != nil {
		c.onCancel()
	}
}

func (c *cliTUIDialogComponent) footerText() string {
	if c == nil {
		return ""
	}
	if strings.TrimSpace(c.footer) != "" {
		return c.footer
	}
	confirm := firstNonEmptyString(formatHotkeyKeys(gitui.GetKeybindings().GetKeys("tui.select.confirm"), true), "Enter")
	cancel := firstNonEmptyString(formatHotkeyKeys(gitui.GetKeybindings().GetKeys("tui.select.cancel"), true), "Esc")
	switch c.footerKind {
	case "input":
		return confirm + " submit   " + cancel + " cancel"
	case "editor":
		newLine := firstNonEmptyString(formatHotkeyKeys(gitui.GetKeybindings().GetKeys("tui.input.newLine"), true), "Shift+Enter")
		footer := confirm + " submit   " + newLine + " newline   " + cancel + " cancel"
		if externalEditorCommand() != "" {
			footer += "   " + c.appKeyText("app.editor.external", "Ctrl+G") + " external editor"
		}
		return footer
	case "select":
		upDown := firstNonEmptyString(formatHotkeyKeys(append(gitui.GetKeybindings().GetKeys("tui.select.up"), gitui.GetKeybindings().GetKeys("tui.select.down")...), true), "Up/Down")
		return "Type search   " + upDown + " navigate   " + confirm + " select   " + cancel + " cancel"
	default:
		return ""
	}
}

func (c *cliTUIDialogComponent) appKeyText(action, fallback string) string {
	keybindings := DefaultProtocolKeybindings()
	if c != nil && c.keybindings != nil {
		keybindings = c.keybindings
	}
	return firstNonEmptyString(formatHotkeyKeys(keybindingValueKeys(keybindings[action]), true), fallback)
}

func appendDialogTextLines(lines []string, text string, innerWidth int) []string {
	if strings.TrimSpace(text) == "" {
		return lines
	}
	for _, line := range strings.Split(text, "\n") {
		lines = append(lines, dialogLine(tuiThemeMuted(line), innerWidth))
	}
	return lines
}

func dialogLine(text string, innerWidth int) string {
	return " " + gitui.TruncateToWidth(text, innerWidth, "", true) + " "
}

func dialogBorder(width int) string {
	return selectorDynamicBorder(width)
}

func (c *cliTUIDialogComponent) isExternalEditorInput(data string) bool {
	return matchesKeybindingAction(data, c.keybindings, "app.editor.external") || gitui.MatchesKey(data, "ctrl+g")
}

func externalEditorCommand() string {
	if command := strings.TrimSpace(os.Getenv("VISUAL")); command != "" {
		return command
	}
	return strings.TrimSpace(os.Getenv("EDITOR"))
}

func (c *cliTUIDialogComponent) openExternalEditor() {
	if c == nil || c.editor == nil {
		return
	}
	command := externalEditorCommand()
	if command == "" {
		return
	}
	name, args, ok := splitExternalEditorCommand(command)
	if !ok {
		return
	}
	tmp, err := os.CreateTemp("", "gi-extension-editor-*.md")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.WriteString(c.editor.GetText()); err != nil {
		tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}

	stoppedTUI := c.tui != nil
	if stoppedTUI {
		c.tui.Stop()
	}
	cmd := exec.Command(name, append(args, tmpName)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if stoppedTUI {
		c.tui.Start()
		c.tui.RequestRender(true)
	}
	if err != nil {
		return
	}
	content, err := os.ReadFile(tmpName)
	if err != nil {
		return
	}
	c.editor.SetText(strings.TrimSuffix(string(content), "\n"))
}

func splitExternalEditorCommand(command string) (string, []string, bool) {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return "", nil, false
	}
	return fields[0], fields[1:], true
}

func dialogOptionKey(option TUIDialogOption, index int) string {
	if option.ID != "" {
		return option.ID
	}
	if option.Label != "" {
		return option.Label
	}
	return fmt.Sprintf("option-%d", index+1)
}

func dialogOptionValue(option TUIDialogOption) any {
	if option.Value != nil {
		return option.Value
	}
	return firstNonEmptyString(option.Label, option.ID)
}

func dialogStringValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func dialogDefaultOptionIndex(options []TUIDialogOption, value any) int {
	for idx, option := range options {
		if dialogDefaultMatchesOption(value, option) {
			return idx
		}
	}
	return 0
}

func dialogDefaultMatchesOption(value any, option TUIDialogOption) bool {
	if value == nil {
		return false
	}
	if reflect.DeepEqual(value, option.Value) {
		return true
	}
	valueText := fmt.Sprint(value)
	return option.ID == valueText || option.Label == valueText || fmt.Sprint(option.Value) == valueText
}
