package gicodingagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	gitui "github.com/nowa/gi/gi-tui"
)

func (h *CLIInteractiveTUIHost) handleHotkeysSlashCommand() error {
	hotkeys := strings.TrimSpace(h.hotkeysMarkdown())
	h.chat.AddChild(gitui.NewSpacer(1))
	h.chat.AddChild(newCLIDynamicBorder())
	h.chat.AddChild(gitui.NewText(tuiThemeBoldAccent("Keyboard Shortcuts"), 1, 0))
	h.chat.AddChild(gitui.NewSpacer(1))
	h.chat.AddChild(newCLIMarkdownWithOptions(hotkeys, gitui.MarkdownOptions{PaddingX: 1, PaddingY: 1}))
	h.chat.AddChild(newCLIDynamicBorder())
	h.requestRender(false)
	return nil
}

func (h *CLIInteractiveTUIHost) hotkeysMarkdown() string {
	keybindings := h.effectiveKeybindings()
	tuiKeys := func(action string) string {
		return formatHotkeyKeys(gitui.GetKeybindings().GetKeys(action), true)
	}
	appKeys := func(action string) string {
		return formatHotkeyKeys(keybindingValueKeys(keybindings[action]), true)
	}
	lines := []string{
		"**Navigation**",
		"",
		"| Key | Action |",
		"|-----|--------|",
		"| " + hotkeyRef(tuiKeys("tui.editor.cursorUp")) + " / " + hotkeyRef(tuiKeys("tui.editor.cursorDown")) + " / " + hotkeyRef(tuiKeys("tui.editor.cursorLeft")) + " / " + hotkeyRef(tuiKeys("tui.editor.cursorRight")) + " | Move cursor / browse history (Up when empty) |",
		"| " + hotkeyRef(tuiKeys("tui.editor.cursorWordLeft")) + " / " + hotkeyRef(tuiKeys("tui.editor.cursorWordRight")) + " | Move by word |",
		"| " + hotkeyRef(tuiKeys("tui.editor.cursorLineStart")) + " | Start of line |",
		"| " + hotkeyRef(tuiKeys("tui.editor.cursorLineEnd")) + " | End of line |",
		"| " + hotkeyRef(tuiKeys("tui.editor.jumpForward")) + " | Jump forward to character |",
		"| " + hotkeyRef(tuiKeys("tui.editor.jumpBackward")) + " | Jump backward to character |",
		"| " + hotkeyRef(tuiKeys("tui.editor.pageUp")) + " / " + hotkeyRef(tuiKeys("tui.editor.pageDown")) + " | Scroll by page |",
		"",
		"**Editing**",
		"",
		"| Key | Action |",
		"|-----|--------|",
		"| " + hotkeyRef(tuiKeys("tui.input.submit")) + " | Send message |",
		"| " + hotkeyRef(tuiKeys("tui.input.newLine")) + " | New line" + windowsTerminalNewLineNote() + " |",
		"| " + hotkeyRef(tuiKeys("tui.editor.deleteWordBackward")) + " | Delete word backwards |",
		"| " + hotkeyRef(tuiKeys("tui.editor.deleteWordForward")) + " | Delete word forwards |",
		"| " + hotkeyRef(tuiKeys("tui.editor.deleteToLineStart")) + " | Delete to start of line |",
		"| " + hotkeyRef(tuiKeys("tui.editor.deleteToLineEnd")) + " | Delete to end of line |",
		"| " + hotkeyRef(tuiKeys("tui.editor.yank")) + " | Paste the most-recently-deleted text |",
		"| " + hotkeyRef(tuiKeys("tui.editor.yankPop")) + " | Cycle through the deleted text after pasting |",
		"| " + hotkeyRef(tuiKeys("tui.editor.undo")) + " | Undo |",
		"",
		"**Other**",
		"",
		"| Key | Action |",
		"|-----|--------|",
		"| " + hotkeyRef(tuiKeys("tui.input.tab")) + " | Path completion / accept autocomplete |",
		"| " + hotkeyRef(appKeys("app.interrupt")) + " | Cancel autocomplete / abort streaming |",
		"| " + hotkeyRef(appKeys("app.clear")) + " | Clear editor (first) / exit (second) |",
		"| " + hotkeyRef(appKeys("app.exit")) + " | Exit (when editor is empty) |",
		"| " + hotkeyRef(appKeys("app.suspend")) + " | Suspend to background |",
		"| " + hotkeyRef(appKeys("app.thinking.cycle")) + " | Cycle thinking level |",
		"| " + hotkeyRef(appKeys("app.model.cycleForward")) + " / " + hotkeyRef(appKeys("app.model.cycleBackward")) + " | Cycle models |",
		"| " + hotkeyRef(appKeys("app.model.select")) + " | Open model selector |",
		"| " + hotkeyRef(appKeys("app.tools.expand")) + " | Toggle tool output expansion |",
		"| " + hotkeyRef(appKeys("app.thinking.toggle")) + " | Toggle thinking block visibility |",
		"| " + hotkeyRef(appKeys("app.editor.external")) + " | Edit message in external editor |",
		"| " + hotkeyRef(appKeys("app.message.copy")) + " | Copy last assistant message |",
		"| " + hotkeyRef(appKeys("app.message.followUp")) + " | Queue follow-up message |",
		"| " + hotkeyRef(appKeys("app.message.dequeue")) + " | Restore queued messages |",
		"| " + hotkeyRef(appKeys("app.clipboard.pasteImage")) + " | Paste image from clipboard |",
		"| `/` | Slash commands |",
		"| `!` | Run bash command |",
		"| `!!` | Run bash command (excluded from context) |",
	}
	if runtime := h.protocolRuntime(); runtime != nil {
		shortcuts := runtime.Shortcuts(keybindings).Shortcuts
		if len(shortcuts) > 0 {
			lines = append(lines, "", "**Extensions**", "", "| Key | Action |", "|-----|--------|")
			keys := make([]string, 0, len(shortcuts))
			for key := range shortcuts {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				shortcut := shortcuts[key]
				description := firstNonEmptyString(shortcut.Description, shortcut.SourceInfo.Path, "Extension shortcut")
				lines = append(lines, "| "+hotkeyRef(formatHotkeyText(key, true))+" | "+markdownTableValue(description)+" |")
			}
		}
	}
	return strings.Join(lines, "\n")
}

