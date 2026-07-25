package gicodingagent

import (
	"context"
	_ "embed"
	"strconv"
	"strings"
	"sync"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

const (
	cliOSC133ZoneStart = "\x1b]133;A\x07"
	cliOSC133ZoneEnd   = "\x1b]133;B\x07"
	cliOSC133ZoneFinal = "\x1b]133;C\x07"
)

type cliOutputPadComponent interface {
	SetOutputPad(int)
}

//go:embed assets/clankolas.png
var cliEarendilClankolasPNG []byte

type cliUserMessageComponent struct {
	mu        sync.RWMutex
	text      string
	outputPad int
	content   *gitui.Box
}

func newCLIUserMessageComponent(text string, outputPad int) *cliUserMessageComponent {
	component := &cliUserMessageComponent{
		text:      strings.TrimSpace(text),
		outputPad: normalizeOutputPad(outputPad),
	}
	component.rebuild()
	return component
}

func (c *cliUserMessageComponent) SetOutputPad(padding int) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.outputPad = normalizeOutputPad(padding)
	c.rebuildLocked()
	c.mu.Unlock()
}

func (c *cliUserMessageComponent) rebuild() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.rebuildLocked()
	c.mu.Unlock()
}

func (c *cliUserMessageComponent) rebuildLocked() {
	box := gitui.NewBox(c.outputPad, 1, func(text string) string {
		return tuiThemeBG("userMessageBg", text)
	})
	box.AddChild(newCLIMarkdownWithOptions(c.text, gitui.MarkdownOptions{
		DefaultTextStyle: &gitui.DefaultTextStyle{
			Color: func(text string) string { return tuiThemeFG("userMessageText", text) },
		},
	}))
	c.content = box
}

func (c *cliUserMessageComponent) Invalidate() {
	if c == nil {
		return
	}
	c.mu.RLock()
	content := c.content
	c.mu.RUnlock()
	if content != nil {
		content.Invalidate()
	}
}

func (c *cliUserMessageComponent) Render(width int) []string {
	if c == nil || strings.TrimSpace(c.text) == "" {
		return nil
	}
	c.mu.RLock()
	content := c.content
	c.mu.RUnlock()
	if content == nil {
		return nil
	}
	lines := content.Render(width)
	return cliOSC133WrappedLines(lines)
}

type cliAssistantMessageComponent struct {
	mu                  sync.Mutex
	message             llm.Message
	hideThinkingBlock   bool
	hiddenThinkingLabel string
	outputPad           int
}

