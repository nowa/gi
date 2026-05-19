package gitui

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

type Text struct {
	mu       sync.Mutex
	text     string
	paddingX int
	paddingY int
	bgFn     func(string) string
	cache    renderCache
}

type renderCache struct {
	text       string
	width      int
	hyperlinks bool
	lines      []string
	ok         bool
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

type MarkdownTheme struct {
	Text            func(string) string
	Heading         func(string) string
	Link            func(string) string
	LinkURL         func(string) string
	Code            func(string) string
	CodeBlock       func(string) string
	CodeBlockBorder func(string) string
	Quote           func(string) string
	QuoteBorder     func(string) string
	HR              func(string) string
	ListBullet      func(string) string
	Bold            func(string) string
	Italic          func(string) string
	Strikethrough   func(string) string
	Underline       func(string) string
	HighlightCode   func(code, lang string) []string
	CodeBlockIndent string
}

type DefaultTextStyle struct {
	Color         func(string) string
	BgColor       func(string) string
	Bold          bool
	Italic        bool
	Strikethrough bool
	Underline     bool
}

type MarkdownOptions struct {
	Theme            MarkdownTheme
	PaddingX         int
	PaddingY         int
	DefaultTextStyle *DefaultTextStyle
}

type Markdown struct {
	mu                 sync.Mutex
	text               string
	theme              MarkdownTheme
	paddingX           int
	paddingY           int
	defaultTextStyle   *DefaultTextStyle
	defaultStylePrefix string
	defaultPrefixOK    bool
	linkDefinitions    map[string]string
	cache              renderCache
}

func NewMarkdown(text string, theme ...MarkdownTheme) *Markdown {
	m := &Markdown{text: text}
	if len(theme) > 0 {
		m.theme = theme[0]
	}
	return m
}

func NewMarkdownWithOptions(text string, options MarkdownOptions) *Markdown {
	return &Markdown{
		text:             text,
		theme:            options.Theme,
		paddingX:         max(0, options.PaddingX),
		paddingY:         max(0, options.PaddingY),
		defaultTextStyle: options.DefaultTextStyle,
	}
}

func (m *Markdown) SetText(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.text = text
	m.cache = renderCache{}
}
func (m *Markdown) Invalidate() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache = renderCache{}
}
func (m *Markdown) Render(width int) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	hyperlinks := GetCapabilities().Hyperlinks
	if m.cache.ok && m.cache.text == m.text && m.cache.width == width && m.cache.hyperlinks == hyperlinks {
		return append([]string(nil), m.cache.lines...)
	}
	if strings.TrimSpace(m.text) == "" {
		m.cache = renderCache{text: m.text, width: width, hyperlinks: hyperlinks, lines: nil, ok: true}
		return nil
	}
	contentWidth := max(1, width-m.paddingX*2)
	rawLines := strings.Split(normalizeMarkdownInput(m.text), "\n")
	document := parseMarkdownDocument(rawLines)
	m.linkDefinitions = document.LinkDefinitions
	var lines []string
	listOrderState := markdownListOrderState{}
	listIndentTracker := markdownListIndentTracker{}
	inListContext := false
	listContinuationPrefix := ""
	listContinuationSourceIndent := 0
	clearListContext := func() {
		listOrderState.clear()
		listIndentTracker.clear()
		inListContext = false
		listContinuationPrefix = ""
		listContinuationSourceIndent = 0
	}
	for i := 0; i < len(rawLines); i++ {
		line := strings.TrimRight(rawLines[i], "\r")
		trimmed := strings.TrimSpace(line)
		renderLine := stripMarkdownHardBreakMarker(line, rawLines, i)
		if definitionEnd, ok := markdownLinkDefinitionEnd(rawLines, i); ok {
			i = definitionEnd
			continue
		}
		if inListContext && hasMarkdownListContinuationIndent(line, listContinuationSourceIndent) {
			body := trimMarkdownListCodeIndent(line, listContinuationSourceIndent)
			if htmlLines, end, ok := markdownListHTMLBlock(rawLines, i, body, listContinuationSourceIndent); ok {
				lines = append(lines, m.renderPrefixedHTMLBlock(htmlLines, listContinuationPrefix, listContinuationPrefix, contentWidth, func(line string) string {
					return style(m.theme.Text, m.applyDefaultTextStyle(line))
				})...)
				i = end
				continue
			}
		}
		if htmlEnd, ok := markdownHTMLBlockEnd(rawLines, i); ok {
			clearListContext()
			lines = append(lines, m.renderHTMLBlock(rawLines[i:htmlEnd+1], contentWidth)...)
			i = htmlEnd
			m.appendBlockSpacing(&lines, rawLines, i+1)
			continue
		}
		if trimmed == "" {
			keepListContext := false
			suppressBlank := false
			if inListContext {
				if next := nextNonBlankMarkdownLine(rawLines, i+1); next >= 0 {
					keepListContext = isMarkdownListContinuationStart(rawLines[next], listContinuationSourceIndent)
					if _, ok := parseMarkdownListLineInfo(strings.TrimRight(rawLines[next], "\r")); ok {
						keepListContext = true
						suppressBlank = true
					}
				}
			}
			if !keepListContext {
				clearListContext()
			}
			if !suppressBlank && len(lines) > 0 && lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}
			continue
		}
		if tableEnd := markdownTableEnd(rawLines, i); tableEnd > i {
			clearListContext()
			lines = append(lines, m.renderTable(rawLines[i:tableEnd+1], contentWidth)...)
			i = tableEnd
			continue
		}
		if isIndentedMarkdownCodeLine(line) {
			_, _, isList := parseMarkdownListLine(line)
			if inListContext && !isList {
				if markdownLeadingSpaces(line) <= listContinuationSourceIndent {
					goto skipIndentedCodeBlock
				}
				if isMarkdownListDefinitionParagraphContinuation(rawLines, i, listContinuationSourceIndent) {
					goto skipIndentedCodeBlock
				}
				listIndent := VisibleWidth(listContinuationPrefix)
				lines = append(lines, listContinuationPrefix+style(m.theme.CodeBlockBorder, "```"))
				codeLines, end := collectIndentedMarkdownCodeBlock(rawLines, i, true)
				i = end
				lines = append(lines, m.renderListCodeBlockContent(codeLines, "", listIndent, contentWidth)...)
				lines = append(lines, listContinuationPrefix+style(m.theme.CodeBlockBorder, "```"))
				continue
			}
			allowIndentedList := inListContext
			if !isList || !allowIndentedList {
				listOrderState.clear()
				lines = append(lines, style(m.theme.CodeBlockBorder, "```"))
				codeLines, end := collectIndentedMarkdownCodeBlock(rawLines, i, allowIndentedList)
				i = end
				lines = append(lines, m.renderCodeBlockContent(codeLines, "", contentWidth)...)
				lines = append(lines, style(m.theme.CodeBlockBorder, "```"))
				m.appendBlockSpacing(&lines, rawLines, i+1)
				inListContext = allowIndentedList
				continue
			}
		}
	skipIndentedCodeBlock:
		if inListContext {
			if rendered, end, ok := m.renderListContinuationBlock(rawLines, i, listContinuationPrefix, listContinuationSourceIndent, contentWidth); ok {
				lines = append(lines, rendered...)
				i = end
				continue
			}
		}
		if inListContext && isMarkdownListDefinitionParagraphContinuation(rawLines, i, listContinuationSourceIndent) {
			body := trimMarkdownListCodeIndent(renderLine, listContinuationSourceIndent)
			continuationText := m.renderInline(body)
			lines = append(lines, wrapWithContinuation(listContinuationPrefix, listContinuationPrefix, continuationText, contentWidth)...)
			continue
		}
		if inListContext && isMarkdownListIndentedParagraphContinuation(rawLines, i, listContinuationSourceIndent) {
			body := trimMarkdownListCodeIndent(renderLine, listContinuationSourceIndent)
			continuationText := m.renderInline(strings.TrimSpace(body))
			lines = append(lines, wrapWithContinuation(listContinuationPrefix, listContinuationPrefix, continuationText, contentWidth)...)
			continue
		}
		if fence, ok := parseMarkdownFenceStart(line); ok {
			clearListContext()
			lines = append(lines, style(m.theme.CodeBlockBorder, renderMarkdownFenceBorder(fence.lang)))
			var codeLines []string
			renderedCode := false
			for i++; i < len(rawLines); i++ {
				codeLine := strings.TrimRight(rawLines[i], "\r")
				if isMarkdownFenceClose(codeLine, fence) {
					lines = append(lines, m.renderCodeBlockContent(codeLines, fence.lang, contentWidth)...)
					renderedCode = true
					lines = append(lines, style(m.theme.CodeBlockBorder, "```"))
					break
				}
				codeLines = append(codeLines, trimMarkdownFenceContentLine(codeLine, fence))
			}
			if !renderedCode {
				lines = append(lines, m.renderCodeBlockContent(codeLines, fence.lang, contentWidth)...)
			}
			m.appendBlockSpacing(&lines, rawLines, i+1)
			continue
		}
		if headingLevel, text := parseHeading(trimmed); headingLevel > 0 {
			clearListContext()
			prefix := ""
			if headingLevel >= 3 {
				prefix = strings.Repeat("#", headingLevel) + " "
			}
			headingStyle := m.headingStyle(headingLevel)
			headingText := m.renderInlineWithStyle(text, headingStyle, stylePrefix(headingStyle))
			if prefix != "" {
				headingText = headingStyle(prefix) + headingText
			}
			lines = append(lines, wrapWithPrefix("", headingText, contentWidth)...)
			m.appendBlockSpacing(&lines, rawLines, i+1)
			continue
		}
		if headingLevel, text, ok := parseSetextHeading(rawLines, i); ok {
			clearListContext()
			headingText := m.renderHeadingText(headingLevel, text)
			lines = append(lines, wrapWithPrefix("", headingText, contentWidth)...)
			i++
			m.appendBlockSpacing(&lines, rawLines, i+1)
			continue
		}
		if isHorizontalRule(trimmed) {
			clearListContext()
			lines = append(lines, style(m.theme.HR, strings.Repeat("─", max(3, min(contentWidth, 80)))))
			m.appendBlockSpacing(&lines, rawLines, i+1)
			continue
		}
		if body, ok := parseMarkdownBlockquoteLine(renderLine); ok {
			clearListContext()
			quoteBodies := []string{body}
			for i+1 < len(rawLines) {
				nextLine := strings.TrimRight(rawLines[i+1], "\r")
				nextTrimmed := strings.TrimSpace(nextLine)
				if nextTrimmed == "" {
					break
				}
				nextRenderLine := stripMarkdownHardBreakMarker(nextLine, rawLines, i+1)
				if nextBody, explicit := parseMarkdownBlockquoteLine(nextRenderLine); explicit {
					quoteBodies = append(quoteBodies, nextBody)
					i++
					continue
				}
				if !isMarkdownLazyBlockquoteContinuation(rawLines, i+1) {
					break
				}
				quoteBodies = append(quoteBodies, strings.TrimSpace(nextRenderLine))
				i++
			}
			quotePrefix := style(m.theme.QuoteBorder, "│ ")
			quoteStyle := m.quoteStyle()
			quoteStylePrefix := stylePrefix(quoteStyle)
			lines = append(lines, m.renderBlockquoteBodies(quoteBodies, quotePrefix, quoteStyle, quoteStylePrefix, contentWidth)...)
			m.appendBlockSpacing(&lines, rawLines, i+1)
			continue
		}
		if info, ok := parseMarkdownListLineInfo(renderLine); ok {
			if inListContext && markdownPreviousListContinuationLooksLikeReferenceDefinition(rawLines, i, listContinuationSourceIndent) && len(lines) > 0 && lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}
			info = info.withIndent(listIndentTracker.indentFor(info.leading, inListContext))
			plainPrefix := listOrderState.prefix(info)
			prefix := m.renderListPrefix(plainPrefix)
			continuationPrefix := strings.Repeat(" ", VisibleWidth(plainPrefix))
			body := info.body
			sourceIndent := VisibleWidth(plainPrefix)
			if indent, ok := markdownListSourceContentIndent(renderLine); ok {
				sourceIndent = indent
			}
			if quoteBodies, end, ok := markdownListBlockquote(rawLines, i, renderLine, body); ok {
				quoteStyle := m.quoteStyle()
				quoteStylePrefix := stylePrefix(quoteStyle)
				lines = append(lines, m.renderListBlockquoteBodies(quoteBodies, prefix, continuationPrefix, quoteStyle, quoteStylePrefix, contentWidth)...)
				i = end
			} else if tableRows, end, ok := markdownListTable(rawLines, i, body, sourceIndent); ok {
				lines = append(lines, m.renderPrefixedTable(tableRows, prefix, continuationPrefix, contentWidth)...)
				i = end
			} else if htmlLines, end, ok := markdownListHTMLBlock(rawLines, i, body, VisibleWidth(plainPrefix)); ok {
				lines = append(lines, m.renderPrefixedHTMLBlock(htmlLines, prefix, continuationPrefix, contentWidth, func(line string) string {
					return style(m.theme.Text, m.applyDefaultTextStyle(line))
				})...)
				i = end
			} else if fence, ok := parseMarkdownFenceStart(body); ok {
				lines = append(lines, prefix+style(m.theme.CodeBlockBorder, renderMarkdownFenceBorder(fence.lang)))
				listIndent := VisibleWidth(plainPrefix)
				var codeLines []string
				renderedCode := false
				for i+1 < len(rawLines) {
					i++
					codeLine := strings.TrimRight(rawLines[i], "\r")
					codeBody := trimMarkdownListCodeIndent(codeLine, listIndent)
					if isMarkdownFenceClose(codeBody, fence) {
						lines = append(lines, m.renderListCodeBlockContent(codeLines, fence.lang, listIndent, contentWidth)...)
						renderedCode = true
						lines = append(lines, strings.Repeat(" ", listIndent)+style(m.theme.CodeBlockBorder, "```"))
						break
					}
					codeLines = append(codeLines, trimMarkdownFenceContentLine(codeBody, fence))
				}
				if !renderedCode {
					lines = append(lines, m.renderListCodeBlockContent(codeLines, fence.lang, listIndent, contentWidth)...)
				}
			} else {
				bodyLines := []string{body}
				for i+1 < len(rawLines) && isMarkdownLazyListContinuation(rawLines, i+1) {
					i++
					nextLine := strings.TrimRight(rawLines[i], "\r")
					bodyLines = append(bodyLines, strings.TrimSpace(stripMarkdownHardBreakMarker(nextLine, rawLines, i)))
				}
				lines = append(lines, wrapWithContinuation(prefix, continuationPrefix, m.renderInline(strings.Join(bodyLines, "\n")), contentWidth)...)
			}
			inListContext = true
			listContinuationPrefix = continuationPrefix
			listContinuationSourceIndent = sourceIndent
			continue
		}
		if inListContext && isMarkdownLazyListContinuation(rawLines, i) {
			continuationText := m.renderInline(strings.TrimSpace(renderLine))
			lines = append(lines, wrapWithContinuation(listContinuationPrefix, listContinuationPrefix, continuationText, contentWidth)...)
			continue
		}
		clearListContext()
		paragraphLines := []string{renderLine}
		for i+1 < len(rawLines) && isMarkdownParagraphContinuationLine(rawLines, i+1) {
			i++
			nextLine := strings.TrimRight(rawLines[i], "\r")
			paragraphLines = append(paragraphLines, stripMarkdownHardBreakMarker(nextLine, rawLines, i))
		}
		paragraphText := strings.Join(paragraphLines, "\n")
		if i+1 < len(rawLines) {
			if headingLevel, ok := parseSetextUnderline(rawLines[i+1]); ok {
				headingText := m.renderHeadingText(headingLevel, strings.TrimSpace(paragraphText))
				lines = append(lines, wrapWithPrefix("", headingText, contentWidth)...)
				i++
				m.appendBlockSpacing(&lines, rawLines, i+1)
				continue
			}
		}
		lines = append(lines, wrapWithPrefix("", style(m.theme.Text, m.renderInline(paragraphText)), contentWidth)...)
		if next := nextNonBlankMarkdownLine(rawLines, i+1); next >= 0 && startsMarkdownStructuralBlock(strings.TrimSpace(rawLines[next])) {
			m.appendBlockSpacing(&lines, rawLines, i+1)
		}
	}
	out := m.applyPadding(lines, width)
	if len(out) == 0 {
		out = []string{""}
	}
	m.cache = renderCache{text: m.text, width: width, hyperlinks: hyperlinks, lines: append([]string(nil), out...), ok: true}
	return out
}

func normalizeMarkdownInput(text string) string {
	return strings.ReplaceAll(text, "\t", "   ")
}

func (m *Markdown) renderBlockquoteBodies(quoteBodies []string, quotePrefix string, quoteStyle func(string) string, quoteStylePrefix string, width int) []string {
	var lines []string
	quoteContentWidth := max(1, width-VisibleWidth(quotePrefix))
	listOrderState := markdownListOrderState{}
	listIndentTracker := markdownListIndentTracker{}
	inListContext := false
	listContinuationPrefix := ""
	listContinuationSourceIndent := 0
	clearListContext := func() {
		listOrderState.clear()
		listIndentTracker.clear()
		inListContext = false
		listContinuationPrefix = ""
		listContinuationSourceIndent = 0
	}
	for i := 0; i < len(quoteBodies); i++ {
		quoteBody := quoteBodies[i]
		if definitionEnd, ok := markdownLinkDefinitionEnd(quoteBodies, i); ok {
			clearListContext()
			i = definitionEnd
			continue
		}
		if inListContext && hasMarkdownListContinuationIndent(quoteBody, listContinuationSourceIndent) {
			body := trimMarkdownListCodeIndent(quoteBody, listContinuationSourceIndent)
			if htmlLines, end, ok := markdownListHTMLBlock(quoteBodies, i, body, listContinuationSourceIndent); ok {
				lines = append(lines, m.renderPrefixedHTMLBlock(htmlLines, quotePrefix+listContinuationPrefix, quotePrefix+listContinuationPrefix, width, func(line string) string {
					return m.applyBlockquoteStyle(line, quoteStyle, quoteStylePrefix)
				})...)
				i = end
				continue
			}
		}
		if htmlEnd, ok := markdownHTMLBlockEnd(quoteBodies, i); ok {
			clearListContext()
			for _, rendered := range m.renderHTMLBlockWithBlockStyle(quoteBodies[i:htmlEnd+1], quoteContentWidth, func(text string) string {
				return m.applyBlockquoteStyle(text, quoteStyle, quoteStylePrefix)
			}) {
				lines = append(lines, quotePrefix+rendered)
			}
			i = htmlEnd
			continue
		}
		if tableEnd := markdownTableEnd(quoteBodies, i); tableEnd > i {
			clearListContext()
			for _, rendered := range m.renderTable(quoteBodies[i:tableEnd+1], quoteContentWidth) {
				lines = append(lines, quotePrefix+m.applyBlockquoteStyle(rendered, quoteStyle, quoteStylePrefix))
			}
			i = tableEnd
			continue
		}
		trimmed := strings.TrimSpace(quoteBody)
		if trimmed == "" {
			keepListContext := false
			skipBlank := false
			if inListContext {
				if next := nextNonBlankMarkdownLine(quoteBodies, i+1); next >= 0 {
					keepListContext = isMarkdownListContinuationStart(quoteBodies[next], listContinuationSourceIndent)
					_, skipBlank = parseMarkdownListLineInfo(quoteBodies[next])
				}
			}
			if !keepListContext {
				clearListContext()
			}
			if skipBlank {
				continue
			}
			if len(lines) == 0 || lines[len(lines)-1] != quotePrefix {
				lines = append(lines, quotePrefix)
			}
			continue
		}
		if headingLevel, text := parseHeading(trimmed); headingLevel > 0 {
			clearListContext()
			headingText := m.renderHeadingText(headingLevel, text)
			lines = append(lines, quotePrefix+m.applyBlockquoteStyle(headingText, quoteStyle, quoteStylePrefix))
			continue
		}
		if headingLevel, text, ok := parseSetextHeading(quoteBodies, i); ok {
			clearListContext()
			headingText := m.renderHeadingText(headingLevel, text)
			lines = append(lines, wrapWithContinuation(quotePrefix, quotePrefix, m.applyBlockquoteStyle(headingText, quoteStyle, quoteStylePrefix), width)...)
			i++
			continue
		}
		if isHorizontalRule(trimmed) {
			clearListContext()
			hr := style(m.theme.HR, strings.Repeat("─", max(3, min(quoteContentWidth, 80))))
			lines = append(lines, quotePrefix+m.applyBlockquoteStyle(hr, quoteStyle, quoteStylePrefix))
			continue
		}
		if nestedBody, ok := parseMarkdownBlockquoteLine(quoteBody); ok {
			clearListContext()
			nestedBodies := []string{nestedBody}
			for i+1 < len(quoteBodies) {
				nextBody, explicit := parseMarkdownBlockquoteLine(quoteBodies[i+1])
				if !explicit {
					break
				}
				nestedBodies = append(nestedBodies, nextBody)
				i++
			}
			nestedPrefix := quotePrefix + style(m.theme.QuoteBorder, "│ ")
			lines = append(lines, m.renderBlockquoteBodies(nestedBodies, nestedPrefix, quoteStyle, quoteStylePrefix, width)...)
			continue
		}
		if isIndentedMarkdownCodeLine(quoteBody) {
			_, _, isList := parseMarkdownListLine(quoteBody)
			if inListContext && !isList {
				if markdownLeadingSpaces(quoteBody) <= listContinuationSourceIndent {
					goto skipBlockquoteIndentedCodeBlock
				}
				if isMarkdownListDefinitionParagraphContinuation(quoteBodies, i, listContinuationSourceIndent) {
					goto skipBlockquoteIndentedCodeBlock
				}
				listIndent := VisibleWidth(listContinuationPrefix)
				lines = append(lines, quotePrefix+m.applyBlockquoteStyle(listContinuationPrefix+style(m.theme.CodeBlockBorder, "```"), quoteStyle, quoteStylePrefix))
				codeLines, end := collectIndentedMarkdownCodeBlock(quoteBodies, i, true)
				i = end
				for _, rendered := range m.renderListCodeBlockContent(codeLines, "", listIndent, quoteContentWidth) {
					lines = append(lines, quotePrefix+m.applyBlockquoteStyle(rendered, quoteStyle, quoteStylePrefix))
				}
				lines = append(lines, quotePrefix+m.applyBlockquoteStyle(listContinuationPrefix+style(m.theme.CodeBlockBorder, "```"), quoteStyle, quoteStylePrefix))
				continue
			}
			if !isList || !inListContext {
				clearListContext()
				lines = append(lines, quotePrefix+m.applyBlockquoteStyle(style(m.theme.CodeBlockBorder, "```"), quoteStyle, quoteStylePrefix))
				codeLines, end := collectIndentedMarkdownCodeBlock(quoteBodies, i, false)
				i = end
				for _, rendered := range m.renderCodeBlockContent(codeLines, "", quoteContentWidth) {
					lines = append(lines, quotePrefix+m.applyBlockquoteStyle(rendered, quoteStyle, quoteStylePrefix))
				}
				lines = append(lines, quotePrefix+m.applyBlockquoteStyle(style(m.theme.CodeBlockBorder, "```"), quoteStyle, quoteStylePrefix))
				continue
			}
		}
	skipBlockquoteIndentedCodeBlock:
		if inListContext {
			if rendered, end, ok := m.renderListContinuationBlock(quoteBodies, i, listContinuationPrefix, listContinuationSourceIndent, quoteContentWidth); ok {
				for _, line := range rendered {
					lines = append(lines, quotePrefix+m.applyBlockquoteStyle(line, quoteStyle, quoteStylePrefix))
				}
				i = end
				continue
			}
		}
		if inListContext && isMarkdownListDefinitionParagraphContinuation(quoteBodies, i, listContinuationSourceIndent) {
			body := trimMarkdownListCodeIndent(quoteBody, listContinuationSourceIndent)
			continuationText := m.renderInlineWithStyle(body, func(text string) string { return text }, quoteStylePrefix)
			for _, rendered := range wrapWithContinuation(listContinuationPrefix, listContinuationPrefix, continuationText, quoteContentWidth) {
				lines = append(lines, quotePrefix+m.applyBlockquoteStyle(rendered, quoteStyle, quoteStylePrefix))
			}
			continue
		}
		if inListContext && isMarkdownListIndentedParagraphContinuation(quoteBodies, i, listContinuationSourceIndent) {
			body := trimMarkdownListCodeIndent(quoteBody, listContinuationSourceIndent)
			continuationText := m.renderInlineWithStyle(strings.TrimSpace(body), func(text string) string { return text }, quoteStylePrefix)
			for _, rendered := range wrapWithContinuation(listContinuationPrefix, listContinuationPrefix, continuationText, quoteContentWidth) {
				lines = append(lines, quotePrefix+m.applyBlockquoteStyle(rendered, quoteStyle, quoteStylePrefix))
			}
			continue
		}
		if fence, ok := parseMarkdownFenceStart(quoteBody); ok {
			clearListContext()
			lines = append(lines, quotePrefix+m.applyBlockquoteStyle(style(m.theme.CodeBlockBorder, renderMarkdownFenceBorder(fence.lang)), quoteStyle, quoteStylePrefix))
			var codeLines []string
			renderedCode := false
			for i+1 < len(quoteBodies) {
				i++
				codeLine := quoteBodies[i]
				if isMarkdownFenceClose(codeLine, fence) {
					for _, rendered := range m.renderCodeBlockContent(codeLines, fence.lang, quoteContentWidth) {
						lines = append(lines, quotePrefix+m.applyBlockquoteStyle(rendered, quoteStyle, quoteStylePrefix))
					}
					lines = append(lines, quotePrefix+m.applyBlockquoteStyle(style(m.theme.CodeBlockBorder, "```"), quoteStyle, quoteStylePrefix))
					renderedCode = true
					break
				}
				codeLines = append(codeLines, trimMarkdownFenceContentLine(codeLine, fence))
			}
			if !renderedCode {
				for _, rendered := range m.renderCodeBlockContent(codeLines, fence.lang, quoteContentWidth) {
					lines = append(lines, quotePrefix+m.applyBlockquoteStyle(rendered, quoteStyle, quoteStylePrefix))
				}
			}
			continue
		}
		if info, ok := parseMarkdownListLineInfo(quoteBody); ok {
			info = info.withIndent(listIndentTracker.indentFor(info.leading, inListContext))
			plainPrefix := listOrderState.prefix(info)
			prefix := m.renderListPrefix(plainPrefix)
			continuationPrefix := strings.Repeat(" ", VisibleWidth(plainPrefix))
			body := info.body
			sourceIndent := VisibleWidth(plainPrefix)
			if indent, ok := markdownListSourceContentIndent(quoteBody); ok {
				sourceIndent = indent
			}
			if nestedQuoteBodies, end, ok := markdownListBlockquote(quoteBodies, i, quoteBody, body); ok {
				lines = append(lines, m.renderListBlockquoteBodies(nestedQuoteBodies, quotePrefix+prefix, quotePrefix+continuationPrefix, quoteStyle, quoteStylePrefix, width)...)
				i = end
				inListContext = true
				listContinuationPrefix = continuationPrefix
				listContinuationSourceIndent = sourceIndent
				continue
			}
			if tableRows, end, ok := markdownListTable(quoteBodies, i, body, sourceIndent); ok {
				for _, rendered := range m.renderPrefixedTable(tableRows, prefix, continuationPrefix, quoteContentWidth) {
					lines = append(lines, quotePrefix+m.applyBlockquoteStyle(rendered, quoteStyle, quoteStylePrefix))
				}
				i = end
				inListContext = true
				listContinuationPrefix = continuationPrefix
				listContinuationSourceIndent = sourceIndent
				continue
			}
			if htmlLines, end, ok := markdownListHTMLBlock(quoteBodies, i, body, VisibleWidth(plainPrefix)); ok {
				for _, rendered := range m.renderPrefixedHTMLBlock(htmlLines, quotePrefix+prefix, quotePrefix+continuationPrefix, width, func(line string) string {
					return m.applyBlockquoteStyle(line, quoteStyle, quoteStylePrefix)
				}) {
					lines = append(lines, rendered)
				}
				i = end
				inListContext = true
				listContinuationPrefix = continuationPrefix
				listContinuationSourceIndent = sourceIndent
				continue
			}
			bodyLines := []string{body}
			for i+1 < len(quoteBodies) && isMarkdownLazyListContinuation(quoteBodies, i+1) {
				i++
				bodyLines = append(bodyLines, strings.TrimSpace(stripMarkdownHardBreakMarker(quoteBodies[i], quoteBodies, i)))
			}
			quoteText := m.renderInlineWithStyle(strings.Join(bodyLines, "\n"), func(text string) string { return text }, quoteStylePrefix)
			for _, rendered := range wrapWithContinuation(prefix, continuationPrefix, quoteText, quoteContentWidth) {
				lines = append(lines, quotePrefix+m.applyBlockquoteStyle(rendered, quoteStyle, quoteStylePrefix))
			}
			inListContext = true
			listContinuationPrefix = continuationPrefix
			listContinuationSourceIndent = sourceIndent
			continue
		}
		if inListContext && isMarkdownLazyListContinuation(quoteBodies, i) {
			continuationText := m.renderInlineWithStyle(strings.TrimSpace(quoteBody), quoteStyle, quoteStylePrefix)
			prefix := quotePrefix + listContinuationPrefix
			lines = append(lines, wrapWithContinuation(prefix, prefix, continuationText, width)...)
			continue
		}
		clearListContext()
		quoteParagraphLines := []string{quoteBody}
		for i+1 < len(quoteBodies) && isMarkdownParagraphContinuationLine(quoteBodies, i+1) {
			i++
			quoteParagraphLines = append(quoteParagraphLines, stripMarkdownHardBreakMarker(quoteBodies[i], quoteBodies, i))
		}
		quoteParagraphText := strings.Join(quoteParagraphLines, "\n")
		if i+1 < len(quoteBodies) {
			if headingLevel, ok := parseSetextUnderline(quoteBodies[i+1]); ok {
				headingText := m.renderHeadingText(headingLevel, strings.TrimSpace(quoteParagraphText))
				lines = append(lines, wrapWithContinuation(quotePrefix, quotePrefix, m.applyBlockquoteStyle(headingText, quoteStyle, quoteStylePrefix), width)...)
				i++
				continue
			}
		}
		quoteText := m.renderInlineWithStyle(quoteParagraphText, quoteStyle, quoteStylePrefix)
		lines = append(lines, wrapWithContinuation(quotePrefix, quotePrefix, quoteText, width)...)
	}
	return lines
}

func (m *Markdown) renderListBlockquoteBodies(quoteBodies []string, firstPrefix, continuationPrefix string, quoteStyle func(string) string, quoteStylePrefix string, width int) []string {
	itemWidth := max(1, width-VisibleWidth(firstPrefix))
	rendered := m.renderBlockquoteBodies(quoteBodies, style(m.theme.QuoteBorder, "│ "), quoteStyle, quoteStylePrefix, itemWidth)
	lines := make([]string, 0, len(rendered))
	for i, line := range rendered {
		prefix := continuationPrefix
		if i == 0 {
			prefix = firstPrefix
		}
		lines = append(lines, prefix+line)
	}
	return lines
}

func markdownListTable(rawLines []string, index int, firstBody string, sourceIndent int) ([]string, int, bool) {
	if index < 0 || index >= len(rawLines) {
		return nil, index, false
	}
	rows := []string{firstBody}
	end := index
	for end+1 < len(rawLines) {
		next := strings.TrimRight(rawLines[end+1], "\r")
		if strings.TrimSpace(next) == "" || !hasMarkdownListContinuationIndent(next, sourceIndent) {
			break
		}
		body := trimMarkdownListCodeIndent(next, sourceIndent)
		if !isMarkdownTableRow(body) {
			break
		}
		rows = append(rows, body)
		end++
	}
	tableEnd := markdownTableEnd(rows, 0)
	if tableEnd <= 0 {
		return nil, index, false
	}
	return rows[:tableEnd+1], index + tableEnd, true
}

func (m *Markdown) renderPrefixedTable(rows []string, firstPrefix, continuationPrefix string, width int) []string {
	prefixWidth := max(VisibleWidth(firstPrefix), VisibleWidth(continuationPrefix))
	rendered := m.renderTable(rows, max(1, width-prefixWidth))
	lines := make([]string, 0, len(rendered))
	for i, line := range rendered {
		prefix := continuationPrefix
		if i == 0 {
			prefix = firstPrefix
		}
		lines = append(lines, prefix+line)
	}
	return lines
}

func (m *Markdown) renderHTMLBlock(rawLines []string, width int) []string {
	return m.renderHTMLBlockWithStyle(rawLines, width, func(line string) string {
		return style(m.theme.Text, m.applyDefaultTextStyle(line))
	})
}

func markdownHTMLBlockText(rawLines []string) string {
	raw := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		raw = append(raw, strings.TrimRight(line, "\r"))
	}
	return strings.TrimSpace(strings.Join(raw, "\n"))
}

func (m *Markdown) renderHTMLBlockWithStyle(rawLines []string, width int, styleLine func(string) string) []string {
	text := markdownHTMLBlockText(rawLines)
	if text == "" {
		return nil
	}
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if line == "" {
			lines = append(lines, "")
			continue
		}
		rendered := line
		if styleLine != nil {
			rendered = styleLine(line)
		}
		lines = append(lines, wrapWithPrefix("", rendered, width)...)
	}
	return lines
}

