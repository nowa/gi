package gicodingagent

import (
	"strconv"
	"strings"

	gitui "github.com/nowa/gi/gi-tui"
)

type UserMessageSelectorComponent struct {
	focus         gitui.FocusState
	messages      []AgentSessionForkMessage
	selectedIndex int
	maxVisible    int
	OnSelect      func(entryID string)
	OnCancel      func()
}

func NewUserMessageSelectorComponent(messages []AgentSessionForkMessage, initialSelectedID string) *UserMessageSelectorComponent {
	copied := append([]AgentSessionForkMessage(nil), messages...)
	selected := len(copied) - 1
	if selected < 0 {
		selected = 0
	}
	if initialSelectedID != "" {
		for idx, message := range copied {
			if message.EntryID == initialSelectedID {
				selected = idx
				break
			}
		}
	}
	return &UserMessageSelectorComponent{
		messages:      copied,
		selectedIndex: selected,
		maxVisible:    10,
	}
}

func (c *UserMessageSelectorComponent) Focused() bool {
	if c == nil {
		return false
	}
	return c.focus.Focused()
}

func (c *UserMessageSelectorComponent) SetFocused(focused bool) {
	if c != nil {
		c.focus.SetFocused(focused)
	}
}

func (c *UserMessageSelectorComponent) Invalidate() {}

func (c *UserMessageSelectorComponent) Render(width int) []string {
	if c == nil {
		return nil
	}
	width = max(24, width)
	lines := []string{
		"",
		gitui.TruncateToWidth(" Fork from Message", width, "", true),
		gitui.TruncateToWidth(" Select a user message to copy the active path up to that point into a new session", width, "", true),
		"",
		userMessageSelectorBorder(width),
		"",
	}
	lines = append(lines, c.renderMessages(width)...)
	lines = append(lines, "", userMessageSelectorBorder(width))
	return lines
}

func (c *UserMessageSelectorComponent) renderMessages(width int) []string {
	if len(c.messages) == 0 {
		return []string{gitui.TruncateToWidth("  No user messages found", width, "", true)}
	}
	maxVisible := c.maxVisible
	if maxVisible <= 0 {
		maxVisible = 10
	}
	start := max(0, min(c.selectedIndex-maxVisible/2, len(c.messages)-maxVisible))
	end := min(start+maxVisible, len(c.messages))
	lines := make([]string, 0, (end-start)*3+1)
	for idx := start; idx < end; idx++ {
		message := c.messages[idx]
		selected := idx == c.selectedIndex
		prefix := "  "
		if selected {
			prefix = "> "
		}
		text := strings.TrimSpace(strings.ReplaceAll(message.Text, "\n", " "))
		lines = append(lines, gitui.TruncateToWidth(prefix+text, width, "", true))
		lines = append(lines, gitui.TruncateToWidth("  Message "+strconv.Itoa(idx+1)+" of "+strconv.Itoa(len(c.messages)), width, "", true))
		lines = append(lines, "")
	}
	if start > 0 || end < len(c.messages) {
		lines = append(lines, gitui.TruncateToWidth("  ("+strconv.Itoa(c.selectedIndex+1)+"/"+strconv.Itoa(len(c.messages))+")", width, "", true))
	}
	return lines
}

func (c *UserMessageSelectorComponent) HandleInput(data string) {
	if c == nil {
		return
	}
	kb := gitui.GetKeybindings()
	switch {
	case data == "k" || kb.Matches(data, "tui.select.up"):
		c.moveSelection(-1)
	case data == "j" || kb.Matches(data, "tui.select.down"):
		c.moveSelection(1)
	case kb.Matches(data, "tui.select.confirm"):
		if len(c.messages) == 0 {
			if c.OnCancel != nil {
				c.OnCancel()
			}
			return
		}
		selected := c.messages[max(0, min(c.selectedIndex, len(c.messages)-1))]
		if c.OnSelect != nil {
			c.OnSelect(selected.EntryID)
		}
	case kb.Matches(data, "tui.select.cancel"):
		if c.OnCancel != nil {
			c.OnCancel()
		}
	}
}

func (c *UserMessageSelectorComponent) moveSelection(delta int) {
	if c == nil || len(c.messages) == 0 {
		return
	}
	c.selectedIndex = (c.selectedIndex + delta + len(c.messages)) % len(c.messages)
}

func userMessageSelectorBorder(width int) string {
	return gitui.TruncateToWidth(" "+strings.Repeat("-", max(0, width-1)), width, "", true)
}
