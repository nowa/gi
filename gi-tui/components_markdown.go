package gitui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

type renderCache struct {
	text       string
	width      int
	hyperlinks bool
	lines      []string
	ok         bool
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