func (m *Markdown) renderHTMLBlockWithBlockStyle(rawLines []string, width int, styleBlock func(string) string) []string {
	text := markdownHTMLBlockText(rawLines)
	if text == "" {
		return nil
	}
	if styleBlock != nil {
		text = styleBlock(text)
	}
	return WrapTextWithANSI(text, width)
}

func (m *Markdown) renderPrefixedHTMLBlock(rawLines []string, firstPrefix, continuationPrefix string, width int, styleLine func(string) string) []string {
	var lines []string
	for i, raw := range rawLines {
		line := strings.TrimRight(raw, "\r")
		prefix := continuationPrefix
		if i == 0 {
			prefix = firstPrefix
		}
		if strings.TrimSpace(line) == "" {
			lines = append(lines, prefix)
			continue
		}
		rendered := line
		if styleLine != nil {
			rendered = styleLine(line)
		}
		lines = append(lines, wrapWithContinuation(prefix, prefix, rendered, width)...)
	}
	return lines
}

func (m *Markdown) renderHeadingText(headingLevel int, text string) string {
	prefix := ""
	if headingLevel >= 3 {
		prefix = strings.Repeat("#", headingLevel) + " "
	}
	headingStyle := m.headingStyle(headingLevel)
	headingText := m.renderInlineWithStyle(text, headingStyle, stylePrefix(headingStyle))
	if prefix != "" {
		headingText = headingStyle(prefix) + headingText
	}
	return headingText
}

func (m *Markdown) applyBlockquoteStyle(text string, quoteStyle func(string) string, quoteStylePrefix string) string {
	if quoteStylePrefix != "" {
		text = strings.ReplaceAll(text, "\x1b[0m", "\x1b[0m"+quoteStylePrefix)
	}
	return style(quoteStyle, text)
}

func (m *Markdown) appendBlockSpacing(lines *[]string, rawLines []string, next int) {
	if nextNonBlankMarkdownLine(rawLines, next) < 0 {
		return
	}
	if len(*lines) == 0 || (*lines)[len(*lines)-1] == "" {
		return
	}
	*lines = append(*lines, "")
}

func (m *Markdown) codeBlockIndent() string {
	if m.theme.CodeBlockIndent != "" {
		return m.theme.CodeBlockIndent
	}
	return "  "
}

func (m *Markdown) renderCodeBlockContent(codeLines []string, lang string, width int) []string {
	indent := m.codeBlockIndent()
	if m.theme.HighlightCode != nil {
		highlighted := m.theme.HighlightCode(strings.Join(codeLines, "\n"), lang)
		out := make([]string, 0, len(highlighted))
		for _, line := range highlighted {
			out = append(out, wrapWithPrefix("", indent+line, width)...)
		}
		return out
	}
	out := make([]string, 0, len(codeLines))
	for _, codeLine := range codeLines {
		out = append(out, wrapWithPrefix("", indent+style(m.theme.CodeBlock, codeLine), width)...)
	}
	return out
}

func (m *Markdown) renderListCodeBlockContent(codeLines []string, lang string, listIndent, width int) []string {
	indent := m.codeBlockIndent()
	firstPrefix := strings.Repeat(" ", listIndent) + indent
	continuation := strings.Repeat(" ", listIndent)
	if m.theme.HighlightCode != nil {
		highlighted := m.theme.HighlightCode(strings.Join(codeLines, "\n"), lang)
		out := make([]string, 0, len(highlighted))
		for _, line := range highlighted {
			out = append(out, wrapWithContinuation(firstPrefix, continuation, line, width)...)
		}
		return out
	}
	out := make([]string, 0, len(codeLines))
	for _, codeLine := range codeLines {
		out = append(out, wrapWithContinuation(firstPrefix, continuation, style(m.theme.CodeBlock, codeLine), width)...)
	}
	return out
}

func (m *Markdown) renderListContinuationBlock(rawLines []string, index int, listContinuationPrefix string, sourceIndent int, width int) ([]string, int, bool) {
	listIndent := VisibleWidth(listContinuationPrefix)
	if listIndent <= 0 || index < 0 || index >= len(rawLines) {
		return nil, index, false
	}
	line := strings.TrimRight(rawLines[index], "\r")
	if !hasMarkdownListContinuationIndent(line, sourceIndent) {
		return nil, index, false
	}
	body := trimMarkdownListCodeIndent(line, sourceIndent)
	fence, ok := parseMarkdownFenceStart(body)
	if !ok {
		return nil, index, false
	}
	lines := []string{listContinuationPrefix + style(m.theme.CodeBlockBorder, renderMarkdownFenceBorder(fence.lang))}
	var codeLines []string
	renderedCode := false
	for index+1 < len(rawLines) {
		index++
		codeLine := strings.TrimRight(rawLines[index], "\r")
		codeBody := trimMarkdownListCodeIndent(codeLine, sourceIndent)
		if isMarkdownFenceClose(codeBody, fence) {
			lines = append(lines, m.renderListCodeBlockContent(codeLines, fence.lang, listIndent, width)...)
			renderedCode = true
			lines = append(lines, listContinuationPrefix+style(m.theme.CodeBlockBorder, "```"))
			break
		}
		codeLines = append(codeLines, trimMarkdownFenceContentLine(codeBody, fence))
	}
	if !renderedCode {
		lines = append(lines, m.renderListCodeBlockContent(codeLines, fence.lang, listIndent, width)...)
	}
	return lines, index, true
}

func (m *Markdown) renderListPrefix(prefix string) string {
	leading := len(prefix) - len(strings.TrimLeft(prefix, " "))
	if leading >= len(prefix) {
		return prefix
	}
	return prefix[:leading] + style(m.theme.ListBullet, prefix[leading:])
}

func (m *Markdown) applyPadding(lines []string, width int) []string {
	if m.paddingX == 0 && m.paddingY == 0 && (m.defaultTextStyle == nil || m.defaultTextStyle.BgColor == nil) {
		return lines
	}
	var bgFn func(string) string
	if m.defaultTextStyle != nil {
		bgFn = m.defaultTextStyle.BgColor
	}
	left := strings.Repeat(" ", m.paddingX)
	right := strings.Repeat(" ", m.paddingX)
	out := make([]string, 0, len(lines)+m.paddingY*2)
	empty := strings.Repeat(" ", max(0, width))
	for i := 0; i < m.paddingY; i++ {
		if bgFn != nil {
			out = append(out, ApplyBackgroundToLine(empty, width, bgFn))
		} else {
			out = append(out, empty)
		}
	}
	for _, line := range lines {
		if IsImageLine(line) {
			out = append(out, line)
			continue
		}
		padded := left + line + right
		if bgFn != nil {
			out = append(out, ApplyBackgroundToLine(padded, width, bgFn))
			continue
		}
		if m.paddingX > 0 {
			padded = TruncateToWidth(padded, width, "", true)
		}
		out = append(out, padded)
	}
	for i := 0; i < m.paddingY; i++ {
		if bgFn != nil {
			out = append(out, ApplyBackgroundToLine(empty, width, bgFn))
		} else {
			out = append(out, empty)
		}
	}
	return out
}

func (m *Markdown) applyDefaultTextStyle(text string) string {
	if m.defaultTextStyle == nil {
		return text
	}
	styled := text
	if m.defaultTextStyle.Color != nil {
		styled = m.defaultTextStyle.Color(styled)
	}
	if m.defaultTextStyle.Bold {
		styled = style(m.theme.Bold, styled)
	}
	if m.defaultTextStyle.Italic {
		styled = style(m.theme.Italic, styled)
	}
	if m.defaultTextStyle.Strikethrough {
		styled = style(m.theme.Strikethrough, styled)
	}
	if m.defaultTextStyle.Underline {
		styled = style(m.theme.Underline, styled)
	}
	return styled
}

func (m *Markdown) defaultTextStylePrefix() string {
	if m.defaultTextStyle == nil {
		return ""
	}
	if m.defaultPrefixOK {
		return m.defaultStylePrefix
	}
	const sentinel = "\x00"
	styled := m.applyDefaultTextStyle(sentinel)
	index := strings.Index(styled, sentinel)
	if index >= 0 {
		m.defaultStylePrefix = styled[:index]
	}
	m.defaultPrefixOK = true
	return m.defaultStylePrefix
}

func (m *Markdown) renderInline(text string) string {
	return m.renderInlineWithStyle(text, m.applyDefaultTextStyle, m.defaultTextStylePrefix())
}

func (m *Markdown) renderInlineWithStyle(text string, baseStyle func(string) string, basePrefix string) string {
	return m.renderInlineWithStyleOptions(text, baseStyle, basePrefix, true)
}

func (m *Markdown) renderInlineWithStyleOptions(text string, baseStyle func(string) string, basePrefix string, parseLinks bool, parseBareLinksOption ...bool) string {
	if text == "" {
		return text
	}
	parseBareLinks := true
	if len(parseBareLinksOption) > 0 {
		parseBareLinks = parseBareLinksOption[0]
	}
	var stashed []string
	stash := func(value string) string {
		token := fmt.Sprintf("\x00md%d\x00", len(stashed))
		stashed = append(stashed, value)
		return token
	}

	text = replaceMarkdownCodeSpans(text, func(code string) string {
		return stash(style(m.theme.Code, code) + basePrefix)
	})
	text = replaceMarkdownInlineHTML(text, func(raw string) string {
		return stash(raw)
	})
	text = replaceMarkdownDirectImages(text, func(label, rawDestination string) string {
		if _, ok := parseMarkdownInlineDestination(rawDestination); !ok {
			return "![" + label + "](" + rawDestination + ")"
		}
		return stash(label)
	})
	text = replaceMarkdownReferenceImages(text, func(label, reference string, _ markdownReferenceKind) (string, bool) {
		if _, ok := m.linkDefinitions[normalizeMarkdownReference(reference)]; !ok {
			return "", false
		}
		return stash(label), true
	})
	if parseLinks {
		text = replaceMarkdownDirectLinks(text, func(labelText, rawDestination string) string {
			url, ok := parseMarkdownInlineDestination(rawDestination)
			if !ok {
				return "[" + labelText + "](" + rawDestination + ")"
			}
			labelSource := unescapeMarkdownLabelClosingBrackets(labelText)
			label := m.renderInlineWithStyleOptions(labelSource, baseStyle, basePrefix, true, false)
			return stash(m.renderLinkDisplay(labelText, label, url) + basePrefix)
		})
		text = replaceMarkdownReferenceLinks(text, func(labelText, reference string, _ markdownReferenceKind) (string, bool) {
			url, ok := m.linkDefinitions[normalizeMarkdownReference(reference)]
			if !ok {
				return "", false
			}
			labelSource := unescapeMarkdownLabelClosingBrackets(labelText)
			label := m.renderInlineWithStyleOptions(labelSource, baseStyle, basePrefix, true, false)
			return stash(m.renderLinkDisplay(labelText, label, url) + basePrefix), true
		})
		text = replaceMarkdownAutoURIs(text, func(url string) string {
			return stash(m.renderLink(url, url) + basePrefix)
		})
		text = replaceMarkdownAutoEmails(text, func(email string) string {
			return stash(m.renderLink(email, "mailto:"+email) + basePrefix)
		})
		if parseBareLinks {
			text = replaceMarkdownBareURLs(text, func(display, href string) string {
				return stash(m.renderLink(display, href) + basePrefix)
			})
			text = replaceMarkdownBareEmails(text, func(display, href string) string {
				return stash(m.renderLink(display, href) + basePrefix)
			})
		}
		text = replaceMarkdownEscapes(text, func(escaped string) string {
			return stash(escaped)
		})
	} else {
		text = replaceMarkdownEscapes(text, func(escaped string) string {
			return stash(escaped)
		})
	}
	text = protectMarkedEscapedAsteriskAfterStrongOpen(text, stashed, stash)
	text = renderAsteriskFiveStrongEmphasis(text, m.theme.Bold, m.theme.Italic, basePrefix, func(content string) string {
		return m.renderInlineWithStyleOptions(content, baseStyle, basePrefix, parseLinks, parseBareLinks)
	})
	text = renderAsteriskQuadStrong(text, m.theme.Bold, basePrefix, func(content string) string {
		return m.renderInlineWithStyleOptions(content, baseStyle, basePrefix, parseLinks, parseBareLinks)
	})
	text = renderAsteriskStrongEmphasis(text, m.theme.Bold, m.theme.Italic, basePrefix, func(content string) string {
		return m.renderInlineWithStyleOptions(content, baseStyle, basePrefix, parseLinks, parseBareLinks)
	})
	text = renderAsteriskSplitStrongEmphasis(text, m.theme.Bold, m.theme.Italic, basePrefix, func(content string) string {
		return m.renderInlineWithStyleOptions(content, baseStyle, basePrefix, parseLinks, parseBareLinks)
	})
	text = renderUnderscoreStrongEmphasis(text, m.theme.Bold, m.theme.Italic, basePrefix, func(content string) string {
		return m.renderInlineWithStyleOptions(content, baseStyle, basePrefix, parseLinks, parseBareLinks)
	})
	text = renderUnderscoreSplitStrongEmphasis(text, m.theme.Bold, m.theme.Italic, basePrefix, func(content string) string {
		return m.renderInlineWithStyleOptions(content, baseStyle, basePrefix, parseLinks, parseBareLinks)
	})
	text = renderAsteriskStrong(text, m.theme.Bold, basePrefix, func(content string) string {
		return m.renderInlineWithStyleOptions(content, baseStyle, basePrefix, parseLinks, parseBareLinks)
	})
	text = renderUnderscoreEmphasis(text, "__", m.theme.Bold, basePrefix, func(content string) string {
		return m.renderInlineWithStyleOptions(content, baseStyle, basePrefix, parseLinks, parseBareLinks)
	})
	text = renderMarkdownStrikethrough(text, m.theme.Strikethrough, basePrefix, func(content string) string {
		return m.renderInlineWithStyleOptions(content, baseStyle, basePrefix, parseLinks, parseBareLinks)
	})
	text = renderAsteriskEmphasis(text, m.theme.Italic, basePrefix, func(content string) string {
		return m.renderInlineWithStyleOptions(content, baseStyle, basePrefix, parseLinks, parseBareLinks)
	})
	text = renderUnderscoreEmphasis(text, "_", m.theme.Italic, basePrefix, func(content string) string {
		return m.renderInlineWithStyleOptions(content, baseStyle, basePrefix, parseLinks, parseBareLinks)
	})
	text = restoreMarkdownStashes(text, stashed)
	return baseStyle(text)
}

func protectMarkedEscapedAsteriskAfterStrongOpen(text string, stashed []string, stash func(string) string) string {
	if text == "" || !strings.Contains(text, "**") {
		return text
	}
	var out strings.Builder
	changed := false
	for i := 0; i < len(text); {
		if strings.HasPrefix(text[i:], "**") {
			if value, end, ok := markdownStashTokenAt(text, i+2, stashed); ok && value == "*" && end < len(text) && text[end] == '*' {
				out.WriteString(stash("**"))
				i += 2
				changed = true
				continue
			}
		}
		out.WriteByte(text[i])
		i++
	}
	if !changed {
		return text
	}
	return out.String()
}

func markdownStashTokenAt(text string, pos int, stashed []string) (string, int, bool) {
	if pos >= len(text) || !strings.HasPrefix(text[pos:], "\x00md") {
		return "", 0, false
	}
	digitStart := pos + len("\x00md")
	digitEnd := digitStart
	for digitEnd < len(text) && text[digitEnd] >= '0' && text[digitEnd] <= '9' {
		digitEnd++
	}
	if digitEnd == digitStart || digitEnd >= len(text) || text[digitEnd] != '\x00' {
		return "", 0, false
	}
	index, err := strconv.Atoi(text[digitStart:digitEnd])
	if err != nil || index < 0 || index >= len(stashed) {
		return "", 0, false
	}
	return stashed[index], digitEnd + 1, true
}

func restoreMarkdownStashes(text string, stashed []string) string {
	for pass := 0; pass <= len(stashed); pass++ {
		replaced := false
		for idx, value := range stashed {
			token := fmt.Sprintf("\x00md%d\x00", idx)
			if !strings.Contains(text, token) {
				continue
			}
			text = strings.ReplaceAll(text, token, value)
			replaced = true
		}
		if !replaced {
			break
		}
	}
	return text
}

func unescapeMarkdownLabelClosingBrackets(text string) string {
	if text == "" || !strings.Contains(text, `\]`) {
		return text
	}
	var out strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] == '\\' && i+1 < len(text) && text[i+1] == ']' {
			out.WriteByte(']')
			i++
			continue
		}
		out.WriteByte(text[i])
	}
	return out.String()
}

func replaceMarkdownCodeSpans(text string, render func(string) string) string {
	if text == "" {
		return text
	}
	var out strings.Builder
	for i := 0; i < len(text); {
		if text[i] != '`' || isEscapedMarkdownByte(text, i) {
			out.WriteByte(text[i])
			i++
			continue
		}
		ticks := countMarkdownBackticks(text, i)
		contentStart := i + ticks
		close := findMarkdownClosingBackticks(text, contentStart, ticks)
		if close < 0 {
			out.WriteString(text[i : i+ticks])
			i += ticks
			continue
		}
		code := normalizeMarkdownCodeSpan(text[contentStart:close])
		out.WriteString(render(code))
		i = close + ticks
	}
	return out.String()
}

func replaceMarkdownInlineHTML(text string, render func(string) string) string {
	if text == "" {
		return text
	}
	var out strings.Builder
	for i := 0; i < len(text); {
		if text[i] != '<' || isEscapedMarkdownByte(text, i) {
			out.WriteByte(text[i])
			i++
			continue
		}
		if end, ok := findMarkdownInlineHTMLEnd(text, i); ok {
			out.WriteString(render(text[i : end+1]))
			i = end + 1
			continue
		}
		out.WriteByte(text[i])
		i++
	}
	return out.String()
}

func findMarkdownInlineHTMLEnd(text string, start int) (int, bool) {
	if start < 0 || start >= len(text) || text[start] != '<' {
		return 0, false
	}
	rest := text[start:]
	lowerRest := strings.ToLower(rest)
	switch {
	case isMarkdownHTMLCommentStart(lowerRest):
		if end := strings.Index(lowerRest, "-->"); end >= 0 {
			return start + end + len("-->") - 1, true
		}
		return 0, false
	case strings.HasPrefix(lowerRest, "<?"):
		if end := strings.Index(lowerRest, "?>"); end >= 0 {
			return start + end + len("?>") - 1, true
		}
		return 0, false
	case strings.HasPrefix(lowerRest, "<![cdata["):
		if end := strings.Index(lowerRest, "]]>"); end >= 0 {
			return start + end + len("]]>") - 1, true
		}
		return 0, false
	case strings.HasPrefix(lowerRest, "<!"):
		return findMarkdownInlineDeclarationEnd(text, start)
	}
	pos := start + 1
	closing := false
	if pos < len(text) && text[pos] == '/' {
		closing = true
		pos++
	}
	if pos >= len(text) || !isMarkdownASCIILetter(text[pos]) {
		return 0, false
	}
	for pos < len(text) {
		ch := text[pos]
		if !isMarkdownHTMLTagNameByte(ch, closing) {
			break
		}
		pos++
	}
	if pos >= len(text) {
		return 0, false
	}
	if closing {
		for pos < len(text) && isMarkdownASCIIWhitespace(text[pos]) {
			pos++
		}
		if pos < len(text) && text[pos] == '>' {
			return pos, true
		}
		return 0, false
	}
	if text[pos] != '>' && text[pos] != '/' && !isMarkdownASCIIWhitespace(text[pos]) {
		return 0, false
	}
	if end, ok := findMarkdownInlineOpenTagEnd(text, pos); ok {
		return end, true
	}
	return 0, false
}

func isMarkdownASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isMarkdownASCIIWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f'
}

func isMarkdownHTMLCommentStart(text string) bool {
	if !strings.HasPrefix(text, "<!--") {
		return false
	}
	if len(text) <= len("<!--") {
		return true
	}
	next := text[len("<!--")]
	if next == '>' {
		return false
	}
	return !(next == '-' && len(text) > len("<!--")+1 && text[len("<!--")+1] == '>')
}

func findMarkdownInlineDeclarationEnd(text string, start int) (int, bool) {
	pos := start + 2
	if pos >= len(text) || !isMarkdownASCIILetter(text[pos]) {
		return 0, false
	}
	for pos < len(text) && isMarkdownASCIILetter(text[pos]) {
		pos++
	}
	if pos >= len(text) || !isMarkdownASCIIWhitespace(text[pos]) {
		return 0, false
	}
	if end := strings.IndexByte(text[pos:], '>'); end >= 0 {
		return pos + end, true
	}
	return 0, false
}

func isMarkdownHTMLTagNameByte(b byte, closing bool) bool {
	if isMarkdownASCIILetter(b) || (b >= '0' && b <= '9') || b == '_' || b == '-' {
		return true
	}
	return closing && b == ':'
}

func findMarkdownInlineOpenTagEnd(text string, pos int) (int, bool) {
	for pos < len(text) {
		for pos < len(text) && isMarkdownASCIIWhitespace(text[pos]) {
			pos++
		}
		if pos >= len(text) {
			return 0, false
		}
		if text[pos] == '>' {
			return pos, true
		}
		if text[pos] == '/' {
			pos++
			for pos < len(text) && isMarkdownASCIIWhitespace(text[pos]) {
				pos++
			}
			if pos < len(text) && text[pos] == '>' {
				return pos, true
			}
			return 0, false
		}
		next, ok := parseMarkdownInlineHTMLAttribute(text, pos)
		if !ok {
			return 0, false
		}
		pos = next
	}
	return 0, false
}

func parseMarkdownInlineHTMLAttribute(text string, pos int) (int, bool) {
	if pos >= len(text) || !isMarkdownHTMLAttributeNameStart(text[pos]) {
		return 0, false
	}
	pos++
	for pos < len(text) && isMarkdownHTMLAttributeNameByte(text[pos]) {
		pos++
	}
	for pos < len(text) && isMarkdownASCIIWhitespace(text[pos]) {
		pos++
	}
	if pos >= len(text) || text[pos] != '=' {
		return pos, true
	}
	pos++
	for pos < len(text) && isMarkdownASCIIWhitespace(text[pos]) {
		pos++
	}
	if pos >= len(text) {
		return 0, false
	}
	switch text[pos] {
	case '"', '\'':
		quote := text[pos]
		pos++
		for pos < len(text) {
			if text[pos] == quote {
				return pos + 1, true
			}
			pos++
		}
		return 0, false
	default:
		start := pos
		for pos < len(text) {
			ch := text[pos]
			if isMarkdownASCIIWhitespace(ch) || ch == '"' || ch == '\'' || ch == '=' || ch == '<' || ch == '>' || ch == '`' {
				break
			}
			pos++
		}
		return pos, pos > start
	}
}

func isMarkdownHTMLAttributeNameStart(b byte) bool {
	return isMarkdownASCIILetter(b) || b == ':' || b == '_'
}

func isMarkdownHTMLAttributeNameByte(b byte) bool {
	return isMarkdownASCIILetter(b) || (b >= '0' && b <= '9') || b == '_' || b == '.' || b == ':' || b == '-'
}

func isMarkdownASCIIAlphaNumeric(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func replaceMarkdownEscapes(text string, render func(string) string) string {
	if text == "" {
		return text
	}
	var out strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] != '\\' || i+1 >= len(text) || !isMarkdownEscapablePunctuation(text[i+1]) {
			out.WriteByte(text[i])
			continue
		}
		out.WriteString(render(text[i+1 : i+2]))
		i++
	}
	return out.String()
}

func isMarkdownEscapablePunctuation(b byte) bool {
	return (b >= '!' && b <= '/') ||
		(b >= ':' && b <= '@') ||
		(b >= '[' && b <= '`') ||
		(b >= '{' && b <= '~')
}

func renderAsteriskStrongEmphasis(text string, boldFn, italicFn func(string) string, basePrefix string, renderContent func(string) string) string {
	return renderMarkdownStrongEmphasis(text, "***", boldFn, italicFn, basePrefix, renderContent, isValidMarkdownAsteriskTripleOpen, isValidMarkdownAsteriskTripleClose)
}

func renderAsteriskFiveStrongEmphasis(text string, boldFn, italicFn func(string) string, basePrefix string, renderContent func(string) string) string {
	return renderMarkdownRepeatedAsteriskRun(text, "*****", basePrefix, renderContent, func(content string) string {
		return style(italicFn, style(boldFn, style(boldFn, content)))
	})
}

func renderAsteriskQuadStrong(text string, boldFn func(string) string, basePrefix string, renderContent func(string) string) string {
	return renderMarkdownRepeatedAsteriskRun(text, "****", basePrefix, renderContent, func(content string) string {
		return style(boldFn, style(boldFn, content))
	})
}

func renderUnderscoreStrongEmphasis(text string, boldFn, italicFn func(string) string, basePrefix string, renderContent func(string) string) string {
	return renderMarkdownStrongEmphasis(text, "___", boldFn, italicFn, basePrefix, renderContent, isValidMarkdownUnderscoreTripleOpen, isValidMarkdownUnderscoreTripleClose)
}

func renderMarkdownRepeatedAsteriskRun(text, delimiter, basePrefix string, renderContent func(string) string, renderStyled func(string) string) string {
	if text == "" {
		return text
	}
	var out strings.Builder
	for idx := 0; idx < len(text); {
		open := findMarkdownRepeatedAsteriskOpen(text, delimiter, idx)
		if open < 0 {
			out.WriteString(text[idx:])
			break
		}
		close, ok := findMarkdownRepeatedAsteriskClose(text, delimiter, open+len(delimiter))
		if !ok {
			out.WriteString(text[idx : open+len(delimiter)])
			idx = open + len(delimiter)
			continue
		}
		content := text[open+len(delimiter) : close]
		if renderContent != nil {
			content = renderContent(content)
		}
		out.WriteString(text[idx:open])
		out.WriteString(renderStyled(content))
		out.WriteString(basePrefix)
		idx = close + len(delimiter)
	}
	return out.String()
}

func findMarkdownRepeatedAsteriskOpen(text, delimiter string, start int) int {
	for search := start; search < len(text); {
		rel := strings.Index(text[search:], delimiter)
		if rel < 0 {
			return -1
		}
		open := search + rel
		if !isEscapedMarkdownByte(text, open) && markdownAsteriskRunLengthAt(text, open) == len(delimiter) && markdownDelimiterCanOpen(text, open, len(delimiter), '*') {
			return open
		}
		search = open + 1
	}
	return -1
}

func findMarkdownRepeatedAsteriskClose(text, delimiter string, start int) (int, bool) {
	for search := start; search < len(text); {
		rel := strings.Index(text[search:], delimiter)
		if rel < 0 {
			return 0, false
		}
		close := search + rel
		content := text[start:close]
		if !isEscapedMarkdownByte(text, close) && isStrictMarkdownAsteriskStrongText(content) && markdownAsteriskRunLengthAt(text, close) == len(delimiter) && markdownDelimiterCanClose(text, close, len(delimiter), '*') {
			return close, true
		}
		search = close + 1
	}
	return 0, false
}

func markdownAsteriskRunLengthAt(text string, pos int) int {
	count := 0
	for pos+count < len(text) && text[pos+count] == '*' {
		count++
	}
	return count
}

func renderUnderscoreSplitStrongEmphasis(text string, boldFn, italicFn func(string) string, basePrefix string, renderContent func(string) string) string {
	text = renderUnderscoreTripleOpenSplit(text, boldFn, italicFn, basePrefix, renderContent)
	text = renderUnderscoreOuterEmphasisInnerStrong(text, boldFn, italicFn, basePrefix, renderContent)
	text = renderUnderscoreOuterStrongInnerEmphasis(text, boldFn, italicFn, basePrefix, renderContent)
	return text
}

func renderUnderscoreTripleOpenSplit(text string, boldFn, italicFn func(string) string, basePrefix string, renderContent func(string) string) string {
	if text == "" {
		return text
	}
	var out strings.Builder
	for idx := 0; idx < len(text); {
		open := findMarkdownDelimiterOpen(text, "___", idx, isValidMarkdownUnderscoreTripleOpen)
		if open < 0 {
			out.WriteString(text[idx:])
			break
		}
		afterOpen := open + 3
		if strongClose, ok := findMarkdownUnderscoreClose(text, afterOpen, "__"); ok {
			if emClose, ok := findMarkdownUnderscoreCloseAfter(text, strongClose+2, "_"); ok {
				strongContent := text[afterOpen:strongClose]
				emRemainder := text[strongClose+2 : emClose]
				if renderContent != nil {
					strongContent = renderContent(strongContent)
					emRemainder = renderContent(emRemainder)
				}
				out.WriteString(text[idx:open])
				out.WriteString(style(italicFn, style(boldFn, strongContent)+basePrefix+emRemainder))
				out.WriteString(basePrefix)
				idx = emClose + 1
				continue
			}
		}
		if emClose, ok := findMarkdownUnderscoreClose(text, afterOpen, "_"); ok {
			if strongClose, ok := findMarkdownUnderscoreCloseAfter(text, emClose+1, "__"); ok {
				emContent := text[afterOpen:emClose]
				strongRemainder := text[emClose+1 : strongClose]
				if renderContent != nil {
					emContent = renderContent(emContent)
					strongRemainder = renderContent(strongRemainder)
				}
				out.WriteString(text[idx:open])
				out.WriteString(style(boldFn, style(italicFn, emContent)+basePrefix+strongRemainder))
				out.WriteString(basePrefix)
				idx = strongClose + 2
				continue
			}
		}
		out.WriteString(text[idx : open+3])
		idx = open + 3
	}
	return out.String()
}

func renderUnderscoreOuterEmphasisInnerStrong(text string, boldFn, italicFn func(string) string, basePrefix string, renderContent func(string) string) string {
	return renderUnderscoreOuterInnerTripleClose(text, "_", "__", italicFn, boldFn, basePrefix, renderContent)
}

func renderUnderscoreOuterStrongInnerEmphasis(text string, boldFn, italicFn func(string) string, basePrefix string, renderContent func(string) string) string {
	return renderUnderscoreOuterInnerTripleClose(text, "__", "_", boldFn, italicFn, basePrefix, renderContent)
}

func renderUnderscoreOuterInnerTripleClose(text, outerDelimiter, innerDelimiter string, outerFn, innerFn func(string) string, basePrefix string, renderContent func(string) string) string {
	if text == "" {
		return text
	}
	var out strings.Builder
	for idx := 0; idx < len(text); {
		openRel := strings.Index(text[idx:], outerDelimiter)
		if openRel < 0 {
			out.WriteString(text[idx:])
			break
		}
		open := idx + openRel
		if !isValidMarkdownUnderscoreOpen(text, open, outerDelimiter) {
			out.WriteString(text[idx : open+len(outerDelimiter)])
			idx = open + len(outerDelimiter)
			continue
		}
		innerOpen, ok := findMarkdownUnderscoreOpenAfter(text, open+len(outerDelimiter), innerDelimiter)
		if !ok {
			out.WriteString(text[idx : open+len(outerDelimiter)])
			idx = open + len(outerDelimiter)
			continue
		}
		close := findMarkdownDelimiterOpen(text, "___", innerOpen+len(innerDelimiter), isValidMarkdownUnderscoreTripleClose)
		if close < 0 {
			out.WriteString(text[idx : open+len(outerDelimiter)])
			idx = open + len(outerDelimiter)
			continue
		}
		outerPrefix := text[open+len(outerDelimiter) : innerOpen]
		innerContent := text[innerOpen+len(innerDelimiter) : close]
		if strings.TrimSpace(outerPrefix) == "" && strings.TrimSpace(innerContent) == "" {
			out.WriteString(text[idx : open+len(outerDelimiter)])
			idx = open + len(outerDelimiter)
			continue
		}
		if renderContent != nil {
			outerPrefix = renderContent(outerPrefix)
			innerContent = renderContent(innerContent)
		}
		out.WriteString(text[idx:open])
		out.WriteString(style(outerFn, outerPrefix+style(innerFn, innerContent)+basePrefix))
		out.WriteString(basePrefix)
		idx = close + 3
	}
	return out.String()
}

func findMarkdownUnderscoreOpenAfter(text string, start int, delimiter string) (int, bool) {
	for search := start; search < len(text); {
		rel := strings.Index(text[search:], delimiter)
		if rel < 0 {
			return 0, false
		}
		open := search + rel
		if isValidMarkdownUnderscoreOpen(text, open, delimiter) {
			return open, true
		}
		search = open + 1
	}
	return 0, false
}

func findMarkdownUnderscoreCloseAfter(text string, start int, delimiter string) (int, bool) {
	for search := start; search < len(text); {
		rel := strings.Index(text[search:], delimiter)
		if rel < 0 {
			return 0, false
		}
		close := search + rel
		if markdownDelimiterCanClose(text, close, len(delimiter), '_') {
			return close, true
		}
		search = close + 1
	}
	return 0, false
}

