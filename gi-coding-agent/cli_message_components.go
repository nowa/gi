package gicodingagent

import (
	"strings"
	"sync"

	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

const (
	cliOSC133ZoneStart = "\x1b]133;A\x07"
	cliOSC133ZoneEnd   = "\x1b]133;B\x07"
	cliOSC133ZoneFinal = "\x1b]133;C\x07"
)

type cliUserMessageComponent struct {
	text string
}

func newCLIUserMessageComponent(text string) gitui.Component {
	return cliUserMessageComponent{text: strings.TrimSpace(text)}
}

func (c cliUserMessageComponent) Invalidate() {}

func (c cliUserMessageComponent) Render(width int) []string {
	if strings.TrimSpace(c.text) == "" {
		return nil
	}
	box := gitui.NewBox(1, 1, func(text string) string {
		return tuiThemeBG("userMessageBg", text)
	})
	box.AddChild(newCLIMarkdownWithOptions(c.text, gitui.MarkdownOptions{
		DefaultTextStyle: &gitui.DefaultTextStyle{
			Color: func(text string) string { return tuiThemeFG("userMessageText", text) },
		},
	}))
	lines := box.Render(width)
	return cliOSC133WrappedLines(lines)
}

type cliAssistantMessageComponent struct {
	mu                  sync.Mutex
	message             llm.Message
	hideThinkingBlock   bool
	hiddenThinkingLabel string
}

func newCLIAssistantMessageComponent(message llm.Message, hideThinkingBlock bool, hiddenThinkingLabel string) *cliAssistantMessageComponent {
	return &cliAssistantMessageComponent{
		message:             message,
		hideThinkingBlock:   hideThinkingBlock,
		hiddenThinkingLabel: firstNonEmptyString(strings.TrimSpace(hiddenThinkingLabel), "Thinking..."),
	}
}

func (c *cliAssistantMessageComponent) SetMessage(message llm.Message) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.message = message
}

func (c *cliAssistantMessageComponent) Invalidate() {}

func (c *cliAssistantMessageComponent) Render(width int) []string {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	message := c.message
	hideThinkingBlock := c.hideThinkingBlock
	hiddenThinkingLabel := c.hiddenThinkingLabel
	c.mu.Unlock()
	lines := renderCLIAssistantMessage(message, width, hideThinkingBlock, hiddenThinkingLabel)
	if assistantMessageHasToolCalls(message) {
		return lines
	}
	return cliOSC133WrappedLines(lines)
}

func renderCLIAssistantMessage(message llm.Message, width int, hideThinkingBlock bool, hiddenThinkingLabel string) []string {
	var blocks [][]string
	for _, part := range message.Content {
		switch part.Type {
		case llm.ContentText:
			if text := strings.TrimSpace(part.Text); text != "" {
				blocks = append(blocks, newCLIMarkdownWithOptions(text, gitui.MarkdownOptions{PaddingX: 1}).Render(width))
			}
		case llm.ContentThinking:
			if thinking := strings.TrimSpace(part.Thinking); thinking != "" {
				if hideThinkingBlock {
					thinking = firstNonEmptyString(strings.TrimSpace(hiddenThinkingLabel), "Thinking...")
				}
				blocks = append(blocks, newCLIMarkdownWithOptions(thinking, gitui.MarkdownOptions{
					PaddingX: 1,
					DefaultTextStyle: &gitui.DefaultTextStyle{
						Color:  tuiThemeMuted,
						Italic: true,
					},
				}).Render(width))
			}
		}
	}
	if !assistantMessageHasToolCalls(message) {
		if status := assistantMessageStatusText(message); status != "" {
			blocks = append(blocks, gitui.NewText(tuiThemeError(status), 1, 0).Render(width))
		}
	}
	if len(blocks) == 0 {
		return nil
	}
	lines := []string{""}
	for index, block := range blocks {
		if index > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, block...)
	}
	return lines
}

func assistantMessageHasToolCalls(message llm.Message) bool {
	for _, part := range message.Content {
		if part.Type == llm.ContentToolCall {
			return true
		}
	}
	return false
}

func cliOSC133WrappedLines(lines []string) []string {
	if len(lines) == 0 {
		return lines
	}
	out := append([]string(nil), lines...)
	out[0] = cliOSC133ZoneStart + out[0]
	out[len(out)-1] = cliOSC133ZoneEnd + cliOSC133ZoneFinal + out[len(out)-1]
	return out
}

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
		return newCLIMarkdownWithOptions(body, gitui.MarkdownOptions{PaddingX: 1, PaddingY: 1}).Render(width)
	}
	collapsed := c.collapsed
	if collapsed == "" {
		collapsed = "Collapsed message"
	}
	return newCLIMarkdownWithOptions("["+label+"] "+collapsed, gitui.MarkdownOptions{PaddingX: 1, PaddingY: 1}).Render(width)
}

type cliDynamicBorderComponent struct{}

func (cliDynamicBorderComponent) Invalidate() {}

func (cliDynamicBorderComponent) Render(width int) []string {
	return []string{tuiThemeBorder(strings.Repeat("─", max(1, width)))}
}

func newCLIDynamicBorder() gitui.Component {
	return cliDynamicBorderComponent{}
}

const (
	cliArminWidth       = 31
	cliArminHeight      = 36
	cliArminDisplayRows = cliArminHeight / 2
	cliEarendilBlogURL  = "https://mariozechner.at/posts/2026-04-08-ive-sold-out/"
)