func windowsTerminalNewLineNote() string {
	if runtime.GOOS == "windows" {
		return " (Ctrl+Enter on Windows Terminal)"
	}
	return ""
}

func hotkeyRef(display string) string {
	display = strings.TrimSpace(display)
	if display == "" {
		return markdownTableValue("")
	}
	return "`" + markdownTableValue(display) + "`"
}

func formatHotkeyKeys(keys []string, capitalize bool) string {
	if len(keys) == 0 {
		return ""
	}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if strings.TrimSpace(key) != "" {
			parts = append(parts, formatHotkeyText(key, capitalize))
		}
	}
	return strings.Join(parts, "/")
}

func formatHotkeyText(key string, capitalize bool) string {
	groups := strings.Split(key, "/")
	for groupIndex, group := range groups {
		parts := strings.Split(group, "+")
		for partIndex, part := range parts {
			part = strings.TrimSpace(part)
			if runtime.GOOS == "darwin" && strings.EqualFold(part, "alt") {
				part = "option"
			}
			if capitalize && part != "" {
				part = strings.ToUpper(part[:1]) + part[1:]
			}
			parts[partIndex] = part
		}
		groups[groupIndex] = strings.Join(parts, "+")
	}
	return strings.Join(groups, "/")
}

func (h *CLIInteractiveTUIHost) handleChangelogSlashCommand() error {
	changelog := h.loadChangelogMarkdown()
	if strings.TrimSpace(changelog) == "" {
		changelog = "No changelog entries found."
	}
	h.chat.AddChild(gitui.NewSpacer(1))
	h.chat.AddChild(newCLIDynamicBorder())
	h.chat.AddChild(gitui.NewText(tuiThemeBoldAccent("What's New"), 1, 0))
	h.chat.AddChild(gitui.NewSpacer(1))
	h.chat.AddChild(newCLIMarkdownWithOptions(changelog, gitui.MarkdownOptions{PaddingX: 1, PaddingY: 1}))
	h.chat.AddChild(newCLIDynamicBorder())
	h.requestRender(false)
	return nil
}