func renderAsteriskSplitStrongEmphasis(text string, boldFn, italicFn func(string) string, basePrefix string, renderContent func(string) string) string {
	if text == "" {
		return text
	}
	var out strings.Builder
	for idx := 0; idx < len(text); {
		open := findMarkdownDelimiterOpen(text, "***", idx, isValidMarkdownAsteriskTripleOpen)
		if open < 0 {
			out.WriteString(text[idx:])
			break
		}
		afterOpen := open + 3
		if strongClose, ok := findMarkdownAsteriskStrongClose(text, afterOpen, open); ok {
			if emClose, ok := findMarkdownAsteriskEmphasisCloseAfter(text, strongClose+2); ok {
				strongContent := text[afterOpen:strongClose]
				emRemainder := text[strongClose+2 : emClose]
				if renderContent != nil {
					strongContent = renderContent(strongContent)
					emRemainder = renderContent(emRemainder)
				}
				out.WriteString(text[idx:open])
				out.WriteString(style(italicFn, style(boldFn, strongContent)+basePrefix+emRemainder))
				out.WriteString(basePrefix)
				idx = emClose + 1
				continue
			}
		}
		if emClose, ok := findMarkdownAsteriskEmphasisClose(text, afterOpen); ok {
			if strongClose, ok := findMarkdownAsteriskStrongCloseAfter(text, emClose+1); ok {
				emContent := text[afterOpen:emClose]
				strongRemainder := text[emClose+1 : strongClose]
				if renderContent != nil {
					emContent = renderContent(emContent)
					strongRemainder = renderContent(strongRemainder)
				}
				out.WriteString(text[idx:open])
				out.WriteString(style(boldFn, style(italicFn, emContent)+basePrefix+strongRemainder))
				out.WriteString(basePrefix)
				idx = strongClose + 2
				continue
			}
		}
		out.WriteString(text[idx : open+3])
		idx = open + 3
	}
	return out.String()
}

func findMarkdownAsteriskEmphasisCloseAfter(text string, start int) (int, bool) {
	for search := start; search < len(text); {
		rel := strings.IndexByte(text[search:], '*')
		if rel < 0 {
			return 0, false
		}
		close := search + rel
		if !isAdjacentAsterisk(text, close) && isValidMarkdownAsteriskEmphasisClose(text, close) {
			return close, true
		}
		search = close + 1
	}
	return 0, false
}

func findMarkdownAsteriskStrongCloseAfter(text string, start int) (int, bool) {
	for search := start; search < len(text); {
		rel := strings.Index(text[search:], "**")
		if rel < 0 {
			return 0, false
		}
		close := search + rel
		if isValidMarkdownAsteriskStrongClose(text, close, false) {
			return close, true
		}
		search = close + 1
	}
	return 0, false
}

func renderMarkdownStrongEmphasis(text, delimiter string, boldFn, italicFn func(string) string, basePrefix string, renderContent func(string) string, validOpen func(string, int) bool, validClose func(string, int) bool) string {
	if text == "" {
		return text
	}
	var out strings.Builder
	for idx := 0; idx < len(text); {
		open := findMarkdownDelimiterOpen(text, delimiter, idx, validOpen)
		if open < 0 {
			out.WriteString(text[idx:])
			break
		}
		close, ok := findMarkdownDelimiterClose(text, delimiter, open+len(delimiter), validClose)
		if !ok {
			out.WriteString(text[idx : open+len(delimiter)])
			idx = open + len(delimiter)
			continue
		}
		content := text[open+len(delimiter) : close]
		if renderContent != nil {
			content = renderContent(content)
		}
		out.WriteString(text[idx:open])
		out.WriteString(style(italicFn, style(boldFn, content)))
		out.WriteString(basePrefix)
		idx = close + len(delimiter)
	}
	return out.String()
}

func findMarkdownDelimiterOpen(text, delimiter string, start int, validOpen func(string, int) bool) int {
	for search := start; search < len(text); {
		rel := strings.Index(text[search:], delimiter)
		if rel < 0 {
			return -1
		}
		open := search + rel
		if validOpen(text, open) {
			return open
		}
		search = open + 1
	}
	return -1
}

func findMarkdownDelimiterClose(text, delimiter string, start int, validClose func(string, int) bool) (int, bool) {
	for search := start; search < len(text); {
		rel := strings.Index(text[search:], delimiter)
		if rel < 0 {
			return 0, false
		}
		close := search + rel
		content := text[start:close]
		if isStrictMarkdownStrongEmphasisText(content, delimiter[0]) && validClose(text, close) {
			return close, true
		}
		search = close + 1
	}
	return 0, false
}

func isValidMarkdownAsteriskTripleOpen(text string, open int) bool {
	if isEscapedMarkdownByte(text, open) {
		return false
	}
	if prev, ok := previousRune(text, open); ok && prev == '*' {
		return false
	}
	if next, ok := nextRune(text, open+3); !ok || next == '*' {
		return false
	}
	return markdownDelimiterCanOpen(text, open, 3, '*')
}

func isValidMarkdownAsteriskTripleClose(text string, close int) bool {
	if isEscapedMarkdownByte(text, close) {
		return false
	}
	if next, ok := nextRune(text, close+3); ok && next == '*' {
		return false
	}
	if prev, ok := previousRune(text, close); !ok || prev == '*' {
		return false
	}
	return markdownDelimiterCanClose(text, close, 3, '*')
}

func isValidMarkdownUnderscoreTripleOpen(text string, open int) bool {
	if isEscapedMarkdownByte(text, open) {
		return false
	}
	if prev, ok := previousRune(text, open); ok && prev == '_' {
		return false
	}
	if next, ok := nextRune(text, open+3); !ok || next == '_' {
		return false
	}
	return markdownDelimiterCanOpen(text, open, 3, '_')
}

func isValidMarkdownUnderscoreTripleClose(text string, close int) bool {
	if isEscapedMarkdownByte(text, close) {
		return false
	}
	if prev, ok := previousRune(text, close); !ok || prev == '_' {
		return false
	}
	if next, ok := nextRune(text, close+3); ok && next == '_' {
		return false
	}
	return markdownDelimiterCanClose(text, close, 3, '_')
}

func isStrictMarkdownStrongEmphasisText(text string, delimiter byte) bool {
	if text == "" {
		return false
	}
	first, _ := utf8.DecodeRuneInString(text)
	last, _ := utf8.DecodeLastRuneInString(text)
	return !unicode.IsSpace(first) && !unicode.IsSpace(last) && first != rune(delimiter) && last != rune(delimiter)
}

func renderAsteriskStrong(text string, styleFn func(string) string, basePrefix string, renderContent func(string) string) string {
	if text == "" {
		return text
	}
	var out strings.Builder
	for idx := 0; idx < len(text); {
		open := findMarkdownAsteriskStrongOpen(text, idx)
		if open < 0 {
			out.WriteString(text[idx:])
			break
		}
		close, ok := findMarkdownAsteriskStrongClose(text, open+2, open)
		if !ok {
			out.WriteString(text[idx : open+2])
			idx = open + 2
			continue
		}
		content := text[open+2 : close]
		if renderContent != nil {
			content = renderContent(content)
		}
		out.WriteString(text[idx:open])
		out.WriteString(style(styleFn, content))
		out.WriteString(basePrefix)
		idx = close + 2
	}
	return out.String()
}

func renderAsteriskEmphasis(text string, styleFn func(string) string, basePrefix string, renderContent func(string) string) string {
	if text == "" {
		return text
	}
	var out strings.Builder
	for idx := 0; idx < len(text); {
		open := findMarkdownAsteriskEmphasisOpen(text, idx)
		if open < 0 {
			out.WriteString(text[idx:])
			break
		}
		close, ok := findMarkdownAsteriskEmphasisClose(text, open+1)
		if !ok {
			out.WriteString(text[idx : open+1])
			idx = open + 1
			continue
		}
		content := text[open+1 : close]
		if renderContent != nil {
			content = renderContent(content)
		}
		out.WriteString(text[idx:open])
		out.WriteString(style(styleFn, content))
		out.WriteString(basePrefix)
		idx = close + 1
	}
	return out.String()
}

func renderMarkdownStrikethrough(text string, styleFn func(string) string, basePrefix string, renderContent func(string) string) string {
	if text == "" {
		return text
	}
	var out strings.Builder
	for idx := 0; idx < len(text); {
		open := findMarkdownStrikeOpen(text, idx)
		if open < 0 {
			out.WriteString(text[idx:])
			break
		}
		close, ok := findMarkdownStrikeClose(text, open+2)
		if !ok {
			out.WriteString(text[idx : open+2])
			idx = open + 2
			continue
		}
		content := text[open+2 : close]
		if renderContent != nil {
			content = renderContent(content)
		}
		out.WriteString(text[idx:open])
		out.WriteString(style(styleFn, content))
		out.WriteString(basePrefix)
		idx = close + 2
	}
	return out.String()
}

func findMarkdownAsteriskStrongOpen(text string, start int) int {
	for search := start; search < len(text); {
		rel := strings.Index(text[search:], "**")
		if rel < 0 {
			return -1
		}
		open := search + rel
		if isValidMarkdownAsteriskStrongOpen(text, open) {
			return open
		}
		search = open + 1
	}
	return -1
}

func findMarkdownAsteriskStrongClose(text string, start int, open int) (int, bool) {
	for search := start; search < len(text); {
		rel := strings.Index(text[search:], "**")
		if rel < 0 {
			return 0, false
		}
		close := search + rel
		content := text[start:close]
		allowTrailing := hasUnclosedAsteriskEmphasisBefore(text, open)
		if !allowTrailing && close+2 < len(text) && text[close+2] == '*' && !hasUnclosedAsteriskEmphasisBefore(content, len(content)) {
			allowTrailing = true
		}
		if isStrictMarkdownAsteriskStrongText(content) && isValidMarkdownAsteriskStrongClose(text, close, allowTrailing) {
			return close, true
		}
		search = close + 1
	}
	return 0, false
}

func isValidMarkdownAsteriskStrongOpen(text string, open int) bool {
	if isEscapedMarkdownByte(text, open) {
		return false
	}
	if prev, ok := previousRune(text, open); ok && prev == '*' {
		if markdownPreviousAsteriskRunLength(text, open) != 1 {
			return false
		}
	}
	if next, ok := nextRune(text, open+2); !ok || next == '*' {
		return false
	}
	return markdownDelimiterCanOpen(text, open, 2, '*')
}

func isValidMarkdownAsteriskStrongClose(text string, close int, allowTrailingAsterisk bool) bool {
	if isEscapedMarkdownByte(text, close) {
		return false
	}
	if next, ok := nextRune(text, close+2); ok && next == '*' && !allowTrailingAsterisk {
		return false
	}
	return markdownDelimiterCanClose(text, close, 2, '*')
}

func hasUnclosedAsteriskEmphasisBefore(text string, limit int) bool {
	start := 0
	if limit > 0 {
		start = strings.LastIndexByte(text[:limit], '\n') + 1
	}
	open := false
	for i := start; i < limit; i++ {
		if text[i] != '*' || isEscapedMarkdownByte(text, i) || isAdjacentAsterisk(text, i) {
			continue
		}
		if !open && isValidMarkdownAsteriskEmphasisOpen(text, i) {
			open = true
			continue
		}
		if open && isValidMarkdownAsteriskEmphasisClose(text, i) {
			open = false
		}
	}
	return open
}

func findMarkdownAsteriskEmphasisOpen(text string, start int) int {
	for search := start; search < len(text); {
		rel := strings.IndexByte(text[search:], '*')
		if rel < 0 {
			return -1
		}
		open := search + rel
		if isValidMarkdownAsteriskEmphasisOpen(text, open) {
			return open
		}
		search = open + 1
	}
	return -1
}

func findMarkdownAsteriskEmphasisClose(text string, start int) (int, bool) {
	for search := start; search < len(text); {
		rel := strings.IndexByte(text[search:], '*')
		if rel < 0 {
			return 0, false
		}
		close := search + rel
		content := text[start:close]
		if isStrictMarkdownAsteriskEmphasisText(content) && isValidMarkdownAsteriskEmphasisClose(text, close) {
			return close, true
		}
		search = close + 1
	}
	return 0, false
}

func findMarkdownStrikeOpen(text string, start int) int {
	for search := start; search < len(text); {
		rel := strings.Index(text[search:], "~~")
		if rel < 0 {
			return -1
		}
		open := search + rel
		if !isEscapedMarkdownByte(text, open) && isValidMarkdownStrikeOpen(text, open) {
			return open
		}
		search = open + 1
	}
	return -1
}

func findMarkdownStrikeClose(text string, start int) (int, bool) {
	for search := start; search < len(text); {
		rel := strings.Index(text[search:], "~~")
		if rel < 0 {
			return 0, false
		}
		close := search + rel
		content := text[start:close]
		if !isEscapedMarkdownByte(text, close) && isStrictMarkdownStrikeText(content) && isValidMarkdownStrikeClose(text, close) {
			return close, true
		}
		search = close + 1
	}
	return 0, false
}

func isAdjacentAsterisk(text string, pos int) bool {
	return (pos > 0 && text[pos-1] == '*') || (pos+1 < len(text) && text[pos+1] == '*')
}

func isValidMarkdownAsteriskEmphasisOpen(text string, open int) bool {
	if isEscapedMarkdownByte(text, open) {
		return false
	}
	if previousRun := markdownPreviousAsteriskRunLength(text, open); previousRun > 0 {
		runStart := open - previousRun
		if !markdownDelimiterCanOpen(text, runStart, previousRun+1, '*') {
			return false
		}
	}
	if next, ok := nextRune(text, open+1); !ok || next == '*' {
		return false
	}
	return markdownDelimiterCanOpen(text, open, 1, '*')
}

func markdownPreviousAsteriskRunLength(text string, pos int) int {
	count := 0
	for i := pos - 1; i >= 0 && text[i] == '*'; i-- {
		count++
	}
	return count
}

func isValidMarkdownAsteriskEmphasisClose(text string, close int) bool {
	if isEscapedMarkdownByte(text, close) {
		return false
	}
	if prev, ok := previousRune(text, close); !ok || prev == '*' {
		return false
	}
	return markdownDelimiterCanClose(text, close, 1, '*')
}

func isValidMarkdownStrikeOpen(text string, open int) bool {
	if prev, ok := previousRune(text, open); ok && prev == '~' {
		return false
	}
	next, ok := nextRune(text, open+2)
	return ok && !unicode.IsSpace(next) && next != '~'
}

func isValidMarkdownStrikeClose(text string, close int) bool {
	if next, ok := nextRune(text, close+2); ok && next == '~' {
		return false
	}
	prev, ok := previousRune(text, close)
	if !ok {
		return false
	}
	if start, ok := previousRuneStart(text, close); ok && isEscapedMarkdownByte(text, start) {
		return true
	}
	return !unicode.IsSpace(prev) && prev != '~'
}

func splitMarkdownBareURLTrailingPunctuation(match string) (url, suffix string) {
	url = match
	for url != "" {
		if entity, ok := splitMarkdownBareURLTrailingEntity(url); ok {
			suffix = entity + suffix
			url = url[:len(url)-len(entity)]
			continue
		}
		r, size := utf8.DecodeLastRuneInString(url)
		if isMarkdownBareURLTrailingPunctuation(r) || isUnmatchedMarkdownBareURLClosingParen(url) {
			suffix = string(r) + suffix
			url = url[:len(url)-size]
			continue
		}
		break
	}
	return url, suffix
}

func splitMarkdownBareURLTrailingEntity(url string) (string, bool) {
	if !strings.HasSuffix(url, ";") {
		return "", false
	}
	amp := strings.LastIndexByte(url, '&')
	if amp < 0 || amp == len(url)-1 {
		return "", false
	}
	for _, r := range url[amp+1 : len(url)-1] {
		if !isMarkdownASCIIAlphaNumeric(r) {
			return "", false
		}
	}
	return url[amp:], true
}

func replaceMarkdownAutoURIs(text string, render func(url string) string) string {
	return replaceMarkdownAutoAngleLinks(text, markdownAutoURIPattern, render)
}

func replaceMarkdownAutoEmails(text string, render func(email string) string) string {
	return replaceMarkdownAutoAngleLinks(text, markdownAutoEmailPattern, render)
}

func replaceMarkdownAutoAngleLinks(text string, pattern *regexp.Regexp, render func(value string) string) string {
	matches := pattern.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return text
	}
	var out strings.Builder
	last := 0
	for _, match := range matches {
		if len(match) < 4 || match[0] < last {
			continue
		}
		start, end := match[0], match[1]
		if isEscapedMarkdownByte(text, start) {
			continue
		}
		out.WriteString(text[last:start])
		out.WriteString(render(text[match[2]:match[3]]))
		last = end
	}
	out.WriteString(text[last:])
	return out.String()
}

func replaceMarkdownBareURLs(text string, render func(display, href string) string) string {
	matches := markdownBareURLPattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text
	}
	var out strings.Builder
	last := 0
	for _, match := range matches {
		if len(match) < 2 || match[0] < last {
			continue
		}
		start, end := match[0], match[1]
		if isMarkdownBareURLInsideEmailDomain(text, start, end) {
			continue
		}
		out.WriteString(text[last:start])
		rawURL := text[start:end]
		url, suffix := splitMarkdownBareURLTrailingPunctuation(rawURL)
		if url == "" {
			out.WriteString(text[start:end])
			last = end
			continue
		}
		display := url
		href := display
		if strings.HasPrefix(strings.ToLower(display), "www.") {
			href = "http://" + display
		}
		out.WriteString(render(display, href))
		out.WriteString(suffix)
		last = end
	}
	out.WriteString(text[last:])
	return out.String()
}

func isMarkdownBareURLInsideEmailDomain(text string, start, end int) bool {
	if start <= 0 || start >= end || text[start-1] != '@' {
		return false
	}
	if !strings.HasPrefix(strings.ToLower(text[start:end]), "www.") {
		return false
	}
	localStart := start - 1
	for localStart > 0 && isMarkdownBareEmailLocalByte(text[localStart-1]) {
		localStart--
	}
	return localStart < start-1
}

func replaceMarkdownBareEmails(text string, render func(display, href string) string) string {
	matches := markdownBareEmailPattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text
	}
	var out strings.Builder
	last := 0
	for _, match := range matches {
		if len(match) < 2 || match[0] < last {
			continue
		}
		start, end := match[0], match[1]
		out.WriteString(text[last:start])
		rawEmail := text[start:end]
		email := rawEmail
		if end < len(text) && (text[end] == '-' || text[end] == '_') {
			if trimmed, ok := trimMarkdownBareEmailBeforeDashUnderscore(rawEmail); ok {
				email = trimmed
			} else {
				out.WriteString(rawEmail)
				last = end
				continue
			}
		}
		out.WriteString(render(email, "mailto:"+email))
		out.WriteString(rawEmail[len(email):])
		last = end
	}
	out.WriteString(text[last:])
	return out.String()
}

func trimMarkdownBareEmailBeforeDashUnderscore(email string) (string, bool) {
	if email == "" {
		return "", false
	}
	_, size := utf8.DecodeLastRuneInString(email)
	if size <= 0 || size >= len(email) {
		return "", false
	}
	trimmed := email[:len(email)-size]
	if markdownBareEmailFullPattern.MatchString(trimmed) {
		return trimmed, true
	}
	return "", false
}

func isMarkdownBareEmailLocalByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '.' || b == '_' || b == '+' || b == '-'
}

func isUnmatchedMarkdownBareURLClosingParen(url string) bool {
	if !strings.HasSuffix(url, ")") {
		return false
	}
	balance := 0
	for _, r := range url {
		switch r {
		case '(':
			balance++
		case ')':
			balance--
		}
	}
	return balance < 0
}

func isMarkdownBareURLTrailingPunctuation(r rune) bool {
	switch r {
	case '.', ',', ':', ';', '!', '?', '\'', '"', '*', '_', '~':
		return true
	default:
		return false
	}
}

func isStrictMarkdownAsteriskStrongText(text string) bool {
	if text == "" {
		return false
	}
	first, _ := utf8.DecodeRuneInString(text)
	last, _ := utf8.DecodeLastRuneInString(text)
	return !unicode.IsSpace(first) && !unicode.IsSpace(last)
}

func isStrictMarkdownAsteriskEmphasisText(text string) bool {
	if text == "" {
		return false
	}
	first, _ := utf8.DecodeRuneInString(text)
	last, _ := utf8.DecodeLastRuneInString(text)
	return !unicode.IsSpace(first) && !unicode.IsSpace(last) && first != '*' && last != '*'
}

func replaceMarkdownDirectImages(text string, render func(label, rawDestination string) string) string {
	return replaceMarkdownDirectBracketDestinations(text, "![", render)
}

func replaceMarkdownDirectLinks(text string, render func(label, rawDestination string) string) string {
	return replaceMarkdownDirectBracketDestinations(text, "[", render)
}

type markdownReferenceKind int

const (
	markdownReferenceExplicit markdownReferenceKind = iota
	markdownReferenceCollapsed
	markdownReferenceShortcut
)

func replaceMarkdownReferenceImages(text string, render func(label, reference string, kind markdownReferenceKind) (string, bool)) string {
	return replaceMarkdownReferenceBracketLabels(text, "![", render)
}

func replaceMarkdownReferenceLinks(text string, render func(label, reference string, kind markdownReferenceKind) (string, bool)) string {
	return replaceMarkdownReferenceBracketLabels(text, "[", render)
}

func replaceMarkdownReferenceBracketLabels(text, opener string, render func(label, reference string, kind markdownReferenceKind) (string, bool)) string {
	if text == "" {
		return text
	}
	var out strings.Builder
	for i := 0; i < len(text); {
		if !strings.HasPrefix(text[i:], opener) || isEscapedMarkdownByte(text, i) {
			out.WriteByte(text[i])
			i++
			continue
		}
		labelStart := i + len(opener)
		labelEnd := findMarkdownInlineLabelEnd(text, labelStart)
		if labelEnd < 0 {
			out.WriteString(opener)
			i += len(opener)
			continue
		}
		label := text[labelStart:labelEnd]
		reference := label
		kind := markdownReferenceShortcut
		consumed := labelEnd + 1
		if labelEnd+1 < len(text) && text[labelEnd+1] == '[' {
			if labelEnd+2 < len(text) && text[labelEnd+2] == ']' {
				kind = markdownReferenceCollapsed
				consumed = labelEnd + 3
			} else {
				refStart := labelEnd + 2
				refEnd := findMarkdownInlineLabelEnd(text, refStart)
				if refEnd < 0 {
					out.WriteString(opener)
					i += len(opener)
					continue
				}
				reference = text[refStart:refEnd]
				kind = markdownReferenceExplicit
				consumed = refEnd + 1
			}
		} else if labelEnd+1 < len(text) && text[labelEnd+1] == '(' {
			out.WriteString(opener)
			i += len(opener)
			continue
		}
		if replacement, ok := render(label, reference, kind); ok {
			out.WriteString(replacement)
		} else {
			if markdownLabelHasUnescapedBracket(label) {
				out.WriteString(opener)
				i += len(opener)
				continue
			}
			out.WriteString(text[i:consumed])
		}
		i = consumed
	}
	return out.String()
}

func markdownLabelHasUnescapedBracket(label string) bool {
	for i := 0; i < len(label); i++ {
		if isEscapedMarkdownByte(label, i) {
			continue
		}
		if label[i] == '[' || label[i] == ']' {
			return true
		}
	}
	return false
}

func replaceMarkdownDirectBracketDestinations(text, opener string, render func(label, rawDestination string) string) string {
	if text == "" {
		return text
	}
	var out strings.Builder
	for i := 0; i < len(text); {
		if !strings.HasPrefix(text[i:], opener) || isEscapedMarkdownByte(text, i) {
			out.WriteByte(text[i])
			i++
			continue
		}
		labelStart := i + len(opener)
		labelEnd := findMarkdownInlineLabelEnd(text, labelStart)
		if labelEnd < 0 || labelEnd+1 >= len(text) || text[labelEnd+1] != '(' {
			out.WriteString(opener)
			i += len(opener)
			continue
		}
		destStart := labelEnd + 2
		destEnd := findMarkdownInlineDestinationEnd(text, destStart)
		if destEnd < 0 {
			out.WriteString(opener)
			i += len(opener)
			continue
		}
		rawDestination := text[destStart:destEnd]
		if _, ok := parseMarkdownInlineDestination(rawDestination); !ok {
			out.WriteString(opener)
			i += len(opener)
			continue
		}
		out.WriteString(render(text[labelStart:labelEnd], rawDestination))
		i = destEnd + 1
	}
	return out.String()
}

func findUnescapedMarkdownByte(text string, start int, target byte) int {
	for i := start; i < len(text); i++ {
		if text[i] == target && !isEscapedMarkdownByte(text, i) {
			return i
		}
	}
	return -1
}

func findMarkdownInlineLabelEnd(text string, start int) int {
	depth := 0
	for i := start; i < len(text); i++ {
		if isEscapedMarkdownByte(text, i) {
			continue
		}
		switch text[i] {
		case '[':
			depth++
		case ']':
			if depth == 0 {
				return i
			}
			depth--
		}
	}
	return -1
}

func findMarkdownInlineDestinationEnd(text string, start int) int {
	depth := 0
	inAngle := false
	titleQuote := byte(0)
	seenWhitespace := false
	for i := start; i < len(text); i++ {
		if isEscapedMarkdownByte(text, i) {
			continue
		}
		if titleQuote != 0 {
			if text[i] == titleQuote {
				titleQuote = 0
			}
			continue
		}
		if depth == 0 && !inAngle && unicode.IsSpace(rune(text[i])) {
			seenWhitespace = true
			continue
		}
		if seenWhitespace && depth == 0 && !inAngle && (text[i] == '"' || text[i] == '\'') {
			titleQuote = text[i]
			continue
		}
		switch text[i] {
		case '<':
			if depth == 0 {
				inAngle = true
			}
		case '>':
			if inAngle {
				inAngle = false
			}
		case '(':
			if !inAngle {
				depth++
			}
		case ')':
			if inAngle {
				continue
			}
			if depth == 0 {
				return i
			}
			depth--
		}
	}
	return -1
}

func isEscapedMarkdownByte(text string, index int) bool {
	backslashes := 0
	for i := index - 1; i >= 0 && text[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func countMarkdownBackticks(text string, index int) int {
	count := 0
	for index+count < len(text) && text[index+count] == '`' {
		count++
	}
	return count
}

func findMarkdownClosingBackticks(text string, start, ticks int) int {
	for i := start; i < len(text); i++ {
		if text[i] != '`' {
			continue
		}
		count := countMarkdownBackticks(text, i)
		if count == ticks {
			return i
		}
		i += count - 1
	}
	return -1
}

func normalizeMarkdownCodeSpan(code string) string {
	code = strings.ReplaceAll(code, "\n", " ")
	if len(code) >= 2 && code[0] == ' ' && code[len(code)-1] == ' ' && strings.TrimSpace(code) != "" {
		return code[1 : len(code)-1]
	}
	return code
}

func renderUnderscoreEmphasis(text, delimiter string, styleFn func(string) string, basePrefix string, renderContent func(string) string) string {
	if text == "" || delimiter == "" {
		return text
	}
	var out strings.Builder
	for idx := 0; idx < len(text); {
		rel := strings.Index(text[idx:], delimiter)
		if rel < 0 {
			out.WriteString(text[idx:])
			break
		}
		open := idx + rel
		if !isValidMarkdownUnderscoreOpen(text, open, delimiter) {
			out.WriteString(text[idx : open+len(delimiter)])
			idx = open + len(delimiter)
			continue
		}
		close, ok := findMarkdownUnderscoreClose(text, open+len(delimiter), delimiter)
		if !ok {
			out.WriteString(text[idx : open+len(delimiter)])
			idx = open + len(delimiter)
			continue
		}
		content := text[open+len(delimiter) : close]
		if renderContent != nil {
			content = renderContent(content)
		}
		out.WriteString(text[idx:open])
		out.WriteString(style(styleFn, content))
		out.WriteString(basePrefix)
		idx = close + len(delimiter)
	}
	return out.String()
}

func findMarkdownUnderscoreClose(text string, start int, delimiter string) (int, bool) {
	for search := start; search < len(text); {
		rel := strings.Index(text[search:], delimiter)
		if rel < 0 {
			return 0, false
		}
		close := search + rel
		content := text[start:close]
		if isStrictMarkdownUnderscoreText(content) && isValidMarkdownUnderscoreClose(text, close, delimiter) {
			return close, true
		}
		search = close + len(delimiter)
	}
	return 0, false
}

func isValidMarkdownUnderscoreOpen(text string, open int, delimiter string) bool {
	if delimiter == "_" {
		if previousRun := markdownPreviousUnderscoreRunLength(text, open); previousRun > 0 {
			runStart := open - previousRun
			if !markdownDelimiterCanOpen(text, runStart, previousRun+1, '_') {
				return false
			}
		}
	}
	if next, ok := nextRune(text, open+len(delimiter)); !ok || next == '_' {
		return false
	}
	return markdownDelimiterCanOpen(text, open, len(delimiter), '_')
}

func isValidMarkdownUnderscoreClose(text string, close int, delimiter string) bool {
	if delimiter == "_" {
		if prev, ok := previousRune(text, close); ok && prev == '_' {
			return false
		}
		if runLen := markdownUnderscoreRunLengthAt(text, close); runLen > 1 {
			return markdownDelimiterCanClose(text, close, runLen, '_')
		}
	}
	if prev, ok := previousRune(text, close); !ok || prev == '_' {
		return false
	}
	if next, ok := nextRune(text, close+len(delimiter)); ok && next == '_' {
		return false
	}
	return markdownDelimiterCanClose(text, close, len(delimiter), '_')
}

func markdownPreviousUnderscoreRunLength(text string, pos int) int {
	count := 0
	for i := pos - 1; i >= 0 && text[i] == '_'; i-- {
		count++
	}
	return count
}

func markdownUnderscoreRunLengthAt(text string, pos int) int {
	count := 0
	for pos+count < len(text) && text[pos+count] == '_' {
		count++
	}
	return count
}

func isStrictMarkdownUnderscoreText(text string) bool {
	if text == "" {
		return false
	}
	first, _ := utf8.DecodeRuneInString(text)
	last, _ := utf8.DecodeLastRuneInString(text)
	return !unicode.IsSpace(first) && !unicode.IsSpace(last) && first != '_' && last != '_'
}

func previousRune(text string, pos int) (rune, bool) {
	if pos <= 0 {
		return 0, false
	}
	r, size := utf8.DecodeLastRuneInString(text[:pos])
	return r, size > 0
}

func previousRuneStart(text string, pos int) (int, bool) {
	if pos <= 0 || pos > len(text) {
		return 0, false
	}
	_, size := utf8.DecodeLastRuneInString(text[:pos])
	if size <= 0 {
		return 0, false
	}
	return pos - size, true
}

func nextRune(text string, pos int) (rune, bool) {
	if pos >= len(text) {
		return 0, false
	}
	r, size := utf8.DecodeRuneInString(text[pos:])
	return r, size > 0
}

func markdownDelimiterCanOpen(text string, pos, length int, delimiter byte) bool {
	left, right := markdownDelimiterFlanking(text, pos, length)
	if delimiter == '_' {
		prev, hasPrev := previousRune(text, pos)
		return left && (!right || (hasPrev && isMarkdownPunctuationRune(prev)))
	}
	return left
}

func markdownDelimiterCanClose(text string, pos, length int, delimiter byte) bool {
	left, right := markdownDelimiterFlanking(text, pos, length)
	if delimiter == '_' {
		next, hasNext := nextRune(text, pos+length)
		return right && (!left || (hasNext && isMarkdownPunctuationRune(next)))
	}
	return right
}

func markdownDelimiterFlanking(text string, pos, length int) (left, right bool) {
	prev, hasPrev := previousRune(text, pos)
	next, hasNext := nextRune(text, pos+length)
	prevSpace := !hasPrev || unicode.IsSpace(prev)
	nextSpace := !hasNext || unicode.IsSpace(next)
	prevPunct := hasPrev && isMarkdownPunctuationRune(prev)
	nextPunct := hasNext && isMarkdownPunctuationRune(next)
	left = !nextSpace && (!nextPunct || prevSpace || prevPunct)
	right = !prevSpace && (!prevPunct || nextSpace || nextPunct)
	return left, right
}

func isMarkdownPunctuationRune(r rune) bool {
	if r == 0 {
		return true
	}
	return r != '~' && (unicode.IsPunct(r) || unicode.IsSymbol(r))
}

func isStrictMarkdownStrikeText(text string) bool {
	runes := []rune(text)
	if len(runes) == 0 {
		return false
	}
	if start, ok := previousRuneStart(text, len(text)); ok && isEscapedMarkdownByte(text, start) {
		return !unicode.IsSpace(runes[0]) && runes[0] != '~'
	}
	return !unicode.IsSpace(runes[0]) && !unicode.IsSpace(runes[len(runes)-1]) && runes[0] != '~' && runes[len(runes)-1] != '~'
}

func (m *Markdown) headingStyle(level int) func(string) string {
	if level == 1 {
		return func(s string) string {
			return style(m.theme.Heading, style(m.theme.Bold, style(m.theme.Underline, s)))
		}
	}
	return func(s string) string {
		return style(m.theme.Heading, style(m.theme.Bold, s))
	}
}

func (m *Markdown) quoteStyle() func(string) string {
	return func(s string) string {
		return style(m.theme.Quote, style(m.theme.Italic, s))
	}
}

func stylePrefix(styleFn func(string) string) string {
	if styleFn == nil {
		return ""
	}
	const sentinel = "\x00"
	styled := styleFn(sentinel)
	index := strings.Index(styled, sentinel)
	if index < 0 {
		return ""
	}
	return styled[:index]
}

func (m *Markdown) renderLink(label, url string) string {
	return m.renderLinkDisplay(label, label, url)
}

func (m *Markdown) renderLinkDisplay(label, displayLabel, url string) string {
	label = strings.TrimSpace(label)
	displayLabel = strings.TrimSpace(displayLabel)
	url = strings.TrimSpace(url)
	display := style(m.theme.Link, style(m.theme.Underline, displayLabel))
	if GetCapabilities().Hyperlinks {
		return Hyperlink(display, url)
	}
	comparisonURL := url
	if strings.HasPrefix(comparisonURL, "mailto:") {
		comparisonURL = strings.TrimPrefix(comparisonURL, "mailto:")
	}
	if label == url || label == comparisonURL {
		return display
	}
	return display + style(m.theme.LinkURL, " ("+url+")")
}

type LoaderIndicatorOptions struct {
	Frames                  []string
	Interval                time.Duration
	IntervalMs              int
	TUI                     *TUI
	SpinnerColor            func(string) string
	MessageColor            func(string) string
	RenderIndicatorVerbatim bool
}

type Loader struct {
	mu                      sync.Mutex
	message                 string
	frames                  []string
	interval                time.Duration
	current                 int
	ui                      *TUI
	spinnerColor            func(string) string
	messageColor            func(string) string
	renderIndicatorVerbatim bool
	stopAnimation           chan struct{}
}

func NewLoader(text string, options ...LoaderIndicatorOptions) *Loader {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	interval := 80 * time.Millisecond
	var opts LoaderIndicatorOptions
	if len(options) > 0 {
		opts = options[0]
		if opts.Frames != nil {
			frames = append([]string(nil), opts.Frames...)
		}
		if opts.Interval > 0 {
			interval = opts.Interval
		} else if opts.IntervalMs > 0 {
			interval = time.Duration(opts.IntervalMs) * time.Millisecond
		}
	}
	loader := &Loader{
		message:                 text,
		frames:                  frames,
		interval:                interval,
		ui:                      opts.TUI,
		spinnerColor:            opts.SpinnerColor,
		messageColor:            opts.MessageColor,
		renderIndicatorVerbatim: opts.RenderIndicatorVerbatim || opts.Frames != nil,
	}
	if opts.TUI != nil {
		loader.Start()
	}
	return loader
}

func (l *Loader) SetText(text string) { l.SetMessage(text) }
func (l *Loader) SetMessage(message string) {
	l.mu.Lock()
	l.message = message
	l.mu.Unlock()
	l.requestRender()
}

func (l *Loader) SetIndicator(options LoaderIndicatorOptions) {
	l.mu.Lock()
	if options.Frames != nil {
		l.frames = append([]string(nil), options.Frames...)
		l.renderIndicatorVerbatim = true
	} else {
		l.frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		l.renderIndicatorVerbatim = false
	}
	if options.Interval > 0 {
		l.interval = options.Interval
	} else if options.IntervalMs > 0 {
		l.interval = time.Duration(options.IntervalMs) * time.Millisecond
	}
	if options.TUI != nil {
		l.ui = options.TUI
	}
	if options.SpinnerColor != nil {
		l.spinnerColor = options.SpinnerColor
	}
	if options.MessageColor != nil {
		l.messageColor = options.MessageColor
	}
	if options.RenderIndicatorVerbatim {
		l.renderIndicatorVerbatim = true
	}
	l.current = 0
	running := l.stopAnimation != nil
	l.mu.Unlock()
	if running {
		l.Start()
	} else {
		l.requestRender()
	}
}

func (l *Loader) SetTUI(ui *TUI) {
	l.mu.Lock()
	l.ui = ui
	l.mu.Unlock()
}

func (l *Loader) Start() {
	l.mu.Lock()
	l.stopAnimationLocked()
	if len(l.frames) <= 1 {
		l.mu.Unlock()
		l.requestRender()
		return
	}
	interval := l.interval
	if interval <= 0 {
		interval = 80 * time.Millisecond
	}
	stop := make(chan struct{})
	l.stopAnimation = stop
	l.mu.Unlock()
	l.requestRender()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				l.mu.Lock()
				if len(l.frames) > 0 {
					l.current = (l.current + 1) % len(l.frames)
				}
				l.mu.Unlock()
				l.requestRender()
			case <-stop:
				return
			}
		}
	}()
}

