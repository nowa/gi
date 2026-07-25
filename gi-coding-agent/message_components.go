package gicodingagent

import (
	llm "github.com/nowa/gi/gi-llm-provider"
)

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
	Content             []AssistantContentBlock
	HideThinkingBlock   bool
	HiddenThinkingLabel string
	outputPad           *int
}

func NewAssistantMessageComponent(content []AssistantContentBlock) AssistantMessageComponent {
	return AssistantMessageComponent{Content: content}
}

func (c *AssistantMessageComponent) SetOutputPad(padding int) {
	if c == nil {
		return
	}
	padding = normalizeOutputPad(padding)
	c.outputPad = &padding
}

func (c AssistantMessageComponent) Render(width int) []string {
	message := c.message()
	lines := renderCLIAssistantMessage(message, width, c.HideThinkingBlock, c.HiddenThinkingLabel, c.resolvedOutputPad())
	if assistantMessageHasToolCalls(message) || len(lines) == 0 {
		return lines
	}
	return cliOSC133WrappedLines(lines)
}

func (c AssistantMessageComponent) resolvedOutputPad() int {
	if c.outputPad == nil {
		return defaultOutputPad
	}
	return normalizeOutputPad(*c.outputPad)
}

func (c AssistantMessageComponent) message() llm.Message {
	message := llm.Message{Role: llm.RoleAssistant, StopReason: llm.StopReasonStop}
	for _, block := range c.Content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				message.Content = append(message.Content, llm.Text(block.Text))
			}
		case "thinking":
			if block.Thinking != "" {
				message.Content = append(message.Content, llm.Thinking(block.Thinking))
			}
		case "toolCall":
			message.Content = append(message.Content, llm.ToolCall(block.ID, block.Name, block.Arguments))
		}
	}
	return message
}

type UserMessageComponent struct {
	Text      string
	outputPad *int
}

func NewUserMessageComponent(text string) UserMessageComponent {
	return UserMessageComponent{Text: text}
}

func (c *UserMessageComponent) SetOutputPad(padding int) {
	if c == nil {
		return
	}
	padding = normalizeOutputPad(padding)
	c.outputPad = &padding
}

func (c UserMessageComponent) Render(width int) []string {
	outputPad := defaultOutputPad
	if c.outputPad != nil {
		outputPad = normalizeOutputPad(*c.outputPad)
	}
	return newCLIUserMessageComponent(c.Text, outputPad).Render(width)
}