func newCLIAssistantMessageComponent(message llm.Message, hideThinkingBlock bool, hiddenThinkingLabel string, outputPad int) *cliAssistantMessageComponent {
	return &cliAssistantMessageComponent{
		message:             message,
		hideThinkingBlock:   hideThinkingBlock,
		hiddenThinkingLabel: firstNonEmptyString(strings.TrimSpace(hiddenThinkingLabel), "Thinking..."),
		outputPad:           normalizeOutputPad(outputPad),
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

func (c *cliAssistantMessageComponent) SetHideThinkingBlock(hide bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hideThinkingBlock = hide
}

func (c *cliAssistantMessageComponent) SetHiddenThinkingLabel(label string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hiddenThinkingLabel = firstNonEmptyString(strings.TrimSpace(label), "Thinking...")
}

func (c *cliAssistantMessageComponent) SetOutputPad(padding int) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.outputPad = normalizeOutputPad(padding)
	c.mu.Unlock()
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
	outputPad := c.outputPad
	c.mu.Unlock()
	lines := renderCLIAssistantMessage(message, width, hideThinkingBlock, hiddenThinkingLabel, outputPad)
	if assistantMessageHasToolCalls(message) {
		return lines
	}
	return cliOSC133WrappedLines(lines)
}

func renderCLIAssistantMessage(message llm.Message, width int, hideThinkingBlock bool, hiddenThinkingLabel string, outputPad int) []string {
	outputPad = normalizeOutputPad(outputPad)
	hasVisibleContent := false
	for _, part := range message.Content {
		if cliAssistantContentPartVisible(part) {
			hasVisibleContent = true
			break
		}
	}

	var lines []string
	if hasVisibleContent {
		lines = append(lines, "")
	}

	for index, part := range message.Content {
		switch part.Type {
		case llm.ContentText:
			if text := strings.TrimSpace(part.Text); text != "" {
				lines = append(lines, newCLIMarkdownWithOptions(text, gitui.MarkdownOptions{PaddingX: outputPad}).Render(width)...)
			}
		case llm.ContentThinking:
			if thinking := strings.TrimSpace(part.Thinking); thinking != "" {
				hasVisibleAfter := cliAssistantHasVisibleContentAfter(message.Content, index)
				if hideThinkingBlock {
					label := firstNonEmptyString(strings.TrimSpace(hiddenThinkingLabel), "Thinking...")
					lines = append(lines, gitui.NewText(tuiThemeItalic(tuiThemeFG("thinkingText", label)), outputPad, 0).Render(width)...)
				} else {
					lines = append(lines, newCLIMarkdownWithOptions(thinking, gitui.MarkdownOptions{
						PaddingX: outputPad,
						DefaultTextStyle: &gitui.DefaultTextStyle{
							Color:  func(text string) string { return tuiThemeFG("thinkingText", text) },
							Italic: true,
						},
					}).Render(width)...)
				}
				if hasVisibleAfter {
					lines = append(lines, "")
				}
			}
		}
	}
	if !assistantMessageHasToolCalls(message) {
		if status := assistantMessageStatusText(message); status != "" {
			lines = append(lines, "")
			lines = append(lines, gitui.NewText(tuiThemeError(status), outputPad, 0).Render(width)...)
		}
	}
	if len(lines) == 0 {
		return nil
	}
	return lines
}

func cliAssistantHasVisibleContentAfter(parts []llm.ContentPart, index int) bool {
	for _, part := range parts[index+1:] {
		if cliAssistantContentPartVisible(part) {
			return true
		}
	}
	return false
}

func cliAssistantContentPartVisible(part llm.ContentPart) bool {
	switch part.Type {
	case llm.ContentText:
		return strings.TrimSpace(part.Text) != ""
	case llm.ContentThinking:
		return strings.TrimSpace(part.Thinking) != ""
	default:
		return false
	}
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

type BorderedLoaderOptions struct {
	Cancellable *bool
}

type BorderedLoaderComponent struct {
	gitui.FocusState
	mu          sync.Mutex
	loader      *gitui.Loader
	cancellable *gitui.CancellableLoader
	ctx         context.Context
	cancel      context.CancelFunc
	OnAbort     func()
}

func NewBorderedLoaderComponent(ui *gitui.TUI, message string, options ...BorderedLoaderOptions) *BorderedLoaderComponent {
	cancellable := true
	if len(options) > 0 && options[0].Cancellable != nil {
		cancellable = *options[0].Cancellable
	}
	component := &BorderedLoaderComponent{}
	indicator := gitui.LoaderIndicatorOptions{
		TUI:          ui,
		SpinnerColor: tuiThemeAccent,
		MessageColor: tuiThemeMuted,
	}
	if cancellable {
		loader := gitui.NewCancellableLoader(message, indicator)
		component.loader = loader.Loader
		component.cancellable = loader
		component.ctx = loader.Context()
		loader.OnAbort = func() {
			component.mu.Lock()
			onAbort := component.OnAbort
			component.mu.Unlock()
			if onAbort != nil {
				onAbort()
			}
		}
		return component
	}
	ctx, cancel := context.WithCancel(context.Background())
	component.loader = gitui.NewLoader(message, indicator)
	component.ctx = ctx
	component.cancel = cancel
	return component
}

func NewNonCancellableBorderedLoaderComponent(ui *gitui.TUI, message string) *BorderedLoaderComponent {
	cancellable := false
	return NewBorderedLoaderComponent(ui, message, BorderedLoaderOptions{Cancellable: &cancellable})
}

func (c *BorderedLoaderComponent) SetOnAbort(fn func()) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.OnAbort = fn
	c.mu.Unlock()
}

func (c *BorderedLoaderComponent) Context() context.Context {
	if c == nil || c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

func (c *BorderedLoaderComponent) Signal() context.Context {
	return c.Context()
}

func (c *BorderedLoaderComponent) Cancelled() bool {
	return c != nil && c.cancellable != nil && c.cancellable.Cancelled()
}

func (c *BorderedLoaderComponent) Aborted() bool {
	return c.Cancelled()
}

func (c *BorderedLoaderComponent) HandleInput(data string) {
	if c == nil || c.cancellable == nil {
		return
	}
	c.cancellable.HandleInput(data)
}

func (c *BorderedLoaderComponent) Dispose() {
	if c == nil || c.loader == nil {
		return
	}
	c.loader.Stop()
}

func (c *BorderedLoaderComponent) Invalidate() {}

func (c *BorderedLoaderComponent) Render(width int) []string {
	if c == nil || c.loader == nil {
		return nil
	}
	var lines []string
	lines = append(lines, newCLIDynamicBorder().Render(width)...)
	lines = append(lines, c.loader.Render(width)...)
	if c.cancellable != nil {
		lines = append(lines, gitui.NewSpacer(1).Render(width)...)
		cancelHint := tuiThemeKeyHint(selectorCancelKeyHint(), "cancel")
		lines = append(lines, gitui.NewText(cancelHint, 1, 0).Render(width)...)
	}
	lines = append(lines, gitui.NewSpacer(1).Render(width)...)
	lines = append(lines, newCLIDynamicBorder().Render(width)...)
	return lines
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
	lines := []string{
		border,
		" " + tuiThemeBoldAccent("gi has joined Earendil"),
		"",
		" " + tuiThemeMuted("Read the blog post:"),
		" " + tuiThemeFG("mdLink", cliEarendilBlogURL),
		"",
	}
	if len(cliEarendilClankolasPNG) > 0 {
		dimensions := gitui.ImageDimensions{Width: 640, Height: 537}
		image := gitui.NewImage(cliEarendilClankolasPNG, gitui.ImageOptions{
			MimeType:      "image/png",
			Filename:      "clankolas.png",
			MaxWidthCells: 56,
			Dimensions:    &dimensions,
		}, gitui.ImageTheme{FallbackColor: tuiThemeMuted})
		lines = append(lines, image.Render(width)...)
		lines = append(lines, "")
	}
	lines = append(lines, border)
	return lines
}

const (
	cliDaxnutsHex         = "bbbab8b9b9b6b9b8b5bcbbb8b8b7b4b7b5b2b6b5b2b8b7b4b7b6b3b6b4b1bdbcb8bab8b6bbb8b5b8b5b1bbb8b4c2bebbc1bebac0bdbabfbcb9c1bebabfbebbc0bfbcc0bdbabbb8b5c1bfbcbfbcb8bbb9b6bfbcb8c2bfbcc1bfbcbfbbb8bdb9b6b8b7b5b9b8b5b8b8b5b5b5b2b6b5b2b8b7b4b9b8b5b9b8b5b6b5b3bab8b5bcbab7bbb9b6bbb8b5bfb9b5bdb2abbcb0a8beb2aabeb5afbfbab6bebab7c0bfbcbebdbabebbb8c0bdbabfbebbc2bebbbdbab7c3c0bdc3c0bdc1bebbc2bebabfbcb8bab9b6b7b6b3b2b1aeb6b5b2b5b4b1b5b4b2b6b5b2b7b6b4b9b8b6b7b6b3bbbab7b2afaba5988fb49e90b09481b79a88b39683b09583b7a395bfb6b0c0bdbabdbbb8bebcb9c1bfbcc0bebbbdbab7bebbb8c2bfbcc0bdbac0bcb9bdb9b6c0bcb8b5b4b2b4b3b0bab9b6b9b9b6b5b4b1b5b4b1b6b5b3b9b8b5b9b8b6b9b8b6b2aeaa968174a6836eaa856eab846eaf8973ac8973b08f79b18f7ab39786b7a89dbbb3aebfbab6c2c0bdbebcb9bfbdbac3c1bdc2bebbc0bcb9bdb9b6c1bdbabfbbb8b4b3b0b9b8b5b8b7b5b4b3b1b5b4b1b8b7b4b8b7b5bab9b6bbbab7b1afad8c7a719d735ca47860a87d65a98069ae8972ae8c75af8d77aa826ba98067aa8974b39e90b6a79dbbb2adc0bdbac1bfbdbfbbb8c1bdb9bebab6c0bdb9bfbbb8c1bdbab4b2b0b7b6b4b7b6b3b4b2b0bab9b7b6b5b2b6b5b2bab9b6bab9b6958c87977663aa836bac8772b08f7aad8c77b2917db0917db0907cac8971a77d64a87f67ac8972b29887b8a89dbfbab5bfbdbac1bebac0bcb9c0bcb9c0bcb9c1bebabebab7b8b7b4b7b6b4b5b4b1b5b4b2b7b6b3b5b4b2bab9b7bab9b6b4b1ada88f7fad8973ae8d78b19684b19685b29786b69a89b29582b1917daa856ea87e66a97e66ad866ea9826baf9280b8ada6bdbbb8bebab7bfbbb8c1bdbabfbbb8bcb8b4bcb8b5b6b4b2b7b5b3b6b5b2b8b7b4b3b2afb8b7b4b6b5b2b3b2b0b3a59aab856fad8d78b0917eb19886b49b8bb49a89b39785b0917eaf8f7cab866fa77d65a77a61a87d64a9816ab08f79b5a296c1bcb8c3bfbcc2bebbbebab7bfbbb7bdbab6c2bebab8b7b4b7b6b4b6b5b3b7b6b3b6b5b2b9b8b6b4b3b1b6b1acac8f7ca9826bae8f7aaf9583b49c8cb49c8bb79d8cb59987b19380ad8e79ae8c77af8e78ac8771a3775faa826bae8972b39888bbb6b2bebbb8bfbbb8bfbbb8c0bdb9bebbb7c0bdb9b6b5b2b9b8b5b4b3b1b8b7b5b4b3b0b7b6b4b6b5b3b1a7a0aa8772a77d65a88570b49887b19b8d9c887c907a6d987f71aa907faf917daf8e7aad8c78ac8b77a8836ca9836cac8770b49b8abdb6b2c0bcb9c0bdb9bfbbb8bebab7bfbcb9bebab7b9b8b6b5b4b2b9b8b5b8b7b5b8b7b4b7b6b4b5b4b2b3a9a2ad8973a1755da9856fb398858c776a65544b776358725d526e594d9c7f6eb1907ba68672ad8e7aab8771ac856db18f79b3a092beb9b5c1bdbabdb9b5bebab7bfbbb7bebab7bcb9b6b7b6b4b6b6b3b8b7b4b5b4b2b8b6b4b7b6b3b4b3b0b4aba4a6826ba3775fb08e79b19584a88e7daa8e7db29481ad8f7c997e6da38674ac8d79ac8e7aae917f9a7c6a896a599a7c6ab3a398c1bdbabdb9b6bcb8b5bebab6bebab7bdb9b5bdb9b6b5b4b1b7b5b3b5b4b2b7b6b3b7b6b4b3b3b0b3b2b0b4aca5a7846fa97f68ae8f7bae9383b59c8bb2937fae8e79ac8b76af927eaf927eb29683b39885b2988891786a72594c6e594d978d86bdbab7bab7b3c0bcb9c0bcb9bebab7bebbb7bdb9b6b3b2b0b4b3b0b5b4b2b4b4b1b4b3b1b4b3b1b4b3b0b6ada5aa8670a57a62ad8e7ab29b8cb69d8dab856fa9826aa88069ab8771af907db49987b19684b29886b59987b39480b09787b5a9a1bcb8b5bebab7bdb9b5bebab7bfbbb8bfbbb7bbb7b4b3b2afb8b7b5b8b7b5b3b2b0b5b4b2b6b5b3b6b4b1afa299a98975a9826baf907cb39988b49a89af8e7aac8973aa856eaf8c74b1917dae907dac907db39988b29785b49785b7a090b9aca3bfbab7bcb8b5bdb9b6bcb8b4bcb8b5bdb9b5bcb8b4b5b4b2b6b5b3b4b3b0b4b3b0b9b8b5b8b6b4908b88887467aa8f7ea78976ad8973b08b74b59885b69e8eb29888b1917cb1917db1937fae907cb19686b39a8ab29886b59b8ab8a192b6aaa3b7b2afbcb8b4bcb8b5bbb7b4c0bcb9bebab7c0bcb9b6b5b2b6b5b3b4b3b0bab9b7b7b6b4b1b0ae7b716ba083709b806f716158967764b08870b29481b69b8ab69f8fb39a89b69f90b49d8db39a89b29988b49c8cb6a090b8a496baa49593867f8f8986bfbbb7bdb9b5bcb7b4bab6b3b9b5b2bab6b2b4b3b1b3b3b0b6b5b3b8b7b5b4b2b0a7a5a38f837dae917ea084725a504c63544da28370b39784b59e8db2a093a698909b918b998e8790857e95877dad998bb39c8cb5a091b9a2938d827c95908dbebab6bbb7b3bdbab7bbb7b4bdb9b6bbb7b4b4b3b0b5b4b1b8b7b5b6b5b3b8b8b5b4b2af968f8ab29a8bab9485544b483a323073655d96887f70655f61595547403e453e3c453f3d57504f655e5b90847db39c8db7a090b6a09189807aaba6a3bdb9b6c0bcb9bebab7bcb7b4bebab7bbb7b4b3b2b0b6b5b3b2b1afb7b6b4b8b7b4b5b4b1aeaba8b5a89fac998d4d44412d25244d46444e4744322b293a3230423937433a37352d2a59504c534b48524a48988a81b59f8fb19c8d827974b2afacbdb9b5bcb8b4bdb9b5bcb8b5bdb9b6bab6b2b8b7b5b5b4b2b6b6b3b9b8b5b7b6b3b6b5b2b8b6b3b9b4b1b2a9a26c64612d25242d2625312a28352d2c453d3a78675c8d7a6ea09792aea6a0615854332b29524a479f8e82b09d90a49b96c1bdb9bebab7bfbbb8bbb8b4b9b5b1b8b4b0b9b4b0b7b6b4b8b7b5b8b7b4b6b5b3b8b6b3bab9b6b9b8b5b4b3b0b7b5b2a5a29f453d3b261e1d261f1e2e2625413936857268977865b19482b5a69caca5a07c7572453d3b746963a0948cc5bfbbc0bbb8beb9b6bbb7b3bbb6b3b7b3afb8b4b0b9b5b1b7b6b3b6b5b3b5b4b2b5b4b2b7b6b3b7b6b3b8b6b3b4b2afb7b6b3b3b1ae6d6765251f1e1e18172a22212d2523443b3971625ab19888b09482a89182877e792c25243e3634766d6abeb9b5bfbbb7bebab6bcb7b3bbb6b3b9b5b1b7b3afb8b4b0b4b3b0b5b4b1b5b4b1b4b3b1b5b4b2b8b6b4b5b3b0b9b6b4b5b4b1b6b4b27f79762a2322221c1b2d2524221b1a443e3c47413f6f676281766f867971675e5a3e37352a222166605dbab7b3bdb9b5beb9b5bcb7b3bcb7b3b9b4b0bab6b2bab6b2b5b3b0b6b4b2b3b2afb7b6b3b4b4b1b4b3b0b6b4b1b5b4b1b4b3b0b9b6b29a8c8252474230292828201f181212322c2c231e1d1c16162c26252923222d26252d2523332b2a8e8885bcb8b5bcb7b3bbb6b2bcb7b3b9b4b1b9b5b1b7b2afb7b2ae7a838e9b9b9caeadacb3b2b0b3b2afb7b7b4b6b5b3b6b6b3b7b6b3b9ada4a991808e7b6f50453f2b24231a14142923221f19181d17161f18182620201d17162a22215d5654b7b3b0bbb7b3bbb6b2b8b4b0bab5b1bbb6b2bab5b1b8b4b0bab6b22c496b4c5d735f68766e727a828285929090adaba8b7b2aeb6a59ab39682a28470a387748e76674e403a1a14141d1716181211221c1c1f1918221c1b2f2827342d2c8d8884bab6b3b9b5b2bab5b1bab5b1b9b4b0bab6b2b8b4b0b9b4b0b7b2ae325e8b365f8a3a5d833f5b7a545f70646469706b6aa08f84b08e78b18e769f7e689e7f6b9e816d907766584940362d2a1c1615201b1a1a1413201a1a251e1d393331a39e9bbab5b1bcb7b3bab6b2b8b3afb8b4b0b9b4b0b9b4b1bab5b2b5b0ac3d6c9843729d44719c426e98415f805a64716f6a699d8677b1927eb3947faa89749d7a649f7f6ba487749e837186716454463f2c25231e181837302e3a33317a7471beb9b6bcb8b4bbb6b2b6b2aebab5b1b9b5b1b8b3afbab6b2b6b1adb5aeaa4877a14c7aa44e7ba345719a3a5d80586b7f767475927b6eb1927faf8e79b08e78a78169a07861a17f6aa58570a688749b83738270666f66618a8480a49e99b7b2aebab6b2bcb8b4b9b5b1b7b2aebab5b1b9b4b0b6b1aeb6b1adb2aca8b2aca84876a04a78a2517fa74771973a5d80405c7a6161677c695fac8a75b08d77b4917aaf8971ad876fa5816aa6846ea78670a98a76ac9484ab9f96b2aca8bdb8b4bcb7b3bcb8b4bcb8b4b8b3afb7b2aeb9b4b0b8b3afb8b2aeb6afabb3aeaab2aeaa4878a14b7aa34c7ba44a759b3d63873b5f825b67766f5f569c7e6caf8c77b18f79b28f78b5927caf8e78a98872aa8a76a98a76ac917fada199b7b0acb9b3afbfb9b5c1bab6bdb6b2b8b3afbab5b1b9b4b0b6afabb7b1adb3ada9b3aeaab0aba8"
	cliDaxnutsImageWidth  = 32
	cliDaxnutsImageHeight = 32
	cliDaxnutsReset       = "\x1b[0m"
)

type cliDaxnutsComponent struct {
	mu          sync.Mutex
	ui          *gitui.TUI
	image       []string
	tick        int
	maxTicks    int
	cachedWidth int
	cachedTick  int
	cachedLines []string
	stop        chan struct{}
}

func newCLIDaxnutsComponent(ui *gitui.TUI) gitui.Component {
	component := &cliDaxnutsComponent{
		ui:          ui,
		image:       buildCLIDaxnutsImage(),
		maxTicks:    25,
		cachedWidth: -1,
		cachedTick:  -1,
	}
	if ui != nil {
		component.start()
	}
	return component
}

func (c *cliDaxnutsComponent) Invalidate() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.cachedWidth = -1
	c.cachedTick = -1
	c.cachedLines = nil
	c.mu.Unlock()
}

func (c *cliDaxnutsComponent) Render(width int) []string {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	if width == c.cachedWidth && c.tick == c.cachedTick {
		lines := append([]string(nil), c.cachedLines...)
		c.mu.Unlock()
		return lines
	}
	tick := c.tick
	maxTicks := c.maxTicks
	image := append([]string(nil), c.image...)
	c.mu.Unlock()

	if maxTicks <= 0 {
		maxTicks = 25
	}
	center := func(text string) string {
		if width > 0 && gitui.VisibleWidth(text) > width {
			text = gitui.TruncateToWidth(text, width, "")
		}
		padding := max(0, (width-gitui.VisibleWidth(text))/2)
		return strings.Repeat(" ", padding) + text
	}
	scanlineWidth := cliDaxnutsImageWidth
	if width > 0 {
		scanlineWidth = min(scanlineWidth, width)
	}

	lines := []string{""}
	revealedRows := min(len(image), (tick*(len(image)+3))/maxTicks)
	for i := range image {
		switch {
		case i < revealedRows:
			lines = append(lines, center(image[i]))
		case i == revealedRows:
			lines = append(lines, center(cliDaxnutsRGB(100, 200, 255, false)+strings.Repeat("▓", scanlineWidth)+cliDaxnutsReset))
		default:
			lines = append(lines, center(strings.Repeat(" ", scanlineWidth)))
		}
	}
	lines = append(lines, "")

	textPhase := tick - maxTicks*6/10
	if textPhase > 0 || tick >= maxTicks {
		lines = append(lines,
			center(tuiThemeFG("accent", "Free Kimi K2.5 via OpenCode Zen")),
			center(tuiThemeFG("success", "\"Powered by daxnuts\"")),
			center(tuiThemeMuted("— @thdxr")),
		)
	} else {
		lines = append(lines, "", "", "")
	}

	lines = append(lines, "")
	if textPhase > 2 || tick >= maxTicks {
		lines = append(lines,
			center(tuiThemeFG("dim", "Try OpenCode")),
			center(tuiThemeFG("mdLink", "https://mistral.ai/news/mistral-vibe-2-0")),
		)
	} else {
		lines = append(lines, "", "")
	}
	lines = append(lines, "")

	c.mu.Lock()
	c.cachedWidth = width
	c.cachedTick = tick
	c.cachedLines = append([]string(nil), lines...)
	c.mu.Unlock()
	return lines
}

func (c *cliDaxnutsComponent) Dispose() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.stopLocked()
	c.mu.Unlock()
}