func (l *Loader) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stopAnimationLocked()
}

func (l *Loader) stopAnimationLocked() {
	if l.stopAnimation != nil {
		close(l.stopAnimation)
		l.stopAnimation = nil
	}
}

func (l *Loader) Invalidate() {}
func (l *Loader) Render(width int) []string {
	l.mu.Lock()
	message := l.message
	frames := append([]string(nil), l.frames...)
	current := l.current
	spinnerColor := l.spinnerColor
	messageColor := l.messageColor
	verbatim := l.renderIndicatorVerbatim
	l.mu.Unlock()

	renderedMessage := style(messageColor, message)
	indicator := ""
	if len(frames) > 0 {
		frame := frames[current%len(frames)]
		renderedFrame := frame
		if !verbatim {
			renderedFrame = style(spinnerColor, frame)
		}
		if frame != "" {
			indicator = renderedFrame + " "
		}
	}
	text := NewText(indicator+renderedMessage, 1, 0)
	return append([]string{""}, text.Render(width)...)
}

func (l *Loader) requestRender() {
	l.mu.Lock()
	ui := l.ui
	l.mu.Unlock()
	if ui != nil {
		ui.RequestRender(false)
	}
}

type CancellableLoader struct {
	*Loader
	cancelMu  sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
	cancelled bool
	OnCancel  func()
	OnAbort   func()
}

func NewCancellableLoader(text string, options ...LoaderIndicatorOptions) *CancellableLoader {
	ctx, cancel := context.WithCancel(context.Background())
	return &CancellableLoader{Loader: NewLoader(text, options...), ctx: ctx, cancel: cancel}
}

func (l *CancellableLoader) HandleInput(data string) {
	if GetKeybindings().Matches(data, "tui.select.cancel") {
		l.Cancel()
	}
}

func (l *CancellableLoader) Cancel() {
	l.cancelMu.Lock()
	if l.cancelled {
		l.cancelMu.Unlock()
		return
	}
	l.cancelled = true
	cancel := l.cancel
	onCancel := l.OnCancel
	onAbort := l.OnAbort
	l.cancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if onCancel != nil {
		onCancel()
	}
	if onAbort != nil {
		onAbort()
	}
}

func (l *CancellableLoader) Context() context.Context {
	l.cancelMu.Lock()
	defer l.cancelMu.Unlock()
	if l.ctx == nil {
		l.ctx, l.cancel = context.WithCancel(context.Background())
	}
	return l.ctx
}

func (l *CancellableLoader) Signal() context.Context {
	return l.Context()
}

func (l *CancellableLoader) Cancelled() bool {
	l.cancelMu.Lock()
	defer l.cancelMu.Unlock()
	return l.cancelled
}
func (l *CancellableLoader) Aborted() bool { return l.Cancelled() }
func (l *CancellableLoader) Dispose()      { l.Stop() }

type Input struct {
	FocusState
	mu            sync.Mutex
	text          string
	placeholder   string
	cursor        int
	killRing      []string
	killIndex     int
	undoStack     []inputSnapshot
	lastAction    string
	pasteBuffer   string
	inPaste       bool
	lastKill      bool
	lastYank      bool
	lastYankWidth int
	OnSubmit      func(string)
	OnEscape      func()
	OnChange      func(string)
}

type inputSnapshot struct {
	text   string
	cursor int
}

type inputCallback func()

func NewInput(placeholder ...string) *Input {
	p := ""
	if len(placeholder) > 0 {
		p = placeholder[0]
	}
	return &Input{placeholder: p}
}

func (i *Input) Text() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.text
}
func (i *Input) GetValue() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.text
}
func (i *Input) SetValue(text string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.text = text
	i.cursor = min(i.cursor, len([]rune(text)))
	i.lastAction = ""
	i.lastKill = false
	i.lastYank = false
}
func (i *Input) SetText(text string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.text = text
	i.cursor = len([]rune(text))
	i.lastAction = ""
	i.lastKill = false
	i.lastYank = false
}
func (i *Input) Invalidate() {}
func (i *Input) Render(width int) []string {
	i.mu.Lock()
	value := i.text
	placeholder := i.placeholder
	cursorValue := i.cursor
	focused := i.Focused()
	i.mu.Unlock()

	prompt := "> "
	availableWidth := width - VisibleWidth(prompt)
	if availableWidth <= 0 {
		return []string{prompt}
	}

	cursor := max(0, min(cursorValue, len([]rune(value))))
	displayValue := value
	if displayValue == "" && placeholder != "" {
		displayValue = placeholder
		cursor = 0
	}

	totalWidth := VisibleWidth(displayValue)
	cursorCol := VisibleWidth(string([]rune(value)[:cursor]))
	scrollWidth := availableWidth
	if cursor == len([]rune(value)) {
		scrollWidth = max(1, availableWidth-1)
	}
	startCol := 0
	if totalWidth > availableWidth {
		halfWidth := scrollWidth / 2
		switch {
		case cursorCol < halfWidth:
			startCol = 0
		case cursorCol > totalWidth-halfWidth:
			startCol = max(0, totalWidth-scrollWidth)
		default:
			startCol = max(0, cursorCol-halfWidth)
		}
	}

	visibleText := inputSliceByColumn(displayValue, startCol, scrollWidth)
	beforeCursor := inputSliceByColumn(displayValue, startCol, max(0, cursorCol-startCol))
	atCursor := " "
	afterCursor := ""
	if len(beforeCursor) < len(visibleText) {
		after := visibleText[len(beforeCursor):]
		clusterBytes := firstGraphemeByteLen(after)
		atCursor = after[:clusterBytes]
		afterCursor = after[clusterBytes:]
	}
	marker := ""
	if focused {
		marker = CursorMarker
	}
	line := prompt + beforeCursor + marker + "\x1b[7m" + atCursor + "\x1b[27m" + afterCursor
	return []string{TruncateToWidth(line, width, "", true)}
}

func inputSliceByColumn(text string, startCol, width int) string {
	if width <= 0 || text == "" {
		return ""
	}
	endCol := startCol + width
	col := 0
	var b strings.Builder
	for _, segment := range terminalWidthSegments(text) {
		nextCol := col + segment.width
		if nextCol <= startCol {
			col = nextCol
			continue
		}
		if col >= endCol || col+segment.width > endCol {
			break
		}
		b.WriteString(segment.text)
		col = nextCol
	}
	return b.String()
}

func (i *Input) HandleInput(data string) {
	i.mu.Lock()
	callbacks := i.handleInputLocked(data)
	i.mu.Unlock()
	runInputCallbacks(callbacks)
}

func (i *Input) handleInputLocked(data string) []inputCallback {
	var callbacks []inputCallback
	if data == "" {
		return callbacks
	}
	if !i.inPaste {
		if start := strings.Index(data, "\x1b[200~"); start >= 0 {
			if start > 0 {
				callbacks = append(callbacks, i.handleInputLocked(data[:start])...)
			}
			i.inPaste = true
			i.pasteBuffer = ""
			callbacks = append(callbacks, i.handleInputLocked(data[start+len("\x1b[200~"):])...)
			return callbacks
		}
	}
	if i.inPaste {
		i.pasteBuffer += data
		if end := strings.Index(i.pasteBuffer, "\x1b[201~"); end >= 0 {
			pasteContent := i.pasteBuffer[:end]
			remaining := i.pasteBuffer[end+len("\x1b[201~"):]
			i.inPaste = false
			i.pasteBuffer = ""
			callbacks = append(callbacks, i.handlePaste(pasteContent)...)
			if remaining != "" {
				callbacks = append(callbacks, i.handleInputLocked(remaining)...)
			}
		}
		return callbacks
	}

	kb := GetKeybindings()

	switch {
	case kb.Matches(data, "tui.select.cancel"):
		if i.OnEscape != nil {
			onEscape := i.OnEscape
			callbacks = append(callbacks, func() { onEscape() })
		}
		i.lastAction = ""
		i.breakKillAndYank()
	case kb.Matches(data, "tui.editor.undo"):
		callbacks = append(callbacks, i.undo()...)
	case kb.Matches(data, "tui.input.submit") || data == "\n":
		if i.OnSubmit != nil {
			onSubmit := i.OnSubmit
			text := i.text
			callbacks = append(callbacks, func() { onSubmit(text) })
		}
		i.lastAction = ""
		i.breakKillAndYank()
	case kb.Matches(data, "tui.editor.deleteCharBackward"):
		runes := []rune(i.text)
		if i.cursor > 0 && len(runes) > 0 {
			start := previousGraphemeBoundary(runes, i.cursor)
			i.pushUndo()
			runes = append(runes[:start], runes[i.cursor:]...)
			i.cursor = start
			i.text = string(runes)
			callbacks = append(callbacks, i.changedCallbacks()...)
		}
		i.lastAction = ""
		i.breakKillAndYank()
	case kb.Matches(data, "tui.editor.deleteCharForward"):
		runes := []rune(i.text)
		if i.cursor < len(runes) {
			end := nextGraphemeBoundary(runes, i.cursor)
			i.pushUndo()
			runes = append(runes[:i.cursor], runes[end:]...)
			i.text = string(runes)
			callbacks = append(callbacks, i.changedCallbacks()...)
		}
		i.lastAction = ""
		i.breakKillAndYank()
	case kb.Matches(data, "tui.editor.deleteWordBackward"):
		callbacks = append(callbacks, i.killWordBackward()...)
	case kb.Matches(data, "tui.editor.deleteWordForward"):
		callbacks = append(callbacks, i.killWordForward()...)
	case kb.Matches(data, "tui.editor.deleteToLineStart"):
		callbacks = append(callbacks, i.killRange(0, i.cursor, true)...)
	case kb.Matches(data, "tui.editor.deleteToLineEnd"):
		callbacks = append(callbacks, i.killRange(i.cursor, len([]rune(i.text)), false)...)
	case kb.Matches(data, "tui.editor.yank"):
		callbacks = append(callbacks, i.yank()...)
	case kb.Matches(data, "tui.editor.yankPop"):
		callbacks = append(callbacks, i.yankPop()...)
	case kb.Matches(data, "tui.editor.cursorLeft"):
		i.cursor = previousGraphemeBoundary([]rune(i.text), i.cursor)
		i.lastAction = ""
		i.breakKillAndYank()
	case kb.Matches(data, "tui.editor.cursorRight"):
		i.cursor = nextGraphemeBoundary([]rune(i.text), i.cursor)
		i.lastAction = ""
		i.breakKillAndYank()
	case kb.Matches(data, "tui.editor.cursorLineStart"):
		i.cursor = 0
		i.lastAction = ""
		i.breakKillAndYank()
	case kb.Matches(data, "tui.editor.cursorLineEnd"):
		i.cursor = len([]rune(i.text))
		i.lastAction = ""
		i.breakKillAndYank()
	case kb.Matches(data, "tui.editor.cursorWordLeft"):
		i.cursor = i.wordBackward()
		i.lastAction = ""
		i.breakKillAndYank()
	case kb.Matches(data, "tui.editor.cursorWordRight"):
		i.cursor = i.wordForward()
		i.lastAction = ""
		i.breakKillAndYank()
	default:
		if isPlainPrintableText(data) {
			i.insertStringWithUndo(data)
			callbacks = append(callbacks, i.changedCallbacks()...)
			i.breakKillAndYank()
			return callbacks
		}
		event := ParseKey(data)
		if event.Rune != 0 && isPlainPrintableRune(event.Rune) && !event.Ctrl && !event.Alt && !event.Super {
			i.insertStringWithUndo(string(event.Rune))
			callbacks = append(callbacks, i.changedCallbacks()...)
			i.breakKillAndYank()
		}
	}
	return callbacks
}

func (i *Input) changed() {
	runInputCallbacks(i.changedCallbacks())
}

func (i *Input) changedCallbacks() []inputCallback {
	if i.OnChange == nil {
		return nil
	}
	onChange := i.OnChange
	text := i.text
	return []inputCallback{func() { onChange(text) }}
}

func runInputCallbacks(callbacks []inputCallback) {
	for _, callback := range callbacks {
		if callback != nil {
			callback()
		}
	}
}

func (i *Input) breakKillAndYank() {
	i.lastKill = false
	i.lastYank = false
}

func (i *Input) insertString(text string) {
	runes := []rune(i.text)
	insert := []rune(text)
	pos := min(i.cursor, len(runes))
	next := make([]rune, 0, len(runes)+len(insert))
	next = append(next, runes[:pos]...)
	next = append(next, insert...)
	next = append(next, runes[pos:]...)
	i.cursor += len(insert)
	i.text = string(next)
}

func (i *Input) insertStringWithUndo(text string) {
	if text == "" {
		return
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return
	}
	if containsWhitespaceRune(text) || i.lastAction != "type-word" {
		i.pushUndo()
	}
	i.lastAction = "type-word"
	i.insertString(text)
}

func (i *Input) killRange(start, end int, backward bool) []inputCallback {
	runes := []rune(i.text)
	start = max(0, min(start, len(runes)))
	end = max(start, min(end, len(runes)))
	if start == end {
		i.lastYank = false
		return nil
	}
	i.pushUndo()
	killed := string(runes[start:end])
	next := append([]rune{}, runes[:start]...)
	next = append(next, runes[end:]...)
	i.text = string(next)
	i.cursor = start
	i.recordKill(killed, backward)
	return i.changedCallbacks()
}

func (i *Input) killWordBackward() []inputCallback {
	start, end := i.wordBackwardDeleteRange()
	return i.killRange(start, end, true)
}

func (i *Input) killWordForward() []inputCallback {
	start, end := i.wordForwardDeleteRange()
	return i.killRange(start, end, false)
}

func (i *Input) recordKill(text string, backward bool) {
	if text == "" {
		return
	}
	if i.lastKill && len(i.killRing) > 0 {
		if backward {
			i.killRing[0] = text + i.killRing[0]
		} else {
			i.killRing[0] += text
		}
	} else {
		i.killRing = append([]string{text}, i.killRing...)
	}
	i.killIndex = 0
	i.lastKill = true
	i.lastYank = false
}

func (i *Input) yank() []inputCallback {
	if len(i.killRing) == 0 {
		i.lastKill = false
		return nil
	}
	i.pushUndo()
	i.killIndex = 0
	text := i.killRing[i.killIndex]
	i.insertString(text)
	i.lastYank = true
	i.lastKill = false
	i.lastYankWidth = len([]rune(text))
	return i.changedCallbacks()
}

func (i *Input) yankPop() []inputCallback {
	if !i.lastYank || len(i.killRing) <= 1 {
		return nil
	}
	i.pushUndo()
	runes := []rune(i.text)
	start := max(0, i.cursor-i.lastYankWidth)
	runes = append(runes[:start], runes[i.cursor:]...)
	i.text = string(runes)
	i.cursor = start
	i.rotateKillRing()
	i.killIndex = 0
	text := i.killRing[0]
	i.insertString(text)
	i.lastYankWidth = len([]rune(text))
	i.lastYank = true
	i.lastKill = false
	return i.changedCallbacks()
}

func (i *Input) rotateKillRing() {
	if len(i.killRing) <= 1 {
		return
	}
	first := i.killRing[0]
	copy(i.killRing, i.killRing[1:])
	i.killRing[len(i.killRing)-1] = first
}

func (i *Input) handlePaste(pastedText string) []inputCallback {
	clean := normalizeEditorText(pastedText)
	clean = strings.ReplaceAll(clean, "\n", "")
	if clean == "" {
		return nil
	}
	i.pushUndo()
	i.lastAction = ""
	i.breakKillAndYank()
	i.insertString(clean)
	return i.changedCallbacks()
}

func (i *Input) pushUndo() {
	snapshot := inputSnapshot{text: i.text, cursor: i.cursor}
	if len(i.undoStack) > 0 {
		last := i.undoStack[len(i.undoStack)-1]
		if last.text == snapshot.text && last.cursor == snapshot.cursor {
			return
		}
	}
	i.undoStack = append(i.undoStack, snapshot)
	if len(i.undoStack) > 200 {
		i.undoStack = i.undoStack[len(i.undoStack)-200:]
	}
}

func (i *Input) undo() []inputCallback {
	if len(i.undoStack) == 0 {
		return nil
	}
	snapshot := i.undoStack[len(i.undoStack)-1]
	i.undoStack = i.undoStack[:len(i.undoStack)-1]
	i.text = snapshot.text
	i.cursor = min(snapshot.cursor, len([]rune(i.text)))
	i.lastAction = ""
	i.breakKillAndYank()
	return i.changedCallbacks()
}

func (i *Input) wordBackwardDeleteRange() (int, int) {
	if i.cursor <= 0 {
		return 0, 0
	}
	runes := []rune(i.text)
	end := min(i.cursor, len(runes))
	start := end
	for start > 0 && unicode.IsSpace(runes[start-1]) {
		start--
	}
	if start > 0 && isEditorPunctuationRune(runes[start-1]) {
		for start > 0 && isEditorPunctuationRune(runes[start-1]) {
			start--
		}
	} else {
		for start > 0 && !unicode.IsSpace(runes[start-1]) && !isEditorPunctuationRune(runes[start-1]) {
			start--
		}
	}
	return start, end
}

func (i *Input) wordForwardDeleteRange() (int, int) {
	runes := []rune(i.text)
	if i.cursor >= len(runes) {
		return i.cursor, i.cursor
	}
	return i.cursor, i.wordForward()
}

func (i *Input) wordBackward() int {
	start, _ := i.wordBackwardDeleteRange()
	return start
}

func (i *Input) wordForward() int {
	runes := []rune(i.text)
	pos := min(i.cursor, len(runes))
	for pos < len(runes) && unicode.IsSpace(runes[pos]) {
		pos++
	}
	if pos < len(runes) && isEditorPunctuationRune(runes[pos]) {
		for pos < len(runes) && isEditorPunctuationRune(runes[pos]) {
			pos++
		}
	} else {
		for pos < len(runes) && !unicode.IsSpace(runes[pos]) && !isEditorPunctuationRune(runes[pos]) {
			pos++
		}
	}
	return pos
}

type EditorOptions struct {
	PaddingX               int
	AutocompleteMaxVisible int
	AutocompleteDebounce   time.Duration
	MaxVisibleLines        int
}

type EditorTheme struct {
	Border     func(string) string
	SelectList SelectListTheme
}

var slashCommandSelectListLayout = SelectListLayoutOptions{
	MinPrimaryColumnWidth: 12,
	MaxPrimaryColumnWidth: 32,
}

// EditorComponent is the core contract for editor-like input widgets.
// Optional Pi editor capabilities are modeled as small extension interfaces
// below so custom Go editors can implement only the features they support.
type EditorComponent interface {
	Component
	GetText() string
	SetText(text string)
	HandleInput(data string)
}

type EditorHistoryComponent interface {
	AddToHistory(text string)
}

type EditorTextInserter interface {
	InsertTextAtCursor(text string)
}

type EditorExpandedTextProvider interface {
	GetExpandedText() string
}

type EditorAutocompleteComponent interface {
	SetAutocompleteProvider(provider AutocompleteProvider)
}

type EditorAppearanceComponent interface {
	SetPaddingX(padding int)
	SetAutocompleteMaxVisible(maxVisible int)
	SetBorderColor(fn func(string) string)
}

type EditorSubmitCallbackComponent interface {
	SetOnSubmit(fn func(string))
}

type EditorChangeCallbackComponent interface {
	SetOnChange(fn func(string))
}

type Editor struct {
	FocusState
	mu                   sync.Mutex
	text                 string
	cursor               int
	theme                EditorTheme
	options              EditorOptions
	autocomplete         *AutocompleteSuggestions
	autocompleteList     *SelectList
	autocompleteForce    bool
	autocompleteProvider AutocompleteProvider
	autocompleteTimer    *time.Timer
	autocompleteCancel   context.CancelFunc
	autocompleteToken    int
	autocompleteRequest  int
	history              []string
	historyIndex         int
	undoStack            []editorSnapshot
	lastAction           string
	pastes               map[int]string
	pasteCounter         int
	pasteBuffer          string
	inPaste              bool
	killRing             []string
	killIndex            int
	lastKill             bool
	lastYank             bool
	lastYankWidth        int
	jumpDirection        int
	preferredColumn      int
	hasPreferredColumn   bool
	snappedFromLine      int
	snappedFromColumn    int
	hasSnappedFromColumn bool
	lastLayoutWidth      int
	scrollOffset         int
	OnSubmit             func(string)
	OnChange             func(string)
	OnAutocompleteChange func()
	DisableSubmit        bool
	pendingCallbacks     []func()
}

type editorSnapshot struct {
	text         string
	cursor       int
	pastes       map[int]string
	pasteCounter int
}

type editorAutocompleteFullProvider interface {
	GetSuggestions(lines []string, cursorLine, cursorCol int, force bool) (*AutocompleteSuggestions, error)
	ApplyCompletion(lines []string, cursorLine, cursorCol int, item AutocompleteItem, prefix string) CompletionResult
}

type editorAutocompleteContextProvider interface {
	GetSuggestionsContext(ctx context.Context, lines []string, cursorLine, cursorCol int, force bool) (*AutocompleteSuggestions, error)
	ApplyCompletion(lines []string, cursorLine, cursorCol int, item AutocompleteItem, prefix string) CompletionResult
}

type editorAutocompleteTriggerProvider interface {
	ShouldTriggerFileCompletion(lines []string, cursorLine, cursorCol int) bool
}

const attachmentAutocompleteDebounce = 20 * time.Millisecond

var (
	pasteMarkerPattern           = regexp.MustCompile(`\[paste #([0-9]+)( (\+[0-9]+ lines|[0-9]+ chars))?\]`)
	editorCSIuControlPattern     = regexp.MustCompile(`\x1b\[([0-9]+);5u`)
	markdownAutoURIPattern       = regexp.MustCompile(`(?i)<([a-z][a-z0-9+.-]{1,31}:[^<>\x00-\x1f\s]*)>`)
	markdownAutoEmailPattern     = regexp.MustCompile("(?i)<([A-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[A-Z0-9](?:[A-Z0-9-]{0,61}[A-Z0-9])?(?:\\.[A-Z0-9](?:[A-Z0-9-]{0,61}[A-Z0-9])?)+)>")
	markdownBareURLPattern       = regexp.MustCompile(`(?i)(?:https?://|ftp://|www\.)(?:[A-Z0-9\-]+\.?)+[^\s<]*`)
	markdownBareEmailPattern     = regexp.MustCompile(`(?i)\b[A-Z0-9._+\-]+@[A-Z0-9_\-]+(?:\.[A-Z0-9_\-]*[A-Z0-9])+`)
	markdownBareEmailFullPattern = regexp.MustCompile(`(?i)^[A-Z0-9._+\-]+@[A-Z0-9_\-]+(?:\.[A-Z0-9_\-]*[A-Z0-9])+$`)
)

func NewEditor(theme EditorTheme, options ...EditorOptions) *Editor {
	opts := EditorOptions{PaddingX: 0, AutocompleteMaxVisible: 5, AutocompleteDebounce: attachmentAutocompleteDebounce}
	if len(options) > 0 {
		opts = options[0]
	}
	opts = normalizeEditorOptions(opts)
	return &Editor{theme: theme, options: opts, historyIndex: -1, pastes: map[int]string{}, lastLayoutWidth: 80}
}

func normalizeEditorOptions(opts EditorOptions) EditorOptions {
	opts.PaddingX = max(0, opts.PaddingX)
	if opts.AutocompleteMaxVisible == 0 {
		opts.AutocompleteMaxVisible = 5
	} else {
		opts.AutocompleteMaxVisible = max(3, min(20, opts.AutocompleteMaxVisible))
	}
	if opts.AutocompleteDebounce == 0 {
		opts.AutocompleteDebounce = attachmentAutocompleteDebounce
	}
	opts.MaxVisibleLines = max(0, opts.MaxVisibleLines)
	return opts
}

func (e *Editor) GetPaddingX() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.options.PaddingX
}

func (e *Editor) SetPaddingX(padding int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.options.PaddingX = max(0, padding)
}

func (e *Editor) SetBorderColor(fn func(string) string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.theme.Border = fn
}

func (e *Editor) GetAutocompleteMaxVisible() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.options.AutocompleteMaxVisible
}

func (e *Editor) SetAutocompleteMaxVisible(maxVisible int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if maxVisible == 0 {
		e.options.AutocompleteMaxVisible = 5
		return
	}
	e.options.AutocompleteMaxVisible = max(3, min(20, maxVisible))
}

func (e *Editor) GetMaxVisibleLines() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.options.MaxVisibleLines
}

func (e *Editor) SetMaxVisibleLines(lines int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.options.MaxVisibleLines = max(0, lines)
}

func (e *Editor) Text() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.text
}

func (e *Editor) GetText() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.text
}

func (e *Editor) GetLines() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), strings.Split(e.text, "\n")...)
}

func (e *Editor) GetCursor() (line, col int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	lines, cursorLine, cursorCol := e.linesAndCursor()
	if len(lines) == 0 {
		return 0, 0
	}
	return cursorLine, cursorCol
}

func (e *Editor) GetExpandedText() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.getExpandedTextLocked()
}

func (e *Editor) getExpandedTextLocked() string {
	if len(e.pastes) == 0 || !strings.Contains(e.text, "[paste #") {
		return e.text
	}
	return pasteMarkerPattern.ReplaceAllStringFunc(e.text, func(marker string) string {
		matches := pasteMarkerPattern.FindStringSubmatch(marker)
		if len(matches) < 2 {
			return marker
		}
		id, err := strconv.Atoi(matches[1])
		if err != nil {
			return marker
		}
		if paste, ok := e.pastes[id]; ok {
			return paste
		}
		return marker
	})
}

func (e *Editor) SetText(text string) {
	e.mu.Lock()
	normalized := normalizeEditorText(text)
	if e.text != normalized {
		e.pushUndoSnapshot()
	}
	e.cancelAutocomplete()
	e.setTextInternal(normalized)
	e.historyIndex = -1
	e.lastAction = ""
	e.resetPreferredColumn()
	e.breakKillAndYank()
	e.changed()
	callbacks := e.takePendingCallbacksLocked()
	e.mu.Unlock()
	runEditorCallbacks(callbacks)
}

func (e *Editor) SetAutocompleteProvider(provider AutocompleteProvider) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cancelAutocomplete()
	e.autocompleteProvider = provider
}

func (e *Editor) SetOnSubmit(fn func(string)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.OnSubmit = fn
}

func (e *Editor) SetOnChange(fn func(string)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.OnChange = fn
}

func (e *Editor) IsShowingAutocomplete() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.isShowingAutocompleteLocked()
}

func (e *Editor) isShowingAutocompleteLocked() bool {
	return e.autocomplete != nil && e.autocompleteList != nil
}

func (e *Editor) takePendingCallbacksLocked() []func() {
	callbacks := e.pendingCallbacks
	e.pendingCallbacks = nil
	return callbacks
}

func runEditorCallbacks(callbacks []func()) {
	for _, callback := range callbacks {
		if callback != nil {
			callback()
		}
	}
}

func (e *Editor) InsertTextAtCursor(text string) {
	e.mu.Lock()
	if text == "" {
		e.mu.Unlock()
		return
	}
	e.cancelAutocomplete()
	e.pushUndoSnapshot()
	e.insertTextInternal(normalizeEditorText(text))
	e.historyIndex = -1
	e.lastAction = ""
	e.resetPreferredColumn()
	e.breakKillAndYank()
	callbacks := e.takePendingCallbacksLocked()
	e.mu.Unlock()
	runEditorCallbacks(callbacks)
}

func (e *Editor) PasteToEditor(text string) {
	e.mu.Lock()
	e.handlePaste(text)
	callbacks := e.takePendingCallbacksLocked()
	e.mu.Unlock()
	runEditorCallbacks(callbacks)
}

func (e *Editor) Invalidate() {}
func (e *Editor) Render(width int) []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.render(width, 0)
}

func (e *Editor) RenderWithSize(width, height int) []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.render(width, height)
}