var cliArminBits = []byte{
	0xff, 0xff, 0xff, 0x7f, 0xff, 0xf0, 0xff, 0x7f, 0xff, 0xed, 0xff, 0x7f, 0xff, 0xdb, 0xff, 0x7f,
	0xff, 0xb7, 0xff, 0x7f, 0xff, 0x77, 0xfe, 0x7f, 0x3f, 0xf8, 0xfe, 0x7f, 0xdf, 0xff, 0xfe, 0x7f,
	0xdf, 0x3f, 0xfc, 0x7f, 0x9f, 0xc3, 0xfb, 0x7f, 0x6f, 0xfc, 0xf4, 0x7f, 0xf7, 0x0f, 0xf7, 0x7f,
	0xf7, 0xff, 0xf7, 0x7f, 0xf7, 0xff, 0xe3, 0x7f, 0xf7, 0x07, 0xe8, 0x7f, 0xef, 0xf8, 0x67, 0x70,
	0x0f, 0xff, 0xbb, 0x6f, 0xf1, 0x00, 0xd0, 0x5b, 0xfd, 0x3f, 0xec, 0x53, 0xc1, 0xff, 0xef, 0x57,
	0x9f, 0xfd, 0xee, 0x5f, 0x9f, 0xfc, 0xae, 0x5f, 0x1f, 0x78, 0xac, 0x5f, 0x3f, 0x00, 0x50, 0x6c,
	0x7f, 0x00, 0xdc, 0x77, 0xff, 0xc0, 0x3f, 0x78, 0xff, 0x01, 0xf8, 0x7f, 0xff, 0x03, 0x9c, 0x78,
	0xff, 0x07, 0x8c, 0x7c, 0xff, 0x0f, 0xce, 0x78, 0xff, 0xff, 0xcf, 0x7f, 0xff, 0xff, 0xcf, 0x78,
	0xff, 0xff, 0xdf, 0x78, 0xff, 0xff, 0xdf, 0x7d, 0xff, 0xff, 0x3f, 0x7e, 0xff, 0xff, 0xff, 0x7f,
}

type cliArminComponent struct{}

func newCLIArminComponent() gitui.Component {
	return cliArminComponent{}
}

func (cliArminComponent) Invalidate() {}

func (cliArminComponent) Render(width int) []string {
	available := max(0, width-1)
	lines := make([]string, 0, cliArminDisplayRows+1)
	for row := 0; row < cliArminDisplayRows; row++ {
		var builder strings.Builder
		for x := 0; x < cliArminWidth && gitui.VisibleWidth(builder.String()) < available; x++ {
			builder.WriteString(cliArminCell(x, row))
		}
		lines = append(lines, cliPaddedAccentLine(builder.String(), width))
	}
	return append(lines, cliPaddedAccentLine("ARMIN SAYS HI", width))
}

func cliArminCell(x, row int) string {
	upper := cliArminPixel(x, row*2)
	lower := cliArminPixel(x, row*2+1)
	switch {
	case upper && lower:
		return "█"
	case upper:
		return "▀"
	case lower:
		return "▄"
	default:
		return " "
	}
}

func cliArminPixel(x, y int) bool {
	if x < 0 || x >= cliArminWidth || y < 0 || y >= cliArminHeight {
		return false
	}
	const bytesPerRow = (cliArminWidth + 7) / 8
	byteIndex := y*bytesPerRow + x/8
	if byteIndex < 0 || byteIndex >= len(cliArminBits) {
		return false
	}
	return ((cliArminBits[byteIndex] >> (x % 8)) & 1) == 0
}

func cliPaddedAccentLine(text string, width int) string {
	available := max(0, width-1)
	text = gitui.TruncateToWidth(text, available, "")
	return " " + tuiThemeAccent(text) + strings.Repeat(" ", max(0, available-gitui.VisibleWidth(text)))
}

type cliEarendilAnnouncementComponent struct{}

func newCLIEarendilAnnouncementComponent() gitui.Component {
	return cliEarendilAnnouncementComponent{}
}

func (cliEarendilAnnouncementComponent) Invalidate() {}

func (cliEarendilAnnouncementComponent) Render(width int) []string {
	border := tuiThemeAccent(strings.Repeat("─", max(1, width)))
	return []string{
		border,
		" " + tuiThemeBoldAccent("gi has joined Earendil"),
		"",
		" " + tuiThemeMuted("Read the blog post:"),
		" " + tuiThemeFG("mdLink", cliEarendilBlogURL),
		"",
		tuiThemeMuted("[Image: clankolas.png [image/png] 640x537]"),
		"",
		border,
	}
}

func newCLIMarkdownWithOptions(text string, options gitui.MarkdownOptions) *gitui.Markdown {
	if markdownThemeIsZero(options.Theme) {
		options.Theme = tuiThemeMarkdown()
	}
	return gitui.NewMarkdownWithOptions(text, options)
}

func markdownThemeIsZero(theme gitui.MarkdownTheme) bool {
	return theme.Text == nil &&
		theme.Heading == nil &&
		theme.Link == nil &&
		theme.LinkURL == nil &&
		theme.Code == nil &&
		theme.CodeBlock == nil &&
		theme.CodeBlockBorder == nil &&
		theme.Quote == nil &&
		theme.QuoteBorder == nil &&
		theme.HR == nil &&
		theme.ListBullet == nil &&
		theme.Bold == nil &&
		theme.Italic == nil &&
		theme.Strikethrough == nil &&
		theme.Underline == nil &&
		theme.HighlightCode == nil &&
		theme.CodeBlockIndent == ""
}
