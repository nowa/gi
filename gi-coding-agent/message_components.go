package gicodingagent

const (
	OSC133ZoneStart = "\x1b]133;A\x07"
	OSC133ZoneEnd   = "\x1b]133;B\x07"
	OSC133ZoneFinal = "\x1b]133;C\x07"
	TerminalBGReset = "\x1b[49m"
)

type AssistantContentBlock struct {
	Type      string
	Text      string
	Thinking  string
	ID        string
	Name      string
	Arguments map[string]any
}

type AssistantMessageComponent struct {
	Content []AssistantContentBlock
}

func NewAssistantMessageComponent(content []AssistantContentBlock) AssistantMessageComponent {
	return AssistantMessageComponent{Content: content}
}

func (c AssistantMessageComponent) Render(width int) []string {
	lines := c.renderContent(width)
	if c.hasToolCalls() || len(lines) == 0 {
		return lines
	}
	lines[0] = OSC133ZoneStart + lines[0]
	lines[len(lines)-1] = OSC133ZoneEnd + OSC133ZoneFinal + lines[len(lines)-1]
	return lines
}

func (c AssistantMessageComponent) renderContent(_ int) []string {
	lines := []string{}
	for _, block := range c.Content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				lines = append(lines, block.Text)
			}
		case "thinking":
			if block.Thinking != "" {
				lines = append(lines, block.Thinking)
			}
		case "toolCall":
			lines = append(lines, block.Name)
		}
	}
	if len(lines) == 0 {
		return nil
	}
	return append([]string{""}, lines...)
}

func (c AssistantMessageComponent) hasToolCalls() bool {
	for _, block := range c.Content {
		if block.Type == "toolCall" {
			return true
		}
	}
	return false
}

type UserMessageComponent struct {
	Text string
}

func NewUserMessageComponent(text string) UserMessageComponent {
	return UserMessageComponent{Text: text}
}

func (c UserMessageComponent) Render(_ int) []string {
	return []string{
		OSC133ZoneStart + TerminalBGReset,
		c.Text,
		OSC133ZoneEnd + OSC133ZoneFinal + TerminalBGReset,
	}
}
