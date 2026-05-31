package gitui

import (
	"strings"
	"sync"
)

type Text struct {
	mu       sync.Mutex
	text     string
	paddingX int
	paddingY int
	bgFn     func(string) string
	cache    renderCache
}

func NewText(text string, paddingX, paddingY int, bgFn ...func(string) string) *Text {
	t := &Text{text: text, paddingX: paddingX, paddingY: paddingY}
	if len(bgFn) > 0 {
		t.bgFn = bgFn[0]
	}
	return t
}

func (t *Text) SetText(text string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.text = text
	t.cache = renderCache{}
}

func (t *Text) SetCustomBackground(fn func(string) string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.bgFn = fn
	t.cache = renderCache{}
}

func (t *Text) SetCustomBgFn(fn func(string) string) {
	t.SetCustomBackground(fn)
}

func (t *Text) Invalidate() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cache = renderCache{}
}

func (t *Text) Render(width int) []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cache.ok && t.cache.text == t.text && t.cache.width == width {
		return append([]string(nil), t.cache.lines...)
	}
	if strings.TrimSpace(t.text) == "" {
		return nil
	}
	contentWidth := max(1, width-t.paddingX*2)
	wrapped := WrapTextWithANSI(strings.ReplaceAll(t.text, "\t", "   "), contentWidth)
	left := strings.Repeat(" ", max(0, t.paddingX))
	right := strings.Repeat(" ", max(0, t.paddingX))
	empty := strings.Repeat(" ", max(0, width))
	var lines []string
	for i := 0; i < t.paddingY; i++ {
		lines = append(lines, ApplyBackgroundToLine(empty, width, t.bgFn))
	}
	for _, line := range wrapped {
		lines = append(lines, ApplyBackgroundToLine(left+line+right, width, t.bgFn))
	}
	for i := 0; i < t.paddingY; i++ {
		lines = append(lines, ApplyBackgroundToLine(empty, width, t.bgFn))
	}
	t.cache = renderCache{text: t.text, width: width, lines: append([]string(nil), lines...), ok: true}
	return lines
}

type Spacer struct {
	mu    sync.Mutex
	lines int
}

func NewSpacer(lines int) *Spacer {
	return &Spacer{lines: max(0, lines)}
}

func (s *Spacer) SetLines(lines int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lines = max(0, lines)
}

func (s *Spacer) Invalidate() {}

func (s *Spacer) Render(width int) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	lines := make([]string, s.lines)
	return lines
}

type TruncatedText struct {
	mu       sync.Mutex
	text     string
	paddingX int
	paddingY int
	ellipsis string
	style    func(string) string
}

type TruncatedTextOptions struct {
	Ellipsis string
	Style    func(string) string
}

func NewTruncatedText(text string, paddingX, paddingY int, options ...TruncatedTextOptions) *TruncatedText {
	opts := TruncatedTextOptions{Ellipsis: "..."}
	if len(options) > 0 {
		opts = options[0]
		if opts.Ellipsis == "" {
			opts.Ellipsis = "..."
		}
	}
	return &TruncatedText{text: text, paddingX: max(0, paddingX), paddingY: max(0, paddingY), ellipsis: opts.Ellipsis, style: opts.Style}
}

func (t *TruncatedText) SetText(text string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.text = text
}
func (t *TruncatedText) Invalidate() {}
func (t *TruncatedText) Render(width int) []string {
	t.mu.Lock()
	text := t.text
	paddingX := t.paddingX
	paddingY := t.paddingY
	ellipsis := t.ellipsis
	styleFn := t.style
	t.mu.Unlock()

	width = max(0, width)
	contentWidth := max(0, width-paddingX*2)
	firstLine := text
	if idx := strings.Index(firstLine, "\n"); idx >= 0 {
		firstLine = firstLine[:idx]
	}
	line := TruncateToWidth(firstLine, contentWidth, ellipsis, true)
	if styleFn != nil {
		line = styleFn(line)
	}
	pad := strings.Repeat(" ", paddingX)
	content := pad + line + pad
	content = TruncateToWidth(content, width, "", true)
	out := make([]string, 0, paddingY*2+1)
	for i := 0; i < paddingY; i++ {
		out = append(out, strings.Repeat(" ", width))
	}
	out = append(out, content)
	for i := 0; i < paddingY; i++ {
		out = append(out, strings.Repeat(" ", width))
	}
	return out
}

type Box struct {
	mu       sync.RWMutex
	children []Component
	paddingX int
	paddingY int
	bgFn     func(string) string
}

func NewBox(paddingX, paddingY int, bgFn ...func(string) string) *Box {
	b := &Box{paddingX: paddingX, paddingY: paddingY}
	if len(bgFn) > 0 {
		b.bgFn = bgFn[0]
	}
	return b
}

func (b *Box) AddChild(component Component) {
	if component != nil {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.children = append(b.children, component)
	}
}

func (b *Box) RemoveChild(component Component) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, child := range b.children {
		if child == component {
			b.children = append(b.children[:i], b.children[i+1:]...)
			return
		}
	}
}

func (b *Box) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.children = nil
}
func (b *Box) SetBackground(fn func(string) string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.bgFn = fn
}
func (b *Box) SetBgFn(fn func(string) string) {
	b.SetBackground(fn)
}
func (b *Box) Invalidate() {
	children, _, _, _ := b.snapshot()
	for _, child := range children {
		child.Invalidate()
	}
}
func (b *Box) Render(width int) []string {
	children, paddingX, paddingY, bgFn := b.snapshot()
	if len(children) == 0 {
		return nil
	}
	contentWidth := max(1, width-paddingX*2)
	left := strings.Repeat(" ", max(0, paddingX))
	var content []string
	for _, child := range children {
		for _, line := range child.Render(contentWidth) {
			content = append(content, left+line)
		}
	}
	if len(content) == 0 {
		return nil
	}
	var out []string
	for i := 0; i < paddingY; i++ {
		out = append(out, ApplyBackgroundToLine("", width, bgFn))
	}
	for _, line := range content {
		out = append(out, ApplyBackgroundToLine(line, width, bgFn))
	}
	for i := 0; i < paddingY; i++ {
		out = append(out, ApplyBackgroundToLine("", width, bgFn))
	}
	return out
}

func (b *Box) snapshot() ([]Component, int, int, func(string) string) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	children := make([]Component, len(b.children))
	copy(children, b.children)
	return children, b.paddingX, b.paddingY, b.bgFn
}