func (e *Editor) render(width, height int) []string {
	if width <= 0 {
		width = 1
	}
	maxPadding := max(0, (width-1)/2)
	paddingX := min(max(0, e.options.PaddingX), maxPadding)
	contentWidth := max(1, width-paddingX*2)
	layoutWidth := contentWidth
	if paddingX == 0 {
		layoutWidth = max(1, contentWidth-1)
	}
	e.lastLayoutWidth = layoutWidth

	layoutLines := e.layoutEditorLines(layoutWidth)
	cursorLineIndex := 0
	for idx, line := range layoutLines {
		if line.hasCursor {
			cursorLineIndex = idx
			break
		}
	}
	maxVisibleLines := e.maxVisibleLines(height)
	if cursorLineIndex < e.scrollOffset {
		e.scrollOffset = cursorLineIndex
	} else if cursorLineIndex >= e.scrollOffset+maxVisibleLines {
		e.scrollOffset = cursorLineIndex - maxVisibleLines + 1
	}
	e.scrollOffset = max(0, min(e.scrollOffset, max(0, len(layoutLines)-maxVisibleLines)))
	visibleLines := layoutLines[e.scrollOffset:min(len(layoutLines), e.scrollOffset+maxVisibleLines)]

	lines := make([]string, 0, len(visibleLines)+2)
	lines = append(lines, e.renderEditorTopBorder(width))
	for _, layoutLine := range visibleLines {
		lines = append(lines, e.renderEditorContentLine(layoutLine, width, contentWidth, paddingX))
	}
	lines = append(lines, e.renderEditorBottomBorder(width, max(0, len(layoutLines)-(e.scrollOffset+len(visibleLines)))))
	if e.isShowingAutocompleteLocked() {
		left := strings.Repeat(" ", paddingX)
		right := left
		for _, line := range e.autocompleteList.Render(contentWidth) {
			lineWidth := VisibleWidth(line)
			lines = append(lines, left+line+strings.Repeat(" ", max(0, contentWidth-lineWidth))+right)
		}
	}
	return lines
}

func (e *Editor) maxVisibleLines(height int) int {
	if e.options.MaxVisibleLines > 0 {
		return e.options.MaxVisibleLines
	}
	if height > 0 {
		return max(5, height*3/10)
	}
	return 5
}

type editorLayoutLine struct {
	text      string
	hasCursor bool
	cursorPos int
}

func (e *Editor) layoutEditorLines(contentWidth int) []editorLayoutLine {
	textLines, cursorLine, cursorCol := e.linesAndCursor()
	if len(textLines) == 0 || (len(textLines) == 1 && textLines[0] == "") {
		return []editorLayoutLine{{text: "", hasCursor: true, cursorPos: 0}}
	}
	layout := make([]editorLayoutLine, 0, len(textLines))
	for lineIndex, line := range textLines {
		chunks := e.wrapEditorLine(line, contentWidth)
		for chunkIndex, chunk := range chunks {
			item := editorLayoutLine{text: chunk.Text}
			if lineIndex == cursorLine {
				cursorByte := runeColToByteIndex(line, cursorCol)
				isLastChunk := chunkIndex == len(chunks)-1
				if cursorByte >= chunk.StartIndex && (cursorByte < chunk.EndIndex || (isLastChunk && cursorByte == chunk.EndIndex)) {
					item.hasCursor = true
					item.cursorPos = max(0, min(len(chunk.Text), cursorByte-chunk.StartIndex))
				}
			}
			layout = append(layout, item)
		}
	}
	if len(layout) == 0 {
		return []editorLayoutLine{{text: "", hasCursor: true, cursorPos: 0}}
	}
	return layout
}

func (e *Editor) renderEditorTopBorder(width int) string {
	horizontal := style(e.theme.Border, "─")
	if e.scrollOffset > 0 {
		indicator := fmt.Sprintf("─── ↑ %d more ", e.scrollOffset)
		return style(e.theme.Border, TruncateToWidth(indicator+strings.Repeat("─", max(0, width-VisibleWidth(indicator))), width, ""))
	}
	return strings.Repeat(horizontal, width)
}

func (e *Editor) renderEditorBottomBorder(width, linesBelow int) string {
	if linesBelow > 0 {
		indicator := fmt.Sprintf("─── ↓ %d more ", linesBelow)
		return style(e.theme.Border, TruncateToWidth(indicator+strings.Repeat("─", max(0, width-VisibleWidth(indicator))), width, ""))
	}
	return strings.Repeat(style(e.theme.Border, "─"), width)
}

func (e *Editor) renderEditorContentLine(layoutLine editorLayoutLine, width, contentWidth, paddingX int) string {
	display := layoutLine.text
	lineWidth := VisibleWidth(display)
	cursorInPadding := false
	if layoutLine.hasCursor {
		before := display[:layoutLine.cursorPos]
		after := display[layoutLine.cursorPos:]
		marker := ""
		if e.Focused() && !e.isShowingAutocompleteLocked() {
			marker = CursorMarker
		}
		if after != "" {
			size := firstGraphemeByteLen(after)
			cursor := "\x1b[7m" + after[:size] + "\x1b[0m"
			display = before + marker + cursor + after[size:]
		} else {
			display = before + marker + "\x1b[7m \x1b[0m"
			lineWidth++
			if lineWidth > contentWidth && paddingX > 0 {
				cursorInPadding = true
			}
		}
	}
	left := strings.Repeat(" ", paddingX)
	right := left
	if cursorInPadding && len(right) > 0 {
		right = right[:len(right)-1]
	}
	line := left + display + strings.Repeat(" ", max(0, contentWidth-lineWidth)) + right
	return TruncateToWidth(line, width, "", true)
}

func (e *Editor) wrapEditorLine(line string, width int) []TextChunk {
	if line == "" {
		return []TextChunk{{Text: "", StartIndex: 0, EndIndex: 0}}
	}
	if VisibleWidth(line) <= width {
		return []TextChunk{{Text: line, StartIndex: 0, EndIndex: len(line)}}
	}
	return wordWrapLineWithSegments(line, width, e.segmentLineForWrap(line))
}

func (e *Editor) HandleInput(data string) {
	e.mu.Lock()
	e.handleInputLocked(data)
	callbacks := e.takePendingCallbacksLocked()
	e.mu.Unlock()
	runEditorCallbacks(callbacks)
}

func (e *Editor) handleInputLocked(data string) {
	if data == "" {
		return
	}
	if !e.inPaste {
		if start := strings.Index(data, "\x1b[200~"); start >= 0 {
			if start > 0 {
				e.handleInputLocked(data[:start])
			}
			e.inPaste = true
			e.pasteBuffer = ""
			e.handleInputLocked(data[start+len("\x1b[200~"):])
			return
		}
	}
	if e.inPaste {
		e.pasteBuffer += data
		if end := strings.Index(e.pasteBuffer, "\x1b[201~"); end >= 0 {
			pasteContent := e.pasteBuffer[:end]
			remaining := e.pasteBuffer[end+len("\x1b[201~"):]
			e.inPaste = false
			e.pasteBuffer = ""
			if pasteContent != "" {
				e.handlePaste(pasteContent)
			}
			if remaining != "" {
				e.handleInputLocked(remaining)
			}
		}
		return
	}

	kb := GetKeybindings()

	if e.jumpDirection != 0 {
		if (e.jumpDirection > 0 && kb.Matches(data, "tui.editor.jumpForward")) || (e.jumpDirection < 0 && kb.Matches(data, "tui.editor.jumpBackward")) {
			e.jumpDirection = 0
			return
		}
		event := ParseKey(data)
		if event.Rune != 0 && !event.Ctrl && !event.Alt && !event.Super {
			e.jumpToRune(event.Rune, e.jumpDirection)
			e.jumpDirection = 0
			e.lastAction = ""
			e.breakKillAndYank()
			return
		}
		e.jumpDirection = 0
	}

	if kb.Matches(data, "tui.input.copy") {
		return
	}

	if kb.Matches(data, "tui.editor.undo") {
		e.undo()
		return
	}

	if e.isShowingAutocompleteLocked() {
		switch {
		case kb.Matches(data, "tui.select.cancel"):
			e.cancelAutocomplete()
			return
		case kb.Matches(data, "tui.select.up"), kb.Matches(data, "tui.select.down"):
			e.autocompleteList.HandleInput(data)
			return
		case kb.Matches(data, "tui.input.tab"):
			e.applySelectedAutocomplete()
			return
		case kb.Matches(data, "tui.select.confirm"):
			autocompletePrefix := ""
			if e.autocomplete != nil {
				autocompletePrefix = e.autocomplete.Prefix
			}
			if !e.applySelectedAutocompleteWithNotify(!strings.HasPrefix(autocompletePrefix, "/")) {
				return
			}
			if !strings.HasPrefix(autocompletePrefix, "/") {
				return
			}
		}
	}

	switch {
	case kb.Matches(data, "tui.input.tab"):
		e.handleTabCompletion()
	case kb.Matches(data, "tui.editor.deleteToLineEnd"):
		start, end := e.deleteToLineEndRange()
		e.killRange(start, end, false)
	case kb.Matches(data, "tui.editor.deleteToLineStart"):
		start, end := e.deleteToLineStartRange()
		e.killRange(start, end, true)
	case kb.Matches(data, "tui.editor.deleteWordBackward"):
		e.killWordBackward()
	case kb.Matches(data, "tui.editor.deleteWordForward"):
		e.killWordForward()
	case kb.Matches(data, "tui.editor.deleteCharBackward") || MatchesKey(data, "shift+backspace"):
		e.backspace()
		e.historyIndex = -1
	case kb.Matches(data, "tui.editor.deleteCharForward") || MatchesKey(data, "shift+delete"):
		e.deleteForward()
		e.historyIndex = -1
	case kb.Matches(data, "tui.editor.yank"):
		e.yank()
	case kb.Matches(data, "tui.editor.yankPop"):
		e.yankPop()
	case kb.Matches(data, "tui.editor.cursorLineStart"):
		e.cursor = e.currentLineStart()
		e.lastAction = ""
		e.resetPreferredColumn()
		e.breakKillAndYank()
	case kb.Matches(data, "tui.editor.cursorLineEnd"):
		e.cursor = e.currentLineEnd()
		e.lastAction = ""
		e.resetPreferredColumn()
		e.breakKillAndYank()
	case kb.Matches(data, "tui.editor.cursorWordLeft"):
		e.cursor = e.wordBackward()
		e.lastAction = ""
		e.resetPreferredColumn()
		e.breakKillAndYank()
	case kb.Matches(data, "tui.editor.cursorWordRight"):
		e.cursor = e.wordForward()
		e.lastAction = ""
		e.resetPreferredColumn()
		e.breakKillAndYank()
	case isEditorNewLineInput(data, kb):
		e.insertRuneWithUndo('\n')
		e.historyIndex = -1
	case kb.Matches(data, "tui.input.submit"):
		if e.DisableSubmit {
			return
		}
		if e.replaceBackslashBeforeCursorWithNewline() {
			return
		}
		if e.OnSubmit != nil {
			fn := e.OnSubmit
			text := strings.TrimSpace(e.getExpandedTextLocked())
			e.pendingCallbacks = append(e.pendingCallbacks, func() { fn(text) })
		}
		e.setTextInternal("")
		e.pastes = map[int]string{}
		e.pasteCounter = 0
		e.undoStack = nil
		e.historyIndex = -1
		e.lastAction = ""
		e.resetPreferredColumn()
		e.breakKillAndYank()
		e.changed()
	case kb.Matches(data, "tui.editor.cursorUp"):
		e.handleUp()
	case kb.Matches(data, "tui.editor.cursorDown"):
		e.handleDown()
	case kb.Matches(data, "tui.editor.cursorRight"):
		previous := e.cursor
		e.cursor = e.rightCursor()
		e.lastAction = ""
		if e.cursor == previous && previous == len([]rune(e.text)) {
			e.capturePreferredColumnFromCurrentVisualLine()
		} else {
			e.resetPreferredColumn()
		}
		e.breakKillAndYank()
	case kb.Matches(data, "tui.editor.cursorLeft"):
		e.cursor = e.leftCursor()
		e.lastAction = ""
		e.resetPreferredColumn()
		e.breakKillAndYank()
	case kb.Matches(data, "tui.editor.pageUp"):
		e.pageScroll(-1)
		e.lastAction = ""
		e.breakKillAndYank()
	case kb.Matches(data, "tui.editor.pageDown"):
		e.pageScroll(1)
		e.lastAction = ""
		e.breakKillAndYank()
	case kb.Matches(data, "tui.editor.jumpForward"):
		e.jumpDirection = 1
		e.lastAction = ""
		e.resetPreferredColumn()
		e.breakKillAndYank()
	case kb.Matches(data, "tui.editor.jumpBackward"):
		e.jumpDirection = -1
		e.lastAction = ""
		e.resetPreferredColumn()
		e.breakKillAndYank()
	default:
		if isPlainPrintableText(data) {
			e.insertTextWithTypingUndo(data)
			e.updateAutocompleteAfterTextInput(data)
			return
		}
		event := ParseKey(data)
		if event.Rune != 0 && isPlainPrintableRune(event.Rune) && !event.Ctrl && !event.Alt && !event.Super {
			e.insertRuneWithUndo(event.Rune)
			e.updateAutocompleteAfterRune(event.Rune)
			e.historyIndex = -1
		}
	}
}

func (e *Editor) AddToHistory(text string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return
	}
	if len(e.history) > 0 && e.history[0] == trimmed {
		return
	}
	e.history = append([]string{trimmed}, e.history...)
	if len(e.history) > 100 {
		e.history = e.history[:100]
	}
}

func (e *Editor) handleUp() {
	if len(e.history) > 0 && (e.text == "" || (e.historyIndex >= 0 && e.isOnFirstVisualLine())) {
		newIndex := e.historyIndex + 1
		if newIndex >= len(e.history) {
			return
		}
		if e.historyIndex == -1 {
			e.pushUndoSnapshot()
		}
		e.historyIndex = newIndex
		e.text = e.history[e.historyIndex]
		e.cursor = len([]rune(e.text))
		e.lastAction = ""
		e.resetPreferredColumn()
		e.breakKillAndYank()
		e.changed()
		return
	}
	if e.isOnFirstVisualLine() {
		e.cursor = e.currentLineStart()
		e.resetPreferredColumn()
		e.lastAction = ""
		e.breakKillAndYank()
		return
	}
	e.moveVertical(-1)
	e.lastAction = ""
	e.breakKillAndYank()
}

func (e *Editor) handleDown() {
	if e.historyIndex >= 0 && e.isOnLastVisualLine() {
		newIndex := e.historyIndex - 1
		if newIndex < -1 {
			return
		}
		e.historyIndex = newIndex
		if newIndex < 0 {
			e.text = ""
			e.cursor = 0
		} else {
			e.text = e.history[newIndex]
			e.cursor = len([]rune(e.text))
		}
		e.lastAction = ""
		e.resetPreferredColumn()
		e.breakKillAndYank()
		e.changed()
		return
	}
	if e.isOnLastVisualLine() {
		e.cursor = e.currentLineEnd()
		e.resetPreferredColumn()
		e.lastAction = ""
		e.breakKillAndYank()
		return
	}
	e.moveVertical(1)
	e.lastAction = ""
	e.breakKillAndYank()
}

func (e *Editor) insertRuneWithUndo(r rune) {
	e.resetPreferredColumn()
	switch {
	case r == '\n':
		e.pushUndoSnapshot()
		e.lastAction = ""
	case unicode.IsSpace(r):
		e.pushUndoSnapshot()
		e.lastAction = "type-word"
	case e.lastAction != "type-word":
		e.pushUndoSnapshot()
		e.lastAction = "type-word"
	default:
		e.lastAction = "type-word"
	}
	e.insertRune(r)
	e.breakKillAndYank()
}

func (e *Editor) insertTextWithTypingUndo(text string) {
	if text == "" {
		return
	}
	e.historyIndex = -1
	e.resetPreferredColumn()
	if containsWhitespaceRune(text) || e.lastAction != "type-word" {
		e.pushUndoSnapshot()
	}
	e.lastAction = "type-word"
	e.insertTextInternal(text)
	e.breakKillAndYank()
}

func (e *Editor) insertRune(r rune) {
	runes := []rune(e.text)
	pos := min(e.cursor, len(runes))
	runes = append(runes[:pos], append([]rune{r}, runes[pos:]...)...)
	e.cursor++
	e.text = string(runes)
	e.changed()
}

func (e *Editor) replaceBackslashBeforeCursorWithNewline() bool {
	runes := []rune(e.text)
	if e.cursor <= 0 || e.cursor > len(runes) || runes[e.cursor-1] != '\\' {
		return false
	}
	e.cancelAutocomplete()
	e.pushUndoSnapshot()
	e.deleteRuneRange(e.cursor-1, e.cursor)
	e.insertTextInternal("\n")
	e.historyIndex = -1
	e.lastAction = ""
	e.resetPreferredColumn()
	e.breakKillAndYank()
	return true
}

func (e *Editor) insertTextInternal(text string) {
	if text == "" {
		return
	}
	runes := []rune(e.text)
	insert := []rune(text)
	pos := min(e.cursor, len(runes))
	next := make([]rune, 0, len(runes)+len(insert))
	next = append(next, runes[:pos]...)
	next = append(next, insert...)
	next = append(next, runes[pos:]...)
	e.text = string(next)
	e.cursor = pos + len(insert)
	e.changed()
}

func (e *Editor) setTextInternal(text string) {
	e.text = text
	e.cursor = len([]rune(text))
}

func (e *Editor) backspace() {
	runes := []rune(e.text)
	if e.cursor <= 0 || len(runes) == 0 {
		e.breakKillAndYank()
		e.updateAutocompleteAfterTextDeletion()
		return
	}
	start, end := e.markerSpanBeforeCursor()
	if start < 0 {
		start, end = previousGraphemeBoundary(runes, e.cursor), e.cursor
	}
	e.pushUndoSnapshot()
	e.deleteRuneRange(start, end)
	e.cursor = start
	e.historyIndex = -1
	e.lastAction = ""
	e.resetPreferredColumn()
	e.breakKillAndYank()
	e.changed()
	e.updateAutocompleteAfterTextDeletion()
}

func (e *Editor) deleteForward() {
	runes := []rune(e.text)
	if e.cursor >= len(runes) {
		e.breakKillAndYank()
		e.updateAutocompleteAfterTextDeletion()
		return
	}
	start, end := e.markerSpanAfterCursor()
	if start < 0 {
		start, end = e.cursor, nextGraphemeBoundary(runes, e.cursor)
	}
	e.pushUndoSnapshot()
	e.deleteRuneRange(start, end)
	e.cursor = start
	e.historyIndex = -1
	e.lastAction = ""
	e.resetPreferredColumn()
	e.breakKillAndYank()
	e.changed()
	e.updateAutocompleteAfterTextDeletion()
}

func (e *Editor) deleteRuneRange(start, end int) string {
	runes := []rune(e.text)
	start = max(0, min(start, len(runes)))
	end = max(start, min(end, len(runes)))
	deleted := string(runes[start:end])
	next := append([]rune{}, runes[:start]...)
	next = append(next, runes[end:]...)
	e.text = string(next)
	e.cursor = min(start, len(next))
	return deleted
}

func (e *Editor) killRange(start, end int, backward bool) {
	runes := []rune(e.text)
	start = max(0, min(start, len(runes)))
	end = max(start, min(end, len(runes)))
	if start == end {
		e.lastYank = false
		e.updateAutocompleteAfterTextDeletion()
		return
	}
	e.pushUndoSnapshot()
	killed := e.deleteRuneRange(start, end)
	e.cursor = start
	e.recordKill(killed, backward)
	e.historyIndex = -1
	e.lastAction = ""
	e.resetPreferredColumn()
	e.changed()
	e.updateAutocompleteAfterTextDeletion()
}

func (e *Editor) killWordBackward() {
	start, end := e.wordBackwardDeleteRange()
	e.killRange(start, end, true)
}

func (e *Editor) killWordForward() {
	start, end := e.wordForwardDeleteRange()
	e.killRange(start, end, false)
}

func (e *Editor) deleteToLineStartRange() (int, int) {
	if e.cursor <= 0 {
		return 0, 0
	}
	start := e.currentLineStart()
	if start == e.cursor && e.cursor > 0 {
		start = e.cursor - 1
	}
	return start, e.cursor
}

func (e *Editor) deleteToLineEndRange() (int, int) {
	runes := []rune(e.text)
	if e.cursor >= len(runes) {
		return e.cursor, e.cursor
	}
	end := e.currentLineEnd()
	if end == e.cursor && end < len(runes) && runes[end] == '\n' {
		end++
	}
	return e.cursor, end
}

func (e *Editor) wordBackwardDeleteRange() (int, int) {
	if e.cursor <= 0 {
		return 0, 0
	}
	if start, end := e.markerSpanBeforeCursor(); start >= 0 {
		return start, end
	}
	runes := []rune(e.text)
	end := e.cursor
	if runes[end-1] == '\n' {
		return end - 1, end
	}
	start := end
	for start > 0 && isHorizontalWhitespace(runes[start-1]) {
		start--
	}
	if start > 0 && isEditorPunctuationRune(runes[start-1]) {
		for start > 0 && runes[start-1] != '\n' && isEditorPunctuationRune(runes[start-1]) {
			if markerStart, _ := e.markerSpanBeforePosition(start); markerStart >= 0 {
				return markerStart, end
			}
			start--
		}
	} else {
		for start > 0 && runes[start-1] != '\n' && !isHorizontalWhitespace(runes[start-1]) && !isEditorPunctuationRune(runes[start-1]) {
			if markerStart, _ := e.markerSpanBeforePosition(start); markerStart >= 0 {
				return markerStart, end
			}
			start--
		}
	}
	return start, end
}

func (e *Editor) wordForwardDeleteRange() (int, int) {
	runes := []rune(e.text)
	if e.cursor >= len(runes) {
		return e.cursor, e.cursor
	}
	if _, end := e.markerSpanAfterCursor(); end >= 0 {
		return e.cursor, end
	}
	if runes[e.cursor] == '\n' {
		return e.cursor, e.cursor + 1
	}
	return e.cursor, e.wordForward()
}

func (e *Editor) recordKill(text string, backward bool) {
	if text == "" {
		return
	}
	if e.lastKill && len(e.killRing) > 0 {
		if backward {
			e.killRing[0] = text + e.killRing[0]
		} else {
			e.killRing[0] += text
		}
	} else {
		e.killRing = append([]string{text}, e.killRing...)
	}
	e.killIndex = 0
	e.lastKill = true
	e.lastYank = false
}

func (e *Editor) yank() {
	if len(e.killRing) == 0 {
		e.lastKill = false
		return
	}
	e.pushUndoSnapshot()
	e.killIndex = 0
	text := e.killRing[e.killIndex]
	e.insertTextInternal(text)
	e.lastYank = true
	e.lastKill = false
	e.lastYankWidth = len([]rune(text))
	e.lastAction = ""
	e.resetPreferredColumn()
}

func (e *Editor) yankPop() {
	if !e.lastYank || len(e.killRing) <= 1 {
		return
	}
	e.pushUndoSnapshot()
	runes := []rune(e.text)
	start := max(0, e.cursor-e.lastYankWidth)
	next := append([]rune{}, runes[:start]...)
	next = append(next, runes[e.cursor:]...)
	e.text = string(next)
	e.cursor = start
	e.rotateKillRing()
	e.killIndex = 0
	text := e.killRing[0]
	e.insertTextInternal(text)
	e.lastYankWidth = len([]rune(text))
	e.lastYank = true
	e.lastKill = false
	e.lastAction = ""
	e.resetPreferredColumn()
}

func (e *Editor) rotateKillRing() {
	if len(e.killRing) <= 1 {
		return
	}
	first := e.killRing[0]
	copy(e.killRing, e.killRing[1:])
	e.killRing[len(e.killRing)-1] = first
}

func (e *Editor) breakKillAndYank() {
	e.lastKill = false
	e.lastYank = false
}

func (e *Editor) handlePaste(pastedText string) {
	e.cancelAutocomplete()
	decoded := decodeCSIuControls(pastedText)
	filtered := filterEditorPaste(normalizeEditorText(decoded))
	if filtered == "" {
		return
	}
	if strings.ContainsAny(filtered[:1], "/~.") {
		runes := []rune(e.text)
		if e.cursor > 0 && e.cursor <= len(runes) && isWordRune(runes[e.cursor-1]) {
			filtered = " " + filtered
		}
	}

	e.pushUndoSnapshot()
	e.historyIndex = -1
	e.lastAction = ""
	e.resetPreferredColumn()
	e.breakKillAndYank()

	pastedLines := strings.Split(filtered, "\n")
	totalChars := len([]rune(filtered))
	if len(pastedLines) > 10 || totalChars > 1000 {
		e.pasteCounter++
		if e.pastes == nil {
			e.pastes = map[int]string{}
		}
		pasteID := e.pasteCounter
		e.pastes[pasteID] = filtered
		marker := fmt.Sprintf("[paste #%d %d chars]", pasteID, totalChars)
		if len(pastedLines) > 10 {
			marker = fmt.Sprintf("[paste #%d +%d lines]", pasteID, len(pastedLines))
		}
		e.insertTextInternal(marker)
		return
	}
	e.insertTextInternal(filtered)
}

func isEditorNewLineInput(data string, kb *KeybindingsManager) bool {
	if kb.Matches(data, "tui.input.newLine") {
		return true
	}
	if data == "\n" || data == "\x1b\r" || data == "\x1b[13;2~" {
		return true
	}
	return (len(data) > 1 && data[0] == '\n') || (len(data) > 1 && strings.Contains(data, "\x1b") && strings.Contains(data, "\r"))
}

func (e *Editor) handleTabCompletion() {
	if e.autocompleteProvider == nil {
		return
	}
	e.requestAutocomplete(true, true)
	if e.isShowingAutocompleteLocked() && len(e.autocomplete.Items) == 1 {
		e.applySelectedAutocomplete()
	}
}

func (e *Editor) updateAutocompleteAfterRune(r rune) {
	if e.autocompleteProvider == nil {
		return
	}
	if e.isShowingAutocompleteLocked() {
		e.requestAutocomplete(e.autocompleteForce)
		return
	}
	if r == '/' && e.isAtStartOfMessage() {
		e.requestAutocomplete(false)
		return
	}
	if r == '@' || r == '#' {
		if e.symbolCompletionContext() {
			e.requestAutocomplete(false)
		}
		return
	}
	if isAutocompleteContinuationRune(r) && (e.inSlashCommandContext() || e.symbolCompletionContext()) {
		e.requestAutocomplete(false)
	}
}

func (e *Editor) updateAutocompleteAfterTextInput(text string) {
	runes := []rune(text)
	if len(runes) == 0 {
		return
	}
	e.updateAutocompleteAfterRune(runes[len(runes)-1])
}

func (e *Editor) updateAutocompleteAfterTextDeletion() {
	if e.autocompleteProvider == nil {
		return
	}
	if e.inSlashCommandContext() || e.symbolCompletionContext() || (e.isShowingAutocompleteLocked() && e.autocompleteForce) {
		e.requestAutocomplete(e.autocompleteForce)
		return
	}
	wasShowing := e.isShowingAutocompleteLocked()
	e.cancelAutocomplete()
	if wasShowing {
		e.notifyAutocompleteChanged()
	}
}

func (e *Editor) requestAutocomplete(force bool, explicitTab ...bool) {
	if e.autocompleteProvider == nil {
		return
	}
	lines, cursorLine, cursorCol := e.linesAndCursor()
	if force {
		if provider, ok := e.autocompleteProvider.(editorAutocompleteTriggerProvider); ok && !provider.ShouldTriggerFileCompletion(lines, cursorLine, cursorCol) {
			return
		}
	}
	tab := len(explicitTab) > 0 && explicitTab[0]
	e.cancelAutocompleteRequest()
	e.autocompleteToken++
	token := e.autocompleteToken
	if e.autocompleteDebounce(force, tab) > 0 {
		e.autocompleteTimer = time.AfterFunc(e.autocompleteDebounce(force, tab), func() {
			e.mu.Lock()
			e.startAutocompleteRequest(token, force, tab)
			callbacks := e.takePendingCallbacksLocked()
			e.mu.Unlock()
			runEditorCallbacks(callbacks)
		})
		return
	}
	e.startAutocompleteRequest(token, force, tab)
}

func (e *Editor) autocompleteDebounce(force, explicitTab bool) time.Duration {
	if force || explicitTab {
		return 0
	}
	lines, cursorLine, cursorCol := e.linesAndCursor()
	if cursorLine < 0 || cursorLine >= len(lines) {
		return 0
	}
	line := lines[cursorLine]
	if cursorCol < 0 {
		cursorCol = 0
	}
	if cursorCol > len([]rune(line)) {
		cursorCol = len([]rune(line))
	}
	before := string([]rune(line)[:cursorCol])
	if symbolAutocompleteContext(before) {
		return e.options.AutocompleteDebounce
	}
	return 0
}

func (e *Editor) startAutocompleteRequest(token int, force, explicitTab bool) {
	if token != e.autocompleteToken || e.autocompleteProvider == nil {
		return
	}
	lines, cursorLine, cursorCol := e.linesAndCursor()
	snapshotText := e.text
	snapshotLine := cursorLine
	snapshotCol := cursorCol
	e.autocompleteRequest++
	requestID := e.autocompleteRequest
	ctx, cancel := context.WithCancel(context.Background())
	e.autocompleteCancel = cancel
	if provider, ok := e.autocompleteProvider.(editorAutocompleteContextProvider); ok {
		linesCopy := append([]string(nil), lines...)
		go func() {
			suggestions, err := provider.GetSuggestionsContext(ctx, linesCopy, cursorLine, cursorCol, force)
			e.mu.Lock()
			e.finishAutocompleteRequest(requestID, ctx, snapshotText, snapshotLine, snapshotCol, force, explicitTab, suggestions, err)
			callbacks := e.takePendingCallbacksLocked()
			e.mu.Unlock()
			runEditorCallbacks(callbacks)
		}()
		return
	}
	var suggestions *AutocompleteSuggestions
	if provider, ok := e.autocompleteProvider.(editorAutocompleteFullProvider); ok {
		got, err := provider.GetSuggestions(lines, cursorLine, cursorCol, force)
		if err == nil {
			suggestions = got
		}
	} else {
		got := e.autocompleteProvider.Suggestions(e.text, e.cursor)
		if len(got.Items) > 0 {
			suggestions = &got
		}
	}
	e.finishAutocompleteRequest(requestID, ctx, snapshotText, snapshotLine, snapshotCol, force, explicitTab, suggestions, nil)
}

func (e *Editor) finishAutocompleteRequest(requestID int, ctx context.Context, snapshotText string, snapshotLine, snapshotCol int, force, explicitTab bool, suggestions *AutocompleteSuggestions, err error) {
	if ctx.Err() != nil || requestID != e.autocompleteRequest || e.text != snapshotText {
		return
	}
	_, currentLine, currentCol := e.linesAndCursor()
	if currentLine != snapshotLine || currentCol != snapshotCol {
		return
	}
	e.autocompleteCancel = nil
	if err != nil || suggestions == nil || len(suggestions.Items) == 0 {
		e.clearAutocompleteUI()
		e.notifyAutocompleteChanged()
		return
	}
	if force && explicitTab && len(suggestions.Items) == 1 {
		e.autocomplete = suggestions
		e.autocompleteForce = force
		e.autocompleteList = e.newAutocompleteList(suggestions.Prefix, []SelectItem{{Value: suggestions.Items[0].Value, Label: suggestions.Items[0].Label, Description: suggestions.Items[0].Description}})
		e.applySelectedAutocomplete()
		return
	}
	e.applyAutocompleteSuggestions(suggestions, force)
	e.notifyAutocompleteChanged()
}

func (e *Editor) applyAutocompleteSuggestions(suggestions *AutocompleteSuggestions, force bool) {
	e.autocomplete = suggestions
	e.autocompleteForce = force
	items := make([]SelectItem, 0, len(suggestions.Items))
	for _, item := range suggestions.Items {
		items = append(items, SelectItem{Value: item.Value, Label: item.Label, Description: item.Description})
	}
	e.autocompleteList = e.newAutocompleteList(suggestions.Prefix, items)
	if index := bestAutocompleteMatchIndex(suggestions.Items, suggestions.Prefix); index >= 0 {
		e.autocompleteList.SetSelectedIndex(index)
	}
}

func (e *Editor) newAutocompleteList(prefix string, items []SelectItem) *SelectList {
	if strings.HasPrefix(prefix, "/") {
		return NewSelectList(items, e.options.AutocompleteMaxVisible, e.theme.SelectList, slashCommandSelectListLayout)
	}
	return NewSelectList(items, e.options.AutocompleteMaxVisible, e.theme.SelectList)
}

func (e *Editor) applySelectedAutocomplete() {
	e.applySelectedAutocompleteWithNotify(true)
}

func (e *Editor) applySelectedAutocompleteWithNotify(notify bool) bool {
	if !e.isShowingAutocompleteLocked() {
		return false
	}
	selected, ok := e.autocompleteList.SelectedItem()
	if !ok {
		e.cancelAutocomplete()
		return false
	}
	item := AutocompleteItem{Value: selected.Value, Label: selected.Label, Description: selected.Description}
	e.pushUndoSnapshot()
	if provider, ok := e.autocompleteProvider.(editorAutocompleteContextProvider); ok {
		lines, cursorLine, cursorCol := e.linesAndCursor()
		result := provider.ApplyCompletion(lines, cursorLine, cursorCol, item, e.autocomplete.Prefix)
		e.text = strings.Join(result.Lines, "\n")
		e.cursor = cursorFromLineCol(result.Lines, result.CursorLine, result.CursorCol)
	} else if provider, ok := e.autocompleteProvider.(editorAutocompleteFullProvider); ok {
		lines, cursorLine, cursorCol := e.linesAndCursor()
		result := provider.ApplyCompletion(lines, cursorLine, cursorCol, item, e.autocomplete.Prefix)
		e.text = strings.Join(result.Lines, "\n")
		e.cursor = cursorFromLineCol(result.Lines, result.CursorLine, result.CursorCol)
	} else {
		e.applyFlatCompletion(item)
	}
	e.historyIndex = -1
	e.lastAction = ""
	e.resetPreferredColumn()
	e.cancelAutocomplete()
	e.breakKillAndYank()
	if notify {
		e.changed()
	}
	return true
}