func (c *cliDaxnutsComponent) start() {
	c.mu.Lock()
	c.stopLocked()
	if c.maxTicks <= 0 {
		c.maxTicks = 25
	}
	stop := make(chan struct{})
	c.stop = stop
	interval := 80 * time.Millisecond
	c.mu.Unlock()
	c.requestRender()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.mu.Lock()
				if c.tick < c.maxTicks {
					c.tick++
				}
				done := c.tick >= c.maxTicks
				c.cachedWidth = -1
				c.cachedTick = -1
				if done {
					c.stopLocked()
				}
				c.mu.Unlock()
				c.requestRender()
				if done {
					return
				}
			case <-stop:
				return
			}
		}
	}()
}

func (c *cliDaxnutsComponent) stopLocked() {
	if c.stop != nil {
		close(c.stop)
		c.stop = nil
	}
}

func (c *cliDaxnutsComponent) requestRender() {
	c.mu.Lock()
	ui := c.ui
	c.mu.Unlock()
	if ui != nil {
		ui.RequestRender(false)
	}
}

func buildCLIDaxnutsImage() []string {
	lines := make([]string, 0, cliDaxnutsImageHeight/2)
	for row := 0; row < cliDaxnutsImageHeight; row += 2 {
		var line strings.Builder
		for col := 0; col < cliDaxnutsImageWidth; col++ {
			topR, topG, topB := cliDaxnutsPixel(col, row)
			bottomR, bottomG, bottomB := cliDaxnutsPixel(col, row+1)
			line.WriteString(cliDaxnutsRGB(bottomR, bottomG, bottomB, false))
			line.WriteString(cliDaxnutsRGB(topR, topG, topB, true))
			line.WriteString("▄")
		}
		line.WriteString(cliDaxnutsReset)
		lines = append(lines, line.String())
	}
	return lines
}

