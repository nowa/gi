package gicodingagent

import (
	"strings"

	gitui "github.com/nowa/gi/gi-tui"
)

type cliCollapsibleMarkdownMessageOptions struct {
	Label     string
	Title     string
	Body      string
	Collapsed string
	Expanded  bool
}

type cliCollapsibleMarkdownMessage struct {
	label     string
	title     string
	body      string
	collapsed string
	expanded  bool
}

func newCLICollapsibleMarkdownMessage(options cliCollapsibleMarkdownMessageOptions) *cliCollapsibleMarkdownMessage {
	return &cliCollapsibleMarkdownMessage{
		label:     strings.TrimSpace(options.Label),
		title:     strings.TrimSpace(options.Title),
		body:      strings.TrimSpace(options.Body),
		collapsed: strings.TrimSpace(options.Collapsed),
		expanded:  options.Expanded,
	}
}

func (c *cliCollapsibleMarkdownMessage) SetExpanded(expanded bool) {
	if c != nil {
		c.expanded = expanded
	}
}

func (c *cliCollapsibleMarkdownMessage) Invalidate() {}

func (c *cliCollapsibleMarkdownMessage) Render(width int) []string {
	if c == nil {
		return nil
	}
	label := c.label
	if label == "" {
		label = "message"
	}
	if c.expanded {
		title := c.title
		if title == "" {
			title = label
		}
		body := strings.TrimSpace("[" + label + "]\n\n**" + title + "**\n\n" + c.body)
		return gitui.NewMarkdownWithOptions(body, gitui.MarkdownOptions{PaddingX: 1, PaddingY: 1}).Render(width)
	}
	collapsed := c.collapsed
	if collapsed == "" {
		collapsed = "Collapsed message"
	}
	return gitui.NewMarkdownWithOptions("["+label+"] "+collapsed, gitui.MarkdownOptions{PaddingX: 1, PaddingY: 1}).Render(width)
}