func (e *Editor) applyFlatCompletion(item AutocompleteItem) {
	runes := []rune(e.text)
	start := max(0, min(e.autocomplete.Start, len(runes)))
	end := max(start, min(e.autocomplete.End, len(runes)))
	value := []rune(item.Value)
	next := append([]rune{}, runes[:start]...)
	next = append(next, value...)
	next = append(next, runes[end:]...)
	e.text = string(next)
	e.cursor = start + len(value)
}

func (e *Editor) cancelAutocomplete() {
	e.cancelAutocompleteRequest()
	e.clearAutocompleteUI()
}

func (e *Editor) cancelAutocompleteRequest() {
	e.autocompleteToken++
	if e.autocompleteTimer != nil {
		e.autocompleteTimer.Stop()
		e.autocompleteTimer = nil
	}
	if e.autocompleteCancel != nil {
		e.autocompleteCancel()
		e.autocompleteCancel = nil
	}
}

func (e *Editor) clearAutocompleteUI() {
	e.autocomplete = nil
	e.autocompleteList = nil
	e.autocompleteForce = false
}

func (e *Editor) notifyAutocompleteChanged() {
	if e.OnAutocompleteChange != nil {
		fn := e.OnAutocompleteChange
		e.pendingCallbacks = append(e.pendingCallbacks, fn)
	}
}

func bestAutocompleteMatchIndex(items []AutocompleteItem, prefix string) int {
	if prefix == "" {
		return -1
	}
	firstPrefix := -1
	for i, item := range items {
		if item.Value == prefix {
			return i
		}
		if firstPrefix < 0 && strings.HasPrefix(item.Value, prefix) {
			firstPrefix = i
		}
	}
	return firstPrefix
}

func symbolAutocompleteContext(before string) bool {
	lineStart := strings.LastIndexByte(before, '\n') + 1
	line := before[lineStart:]
	for start := 0; start < len(line); start++ {
		if start > 0 && line[start-1] != ' ' && line[start-1] != '\t' {
			continue
		}
		if symbolAutocompleteTokenMatchesPi(line[start:]) {
			return true
		}
	}
	return false
}

func symbolAutocompleteTokenMatchesPi(token string) bool {
	if strings.HasPrefix(token, "@\"") {
		return !strings.Contains(token[2:], "\"")
	}
	if strings.HasPrefix(token, "@") || strings.HasPrefix(token, "#") {
		return !strings.ContainsAny(token, " \t")
	}
	return false
}

func (e *Editor) linesAndCursor() ([]string, int, int) {
	lines := strings.Split(e.text, "\n")
	runes := []rune(e.text)
	cursor := max(0, min(e.cursor, len(runes)))
	before := string(runes[:cursor])
	cursorLine := strings.Count(before, "\n")
	lastNewline := strings.LastIndex(before, "\n")
	cursorCol := len([]rune(before))
	if lastNewline >= 0 {
		cursorCol = len([]rune(before[lastNewline+1:]))
	}
	return lines, cursorLine, cursorCol
}

func cursorFromLineCol(lines []string, cursorLine, cursorCol int) int {
	if len(lines) == 0 {
		return 0
	}
	cursorLine = max(0, min(cursorLine, len(lines)-1))
	cursor := 0
	for i := 0; i < cursorLine; i++ {
		cursor += len([]rune(lines[i])) + 1
	}
	lineRunes := []rune(lines[cursorLine])
	cursorCol = max(0, min(cursorCol, len(lineRunes)))
	cursor += cursorCol
	return cursor
}

func (e *Editor) isAtStartOfMessage() bool {
	before := string([]rune(e.text)[:max(0, min(e.cursor, len([]rune(e.text))))])
	lineStart := strings.LastIndex(before, "\n") + 1
	return strings.TrimSpace(before[lineStart:]) == "/"
}

func (e *Editor) inSlashCommandContext() bool {
	before := string([]rune(e.text)[:max(0, min(e.cursor, len([]rune(e.text))))])
	lineStart := strings.LastIndex(before, "\n") + 1
	line := before[lineStart:]
	return strings.HasPrefix(line, "/") && !strings.Contains(line, "\n")
}

func (e *Editor) symbolCompletionContext() bool {
	before := string([]rune(e.text)[:max(0, min(e.cursor, len([]rune(e.text))))])
	start := findLastDelimiter(before) + 1
	if start < 0 || start >= len(before) {
		return false
	}
	token := before[start:]
	return strings.HasPrefix(token, "@") || strings.HasPrefix(token, "#")
}

func isAutocompleteContinuationRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_' || r == '/'
}

func containsWhitespaceRune(text string) bool {
	return strings.IndexFunc(text, unicode.IsSpace) >= 0
}

func decodeCSIuControls(text string) string {
	return editorCSIuControlPattern.ReplaceAllStringFunc(text, func(seq string) string {
		matches := editorCSIuControlPattern.FindStringSubmatch(seq)
		if len(matches) != 2 {
			return seq
		}
		code, err := strconv.Atoi(matches[1])
		if err != nil {
			return seq
		}
		switch {
		case code >= 'a' && code <= 'z':
			return string(rune(code - 'a' + 1))
		case code >= 'A' && code <= 'Z':
			return string(rune(code - 'A' + 1))
		default:
			return seq
		}
	})
}

func filterEditorPaste(text string) string {
	var b strings.Builder
	for _, r := range text {
		if r == '\n' || r >= 32 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func normalizeEditorText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "\t", "    ")
	return text
}

func (e *Editor) pushUndoSnapshot() {
	snapshot := editorSnapshot{
		text:         e.text,
		cursor:       e.cursor,
		pastes:       clonePasteMap(e.pastes),
		pasteCounter: e.pasteCounter,
	}
	if len(e.undoStack) > 0 {
		last := e.undoStack[len(e.undoStack)-1]
		if last.text == snapshot.text && last.cursor == snapshot.cursor {
			return
		}
	}
	e.undoStack = append(e.undoStack, snapshot)
	if len(e.undoStack) > 200 {
		e.undoStack = e.undoStack[len(e.undoStack)-200:]
	}
}

func (e *Editor) undo() {
	if len(e.undoStack) == 0 {
		return
	}
	snapshot := e.undoStack[len(e.undoStack)-1]
	e.undoStack = e.undoStack[:len(e.undoStack)-1]
	e.text = snapshot.text
	e.cursor = min(snapshot.cursor, len([]rune(e.text)))
	e.pastes = clonePasteMap(snapshot.pastes)
	e.pasteCounter = snapshot.pasteCounter
	e.historyIndex = -1
	e.lastAction = ""
	e.resetPreferredColumn()
	e.breakKillAndYank()
	e.changed()
}

func clonePasteMap(in map[int]string) map[int]string {
	out := make(map[int]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (e *Editor) leftCursor() int {
	if start, end := e.markerSpanBeforeCursor(); start >= 0 && e.cursor <= end {
		return start
	}
	return previousGraphemeBoundary([]rune(e.text), e.cursor)
}

func (e *Editor) rightCursor() int {
	if start, end := e.markerSpanAfterCursor(); start >= 0 && e.cursor >= start {
		return end
	}
	return nextGraphemeBoundary([]rune(e.text), e.cursor)
}

func (e *Editor) wordBackward() int {
	if start, _ := e.markerSpanBeforeCursor(); start >= 0 {
		return start
	}
	runes := []rune(e.text)
	pos := min(e.cursor, len(runes))
	for pos > 0 && unicode.IsSpace(runes[pos-1]) {
		pos--
	}
	if pos > 0 && isEditorPunctuationRune(runes[pos-1]) {
		for pos > 0 && isEditorPunctuationRune(runes[pos-1]) {
			if markerStart, _ := e.markerSpanBeforePosition(pos); markerStart >= 0 {
				return markerStart
			}
			pos--
		}
	} else {
		for pos > 0 && !unicode.IsSpace(runes[pos-1]) && !isEditorPunctuationRune(runes[pos-1]) {
			if markerStart, _ := e.markerSpanBeforePosition(pos); markerStart >= 0 {
				return markerStart
			}
			pos--
		}
	}
	return pos
}

func (e *Editor) wordForward() int {
	if _, end := e.markerSpanAfterCursor(); end >= 0 {
		return end
	}
	runes := []rune(e.text)
	pos := min(e.cursor, len(runes))
	for pos < len(runes) && unicode.IsSpace(runes[pos]) {
		pos++
	}
	if _, end := e.markerSpanAtPosition(pos); end >= 0 {
		return end
	}
	if pos < len(runes) && isEditorPunctuationRune(runes[pos]) {
		for pos < len(runes) && isEditorPunctuationRune(runes[pos]) {
			if start, end := e.markerSpanAtPosition(pos); start >= 0 {
				pos = end
				continue
			}
			pos++
		}
	} else {
		for pos < len(runes) && !unicode.IsSpace(runes[pos]) && !isEditorPunctuationRune(runes[pos]) {
			if start, end := e.markerSpanAtPosition(pos); start >= 0 {
				pos = end
				continue
			}
			pos++
		}
	}
	return pos
}

func (e *Editor) markerSpanBeforeCursor() (int, int) {
	return e.markerSpanBeforePosition(e.cursor)
}

func (e *Editor) markerSpanAfterCursor() (int, int) {
	return e.markerSpanAtPosition(e.cursor)
}

func (e *Editor) markerSpanBeforePosition(pos int) (int, int) {
	for _, span := range e.validPasteMarkerSpans() {
		if pos > span.start && pos <= span.end {
			return span.start, span.end
		}
	}
	return -1, -1
}

func (e *Editor) markerSpanAtPosition(pos int) (int, int) {
	for _, span := range e.validPasteMarkerSpans() {
		if pos >= span.start && pos < span.end {
			return span.start, span.end
		}
	}
	return -1, -1
}

type editorMarkerSpan struct {
	start int
	end   int
}

func (e *Editor) validPasteMarkerSpans() []editorMarkerSpan {
	if len(e.pastes) == 0 || !strings.Contains(e.text, "[paste #") {
		return nil
	}
	matches := pasteMarkerPattern.FindAllStringSubmatchIndex(e.text, -1)
	spans := make([]editorMarkerSpan, 0, len(matches))
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		id, err := strconv.Atoi(e.text[match[2]:match[3]])
		if err != nil {
			continue
		}
		if _, ok := e.pastes[id]; !ok {
			continue
		}
		start := len([]rune(e.text[:match[0]]))
		end := len([]rune(e.text[:match[1]]))
		spans = append(spans, editorMarkerSpan{start: start, end: end})
	}
	return spans
}

func isHorizontalWhitespace(r rune) bool {
	return r == ' ' || r == '\t'
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func isEditorPunctuationRune(r rune) bool {
	return strings.ContainsRune(`(){}[]<>.,;:'"!?+-=*/\|&%^$#@~_`, r)
}

func (e *Editor) changed() {
	if e.OnChange != nil {
		fn := e.OnChange
		text := e.text
		e.pendingCallbacks = append(e.pendingCallbacks, func() { fn(text) })
	}
}

func (e *Editor) currentLineStart() int {
	runes := []rune(e.text)
	pos := min(e.cursor, len(runes))
	for pos > 0 && runes[pos-1] != '\n' {
		pos--
	}
	return pos
}

func (e *Editor) currentLineEnd() int {
	runes := []rune(e.text)
	pos := min(e.cursor, len(runes))
	for pos < len(runes) && runes[pos] != '\n' {
		pos++
	}
	return pos
}

func (e *Editor) currentLineCol() int {
	return e.cursor - e.currentLineStart()
}

func (e *Editor) resetPreferredColumn() {
	e.resetPreferredColumnOnly()
	e.clearSnappedFromColumn()
}

func (e *Editor) resetPreferredColumnOnly() {
	e.hasPreferredColumn = false
	e.preferredColumn = 0
}

func (e *Editor) clearSnappedFromColumn() {
	e.hasSnappedFromColumn = false
	e.snappedFromLine = 0
	e.snappedFromColumn = 0
}

func (e *Editor) capturePreferredColumnFromCurrentVisualLine() {
	lines := strings.Split(e.text, "\n")
	visualLines := e.editorVisualLines(max(1, e.lastLayoutWidth), lines)
	if len(visualLines) == 0 {
		e.resetPreferredColumn()
		return
	}
	_, cursorLine, cursorCol := e.linesAndCursor()
	current := findEditorVisualLine(visualLines, cursorLine, cursorCol)
	currentVL := visualLines[current]
	e.preferredColumn = max(0, cursorCol-currentVL.startCol)
	e.hasPreferredColumn = true
}

func (e *Editor) isOnFirstLine() bool { return e.currentLineStart() == 0 }

func (e *Editor) isOnLastLine() bool { return e.currentLineEnd() == len([]rune(e.text)) }

func (e *Editor) isOnFirstVisualLine() bool {
	visualLines, current := e.currentEditorVisualLine()
	return len(visualLines) == 0 || current == 0
}

func (e *Editor) isOnLastVisualLine() bool {
	visualLines, current := e.currentEditorVisualLine()
	return len(visualLines) == 0 || current == len(visualLines)-1
}

func (e *Editor) currentEditorVisualLine() ([]editorVisualLine, int) {
	lines := strings.Split(e.text, "\n")
	visualLines := e.editorVisualLines(max(1, e.lastLayoutWidth), lines)
	if len(visualLines) == 0 {
		return visualLines, 0
	}
	_, cursorLine, cursorCol := e.linesAndCursor()
	return visualLines, findEditorVisualLine(visualLines, cursorLine, cursorCol)
}

func (e *Editor) pageScroll(direction int) {
	if direction == 0 || e.text == "" {
		return
	}
	lines := strings.Split(e.text, "\n")
	visualLines := e.editorVisualLines(max(1, e.lastLayoutWidth), lines)
	if len(visualLines) == 0 {
		return
	}
	_, cursorLine, cursorCol := e.linesAndCursor()
	current := findEditorVisualLine(visualLines, cursorLine, cursorCol)
	pageSize := e.editorPageSize()
	target := current + direction*pageSize
	if target < 0 {
		target = 0
	}
	if target >= len(visualLines) {
		target = len(visualLines) - 1
	}
	e.moveToEditorVisualLine(lines, visualLines, current, target)
}

func (e *Editor) editorPageSize() int {
	return 5
}

func (e *Editor) moveVertical(delta int) {
	if delta == 0 || e.text == "" {
		return
	}
	lines := strings.Split(e.text, "\n")
	visualLines := e.editorVisualLines(max(1, e.lastLayoutWidth), lines)
	if len(visualLines) == 0 {
		return
	}
	_, cursorLine, cursorCol := e.linesAndCursor()
	current := findEditorVisualLine(visualLines, cursorLine, cursorCol)
	target := current + delta
	if target < 0 || target >= len(visualLines) {
		return
	}
	e.moveToEditorVisualLine(lines, visualLines, current, target)
}

type editorVisualLine struct {
	logicalLine int
	startCol    int
	length      int
}

func (e *Editor) editorVisualLines(width int, lines []string) []editorVisualLine {
	width = max(1, width)
	visualLines := make([]editorVisualLine, 0, len(lines))
	for lineIndex, line := range lines {
		lineLen := len([]rune(line))
		if line == "" {
			visualLines = append(visualLines, editorVisualLine{logicalLine: lineIndex})
			continue
		}
		if VisibleWidth(line) <= width {
			visualLines = append(visualLines, editorVisualLine{logicalLine: lineIndex, length: lineLen})
			continue
		}
		chunks := wordWrapLineWithSegments(line, width, e.segmentLineForWrap(line))
		for _, chunk := range chunks {
			startCol := byteIndexToRuneCol(line, chunk.StartIndex)
			endCol := byteIndexToRuneCol(line, chunk.EndIndex)
			visualLines = append(visualLines, editorVisualLine{
				logicalLine: lineIndex,
				startCol:    startCol,
				length:      max(0, endCol-startCol),
			})
		}
	}
	if len(visualLines) == 0 {
		return []editorVisualLine{{}}
	}
	return visualLines
}

func findEditorVisualLine(visualLines []editorVisualLine, line, col int) int {
	lastForLine := -1
	for i, vl := range visualLines {
		if vl.logicalLine != line {
			continue
		}
		lastForLine = i
		offset := col - vl.startCol
		isLastSegment := i == len(visualLines)-1 || visualLines[i+1].logicalLine != line
		if offset >= 0 && (offset < vl.length || (isLastSegment && offset == vl.length)) {
			return i
		}
	}
	if lastForLine >= 0 {
		return lastForLine
	}
	return max(0, min(len(visualLines)-1, line))
}

func (e *Editor) moveToEditorVisualLine(lines []string, visualLines []editorVisualLine, current, target int) {
	if current < 0 || current >= len(visualLines) || target < 0 || target >= len(visualLines) {
		return
	}
	currentVL := visualLines[current]
	targetVL := visualLines[target]

	currentLineCol := e.currentLineCol()
	currentVisualCol := max(0, currentLineCol-currentVL.startCol)
	if e.hasSnappedFromColumn && e.snappedFromLine == currentVL.logicalLine {
		snappedLine := findEditorVisualLine(visualLines, currentVL.logicalLine, e.snappedFromColumn)
		if snappedLine >= 0 && snappedLine < len(visualLines) {
			currentVisualCol = max(0, e.snappedFromColumn-visualLines[snappedLine].startCol)
		}
	}
	sourceMaxCol := currentVL.length
	if current+1 < len(visualLines) && visualLines[current+1].logicalLine == currentVL.logicalLine {
		sourceMaxCol = max(0, currentVL.length-1)
	}
	targetMaxCol := targetVL.length
	if target+1 < len(visualLines) && visualLines[target+1].logicalLine == targetVL.logicalLine {
		targetMaxCol = max(0, targetVL.length-1)
	}
	moveCol := e.computeVerticalMoveColumn(currentVisualCol, sourceMaxCol, targetMaxCol)
	targetLine := lines[targetVL.logicalLine]
	targetCol := min(targetVL.startCol+moveCol, len([]rune(targetLine)))
	targetCursor := cursorFromLineCol(lines, targetVL.logicalLine, targetCol)

	if markerStart, markerEnd := e.markerSpanAtPosition(targetCursor); markerStart >= 0 {
		lineStart := cursorFromLineCol(lines, targetVL.logicalLine, 0)
		markerStartCol := markerStart - lineStart
		markerEndCol := markerEnd - lineStart
		isContinuation := markerStartCol < targetVL.startCol
		isMovingDown := target > current
		if isContinuation && isMovingDown {
			next := target + 1
			for next < len(visualLines) &&
				visualLines[next].logicalLine == targetVL.logicalLine &&
				visualLines[next].startCol < markerEndCol {
				next++
			}
			if next < len(visualLines) {
				e.moveToEditorVisualLine(lines, visualLines, current, next)
				return
			}
		}
		e.snappedFromLine = targetVL.logicalLine
		e.snappedFromColumn = targetCol
		e.hasSnappedFromColumn = true
		e.cursor = markerStart
		return
	}

	e.clearSnappedFromColumn()
	e.cursor = targetCursor
}

func (e *Editor) computeVerticalMoveColumn(currentVisualCol, sourceMaxVisualCol, targetMaxVisualCol int) int {
	currentVisualCol = max(0, currentVisualCol)
	sourceMaxVisualCol = max(0, sourceMaxVisualCol)
	targetMaxVisualCol = max(0, targetMaxVisualCol)

	cursorInMiddle := currentVisualCol < sourceMaxVisualCol
	targetTooShort := targetMaxVisualCol < currentVisualCol

	if !e.hasPreferredColumn || cursorInMiddle {
		if targetTooShort {
			e.preferredColumn = currentVisualCol
			e.hasPreferredColumn = true
			return targetMaxVisualCol
		}
		e.resetPreferredColumnOnly()
		return currentVisualCol
	}

	if targetTooShort || targetMaxVisualCol < e.preferredColumn {
		return targetMaxVisualCol
	}

	moveCol := e.preferredColumn
	e.resetPreferredColumnOnly()
	return moveCol
}

func (e *Editor) segmentLineForWrap(line string) []wrapSegment {
	if len(e.pastes) == 0 || !strings.Contains(line, "[paste #") {
		return segmentTextForWrap(line)
	}
	matches := pasteMarkerPattern.FindAllStringSubmatchIndex(line, -1)
	type byteSpan struct {
		start int
		end   int
	}
	markers := make([]byteSpan, 0, len(matches))
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		id, err := strconv.Atoi(line[match[2]:match[3]])
		if err != nil {
			continue
		}
		if _, ok := e.pastes[id]; !ok {
			continue
		}
		markers = append(markers, byteSpan{start: match[0], end: match[1]})
	}
	if len(markers) == 0 {
		return segmentTextForWrap(line)
	}
	segments := make([]wrapSegment, 0, len(line))
	markerIndex := 0
	for idx, r := range line {
		for markerIndex < len(markers) && idx >= markers[markerIndex].end {
			markerIndex++
		}
		if markerIndex < len(markers) && idx >= markers[markerIndex].start && idx < markers[markerIndex].end {
			if idx == markers[markerIndex].start {
				segments = append(segments, wrapSegment{text: line[markers[markerIndex].start:markers[markerIndex].end], index: markers[markerIndex].start})
			}
			continue
		}
		segments = append(segments, wrapSegment{text: string(r), index: idx})
	}
	return segments
}

func byteIndexToRuneCol(text string, byteIndex int) int {
	byteIndex = max(0, min(byteIndex, len(text)))
	return len([]rune(text[:byteIndex]))
}

func runeColToByteIndex(text string, col int) int {
	if col <= 0 {
		return 0
	}
	runeCol := 0
	for idx := range text {
		if runeCol == col {
			return idx
		}
		runeCol++
	}
	return len(text)
}

func (e *Editor) jumpToRune(target rune, direction int) {
	runes := []rune(e.text)
	if len(runes) == 0 || target == 0 {
		return
	}
	if direction >= 0 {
		for pos := min(e.cursor+1, len(runes)); pos < len(runes); pos++ {
			if runes[pos] == target {
				e.cursor = pos
				return
			}
		}
		return
	}
	for pos := min(e.cursor-1, len(runes)-1); pos >= 0; pos-- {
		if runes[pos] == target {
			e.cursor = pos
			return
		}
	}
}

type TextChunk struct {
	Text       string
	StartIndex int
	EndIndex   int
}

func WordWrapLine(line string, maxWidth int) []TextChunk {
	return wordWrapLineWithSegments(line, maxWidth, segmentTextForWrap(line))
}

func wordWrapLineWithSegments(line string, maxWidth int, segments []wrapSegment) []TextChunk {
	if line == "" || maxWidth <= 0 {
		return []TextChunk{{Text: "", StartIndex: 0, EndIndex: 0}}
	}
	if VisibleWidth(line) <= maxWidth {
		return []TextChunk{{Text: line, StartIndex: 0, EndIndex: len(line)}}
	}
	chunks := make([]TextChunk, 0, len(segments))
	currentWidth := 0
	chunkStart := 0
	wrapOppIndex := -1
	wrapOppWidth := 0

	for i, seg := range segments {
		segWidth := VisibleWidth(seg.text)
		isWhitespace := segmentIsWhitespace(seg.text)

		if currentWidth+segWidth > maxWidth {
			if wrapOppIndex >= 0 && currentWidth-wrapOppWidth+segWidth <= maxWidth {
				chunks = append(chunks, TextChunk{
					Text:       line[chunkStart:wrapOppIndex],
					StartIndex: chunkStart,
					EndIndex:   wrapOppIndex,
				})
				chunkStart = wrapOppIndex
				currentWidth -= wrapOppWidth
			} else if chunkStart < seg.index {
				chunks = append(chunks, TextChunk{
					Text:       line[chunkStart:seg.index],
					StartIndex: chunkStart,
					EndIndex:   seg.index,
				})
				chunkStart = seg.index
				currentWidth = 0
			}
			wrapOppIndex = -1
		}

		if segWidth > maxWidth {
			subChunks := splitOversizedWrapSegment(seg.text, maxWidth)
			for _, sub := range subChunks[:max(0, len(subChunks)-1)] {
				chunks = append(chunks, TextChunk{
					Text:       sub.Text,
					StartIndex: seg.index + sub.StartIndex,
					EndIndex:   seg.index + sub.EndIndex,
				})
			}
			last := subChunks[len(subChunks)-1]
			chunkStart = seg.index + last.StartIndex
			currentWidth = VisibleWidth(last.Text)
			wrapOppIndex = -1
			continue
		}

		currentWidth += segWidth
		if isWhitespace && i+1 < len(segments) && !segmentIsWhitespace(segments[i+1].text) {
			wrapOppIndex = segments[i+1].index
			wrapOppWidth = currentWidth
		}
	}
	chunks = append(chunks, TextChunk{Text: line[chunkStart:], StartIndex: chunkStart, EndIndex: len(line)})
	return chunks
}

type wrapSegment struct {
	text  string
	index int
}

func segmentTextForWrap(text string) []wrapSegment {
	segments := make([]wrapSegment, 0, len(text))
	for idx, r := range text {
		segments = append(segments, wrapSegment{text: string(r), index: idx})
	}
	return segments
}

func segmentIsWhitespace(segment string) bool {
	if segment == "" {
		return false
	}
	for _, r := range segment {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func splitOversizedWrapSegment(text string, maxWidth int) []TextChunk {
	if text == "" {
		return []TextChunk{{Text: "", StartIndex: 0, EndIndex: 0}}
	}
	if len([]rune(text)) > 1 {
		return wordWrapLineWithSegments(text, maxWidth, segmentTextForWrap(text))
	}
	var chunks []TextChunk
	start := 0
	width := 0
	for idx, r := range text {
		rw := runeWidth(r)
		if width > 0 && width+rw > maxWidth {
			chunks = append(chunks, TextChunk{Text: text[start:idx], StartIndex: start, EndIndex: idx})
			start = idx
			width = 0
		}
		width += rw
	}
	chunks = append(chunks, TextChunk{Text: text[start:], StartIndex: start, EndIndex: len(text)})
	return chunks
}

type SelectItem struct {
	Value       string
	Label       string
	Description string
}

type SelectListTheme struct {
	SelectedPrefix func(string) string
	SelectedText   func(string) string
	Description    func(string) string
	ScrollInfo     func(string) string
	NoMatch        func(string) string
}

type SelectListLayoutOptions struct {
	MinPrimaryColumnWidth int
	MaxPrimaryColumnWidth int
	TruncatePrimary       func(SelectListTruncatePrimaryContext) string
}

type SelectListTruncatePrimaryContext struct {
	Text        string
	MaxWidth    int
	ColumnWidth int
	Item        SelectItem
	IsSelected  bool
}

const (
	defaultSelectListPrimaryColumnWidth = 32
	selectListPrimaryColumnGap          = 2
	selectListMinDescriptionWidth       = 10
)

type SelectList struct {
	mu                sync.Mutex
	items             []SelectItem
	filtered          []SelectItem
	selectedIndex     int
	maxVisible        int
	theme             SelectListTheme
	layout            SelectListLayoutOptions
	OnSelect          func(SelectItem)
	OnCancel          func()
	OnSelectionChange func(SelectItem)
}

func NewSelectList(items []SelectItem, maxVisible int, theme SelectListTheme, layout ...SelectListLayoutOptions) *SelectList {
	opts := SelectListLayoutOptions{MinPrimaryColumnWidth: defaultSelectListPrimaryColumnWidth, MaxPrimaryColumnWidth: defaultSelectListPrimaryColumnWidth}
	if len(layout) > 0 {
		opts = layout[0]
	}
	if maxVisible <= 0 {
		maxVisible = 5
	}
	filtered := append([]SelectItem(nil), items...)
	return &SelectList{items: append([]SelectItem(nil), items...), filtered: filtered, maxVisible: maxVisible, theme: theme, layout: opts}
}

func (s *SelectList) SetFilter(filter string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lowerFilter := strings.ToLower(filter)
	s.filtered = s.filtered[:0]
	for _, item := range s.items {
		if strings.HasPrefix(strings.ToLower(item.Value), lowerFilter) {
			s.filtered = append(s.filtered, item)
		}
	}
	s.selectedIndex = 0
}

func (s *SelectList) SetSelectedIndex(index int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selectedIndex = max(0, min(index, len(s.filtered)-1))
}

func (s *SelectList) SelectedItem() (SelectItem, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.filtered) == 0 || s.selectedIndex < 0 || s.selectedIndex >= len(s.filtered) {
		return SelectItem{}, false
	}
	return s.filtered[s.selectedIndex], true
}

func (s *SelectList) GetSelectedItem() (SelectItem, bool) {
	return s.SelectedItem()
}

func (s *SelectList) Invalidate() {}
func (s *SelectList) Render(width int) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.filtered) == 0 {
		return []string{style(s.theme.NoMatch, "  No matching commands")}
	}
	start := max(0, min(s.selectedIndex-s.maxVisible/2, len(s.filtered)-s.maxVisible))
	end := min(start+s.maxVisible, len(s.filtered))
	primaryWidth := s.primaryColumnWidth()
	var lines []string
	for idx := start; idx < end; idx++ {
		item := s.filtered[idx]
		lines = append(lines, s.renderItem(item, idx == s.selectedIndex, width, primaryWidth))
	}
	if start > 0 || end < len(s.filtered) {
		lines = append(lines, style(s.theme.ScrollInfo, TruncateToWidth(fmt.Sprintf("  (%d/%d)", s.selectedIndex+1, len(s.filtered)), width-2, "")))
	}
	return lines
}

func (s *SelectList) HandleInput(data string) {
	kb := GetKeybindings()
	var onSelect func(SelectItem)
	var selectItem SelectItem
	var hasSelectItem bool
	var onCancel func()
	var onSelectionChange func(SelectItem)
	var selectionItem SelectItem
	var hasSelectionItem bool
	s.mu.Lock()
	switch {
	case kb.Matches(data, "tui.select.up"):
		if len(s.filtered) > 0 {
			if s.selectedIndex == 0 {
				s.selectedIndex = len(s.filtered) - 1
			} else {
				s.selectedIndex--
			}
			onSelectionChange = s.OnSelectionChange
			selectionItem = s.filtered[s.selectedIndex]
			hasSelectionItem = true
		}
	case kb.Matches(data, "tui.select.down"):
		if len(s.filtered) > 0 {
			s.selectedIndex = (s.selectedIndex + 1) % len(s.filtered)
			onSelectionChange = s.OnSelectionChange
			selectionItem = s.filtered[s.selectedIndex]
			hasSelectionItem = true
		}
	case kb.Matches(data, "tui.select.pageUp"):
		if len(s.filtered) > 0 {
			s.selectedIndex = max(0, s.selectedIndex-max(1, s.maxVisible))
			onSelectionChange = s.OnSelectionChange
			selectionItem = s.filtered[s.selectedIndex]
			hasSelectionItem = true
		}
	case kb.Matches(data, "tui.select.pageDown"):
		if len(s.filtered) > 0 {
			s.selectedIndex = min(len(s.filtered)-1, s.selectedIndex+max(1, s.maxVisible))
			onSelectionChange = s.OnSelectionChange
			selectionItem = s.filtered[s.selectedIndex]
			hasSelectionItem = true
		}
	case kb.Matches(data, "tui.select.confirm"):
		if len(s.filtered) > 0 {
			onSelect = s.OnSelect
			selectItem = s.filtered[s.selectedIndex]
			hasSelectItem = true
		}
	case kb.Matches(data, "tui.select.cancel"):
		onCancel = s.OnCancel
	}
	s.mu.Unlock()
	if hasSelectionItem && onSelectionChange != nil {
		onSelectionChange(selectionItem)
	}
	if hasSelectItem && onSelect != nil {
		onSelect(selectItem)
	}
	if onCancel != nil {
		onCancel()
	}
}

func (s *SelectList) renderItem(item SelectItem, selected bool, width, primaryWidth int) string {
	label := item.Label
	if label == "" {
		label = item.Value
	}
	prefix := "  "
	if selected {
		prefix = "→ "
	}
	prefixWidth := VisibleWidth(prefix)
	if item.Description != "" && width > 40 {
		effectivePrimaryWidth := max(1, min(primaryWidth, width-prefixWidth-4))
		maxPrimary := max(1, effectivePrimaryWidth-selectListPrimaryColumnGap)
		truncatedLabel := s.truncatePrimary(item, label, selected, maxPrimary, effectivePrimaryWidth)
		spacing := strings.Repeat(" ", max(1, effectivePrimaryWidth-VisibleWidth(truncatedLabel)))
		descWidth := width - prefixWidth - VisibleWidth(truncatedLabel) - VisibleWidth(spacing) - 2
		if descWidth > selectListMinDescriptionWidth {
			desc := TruncateToWidth(normalizeSelectListDescription(item.Description), descWidth, "")
			if selected {
				return style(s.theme.SelectedText, prefix+truncatedLabel+spacing+desc)
			}
			return prefix + truncatedLabel + style(s.theme.Description, spacing+desc)
		}
	}
	maxPrimary := width - prefixWidth - 2
	label = s.truncatePrimary(item, label, selected, maxPrimary, maxPrimary)
	if selected {
		return style(s.theme.SelectedText, prefix+label)
	}
	return prefix + label
}