func (h *CLIInteractiveTUIHost) handleDebugCommand() error {
	if h == nil {
		return errors.New("interactive TUI host is not ready")
	}
	width := 80
	height := 24
	if h.terminal != nil {
		width = h.terminal.Columns()
		height = h.terminal.Rows()
	}
	lines := []string(nil)
	if h.layout != nil {
		lines = h.layout.RenderWithSize(width, height)
	} else if h.ui != nil {
		lines = h.ui.Render(width)
	}
	debugPath := h.debugLogPath()
	if err := os.MkdirAll(filepath.Dir(debugPath), 0o755); err != nil {
		return err
	}
	var builder strings.Builder
	builder.WriteString("Debug output at ")
	builder.WriteString(time.Now().UTC().Format(time.RFC3339Nano))
	builder.WriteString("\n")
	builder.WriteString(fmt.Sprintf("Terminal: %dx%d\n", width, height))
	builder.WriteString(fmt.Sprintf("Total lines: %d\n\n", len(lines)))
	builder.WriteString("=== All rendered lines with visible widths ===\n")
	for index, line := range lines {
		encoded, _ := json.Marshal(line)
		builder.WriteString(fmt.Sprintf("[%d] (w=%d) %s\n", index, gitui.VisibleWidth(line), string(encoded)))
	}
	builder.WriteString("\n=== Agent messages (JSONL) ===\n")
	if session := h.agentSession(); session != nil {
		for _, message := range session.Messages() {
			encoded, err := json.Marshal(message)
			if err != nil {
				continue
			}
			builder.Write(encoded)
			builder.WriteString("\n")
		}
	}
	if err := os.WriteFile(debugPath, []byte(builder.String()), 0o644); err != nil {
		return err
	}
	if h.chat != nil {
		h.chat.AddChild(gitui.NewSpacer(1))
		h.chat.AddChild(gitui.NewText(tuiThemeAccent("✓ Debug log written")+"\n"+tuiThemeMuted(debugPath), 1, 1))
		h.requestRender(false)
	}
	return nil
}

func (h *CLIInteractiveTUIHost) handleArminSaysHiCommand() error {
	h.chat.AddChild(gitui.NewSpacer(1))
	h.chat.AddChild(newCLIArminComponent())
	h.requestRender(false)
	return nil
}

func (h *CLIInteractiveTUIHost) handleDementedDelvesCommand() error {
	h.chat.AddChild(gitui.NewSpacer(1))
	h.chat.AddChild(newCLIEarendilAnnouncementComponent())
	h.requestRender(false)
	return nil
}

func (h *CLIInteractiveTUIHost) debugLogPath() string {
	if settings := h.settingsManager(); settings != nil && settings.agentDir != "" {
		return filepath.Join(settings.agentDir, firstNonEmptyString(h.packageName, DefaultCodingAgentPackageName)+"-debug.log")
	}
	cwd := "."
	if h != nil {
		cwd = firstNonEmptyString(h.interactiveCWD(), ".")
	}
	return filepath.Join(cwd, ConfigDirName, "agent", firstNonEmptyString(h.packageName, DefaultCodingAgentPackageName)+"-debug.log")
}

func (h *CLIInteractiveTUIHost) loadChangelogMarkdown() string {
	cwd := h.interactiveCWD()
	if strings.TrimSpace(cwd) != "" {
		for _, name := range []string{"CHANGELOG.md", "CHANGELOG"} {
			content, err := os.ReadFile(filepath.Join(cwd, name))
			if err == nil {
				return string(content)
			}
		}
	}
	return embeddedCodingAgentChangelog
}