func cliDaxnutsPixel(x, y int) (int, int, int) {
	if x < 0 || x >= cliDaxnutsImageWidth || y < 0 || y >= cliDaxnutsImageHeight {
		return 0, 0, 0
	}
	index := (y*cliDaxnutsImageWidth + x) * 6
	if index+5 >= len(cliDaxnutsHex) {
		return 0, 0, 0
	}
	return cliDaxnutsHexByte(index), cliDaxnutsHexByte(index + 2), cliDaxnutsHexByte(index + 4)
}

func cliDaxnutsHexByte(index int) int {
	return cliDaxnutsHexNibble(cliDaxnutsHex[index])<<4 | cliDaxnutsHexNibble(cliDaxnutsHex[index+1])
}

func cliDaxnutsHexNibble(value byte) int {
	switch {
	case value >= '0' && value <= '9':
		return int(value - '0')
	case value >= 'a' && value <= 'f':
		return int(value-'a') + 10
	case value >= 'A' && value <= 'F':
		return int(value-'A') + 10
	default:
		return 0
	}
}

func cliDaxnutsRGB(r, g, b int, bg bool) string {
	target := "38"
	if bg {
		target = "48"
	}
	return "\x1b[" + target + ";2;" + strconv.Itoa(r) + ";" + strconv.Itoa(g) + ";" + strconv.Itoa(b) + "m"
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