func (s *SelectList) primaryColumnWidth() int {
	minWidth, maxWidth := s.primaryColumnBounds()
	widest := 0
	for _, item := range s.filtered {
		label := item.Label
		if label == "" {
			label = item.Value
		}
		widest = max(widest, VisibleWidth(label)+selectListPrimaryColumnGap)
	}
	return max(minWidth, min(widest, maxWidth))
}

func (s *SelectList) primaryColumnBounds() (int, int) {
	rawMin := s.layout.MinPrimaryColumnWidth
	rawMax := s.layout.MaxPrimaryColumnWidth
	if rawMin == 0 {
		if rawMax == 0 {
			rawMin = defaultSelectListPrimaryColumnWidth
		} else {
			rawMin = rawMax
		}
	}
	if rawMax == 0 {
		rawMax = rawMin
	}
	return max(1, min(rawMin, rawMax)), max(1, max(rawMin, rawMax))
}

func (s *SelectList) truncatePrimary(item SelectItem, label string, selected bool, maxWidth, columnWidth int) string {
	maxWidth = max(1, maxWidth)
	if s.layout.TruncatePrimary != nil {
		label = s.layout.TruncatePrimary(SelectListTruncatePrimaryContext{Text: label, MaxWidth: maxWidth, ColumnWidth: columnWidth, Item: item, IsSelected: selected})
	}
	return TruncateToWidth(label, maxWidth, "")
}

func normalizeSelectListDescription(text string) string {
	var out strings.Builder
	lastWasLineBreak := false
	for _, r := range text {
		if r == '\r' || r == '\n' {
			if !lastWasLineBreak {
				out.WriteByte(' ')
				lastWasLineBreak = true
			}
			continue
		}
		out.WriteRune(r)
		lastWasLineBreak = false
	}
	return strings.TrimSpace(out.String())
}

func (s *SelectList) notifySelectionChange() {
	s.mu.Lock()
	var callback func(SelectItem)
	var item SelectItem
	ok := len(s.filtered) > 0
	if ok {
		callback = s.OnSelectionChange
		item = s.filtered[s.selectedIndex]
	}
	s.mu.Unlock()
	if ok && callback != nil {
		callback(item)
	}
}

type SettingItem struct {
	ID           string
	Label        string
	Description  string
	Value        string
	CurrentValue string
	Values       []string
	Submenu      func(currentValue string, done func(selectedValue string, changed bool)) Component
}

type SettingsListTheme struct {
	Label        func(text string, selected bool) string
	CurrentValue func(text string, selected bool) string
	Description  func(string) string
	Hint         func(string) string
	Selected     func(string) string
	Value        func(string) string
	Cursor       string
}

type SettingsListOptions struct {
	EnableSearch bool
	OnChange     func(id string, newValue string)
	OnCancel     func()
}

type SettingsList struct {
	mu               sync.Mutex
	items            []SettingItem
	filteredIndices  []int
	selectedIndex    int
	maxVisible       int
	theme            SettingsListTheme
	options          SettingsListOptions
	searchInput      *Input
	submenu          Component
	submenuItemIndex int
}

func NewSettingsList(items []SettingItem, maxVisible int, theme SettingsListTheme, options ...SettingsListOptions) *SettingsList {
	opts := SettingsListOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	if maxVisible <= 0 {
		maxVisible = 5
	}
	s := &SettingsList{
		items:            append([]SettingItem(nil), items...),
		maxVisible:       maxVisible,
		theme:            theme,
		options:          opts,
		submenuItemIndex: -1,
	}
	s.resetFilter()
	if opts.EnableSearch {
		s.searchInput = NewInput()
	}
	return s
}

func (s *SettingsList) UpdateValue(id string, newValue string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for idx := range s.items {
		if s.settingID(s.items[idx]) == id {
			s.items[idx].CurrentValue = newValue
			s.items[idx].Value = newValue
			return
		}
	}
}

func (s *SettingsList) Invalidate() {
	s.mu.Lock()
	submenu := s.submenu
	searchInput := s.searchInput
	s.mu.Unlock()
	if submenu != nil {
		submenu.Invalidate()
	}
	if searchInput != nil {
		searchInput.Invalidate()
	}
}

func (s *SettingsList) Render(width int) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.submenu != nil {
		return s.submenu.Render(width)
	}
	var lines []string
	if s.searchInput != nil {
		lines = append(lines, s.searchInput.Render(width)...)
		lines = append(lines, "")
	}
	if len(s.items) == 0 {
		lines = append(lines, style(s.theme.Hint, "  No settings available"))
		if s.searchInput != nil {
			return s.appendSettingsHint(lines, width)
		}
		return lines
	}
	if len(s.filteredIndices) == 0 {
		lines = append(lines, TruncateToWidth(style(s.theme.Hint, "  No matching settings"), width, ""))
		return s.appendSettingsHint(lines, width)
	}

	start := max(0, min(s.selectedIndex-s.maxVisible/2, len(s.filteredIndices)-s.maxVisible))
	end := min(start+s.maxVisible, len(s.filteredIndices))
	labelWidth := s.maxLabelWidth()
	for row := start; row < end; row++ {
		item := s.items[s.filteredIndices[row]]
		selected := row == s.selectedIndex
		prefix := "  "
		if selected {
			prefix = s.theme.Cursor
			if prefix == "" {
				prefix = "→ "
			}
		}
		label := item.Label
		label = label + strings.Repeat(" ", max(0, labelWidth-VisibleWidth(label)))
		label = s.styleSettingLabel(label, selected)
		valueWidth := max(1, width-VisibleWidth(prefix)-labelWidth-4)
		value := TruncateToWidth(s.settingValue(item), valueWidth, "")
		value = s.styleSettingValue(value, selected)
		lines = append(lines, TruncateToWidth(prefix+label+"  "+value, width, ""))
	}
	if start > 0 || end < len(s.filteredIndices) {
		lines = append(lines, style(s.theme.Hint, TruncateToWidth(fmt.Sprintf("  (%d/%d)", s.selectedIndex+1, len(s.filteredIndices)), width, "")))
	}
	item := s.items[s.filteredIndices[s.selectedIndex]]
	if item.Description != "" {
		lines = append(lines, "")
		for _, line := range WrapTextWithANSI(item.Description, max(1, width-4)) {
			lines = append(lines, style(s.theme.Description, "  "+line))
		}
	}
	return s.appendSettingsHint(lines, width)
}

func (s *SettingsList) HandleInput(data string) {
	var after func()
	s.mu.Lock()
	defer func() {
		s.mu.Unlock()
		if after != nil {
			after()
		}
	}()
	if s.submenu != nil {
		if handler, ok := s.submenu.(InputHandler); ok {
			s.mu.Unlock()
			handler.HandleInput(data)
			s.mu.Lock()
		}
		return
	}
	kb := GetKeybindings()
	switch {
	case kb.Matches(data, "tui.select.up"):
		if len(s.filteredIndices) > 0 {
			if s.selectedIndex == 0 {
				s.selectedIndex = len(s.filteredIndices) - 1
			} else {
				s.selectedIndex--
			}
		}
	case kb.Matches(data, "tui.select.down"):
		if len(s.filteredIndices) > 0 {
			s.selectedIndex = (s.selectedIndex + 1) % len(s.filteredIndices)
		}
	case kb.Matches(data, "tui.select.confirm") || data == " ":
		after = s.activateSelectedSetting()
	case kb.Matches(data, "tui.select.cancel"):
		if s.options.OnCancel != nil {
			onCancel := s.options.OnCancel
			after = func() { onCancel() }
		}
	default:
		if s.searchInput == nil {
			return
		}
		sanitized := strings.ReplaceAll(data, " ", "")
		if sanitized == "" {
			return
		}
		s.searchInput.HandleInput(sanitized)
		s.applySettingsFilter(s.searchInput.GetValue())
	}
}

func (s *SettingsList) activateSelectedSetting() func() {
	if len(s.filteredIndices) == 0 {
		return nil
	}
	itemIndex := s.filteredIndices[s.selectedIndex]
	item := &s.items[itemIndex]
	if item.Submenu != nil {
		s.submenuItemIndex = s.selectedIndex
		s.submenu = item.Submenu(s.settingValue(*item), func(selectedValue string, changed bool) {
			var after func()
			s.mu.Lock()
			if changed {
				if itemIndex >= 0 && itemIndex < len(s.items) {
					s.items[itemIndex].CurrentValue = selectedValue
					s.items[itemIndex].Value = selectedValue
					id := s.settingID(s.items[itemIndex])
					if s.options.OnChange != nil {
						onChange := s.options.OnChange
						after = func() { onChange(id, selectedValue) }
					}
				}
			}
			s.submenu = nil
			if s.submenuItemIndex >= 0 && s.submenuItemIndex < len(s.filteredIndices) {
				s.selectedIndex = s.submenuItemIndex
			}
			s.submenuItemIndex = -1
			s.mu.Unlock()
			if after != nil {
				after()
			}
		})
		return nil
	}
	if len(item.Values) == 0 {
		return nil
	}
	current := s.settingValue(*item)
	nextIndex := 0
	for idx, value := range item.Values {
		if value == current {
			nextIndex = (idx + 1) % len(item.Values)
			break
		}
	}
	next := item.Values[nextIndex]
	item.CurrentValue = next
	item.Value = next
	if s.options.OnChange == nil {
		return nil
	}
	id := s.settingID(*item)
	onChange := s.options.OnChange
	return func() { onChange(id, next) }
}

func (s *SettingsList) notifySettingsChange(item SettingItem, value string) {
	if s.options.OnChange != nil {
		s.options.OnChange(s.settingID(item), value)
	}
}

func (s *SettingsList) applySettingsFilter(query string) {
	if strings.TrimSpace(query) == "" {
		s.resetFilter()
		return
	}
	type candidate struct {
		index int
		label string
	}
	candidates := make([]candidate, len(s.items))
	for idx, item := range s.items {
		candidates[idx] = candidate{index: idx, label: item.Label}
	}
	matches := FuzzyFilter(candidates, query, func(item candidate) string { return item.label })
	s.filteredIndices = s.filteredIndices[:0]
	for _, match := range matches {
		s.filteredIndices = append(s.filteredIndices, match.index)
	}
	s.selectedIndex = 0
}

func (s *SettingsList) resetFilter() {
	s.filteredIndices = make([]int, len(s.items))
	for idx := range s.items {
		s.filteredIndices[idx] = idx
	}
	s.selectedIndex = 0
}

func (s *SettingsList) maxLabelWidth() int {
	width := 0
	for _, item := range s.items {
		width = max(width, VisibleWidth(item.Label))
	}
	return min(30, width)
}

func (s *SettingsList) settingID(item SettingItem) string {
	if item.ID != "" {
		return item.ID
	}
	return item.Label
}

func (s *SettingsList) settingValue(item SettingItem) string {
	if item.CurrentValue != "" {
		return item.CurrentValue
	}
	if settingValuesContainEmpty(item.Values) {
		return ""
	}
	return item.Value
}

func settingValuesContainEmpty(values []string) bool {
	for _, value := range values {
		if value == "" {
			return true
		}
	}
	return false
}

func (s *SettingsList) styleSettingLabel(text string, selected bool) string {
	if s.theme.Label != nil {
		return s.theme.Label(text, selected)
	}
	if selected {
		return style(s.theme.Selected, text)
	}
	return text
}

func (s *SettingsList) styleSettingValue(text string, selected bool) string {
	if s.theme.CurrentValue != nil {
		return s.theme.CurrentValue(text, selected)
	}
	if s.theme.Value != nil {
		return s.theme.Value(text)
	}
	return text
}

func (s *SettingsList) appendSettingsHint(lines []string, width int) []string {
	lines = append(lines, "")
	hint := "  Enter/Space to change · Esc to cancel"
	if s.searchInput != nil {
		hint = "  Type to search · Enter/Space to change · Esc to cancel"
	}
	return append(lines, TruncateToWidth(style(s.theme.Hint, hint), width, ""))
}

type ImageOptions struct {
	Alt            string
	MimeType       string
	Filename       string
	MaxWidth       int
	MaxHeight      int
	MaxWidthCells  int
	MaxHeightCells int
	ImageID        uint32
	ImageId        uint32
	Dimensions     *ImageDimensions
}

type ImageTheme struct {
	Fallback      func(string) string
	FallbackColor func(string) string
}

type Image struct {
	mu      sync.Mutex
	Data    []byte
	Options ImageOptions
	Theme   ImageTheme
	imageID uint32
}

func NewImage(data []byte, options ImageOptions, theme ...ImageTheme) *Image {
	img := &Image{Data: append([]byte(nil), data...), Options: options, imageID: imageOptionID(options)}
	if len(theme) > 0 {
		img.Theme = theme[0]
	}
	return img
}

func (i *Image) Invalidate() {}
func (i *Image) Render(width int) []string {
	i.mu.Lock()
	defer i.mu.Unlock()
	caps := GetCapabilities()
	if caps.Images && len(i.Data) > 0 {
		dims := i.imageDimensions()
		maxWidth := i.maxWidthCells(width)
		maxHeight := i.maxHeightCells(maxWidth)
		if caps.Protocol == ImageProtocolKitty {
			if i.imageID == 0 {
				i.imageID = AllocateImageID()
			}
		}
		moveCursor := false
		result := RenderImageWithDimensions(i.Data, dims, ImageRenderOptions{
			ID:             i.imageID,
			MaxWidthCells:  maxWidth,
			MaxHeightCells: maxHeight,
			Alt:            i.Options.Alt,
			Protocol:       caps.Protocol,
			MoveCursor:     &moveCursor,
		})
		if result != nil && IsImageLine(result.Sequence) {
			if result.ImageID > 0 {
				i.imageID = result.ImageID
			}
			lines := make([]string, max(1, result.Rows))
			if caps.Protocol == ImageProtocolITerm {
				rowOffset := max(0, result.Rows-1)
				moveUp := ""
				if rowOffset > 0 {
					moveUp = fmt.Sprintf("\x1b[%dA", rowOffset)
				}
				lines[len(lines)-1] = moveUp + result.Sequence
			} else {
				lines[0] = result.Sequence
			}
			return lines
		}
	}
	text := i.fallbackText()
	if i.Options.MimeType == "" && i.Options.Filename == "" && i.Options.Dimensions == nil {
		text = ImageFallback(text, width)
	}
	if i.Theme.Fallback != nil {
		text = i.Theme.Fallback(text)
	} else if i.Theme.FallbackColor != nil {
		text = i.Theme.FallbackColor(text)
	}
	return []string{text}
}

func (i *Image) ImageID() uint32 {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.imageID
}

func (i *Image) GetImageID() uint32 {
	return i.ImageID()
}

func (i *Image) GetImageId() uint32 {
	return i.ImageID()
}

func (i *Image) imageDimensions() ImageDimensions {
	if i.Options.Dimensions != nil {
		dims := normalizeImageDimensions(*i.Options.Dimensions)
		if dims.Width > 0 && dims.Height > 0 {
			return dims
		}
	}
	if dims, err := GetImageDimensions(i.Data); err == nil {
		return dims
	}
	return ImageDimensions{Width: 800, Height: 600}
}

func (i *Image) maxWidthCells(width int) int {
	maxWidth := i.Options.MaxWidthCells
	if maxWidth <= 0 {
		maxWidth = i.Options.MaxWidth
	}
	if maxWidth <= 0 {
		maxWidth = 60
	}
	return max(1, min(max(1, width-2), maxWidth))
}

func (i *Image) maxHeightCells(maxWidth int) int {
	maxHeight := i.Options.MaxHeightCells
	if maxHeight <= 0 {
		maxHeight = i.Options.MaxHeight
	}
	if maxHeight > 0 {
		return maxHeight
	}
	cell := GetCellDimensions()
	cell = normalizeCellDimensions(cell)
	if cell.Height <= 0 {
		return 1
	}
	return max(1, (maxWidth*cell.Width+cell.Height-1)/cell.Height)
}

func (i *Image) fallbackText() string {
	if i.Options.MimeType != "" || i.Options.Filename != "" || i.Options.Dimensions != nil {
		return ImageFallbackDescription(i.Options.MimeType, i.Options.Dimensions, i.Options.Filename)
	}
	if i.Options.Alt != "" {
		return i.Options.Alt
	}
	return fmt.Sprintf("[image %d bytes]", len(i.Data))
}

func imageOptionID(options ImageOptions) uint32 {
	if options.ImageID > 0 {
		return options.ImageID
	}
	return options.ImageId
}

func style(fn func(string) string, text string) string {
	if fn == nil {
		return text
	}
	return fn(text)
}

func parseHeading(line string) (int, string) {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 {
		return 0, ""
	}
	if level >= len(line) {
		return level, ""
	}
	if !isMarkdownSpaceOrTab(line[level]) {
		return 0, ""
	}
	text := strings.TrimSpace(line[level+1:])
	if text != "" {
		hashStart := len(text)
		for hashStart > 0 && text[hashStart-1] == '#' {
			hashStart--
		}
		if hashStart < len(text) && hashStart > 0 && isMarkdownSpaceOrTab(text[hashStart-1]) {
			text = strings.TrimSpace(text[:hashStart-1])
		}
	}
	return level, text
}

func isMarkdownSpaceOrTab(b byte) bool {
	return b == ' ' || b == '\t'
}

func parseSetextHeading(lines []string, index int) (int, string, bool) {
	if index+1 >= len(lines) {
		return 0, "", false
	}
	text := strings.TrimSpace(strings.TrimRight(lines[index], "\r"))
	if text == "" || startsMarkdownStructuralBlock(text) {
		return 0, "", false
	}
	if _, _, ok := parseMarkdownListLine(lines[index]); ok {
		return 0, "", false
	}
	level, ok := parseSetextUnderline(lines[index+1])
	if !ok {
		return 0, "", false
	}
	return level, text, true
}

func parseSetextUnderline(line string) (int, bool) {
	line = strings.TrimRight(line, "\r")
	if markdownLeadingSpaces(line) > 3 {
		return 0, false
	}
	underline := strings.TrimSpace(line)
	if underline == "" {
		return 0, false
	}
	marker := rune(underline[0])
	if marker != '=' && marker != '-' {
		return 0, false
	}
	for _, r := range underline {
		if r != marker {
			return 0, false
		}
	}
	if marker == '=' {
		return 1, true
	}
	return 2, true
}

func parseMarkdownLinkDefinitionsLegacy(lines []string) map[string]string {
	definitions := map[string]string{}
	for i := 0; i < len(lines); i++ {
		if fence, ok := parseMarkdownFenceStart(lines[i]); ok {
			for i+1 < len(lines) {
				i++
				if isMarkdownFenceClose(lines[i], fence) {
					break
				}
			}
			continue
		}
		if fence, contentIndent, ok := markdownContextualFenceStart(lines, i); ok {
			i = markdownContextualFenceEnd(lines, i, fence, contentIndent)
			continue
		}
		if htmlEnd, ok := markdownHTMLBlockEnd(lines, i); ok {
			i = htmlEnd
			continue
		}
		if htmlEnd, ok := markdownContextualHTMLBlockEnd(lines, i); ok {
			i = htmlEnd
			continue
		}
		if !markdownLinkDefinitionCanStartAt(lines, i) {
			continue
		}
		if definition, end, ok := parseMarkdownTopLevelLinkDefinitionSpan(lines, i); ok {
			addMarkdownLinkDefinition(definitions, definition.label, definition.url)
			i = end
			continue
		}
		if body, ok := parseMarkdownBlockquoteLine(strings.TrimRight(lines[i], "\r")); ok {
			quoteBodies := []string{body}
			end := i
			for end+1 < len(lines) {
				nextBody, explicit := parseMarkdownBlockquoteLine(strings.TrimRight(lines[end+1], "\r"))
				if !explicit {
					break
				}
				quoteBodies = append(quoteBodies, nextBody)
				end++
			}
			for label, url := range parseMarkdownLinkDefinitionsLegacy(quoteBodies) {
				addNormalizedMarkdownLinkDefinition(definitions, label, url)
			}
			i = end
		}
	}
	return definitions
}

func addMarkdownLinkDefinition(definitions map[string]string, label, url string) {
	addNormalizedMarkdownLinkDefinition(definitions, normalizeMarkdownReference(label), url)
}

func addNormalizedMarkdownLinkDefinition(definitions map[string]string, normalizedLabel, url string) {
	if _, exists := definitions[normalizedLabel]; exists {
		return
	}
	definitions[normalizedLabel] = url
}

func markdownContextualFenceStart(lines []string, index int) (markdownFence, int, bool) {
	body, contentIndent, ok := markdownContextualContainerBodyLine(lines, index)
	if !ok {
		return markdownFence{}, 0, false
	}
	fence, ok := parseMarkdownFenceStart(body)
	if !ok {
		return markdownFence{}, 0, false
	}
	return fence, contentIndent, true
}

func markdownContextualFenceEnd(lines []string, index int, fence markdownFence, contentIndent int) int {
	end := index
	for end+1 < len(lines) {
		end++
		body, ok := markdownContainerBodyForIndent(lines[end], contentIndent)
		if !ok {
			continue
		}
		if isMarkdownFenceClose(body, fence) {
			break
		}
	}
	return end
}

func markdownContextualHTMLBlockEnd(lines []string, index int) (int, bool) {
	body, contentIndent, ok := markdownContextualContainerBodyLine(lines, index)
	if !ok {
		return 0, false
	}
	_, end, ok := markdownListHTMLBlock(lines, index, body, contentIndent)
	return end, ok
}

func markdownContextualContainerBodyLine(lines []string, index int) (string, int, bool) {
	if index < 0 || index >= len(lines) {
		return "", 0, false
	}
	line := strings.TrimRight(lines[index], "\r")
	indent := markdownLeadingSpaces(line)
	if indent <= 3 || indent >= len(line) {
		return "", 0, false
	}
	contentIndent, ok := markdownPreviousListContentIndent(lines, index, indent)
	if !ok {
		return "", 0, false
	}
	return line[contentIndent:], contentIndent, true
}

func markdownContainerBodyForIndent(line string, contentIndent int) (string, bool) {
	line = strings.TrimRight(line, "\r")
	if strings.TrimSpace(line) == "" {
		return "", true
	}
	if markdownLeadingSpaces(line) < contentIndent {
		return line, false
	}
	return line[contentIndent:], true
}

func isMarkdownLinkDefinition(line string) bool {
	_, _, ok := parseMarkdownLinkDefinition(line)
	return ok
}

func parseMarkdownLinkDefinition(line string) (label, url string, ok bool) {
	definition, ok := parseMarkdownLinkDefinitionInfo(line)
	if !ok {
		return "", "", false
	}
	return definition.label, definition.url, true
}

type markdownLinkDefinitionInfo struct {
	label    string
	url      string
	hasTitle bool
}

func parseMarkdownLinkDefinitionInfo(line string) (markdownLinkDefinitionInfo, bool) {
	line = strings.TrimRight(line, "\r")
	if markdownLeadingSpaces(line) > 3 {
		return markdownLinkDefinitionInfo{}, false
	}
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "[") {
		return markdownLinkDefinitionInfo{}, false
	}
	close := findMarkdownInlineLabelEnd(line, 1)
	if close <= 1 || close+1 >= len(line) || line[close+1] != ':' {
		return markdownLinkDefinitionInfo{}, false
	}
	label := line[1:close]
	rest := strings.TrimSpace(line[close+2:])
	if label == "" || rest == "" || markdownLabelHasUnescapedBracket(label) {
		return markdownLinkDefinitionInfo{}, false
	}
	url := ""
	remainder := ""
	if strings.HasPrefix(rest, "<") {
		end := findMarkdownAngleDestinationEnd(rest, 1)
		if end >= 1 {
			url = unescapeMarkdownDestination(rest[1:end])
			remainder = strings.TrimSpace(rest[end+1:])
		} else {
			return markdownLinkDefinitionInfo{}, false
		}
	} else {
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			return markdownLinkDefinitionInfo{}, false
		}
		url = unescapeMarkdownDestination(fields[0])
		remainder = strings.TrimSpace(strings.TrimPrefix(rest, fields[0]))
	}
	if remainder != "" && !isMarkdownLinkTitleText(remainder) {
		return markdownLinkDefinitionInfo{}, false
	}
	return markdownLinkDefinitionInfo{
		label:    label,
		url:      url,
		hasTitle: isMarkdownLinkTitleText(remainder),
	}, true
}

func parseMarkdownContextualLinkDefinitionInfo(lines []string, index int) (markdownLinkDefinitionInfo, int, bool) {
	definition, indent, _, ok := parseMarkdownContextualLinkDefinitionSpan(lines, index)
	return definition, indent, ok
}

func parseMarkdownContextualLinkDefinitionSpan(lines []string, index int) (markdownLinkDefinitionInfo, int, int, bool) {
	if index < 0 || index >= len(lines) {
		return markdownLinkDefinitionInfo{}, 0, 0, false
	}
	if definition, end, ok := parseMarkdownLinkDefinitionSpan(lines, index, func(lineIndex int) (string, bool) {
		line := strings.TrimRight(lines[lineIndex], "\r")
		if markdownLeadingSpaces(line) > 3 {
			return "", false
		}
		return line, true
	}, func(lineIndex int) bool {
		return isMarkdownLinkDefinitionTitleLine(lines[lineIndex])
	}); ok {
		return definition, 0, end, true
	}
	line := strings.TrimRight(lines[index], "\r")
	indent := markdownLeadingSpaces(line)
	if indent <= 3 || indent >= len(line) {
		return markdownLinkDefinitionInfo{}, 0, 0, false
	}
	contentIndent, ok := markdownPreviousListContentIndent(lines, index, indent)
	if !ok || indent < contentIndent || indent > contentIndent+3 {
		return markdownLinkDefinitionInfo{}, 0, 0, false
	}
	definition, end, ok := parseMarkdownLinkDefinitionSpan(lines, index, func(lineIndex int) (string, bool) {
		return markdownContainerBodyForIndent(lines[lineIndex], contentIndent)
	}, func(lineIndex int) bool {
		return isMarkdownContextualLinkDefinitionTitleLine(lines[lineIndex], contentIndent)
	})
	if !ok {
		return markdownLinkDefinitionInfo{}, 0, 0, false
	}
	return definition, contentIndent, end, true
}

func parseMarkdownLinkDefinitionSpan(lines []string, index int, bodyForLine func(int) (string, bool), titleLine func(int) bool) (markdownLinkDefinitionInfo, int, bool) {
	var candidate strings.Builder
	for end := index; end < len(lines); end++ {
		body, ok := bodyForLine(end)
		if !ok {
			break
		}
		body = strings.TrimRight(body, "\r")
		if end == index && !strings.HasPrefix(strings.TrimSpace(body), "[") {
			return markdownLinkDefinitionInfo{}, 0, false
		}
		if end > index {
			if strings.TrimSpace(body) == "" {
				break
			}
			candidate.WriteByte('\n')
		}
		candidate.WriteString(body)
		if candidate.Len() > 4096 {
			break
		}
		definition, ok := parseMarkdownLinkDefinitionInfo(candidate.String())
		if !ok {
			continue
		}
		if !definition.hasTitle && end+1 < len(lines) && titleLine(end+1) {
			return definition, end + 1, true
		}
		return definition, end, true
	}
	return markdownLinkDefinitionInfo{}, 0, false
}

func parseMarkdownTopLevelLinkDefinitionSpan(lines []string, index int) (markdownLinkDefinitionInfo, int, bool) {
	return parseMarkdownLinkDefinitionSpan(lines, index, func(lineIndex int) (string, bool) {
		line := strings.TrimRight(lines[lineIndex], "\r")
		if markdownLeadingSpaces(line) > 3 {
			return "", false
		}
		return line, true
	}, func(lineIndex int) bool {
		return isMarkdownLinkDefinitionTitleLine(lines[lineIndex])
	})
}

func isMarkdownContextualLinkDefinitionTitleLine(line string, indent int) bool {
	if indent <= 0 {
		return isMarkdownLinkDefinitionTitleLine(line)
	}
	line = strings.TrimRight(line, "\r")
	if markdownLeadingSpaces(line) < indent {
		return false
	}
	return isMarkdownLinkTitleText(strings.TrimSpace(line[indent:]))
}

func markdownLinkDefinitionEnd(lines []string, index int) (int, bool) {
	if index < 0 || index >= len(lines) {
		return 0, false
	}
	if !markdownLinkDefinitionCanStartAt(lines, index) {
		return 0, false
	}
	_, end, ok := parseMarkdownTopLevelLinkDefinitionSpan(lines, index)
	if !ok {
		return 0, false
	}
	return end, true
}

func markdownLinkDefinitionCanStartAt(lines []string, index int) bool {
	if index <= 0 {
		return true
	}
	if index >= len(lines) {
		return false
	}
	prev := strings.TrimRight(lines[index-1], "\r")
	prevTrimmed := strings.TrimSpace(prev)
	if prevTrimmed == "" {
		return true
	}
	if _, _, ok := parseMarkdownLinkDefinition(prev); ok && markdownLinkDefinitionCanStartAt(lines, index-1) {
		return true
	}
	if isMarkdownLinkDefinitionTitleLine(prev) {
		for start := index - 2; start >= 0 && strings.TrimSpace(lines[start]) != ""; start-- {
			if _, end, ok := parseMarkdownTopLevelLinkDefinitionSpan(lines, start); ok && end == index-1 && markdownLinkDefinitionCanStartAt(lines, start) {
				return true
			}
		}
	}
	if level, _ := parseHeading(prevTrimmed); level > 0 {
		return true
	}
	if isHorizontalRule(prevTrimmed) {
		return true
	}
	return false
}

var markdownHTMLBlockTags = map[string]struct{}{
	"address": {}, "article": {}, "aside": {}, "base": {}, "basefont": {},
	"blockquote": {}, "body": {}, "caption": {}, "center": {}, "col": {},
	"colgroup": {}, "dd": {}, "details": {}, "dialog": {}, "dir": {},
	"div": {}, "dl": {}, "dt": {}, "fieldset": {}, "figcaption": {},
	"figure": {}, "footer": {}, "form": {}, "frame": {}, "frameset": {},
	"h1": {}, "h2": {}, "h3": {}, "h4": {}, "h5": {}, "h6": {},
	"head": {}, "header": {}, "hr": {}, "html": {}, "iframe": {},
	"legend": {}, "li": {}, "link": {}, "main": {}, "menu": {},
	"menuitem": {}, "nav": {}, "noframes": {}, "ol": {}, "optgroup": {},
	"option": {}, "p": {}, "param": {}, "search": {}, "section": {},
	"summary": {}, "table": {}, "tbody": {}, "td": {}, "tfoot": {},
	"th": {}, "thead": {}, "title": {}, "tr": {}, "track": {}, "ul": {},
}

func markdownHTMLType1Tag(lower string) (string, bool) {
	if !strings.HasPrefix(lower, "<") {
		return "", false
	}
	for _, tag := range []string{"pre", "script", "style", "textarea"} {
		prefix := "<" + tag
		if !strings.HasPrefix(lower, prefix) {
			continue
		}
		pos := len(prefix)
		if pos == len(lower) || lower[pos] == ' ' || lower[pos] == '\t' || lower[pos] == '>' {
			return tag, true
		}
	}
	return "", false
}

func isMarkdownHTMLType1TagName(tag string) bool {
	switch tag {
	case "pre", "script", "style", "textarea":
		return true
	default:
		return false
	}
}

func markdownHTMLBlockTerminator(lower string) (string, bool) {
	if tag, ok := markdownHTMLType1Tag(lower); ok {
		return "</" + tag + ">", true
	}
	switch {
	case isMarkdownHTMLCommentStart(lower):
		return "-->", true
	case strings.HasPrefix(lower, "<?"):
		return "?>", true
	case strings.HasPrefix(lower, "<![cdata["):
		return "]]>", true
	case strings.HasPrefix(lower, "<!") && len(lower) > 2 && isMarkdownASCIILetter(lower[2]):
		return ">", true
	default:
		return "", false
	}
}

func markdownHTMLBlockEnd(lines []string, index int) (int, bool) {
	if index < 0 || index >= len(lines) {
		return 0, false
	}
	line := strings.TrimRight(lines[index], "\r")
	if markdownLeadingSpaces(line) > 3 {
		return 0, false
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || !strings.HasPrefix(trimmed, "<") {
		return 0, false
	}
	lower := strings.ToLower(trimmed)
	if terminator, ok := markdownHTMLBlockTerminator(lower); ok {
		return markdownHTMLBlockEndByTerminator(lines, index, terminator), true
	}
	_, ok := markdownHTMLBlockTag(lower)
	if !ok {
		return 0, false
	}
	end := index
	for end+1 < len(lines) && strings.TrimSpace(strings.TrimRight(lines[end+1], "\r")) != "" {
		end++
	}
	return end, true
}

func markdownHTMLBlockCanInterruptParagraph(line string) bool {
	line = strings.TrimRight(line, "\r")
	if markdownLeadingSpaces(line) > 3 {
		return false
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || !strings.HasPrefix(trimmed, "<") {
		return false
	}
	lower := strings.ToLower(trimmed)
	if _, ok := markdownHTMLBlockTerminator(lower); ok {
		return true
	}
	tag, ok := markdownHTMLBlockTag(lower)
	if !ok {
		return false
	}
	_, ok = markdownHTMLBlockTags[tag]
	return ok
}

func markdownHTMLBlockEndByTerminator(lines []string, index int, terminator string) int {
	terminator = strings.ToLower(terminator)
	for i := index; i < len(lines); i++ {
		if strings.Contains(strings.ToLower(strings.TrimRight(lines[i], "\r")), terminator) {
			return i
		}
	}
	return len(lines) - 1
}

func markdownListHTMLBlock(lines []string, index int, firstBody string, indent int) ([]string, int, bool) {
	candidates := []string{firstBody}
	endIndex := index
	terminator, terminatorBased := markdownHTMLBlockTerminator(strings.ToLower(strings.TrimSpace(firstBody)))
	terminator = strings.ToLower(terminator)
	collectMore := true
	if terminatorBased && strings.Contains(strings.ToLower(strings.TrimRight(firstBody, "\r")), terminator) {
		collectMore = false
	}
	if collectMore {
		for j := index + 1; j < len(lines); j++ {
			line := strings.TrimRight(lines[j], "\r")
			if strings.TrimSpace(line) == "" {
				candidates = append(candidates, "")
				endIndex = j
				if !terminatorBased {
					break
				}
				continue
			}
			if !hasMarkdownListContinuationIndent(line, indent) {
				break
			}
			body := trimMarkdownListCodeIndent(line, indent)
			candidates = append(candidates, body)
			endIndex = j
			if terminatorBased && strings.Contains(strings.ToLower(strings.TrimRight(body, "\r")), terminator) {
				break
			}
		}
	}
	htmlEnd, ok := markdownHTMLBlockEnd(candidates, 0)
	if !ok {
		return nil, index, false
	}
	if htmlEnd < len(candidates)-1 {
		endIndex = index + htmlEnd
	}
	return candidates[:htmlEnd+1], endIndex, true
}

func markdownListBlockquote(lines []string, index int, sourceLine string, firstBody string) ([]string, int, bool) {
	firstQuoteBody, ok := parseMarkdownBlockquoteLine(firstBody)
	if !ok {
		return nil, index, false
	}
	contentIndent, ok := markdownListSourceContentIndent(sourceLine)
	if !ok {
		return nil, index, false
	}
	quoteBodies := []string{firstQuoteBody}
	endIndex := index
	for j := index + 1; j < len(lines); j++ {
		line := strings.TrimRight(lines[j], "\r")
		if strings.TrimSpace(line) == "" {
			break
		}
		if !hasMarkdownListContinuationIndent(line, contentIndent) {
			break
		}
		body := trimMarkdownListCodeIndent(line, contentIndent)
		quoteBody, ok := parseMarkdownBlockquoteLine(body)
		if !ok {
			break
		}
		quoteBodies = append(quoteBodies, quoteBody)
		endIndex = j
	}
	return quoteBodies, endIndex, true
}

func markdownHTMLBlockTag(lower string) (string, bool) {
	if !strings.HasPrefix(lower, "<") {
		return "", false
	}
	pos := 1
	if pos < len(lower) && lower[pos] == '/' {
		pos++
	}
	if pos >= len(lower) || lower[pos] < 'a' || lower[pos] > 'z' {
		return "", false
	}
	start := pos
	for pos < len(lower) {
		ch := lower[pos]
		if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '-' && ch != '_' {
			break
		}
		pos++
	}
	if pos == start {
		return "", false
	}
	tag := lower[start:pos]
	if pos >= len(lower) {
		return "", false
	}
	if lower[pos] != '>' && lower[pos] != '/' && !isMarkdownASCIIWhitespace(lower[pos]) {
		return "", false
	}
	if _, ok := markdownHTMLBlockTags[tag]; ok {
		if markdownHTMLStandardBlockTagStart(lower, pos) {
			return tag, true
		}
		return "", false
	}
	if isMarkdownHTMLType1TagName(tag) {
		return "", false
	}
	if lower[1] == '/' {
		if markdownHTMLBlockClosingTagRest(lower, pos) {
			return tag, true
		}
		return "", false
	}
	if end, ok := findMarkdownInlineOpenTagEnd(lower, pos); ok && strings.TrimSpace(lower[end+1:]) == "" {
		return tag, true
	}
	return "", false
}

func markdownHTMLStandardBlockTagStart(lower string, pos int) bool {
	if pos >= len(lower) {
		return false
	}
	switch lower[pos] {
	case '>', ' ', '\n':
		return true
	case '/':
		return pos+1 < len(lower) && lower[pos+1] == '>'
	default:
		return false
	}
}

func markdownHTMLBlockClosingTagRest(lower string, pos int) bool {
	for pos < len(lower) && isMarkdownASCIIWhitespace(lower[pos]) {
		pos++
	}
	return pos < len(lower) && lower[pos] == '>' && strings.TrimSpace(lower[pos+1:]) == ""
}

func parseMarkdownBlockquoteLinkDefinition(lines []string, index int) (markdownLinkDefinitionInfo, int, bool) {
	line := strings.TrimRight(lines[index], "\r")
	body, explicit := parseMarkdownBlockquoteLine(line)
	if !explicit {
		return markdownLinkDefinitionInfo{}, 0, false
	}
	definition, ok := parseMarkdownLinkDefinitionInfo(body)
	if !ok {
		return markdownLinkDefinitionInfo{}, 0, false
	}
	end := index
	if !definition.hasTitle && index+1 < len(lines) {
		nextLine := strings.TrimRight(lines[index+1], "\r")
		if nextBody, nextExplicit := parseMarkdownBlockquoteLine(nextLine); nextExplicit && isMarkdownLinkDefinitionTitleLine(nextBody) {
			end = index + 1
		}
	}
	return definition, end, true
}

func isMarkdownLinkDefinitionTitleLine(line string) bool {
	line = strings.TrimRight(line, "\r")
	if markdownLeadingSpaces(line) > 3 {
		return false
	}
	return isMarkdownLinkTitleText(strings.TrimSpace(line))
}

func isMarkdownLinkTitleText(text string) bool {
	if len(text) < 2 {
		return false
	}
	switch text[0] {
	case '"':
		return text[len(text)-1] == '"' && isMarkdownDelimitedTitleContent(text[1:len(text)-1], '"')
	case '\'':
		return text[len(text)-1] == '\'' && isMarkdownDelimitedTitleContent(text[1:len(text)-1], '\'')
	case '(':
		return text[len(text)-1] == ')' && isMarkdownDelimitedTitleContent(text[1:len(text)-1], ')')
	default:
		return false
	}
}

func isMarkdownDelimitedTitleContent(text string, delimiter byte) bool {
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\\':
			if i+1 < len(text) {
				i++
			}
		case delimiter:
			return false
		}
	}
	return true
}

func parseMarkdownInlineDestination(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", true
	}
	if strings.HasPrefix(raw, "<") {
		end := findMarkdownAngleDestinationEnd(raw, 1)
		if end >= 1 {
			remainder := strings.TrimSpace(raw[end+1:])
			if remainder != "" && !isMarkdownLinkTitleText(remainder) {
				return "", false
			}
			return unescapeMarkdownDestination(raw[1:end]), true
		}
		return "", false
	}
	for i, r := range raw {
		if unicode.IsSpace(r) {
			if i == 0 {
				return "", false
			}
			remainder := strings.TrimSpace(raw[i:])
			if remainder != "" && !isMarkdownLinkTitleText(remainder) {
				return "", false
			}
			return unescapeMarkdownDestination(raw[:i]), true
		}
	}
	return unescapeMarkdownDestination(raw), true
}

func unescapeMarkdownDestination(text string) string {
	return unescapeMarkdownPunctuation(text)
}

func findMarkdownAngleDestinationEnd(text string, start int) int {
	for i := start; i < len(text); i++ {
		if text[i] == '>' && !isEscapedMarkdownByte(text, i) {
			return i
		}
	}
	return -1
}

func unescapeMarkdownPunctuation(text string) string {
	if text == "" || !strings.Contains(text, "\\") {
		return text
	}
	var out strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] == '\\' && i+1 < len(text) && isMarkdownEscapablePunctuation(text[i+1]) {
			out.WriteByte(text[i+1])
			i++
			continue
		}
		out.WriteByte(text[i])
	}
	return out.String()
}

func normalizeMarkdownReference(label string) string {
	return strings.ToLower(strings.Join(strings.Fields(label), " "))
}

func isHorizontalRule(line string) bool {
	marker := rune(0)
	count := 0
	for _, r := range line {
		if unicode.IsSpace(r) {
			continue
		}
		if r != '-' && r != '_' && r != '*' {
			return false
		}
		if marker == 0 {
			marker = r
		} else if r != marker {
			return false
		}
		count++
	}
	if count < 3 {
		return false
	}
	return true
}

func nextNonBlankMarkdownLine(lines []string, start int) int {
	for i := start; i < len(lines); i++ {
		if strings.TrimSpace(strings.TrimRight(lines[i], "\r")) != "" {
			return i
		}
	}
	return -1
}

func stripMarkdownHardBreakMarker(line string, lines []string, index int) string {
	if index+1 >= len(lines) {
		return line
	}
	if strings.TrimSpace(strings.TrimRight(lines[index+1], "\r")) == "" {
		return line
	}
	if strings.HasSuffix(line, "\\") {
		return strings.TrimSuffix(line, "\\")
	}
	withoutCR := strings.TrimRight(line, "\r")
	if strings.HasSuffix(withoutCR, "  ") {
		return strings.TrimRight(withoutCR, " ")
	}
	return line
}

func startsMarkdownStructuralBlock(trimmed string) bool {
	if trimmed == "" {
		return false
	}
	if _, ok := parseMarkdownFenceStart(trimmed); ok {
		return true
	}
	if strings.HasPrefix(trimmed, ">") {
		return true
	}
	if level, _ := parseHeading(trimmed); level > 0 {
		return true
	}
	if isHorizontalRule(trimmed) {
		return true
	}
	return false
}

func isMarkdownParagraphContinuationLine(lines []string, index int) bool {
	if index < 0 || index >= len(lines) {
		return false
	}
	line := strings.TrimRight(lines[index], "\r")
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	if _, ok := markdownHTMLBlockEnd(lines, index); ok && markdownHTMLBlockCanInterruptParagraph(lines[index]) {
		return false
	}
	if markdownTableEnd(lines, index) > index {
		return false
	}
	if isIndentedMarkdownCodeLine(line) {
		return false
	}
	if _, ok := parseMarkdownFenceStart(line); ok {
		return false
	}
	if level, _ := parseHeading(trimmed); level > 0 {
		return false
	}
	if isHorizontalRule(trimmed) {
		return false
	}
	renderLine := stripMarkdownHardBreakMarker(line, lines, index)
	if _, ok := parseMarkdownBlockquoteLine(renderLine); ok {
		return false
	}
	if info, ok := parseMarkdownListLineInfo(renderLine); ok {
		return !isMarkdownInterruptingListStart(info)
	}
	return true
}

func isMarkdownLazyBlockquoteContinuation(lines []string, index int) bool {
	if index < 0 || index >= len(lines) {
		return false
	}
	line := strings.TrimRight(lines[index], "\r")
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || startsMarkdownStructuralBlock(trimmed) {
		return false
	}
	if _, ok := markdownLinkDefinitionEnd(lines, index); ok {
		return false
	}
	if markdownHTMLBlockCanInterruptParagraph(line) {
		return false
	}
	if _, ok := parseMarkdownListLineInfo(line); ok {
		return false
	}
	if markdownTableEnd(lines, index) > index {
		return false
	}
	return true
}

func isMarkdownLazyListContinuation(lines []string, index int) bool {
	if index < 0 || index >= len(lines) {
		return false
	}
	line := strings.TrimRight(lines[index], "\r")
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || isIndentedMarkdownCodeLine(line) || startsMarkdownStructuralBlock(trimmed) {
		return false
	}
	if _, ok := markdownLinkDefinitionEnd(lines, index); ok {
		return false
	}
	if markdownHTMLBlockCanInterruptParagraph(line) {
		return false
	}
	if _, ok := parseMarkdownListLineInfo(line); ok {
		return false
	}
	if markdownTableEnd(lines, index) > index {
		return false
	}
	return true
}

type markdownFence struct {
	char   byte
	length int
	lang   string
	indent int
}

func parseMarkdownFenceStart(line string) (markdownFence, bool) {
	line = strings.TrimRight(line, "\r")
	indent := markdownLeadingSpaces(line)
	if indent > 3 {
		return markdownFence{}, false
	}
	line = line[indent:]
	if len(line) < 3 {
		return markdownFence{}, false
	}
	char := line[0]
	if char != '`' && char != '~' {
		return markdownFence{}, false
	}
	length := 0
	for length < len(line) && line[length] == char {
		length++
	}
	if length < 3 {
		return markdownFence{}, false
	}
	lang := strings.TrimSpace(line[length:])
	if char == '`' && strings.Contains(lang, "`") {
		return markdownFence{}, false
	}
	return markdownFence{char: char, length: length, lang: lang, indent: indent}, true
}

func isMarkdownFenceClose(line string, fence markdownFence) bool {
	line = strings.TrimRight(line, "\r")
	indent := markdownLeadingSpaces(line)
	if indent > 3 {
		return false
	}
	line = line[indent:]
	if len(line) < fence.length {
		return false
	}
	length := 0
	for length < len(line) && line[length] == fence.char {
		length++
	}
	return length >= fence.length && strings.TrimSpace(line[length:]) == ""
}

func trimMarkdownFenceContentLine(line string, fence markdownFence) string {
	remove := 0
	for remove < len(line) && remove < fence.indent && line[remove] == ' ' {
		remove++
	}
	return line[remove:]
}

func markdownLeadingSpaces(line string) int {
	count := 0
	for count < len(line) && line[count] == ' ' {
		count++
	}
	return count
}

func renderMarkdownFenceBorder(lang string) string {
	if lang == "" {
		return "```"
	}
	return "```" + lang
}

func isIndentedMarkdownCodeLine(line string) bool {
	return strings.HasPrefix(line, "    ") && strings.TrimSpace(line) != ""
}

func collectIndentedMarkdownCodeBlock(lines []string, start int, breakOnList bool) ([]string, int) {
	codeLines := []string{}
	lastIncluded := start - 1
	pendingBlankLines := 0
	for i := start; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\r")
		if strings.TrimSpace(line) == "" {
			pendingBlankLines++
			continue
		}
		if !isIndentedMarkdownCodeLine(line) {
			break
		}
		if breakOnList {
			if _, _, isList := parseMarkdownListLine(line); isList {
				break
			}
		}
		for pendingBlankLines > 0 {
			codeLines = append(codeLines, "")
			pendingBlankLines--
		}
		codeLines = append(codeLines, trimIndentedMarkdownCodeLine(line))
		lastIncluded = i
	}
	return codeLines, lastIncluded
}

func trimIndentedMarkdownCodeLine(line string) string {
	if strings.HasPrefix(line, "    ") {
		return line[4:]
	}
	return line
}

func parseMarkdownListLine(line string) (prefix, body string, ok bool) {
	info, ok := parseMarkdownListLineInfo(line)
	if !ok {
		return "", "", false
	}
	return info.prefix, info.body, true
}

type markdownListLineInfo struct {
	prefix     string
	body       string
	ordered    bool
	number     int
	indent     int
	leading    int
	taskMarker string
}

func (info markdownListLineInfo) withIndent(indent int) markdownListLineInfo {
	marker := strings.TrimLeft(info.prefix, " ")
	info.prefix = strings.Repeat(" ", indent) + marker
	info.indent = indent
	return info
}

func parseMarkdownListLineInfo(line string) (markdownListLineInfo, bool) {
	leading := len(line) - len(strings.TrimLeft(line, " "))
	trimmed := strings.TrimLeft(line, " ")
	marker, markerEnd, ordered, number, ok := parseMarkdownListMarker(trimmed)
	if !ok {
		return markdownListLineInfo{}, false
	}
	body := trimmed[markerEnd:]
	taskMarker := ""
	if markerText, rest, ok := parseMarkdownTaskMarker(body); ok {
		taskMarker = markerText
		body = rest
	}
	indent := (leading / 2) * 4
	return markdownListLineInfo{
		prefix:     strings.Repeat(" ", indent) + marker + taskMarker,
		body:       body,
		ordered:    ordered,
		number:     number,
		indent:     indent,
		leading:    leading,
		taskMarker: taskMarker,
	}, true
}

func parseMarkdownTaskMarker(body string) (marker string, rest string, ok bool) {
	if len(body) < 4 || body[0] != '[' || body[2] != ']' || body[3] != ' ' {
		return "", body, false
	}
	switch body[1] {
	case ' ':
		return "[ ] ", body[4:], true
	case 'x', 'X':
		return "[x] ", body[4:], true
	default:
		return "", body, false
	}
}

func parseMarkdownListMarker(trimmed string) (marker string, markerEnd int, ordered bool, number int, ok bool) {
	if len(trimmed) > 0 && (trimmed[0] == '-' || trimmed[0] == '*' || trimmed[0] == '+') && (len(trimmed) == 1 || trimmed[1] == ' ') {
		marker = "- "
		markerEnd = 1
		if len(trimmed) > markerEnd && trimmed[markerEnd] == ' ' {
			markerEnd++
		}
		return marker, markerEnd, false, 0, true
	}
	separator := markdownOrderedListSeparator(trimmed)
	if separator <= 0 || (separator+1 < len(trimmed) && trimmed[separator+1] != ' ') {
		return "", 0, false, 0, false
	}
	allDigits := true
	for _, r := range trimmed[:separator] {
		if r < '0' || r > '9' {
			allDigits = false
			break
		}
	}
	if !allDigits || separator > 9 {
		return "", 0, false, 0, false
	}
	parsed, err := strconv.Atoi(trimmed[:separator])
	if err != nil {
		return "", 0, false, 0, false
	}
	marker = strconv.Itoa(parsed) + ". "
	markerEnd = separator + 1
	if len(trimmed) > markerEnd && trimmed[markerEnd] == ' ' {
		markerEnd++
	}
	return marker, markerEnd, true, parsed, true
}

func markdownListSourceContentIndent(line string) (int, bool) {
	leading := markdownLeadingSpaces(line)
	trimmed := strings.TrimLeft(line, " ")
	_, markerEnd, _, _, ok := parseMarkdownListMarker(trimmed)
	if !ok {
		return 0, false
	}
	return leading + markerEnd, true
}

func markdownPreviousListContentIndent(lines []string, index int, indent int) (int, bool) {
	for i := index - 1; i >= 0; i-- {
		line := strings.TrimRight(lines[i], "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if contentIndent, ok := markdownListSourceContentIndent(line); ok {
			return contentIndent, indent >= contentIndent && indent <= contentIndent+3
		}
		if markdownLeadingSpaces(line) < indent {
			return 0, false
		}
	}
	return 0, false
}

func markdownOrderedListSeparator(trimmed string) int {
	dot := strings.IndexByte(trimmed, '.')
	paren := strings.IndexByte(trimmed, ')')
	if dot < 0 {
		return paren
	}
	if paren >= 0 && paren < dot {
		return paren
	}
	return dot
}

type markdownListIndentTracker struct {
	sourceIndents []int
}

func (t *markdownListIndentTracker) clear() {
	t.sourceIndents = nil
}

func (t *markdownListIndentTracker) indentFor(sourceIndent int, inListContext bool) int {
	if !inListContext || len(t.sourceIndents) == 0 {
		t.sourceIndents = []int{sourceIndent}
		return 0
	}
	for len(t.sourceIndents) > 0 && sourceIndent < t.sourceIndents[len(t.sourceIndents)-1] {
		t.sourceIndents = t.sourceIndents[:len(t.sourceIndents)-1]
	}
	if len(t.sourceIndents) == 0 {
		t.sourceIndents = []int{sourceIndent}
		return 0
	}
	if sourceIndent > t.sourceIndents[len(t.sourceIndents)-1] {
		t.sourceIndents = append(t.sourceIndents, sourceIndent)
	}
	return (len(t.sourceIndents) - 1) * 4
}

type markdownListOrderState map[int]int

func (s markdownListOrderState) clear() {
	for indent := range s {
		delete(s, indent)
	}
}

func (s markdownListOrderState) prefix(info markdownListLineInfo) string {
	for indent := range s {
		if indent > info.indent {
			delete(s, indent)
		}
	}
	if !info.ordered {
		delete(s, info.indent)
		return info.prefix
	}
	number := info.number
	if next, ok := s[info.indent]; ok {
		number = next
	}
	s[info.indent] = number + 1
	return strings.Repeat(" ", info.indent) + strconv.Itoa(number) + ". " + info.taskMarker
}

func parseMarkdownBlockquoteLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, ">") {
		return "", false
	}
	body := strings.TrimPrefix(trimmed, ">")
	body = strings.TrimPrefix(body, " ")
	return body, true
}

func trimMarkdownListCodeIndent(line string, indent int) string {
	removed := 0
	for removed < len(line) && removed < indent && line[removed] == ' ' {
		removed++
	}
	return line[removed:]
}

func hasMarkdownListContinuationIndent(line string, indent int) bool {
	if indent <= 0 {
		return true
	}
	if len(line) < indent {
		return false
	}
	for i := 0; i < indent; i++ {
		if line[i] != ' ' {
			return false
		}
	}
	return true
}

func isMarkdownListContinuationStart(line string, sourceIndent int) bool {
	line = strings.TrimRight(line, "\r")
	if sourceIndent <= 0 || !hasMarkdownListContinuationIndent(line, sourceIndent) {
		return false
	}
	return strings.TrimSpace(trimMarkdownListCodeIndent(line, sourceIndent)) != ""
}

func isMarkdownListIndentedParagraphContinuation(lines []string, index int, sourceIndent int) bool {
	if index < 0 || index >= len(lines) || sourceIndent <= 0 {
		return false
	}
	line := strings.TrimRight(lines[index], "\r")
	if markdownLeadingSpaces(line) != sourceIndent {
		return false
	}
	body := strings.TrimSpace(trimMarkdownListCodeIndent(line, sourceIndent))
	if body == "" || startsMarkdownStructuralBlock(body) {
		return false
	}
	if _, ok := markdownLinkDefinitionEnd(lines, index); ok {
		return false
	}
	if markdownHTMLBlockCanInterruptParagraph(body) {
		return false
	}
	if info, ok := parseMarkdownListLineInfo(line); ok {
		return !isMarkdownInterruptingListStart(info)
	}
	return markdownTableEnd(lines, index) <= index
}

func isMarkdownListDefinitionParagraphContinuation(lines []string, index int, sourceIndent int) bool {
	if index <= 0 || index >= len(lines) || sourceIndent <= 0 {
		return false
	}
	line := strings.TrimRight(lines[index], "\r")
	if markdownLeadingSpaces(line) <= sourceIndent || !hasMarkdownListContinuationIndent(line, sourceIndent) {
		return false
	}
	body := strings.TrimSpace(trimMarkdownListCodeIndent(line, sourceIndent))
	if body == "" || startsMarkdownStructuralBlock(body) || markdownHTMLBlockCanInterruptParagraph(body) {
		return false
	}
	prev := strings.TrimRight(lines[index-1], "\r")
	if !hasMarkdownListContinuationIndent(prev, sourceIndent) {
		return false
	}
	prevBody := strings.TrimSpace(trimMarkdownListCodeIndent(prev, sourceIndent))
	return markdownLooksLikeReferenceDefinitionText(prevBody)
}

func markdownPreviousListContinuationLooksLikeReferenceDefinition(lines []string, index int, sourceIndent int) bool {
	if index <= 0 || index >= len(lines) || sourceIndent <= 0 {
		return false
	}
	prev := strings.TrimRight(lines[index-1], "\r")
	if !hasMarkdownListContinuationIndent(prev, sourceIndent) {
		return false
	}
	prevBody := strings.TrimSpace(trimMarkdownListCodeIndent(prev, sourceIndent))
	return markdownLooksLikeReferenceDefinitionText(prevBody)
}

func markdownLooksLikeReferenceDefinitionText(text string) bool {
	text = strings.TrimSpace(text)
	return strings.HasPrefix(text, "[") && strings.Contains(text, "]:")
}

func isMarkdownInterruptingListStart(info markdownListLineInfo) bool {
	return !info.ordered || info.number == 1
}

func wrapWithPrefix(prefix, text string, width int) []string {
	return wrapWithContinuation(prefix, strings.Repeat(" ", VisibleWidth(prefix)), text, width)
}

func wrapWithContinuation(prefix, continuation, text string, width int) []string {
	if IsImageLine(text) {
		return []string{prefix + text}
	}
	availableFirst := max(1, width-VisibleWidth(prefix))
	wrapped := WrapTextWithANSI(text, availableFirst)
	if len(wrapped) == 0 {
		return []string{prefix}
	}
	lines := []string{prefix + wrapped[0]}
	availableNext := max(1, width-VisibleWidth(continuation))
	for _, extra := range wrapped[1:] {
		for _, part := range WrapTextWithANSI(extra, availableNext) {
			lines = append(lines, continuation+part)
		}
	}
	return lines
}

func markdownTableEnd(lines []string, start int) int {
	if start+1 >= len(lines) || !isMarkdownTableRow(lines[start]) {
		return start
	}
	headerCells := splitMarkdownTableRow(lines[start])
	if len(headerCells) == 0 || !isMarkdownTableSeparatorForColumns(lines[start+1], len(headerCells)) {
		return start
	}
	end := start + 1
	for end+1 < len(lines) && isMarkdownTableRow(lines[end+1]) {
		end++
	}
	return end
}

func isMarkdownTableRow(line string) bool {
	trimmed, ok := markdownTableLineBody(line)
	if !ok || !markdownTableHasSeparatorPipe(trimmed) {
		return false
	}
	return len(splitMarkdownTableRow(trimmed)) > 0
}

func isMarkdownTableSeparator(line string) bool {
	return isMarkdownTableSeparatorForColumns(line, 0)
}

func isMarkdownTableSeparatorForColumns(line string, colCount int) bool {
	if !isMarkdownTableRow(line) {
		return false
	}
	cells := splitMarkdownTableRow(line)
	if colCount > 0 && len(cells) != colCount {
		return false
	}
	validCells := 0
	for _, cell := range cells {
		if !isMarkdownTableDelimiterCell(cell) {
			return false
		}
		validCells++
	}
	return validCells > 0
}

func markdownTableLineBody(line string) (string, bool) {
	line = strings.TrimRight(line, "\r")
	if markdownLeadingSpaces(line) > 3 {
		return "", false
	}
	return strings.TrimSpace(line), true
}

func markdownTableHasSeparatorPipe(line string) bool {
	return strings.Contains(line, "|")
}

func isMarkdownTableDelimiterCell(cell string) bool {
	cell = strings.TrimSpace(cell)
	if cell == "" {
		return false
	}
	if strings.HasPrefix(cell, ":") {
		cell = cell[1:]
	}
	if strings.HasSuffix(cell, ":") {
		cell = cell[:len(cell)-1]
	}
	if strings.Contains(cell, ":") {
		return false
	}
	return len(cell) >= 1 && strings.Trim(cell, "-") == ""
}

func splitMarkdownTableRow(line string) []string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "|") {
		trimmed = trimmed[1:]
	}
	if strings.HasSuffix(trimmed, "|") {
		trimmed = trimmed[:len(trimmed)-1]
	}
	parts := strings.Split(trimmed, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func (m *Markdown) renderTable(rows []string, width int) []string {
	if len(rows) < 2 {
		return rows
	}
	header := splitMarkdownTableRow(rows[0])
	colCount := len(header)
	if colCount == 0 {
		return nil
	}
	if width-(3*colCount+1) < colCount {
		return WrapTextWithANSI(strings.Join(rows, "\n"), max(1, width))
	}
	for i := range header {
		header[i] = m.renderInline(header[i])
	}
	var data [][]string
	for _, row := range rows[2:] {
		cells := splitMarkdownTableRow(row)
		for i := range cells {
			cells[i] = m.renderInline(cells[i])
		}
		data = append(data, cells)
	}
	naturalWidths := make([]int, colCount)
	minWidths := make([]int, colCount)
	for i, cell := range header {
		naturalWidths[i] = max(naturalWidths[i], VisibleWidth(cell))
		minWidths[i] = max(minWidths[i], longestMarkdownTokenWidth(cell))
	}
	for _, row := range data {
		for i := 0; i < colCount && i < len(row); i++ {
			minWidths[i] = max(minWidths[i], longestMarkdownTokenWidth(row[i]))
			naturalWidths[i] = max(naturalWidths[i], VisibleWidth(row[i]))
		}
	}
	widths := allocateMarkdownTableWidths(naturalWidths, minWidths, width)
	var out []string
	out = append(out, renderTableBorder(widths, "┌", "┬", "┐"))
	out = append(out, renderWrappedTableRow(header, widths, m.theme.Bold)...)
	out = append(out, renderTableBorder(widths, "├", "┼", "┤"))
	for idx, row := range data {
		out = append(out, renderWrappedTableRow(row, widths, nil)...)
		if idx < len(data)-1 {
			out = append(out, renderTableBorder(widths, "├", "┼", "┤"))
		}
	}
	out = append(out, renderTableBorder(widths, "└", "┴", "┘"))
	return out
}

func allocateMarkdownTableWidths(naturalWidths, minWidths []int, tableWidth int) []int {
	colCount := len(naturalWidths)
	if colCount == 0 {
		return nil
	}
	contentBudget := tableWidth - (3*colCount + 1)
	if contentBudget < colCount {
		contentBudget = colCount
	}

	minColumnWidths := make([]int, colCount)
	for i := range minColumnWidths {
		minColumnWidths[i] = max(1, minWidths[i])
	}
	minCellsWidth := sumInts(minColumnWidths)
	if minCellsWidth > contentBudget {
		minColumnWidths = make([]int, colCount)
		for i := range minColumnWidths {
			minColumnWidths[i] = 1
		}
		remaining := contentBudget - colCount
		if remaining > 0 {
			totalWeight := 0
			for _, width := range minWidths {
				totalWeight += max(0, width-1)
			}
			allocated := 0
			for i, width := range minWidths {
				grow := 0
				if totalWeight > 0 {
					grow = max(0, width-1) * remaining / totalWeight
				}
				minColumnWidths[i] += grow
				allocated += grow
			}
			leftover := remaining - allocated
			for i := 0; leftover > 0 && i < colCount; i++ {
				minColumnWidths[i]++
				leftover--
			}
		}
		minCellsWidth = sumInts(minColumnWidths)
	}

	totalNaturalWidth := sumInts(naturalWidths) + (3*colCount + 1)
	if totalNaturalWidth <= tableWidth {
		widths := make([]int, colCount)
		for i := range widths {
			widths[i] = max(naturalWidths[i], minColumnWidths[i])
		}
		return widths
	}

	totalGrowPotential := 0
	for i, width := range naturalWidths {
		totalGrowPotential += max(0, width-minColumnWidths[i])
	}
	extraWidth := max(0, contentBudget-minCellsWidth)
	widths := make([]int, colCount)
	for i, minWidth := range minColumnWidths {
		naturalWidth := naturalWidths[i]
		grow := 0
		if totalGrowPotential > 0 {
			grow = max(0, naturalWidth-minWidth) * extraWidth / totalGrowPotential
		}
		widths[i] = minWidth + grow
	}
	remaining := contentBudget - sumInts(widths)
	for remaining > 0 {
		changed := false
		for i := range widths {
			if remaining == 0 {
				break
			}
			if widths[i] < naturalWidths[i] {
				widths[i]++
				remaining--
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return widths
}

func sumInts(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

const markdownTableMaxUnbrokenTokenWidth = 30

func longestMarkdownTokenWidth(cell string) int {
	longest := 1
	for _, word := range strings.Fields(stripANSI(cell)) {
		longest = max(longest, min(VisibleWidth(word), markdownTableMaxUnbrokenTokenWidth))
	}
	return longest
}

func renderTableRow(cells []string, widths []int, styleFn func(string) string) string {
	var b strings.Builder
	b.WriteString("│")
	for i, width := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		cell = cell + strings.Repeat(" ", max(0, width-VisibleWidth(cell)))
		if styleFn != nil {
			cell = style(styleFn, cell)
		}
		b.WriteString(" ")
		b.WriteString(cell)
		b.WriteString(" │")
	}
	return b.String()
}

func renderWrappedTableRow(cells []string, widths []int, styleFn func(string) string) []string {
	wrappedCells := make([][]string, len(widths))
	height := 1
	for i, width := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		wrapped := WrapTextWithANSI(cell, max(1, width))
		if len(wrapped) == 0 {
			wrapped = []string{""}
		}
		wrappedCells[i] = wrapped
		height = max(height, len(wrapped))
	}
	lines := make([]string, 0, height)
	for row := 0; row < height; row++ {
		rowCells := make([]string, len(widths))
		for col := range widths {
			if row < len(wrappedCells[col]) {
				rowCells[col] = wrappedCells[col][row]
			}
		}
		lines = append(lines, renderTableRow(rowCells, widths, styleFn))
	}
	return lines
}

func renderTableBorder(widths []int, left, middle, right string) string {
	var b strings.Builder
	b.WriteString(left)
	for i, width := range widths {
		if i > 0 {
			b.WriteString(middle)
		}
		b.WriteString(strings.Repeat("─", width+2))
	}
	b.WriteString(right)
	return b.String()
}
