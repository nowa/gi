package gicodingagent

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

type ToolCallRenderer func(args any, context ToolRenderContext) []string

type ToolResultRenderer func(result FileToolResult, options ToolRenderResultOptions, context ToolRenderContext) []string

type ToolRenderResultOptions struct {
	Expanded  bool
	IsPartial bool
}

type ToolRenderContext struct {
	Args             any
	ToolCallID       string
	State            map[string]any
	CWD              string
	ArgsComplete     bool
	IsPartial        bool
	Expanded         bool
	ShowImages       bool
	IsError          bool
	PreflightDiff    *EditDiffResult
	ExecutionStarted bool
}

type ToolExecutionOptions struct {
	ShowImages      *bool
	ImageWidthCells int
}

type ToolExecutionComponent struct {
	name             string
	callID           string
	args             any
	definition       ToolDefinition
	cwd              string
	argsComplete     bool
	preflightDiff    *EditDiffResult
	result           *FileToolResult
	resultIsError    bool
	resultIsPartial  bool
	expanded         bool
	showImages       bool
	imageWidthCells  int
	executionStarted bool
	rendererState    map[string]any
}

func NewToolExecutionComponent(name, callID string, args any, definition ToolDefinition, cwd string, options ...ToolExecutionOptions) *ToolExecutionComponent {
	opts := normalizeToolExecutionOptions(options...)
	return &ToolExecutionComponent{
		name:            name,
		callID:          callID,
		args:            args,
		definition:      definition,
		cwd:             cwd,
		showImages:      opts.showImages,
		imageWidthCells: opts.imageWidthCells,
		rendererState:   map[string]any{},
	}
}

type normalizedToolExecutionOptions struct {
	showImages      bool
	imageWidthCells int
}

func normalizeToolExecutionOptions(options ...ToolExecutionOptions) normalizedToolExecutionOptions {
	normalized := normalizedToolExecutionOptions{showImages: true, imageWidthCells: 60}
	for _, option := range options {
		if option.ShowImages != nil {
			normalized.showImages = *option.ShowImages
		}
		if option.ImageWidthCells > 0 {
			normalized.imageWidthCells = option.ImageWidthCells
		}
	}
	return normalized
}

func (c *ToolExecutionComponent) Invalidate() {}

func (c *ToolExecutionComponent) UpdateArgs(args any) {
	c.args = args
	c.argsComplete = false
	c.preflightDiff = nil
}

func (c *ToolExecutionComponent) MarkExecutionStarted() {
	c.executionStarted = true
}

func (c *ToolExecutionComponent) SetArgsComplete() {
	c.argsComplete = true
	if c.name != "edit" {
		return
	}
	input, err := toolExecutionEditInput(c.args)
	if err != nil {
		c.preflightDiff = &EditDiffResult{Error: err.Error()}
		return
	}
	diff := ComputeEditsDiff(input.Path, input.Edits, c.cwd)
	c.preflightDiff = &diff
}

func (c *ToolExecutionComponent) SetExpanded(expanded bool) {
	c.expanded = expanded
}

func (c *ToolExecutionComponent) SetShowImages(show bool) {
	c.showImages = show
}

func (c *ToolExecutionComponent) SetImageWidthCells(width int) {
	if width <= 0 {
		width = 60
	}
	c.imageWidthCells = width
}

func (c *ToolExecutionComponent) UpdateResult(result FileToolResult, isError bool) bool {
	return c.UpdateResultWithOptions(result, isError, false)
}

func (c *ToolExecutionComponent) UpdatePartialResult(result FileToolResult, isError bool) bool {
	return c.UpdateResultWithOptions(result, isError, true)
}

func (c *ToolExecutionComponent) UpdateResultWithOptions(result FileToolResult, isError, isPartial bool) bool {
	copied := result
	c.result = &copied
	c.resultIsError = isError
	c.resultIsPartial = isPartial
	return false
}

func (c *ToolExecutionComponent) Render(width int) []string {
	var lines []string
	context := c.renderContext()
	if renderer := c.callRenderer(); renderer != nil {
		rendered, ok := safeRenderToolCall(renderer, c.args, context)
		if ok {
			lines = append(lines, normalizeRenderedLines(rendered)...)
		} else {
			lines = append(lines, c.renderCallFallback()...)
		}
	} else {
		lines = append(lines, c.renderCallFallback()...)
	}
	if len(lines) == 0 {
		lines = append(lines, c.name)
	}
	if c.result == nil {
		return c.renderThemedShell(lines, width)
	}
	context = c.renderContext()
	if renderer := c.resultRenderer(); renderer != nil {
		rendered, ok := safeRenderToolResult(renderer, *c.result, ToolRenderResultOptions{
			Expanded:  c.expanded,
			IsPartial: c.resultIsPartial,
		}, context)
		if ok {
			lines = append(lines, normalizeRenderedLines(rendered)...)
		} else {
			lines = append(lines, c.renderResultFallback()...)
		}
	} else {
		lines = append(lines, c.renderResultFallback()...)
	}
	lines = append(lines, c.renderImageBlocks(width)...)
	return c.renderThemedShell(lines, width)
}

func (c *ToolExecutionComponent) renderThemedShell(lines []string, width int) []string {
	if len(lines) == 0 {
		return nil
	}
	if c.shouldRenderPlainShell(lines) {
		return append([]string{""}, truncateToolExecutionLines(lines, width)...)
	}
	innerWidth := max(1, width-2)
	renderedLines := truncateToolExecutionLines(lines, innerWidth)
	box := gitui.NewBox(1, 1, func(text string) string {
		return tuiThemeBG(c.shellBackgroundColor(), text)
	})
	box.AddChild(cliRenderedLinesComponent{render: func(int) []string {
		return renderedLines
	}})
	boxLines := box.Render(width)
	if len(boxLines) == 0 {
		return nil
	}
	return append([]string{""}, boxLines...)
}

func (c *ToolExecutionComponent) shouldRenderPlainShell(lines []string) bool {
	// Large edit previews can span hundreds of lines; recoloring every row on
	// settle causes terminal-wide redraws without changing the visible diff.
	return c.name == "edit" && len(lines) > 120
}

func (c *ToolExecutionComponent) shellBackgroundColor() string {
	switch {
	case c.result == nil || c.resultIsPartial:
		return "toolPendingBg"
	case c.resultIsError:
		return "toolErrorBg"
	default:
		return "toolSuccessBg"
	}
}

func (c *ToolExecutionComponent) renderContext() ToolRenderContext {
	return ToolRenderContext{
		Args:             c.args,
		ToolCallID:       c.callID,
		State:            c.rendererState,
		CWD:              c.cwd,
		ArgsComplete:     c.argsComplete,
		IsPartial:        c.result == nil || c.resultIsPartial,
		Expanded:         c.expanded,
		ShowImages:       c.showImages,
		IsError:          c.resultIsError,
		PreflightDiff:    c.preflightDiff,
		ExecutionStarted: c.executionStarted,
	}
}

func (c *ToolExecutionComponent) callRenderer() ToolCallRenderer {
	if c.definition.RenderCall != nil {
		return c.definition.RenderCall
	}
	if builtIn := builtInToolDefinition(c.name, c.cwd); builtIn.RenderCall != nil {
		return builtIn.RenderCall
	}
	return nil
}

func (c *ToolExecutionComponent) resultRenderer() ToolResultRenderer {
	if c.definition.RenderResult != nil {
		return c.definition.RenderResult
	}
	if builtIn := builtInToolDefinition(c.name, c.cwd); builtIn.RenderResult != nil {
		return builtIn.RenderResult
	}
	return nil
}

func safeRenderToolCall(renderer ToolCallRenderer, args any, context ToolRenderContext) (lines []string, ok bool) {
	defer func() {
		if recover() != nil {
			lines = nil
			ok = false
		}
	}()
	return renderer(args, context), true
}

func safeRenderToolResult(renderer ToolResultRenderer, result FileToolResult, options ToolRenderResultOptions, context ToolRenderContext) (lines []string, ok bool) {
	defer func() {
		if recover() != nil {
			lines = nil
			ok = false
		}
	}()
	return renderer(result, options, context), true
}

func (c *ToolExecutionComponent) renderCallFallback() []string {
	lines := []string{c.name}
	if args := formatToolExecutionArgs(c.args); args != "" {
		lines = append(lines, "")
		lines = append(lines, strings.Split(args, "\n")...)
	}
	return lines
}

func formatToolExecutionArgs(args any) string {
	if args == nil {
		return ""
	}
	data, err := json.MarshalIndent(args, "", "  ")
	if err != nil || len(data) == 0 {
		return ""
	}
	return string(data)
}

func (c *ToolExecutionComponent) renderResultFallback() []string {
	if c.result == nil {
		return nil
	}
	text := fileToolResultText(*c.result)
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func (c *ToolExecutionComponent) renderImageBlocks(width int) []string {
	if c.result == nil {
		return nil
	}
	var lines []string
	caps := gitui.GetCapabilities()
	for _, part := range c.result.Content {
		if part.Type != llm.ContentImage {
			continue
		}
		mimeType := strings.TrimSpace(part.MIMEType)
		if mimeType == "" {
			mimeType = "image/unknown"
		}
		data, err := base64.StdEncoding.DecodeString(part.Data)
		var dimensions *gitui.ImageDimensions
		if err == nil && len(data) > 0 {
			if dims, dimsErr := gitui.GetImageDimensions(data); dimsErr == nil {
				dimensions = &dims
			}
		}
		if caps.Images && c.showImages && err == nil && len(data) > 0 {
			image := gitui.NewImage(data, gitui.ImageOptions{
				MimeType:      mimeType,
				Dimensions:    dimensions,
				MaxWidthCells: c.imageWidthCells,
			})
			lines = append(lines, image.Render(width)...)
			continue
		}
		lines = append(lines, gitui.ImageFallbackDescription(mimeType, dimensions, ""))
	}
	return lines
}

func builtInToolDefinition(name, cwd string) ToolDefinition {
	switch name {
	case "read":
		return CreateReadToolDefinition(cwd)
	case "write":
		return CreateWriteToolDefinition(cwd)
	case "edit":
		return CreateEditToolDefinition(cwd)
	case "bash":
		return CreateBashToolDefinition(cwd)
	case "grep":
		return ToolDefinition{Name: "grep", RenderCall: renderGrepToolCall, RenderResult: renderGrepToolResult}
	case "find":
		return ToolDefinition{Name: "find", RenderCall: renderFindToolCall, RenderResult: renderFindToolResult}
	case "ls":
		return ToolDefinition{Name: "ls", RenderCall: renderLsToolCall, RenderResult: renderLsToolResult}
	default:
		return ToolDefinition{}
	}
}

func renderEditToolCall(args any, context ToolRenderContext) []string {
	input, err := toolExecutionEditInput(args)
	path := editRenderPath(args)
	if err == nil {
		path = input.Path
	}
	pathDisplay := renderToolPath(toolPathArgument(args, path, "file_path", "path"), context.CWD)
	lines := []string{"edit " + pathDisplay}
	if context.PreflightDiff != nil {
		if context.PreflightDiff.Error != "" {
			return append(lines, context.PreflightDiff.Error)
		}
		if context.PreflightDiff.Diff != "" {
			return append(lines, splitNonEmptyLines(RenderDiff(context.PreflightDiff.Diff))...)
		}
	}
	if context.ArgsComplete && !context.IsError && err == nil {
		return append(lines, "Preparing edit preview...")
	}
	return lines
}

func renderEditToolResult(result FileToolResult, _ ToolRenderResultOptions, context ToolRenderContext) []string {
	if result.Details != nil && result.Details.Diff != "" && !context.IsError {
		return splitNonEmptyLines(RenderDiff(result.Details.Diff))
	}
	if context.IsError {
		return splitNonEmptyLines(fileToolResultText(result))
	}
	return nil
}

func renderReadToolCall(args any, context ToolRenderContext) []string {
	if classification := compactReadClassification(args, context.CWD); classification != nil && !context.Expanded {
		return []string{formatCompactReadCall(*classification, args)}
	}
	path, _, _ := readRenderArgs(args)
	pathDisplay := renderToolPath(toolPathArgument(args, path, "file_path", "path"), context.CWD)
	return []string{"read " + pathDisplay}
}

func renderReadToolResult(result FileToolResult, options ToolRenderResultOptions, context ToolRenderContext) []string {
	if !options.Expanded && !context.IsError && compactReadClassification(context.Args, context.CWD) != nil {
		return nil
	}
	lines := trimTrailingEmptyLines(strings.Split(fileToolResultText(result), "\n"))
	if len(lines) == 0 {
		return nil
	}
	for i := range lines {
		lines[i] = replaceDisplayTabs(lines[i])
	}
	if !options.Expanded && len(lines) > 10 {
		remaining := len(lines) - 10
		lines = append(lines[:10], fmt.Sprintf("... (%d more lines, Ctrl+O to expand)", remaining))
	}
	lines = appendReadTruncationWarning(lines, result.Details)
	return lines
}

func appendReadTruncationWarning(lines []string, details *FileToolDetails) []string {
	if details == nil || details.Truncation == nil || !details.Truncation.Truncated {
		return lines
	}
	truncation := details.Truncation
	if truncation.FirstLineExceedsLimit {
		maxBytes := truncation.MaxBytes
		if maxBytes == 0 {
			maxBytes = defaultReadToolByteLimit
		}
		return append(lines, fmt.Sprintf("[First line exceeds %s limit]", formatBashOutputSize(maxBytes)))
	}
	switch truncation.TruncatedBy {
	case "lines":
		maxLines := truncation.MaxLines
		if maxLines == 0 {
			maxLines = defaultReadToolLineLimit
		}
		return append(lines, fmt.Sprintf("[Truncated: showing %d of %d lines (%d line limit)]", truncation.OutputLines, truncation.TotalLines, maxLines))
	case "bytes":
		maxBytes := truncation.MaxBytes
		if maxBytes == 0 {
			maxBytes = defaultReadToolByteLimit
		}
		return append(lines, fmt.Sprintf("[Truncated: %d lines shown (%s limit)]", truncation.OutputLines, formatBashOutputSize(maxBytes)))
	case "limit":
		return append(lines, fmt.Sprintf("[Truncated: showing %d of %d lines (requested limit)]", truncation.OutputLines, truncation.TotalLines))
	default:
		return append(lines, fmt.Sprintf("[Truncated: %d lines shown]", truncation.OutputLines))
	}
}

func formatDisplayByteSize(size int) string {
	if size > 0 && size%1024 == 0 {
		return fmt.Sprintf("%dKB", size/1024)
	}
	return fmt.Sprintf("%d bytes", size)
}

const writePartialFullHighlightLines = 50

type writeHighlightCache struct {
	rawPath          string
	lang             string
	rawContent       string
	normalizedLines  []string
	highlightedLines []string
}

func renderWriteToolCall(args any, context ToolRenderContext) []string {
	path, content, hasContent := writeRenderArgs(args)
	pathDisplay := renderToolPath(toolPathArgument(args, path, "file_path", "path"), context.CWD)
	lines := []string{"write " + pathDisplay}
	if hasContent {
		contentLines := trimTrailingEmptyLines(renderWritePreviewLines(path, content, context))
		if !context.Expanded && len(contentLines) > 10 {
			remaining := len(contentLines) - 10
			contentLines = append(contentLines[:10], fmt.Sprintf("... (%d more lines, Ctrl+O to expand)", remaining))
		}
		lines = append(lines, contentLines...)
	}
	return lines
}

func renderWritePreviewLines(path, content string, context ToolRenderContext) []string {
	if context.ArgsComplete {
		cache := rebuildWriteHighlightCache(path, content)
		setWriteHighlightCache(context.State, cache)
		if cache != nil {
			return append([]string(nil), cache.highlightedLines...)
		}
		return renderPlainWritePreviewLines(content)
	}
	cache := updateWriteHighlightCache(context.State, path, content)
	if cache != nil {
		return append([]string(nil), cache.highlightedLines...)
	}
	return renderPlainWritePreviewLines(content)
}

func renderPlainWritePreviewLines(content string) []string {
	contentLines := strings.Split(normalizeDisplayText(content), "\n")
	for i := range contentLines {
		contentLines[i] = tuiThemeToolOutput(replaceDisplayTabs(contentLines[i]))
	}
	return contentLines
}

func updateWriteHighlightCache(state map[string]any, rawPath, content string) *writeHighlightCache {
	lang := writeHighlightLanguage(rawPath)
	if lang == "" {
		setWriteHighlightCache(state, nil)
		return nil
	}
	cache := getWriteHighlightCache(state)
	if cache == nil || cache.lang != lang || cache.rawPath != rawPath || !strings.HasPrefix(content, cache.rawContent) {
		cache = rebuildWriteHighlightCache(rawPath, content)
		setWriteHighlightCache(state, cache)
		return cache
	}
	if len(content) == len(cache.rawContent) {
		return cache
	}

	delta := replaceDisplayTabs(normalizeDisplayText(content[len(cache.rawContent):]))
	cache.rawContent = content
	if len(cache.normalizedLines) == 0 {
		cache.normalizedLines = append(cache.normalizedLines, "")
		cache.highlightedLines = append(cache.highlightedLines, "")
	}
	segments := strings.Split(delta, "\n")
	lastIndex := len(cache.normalizedLines) - 1
	cache.normalizedLines[lastIndex] += segments[0]
	cache.highlightedLines[lastIndex] = highlightWriteLine(cache.normalizedLines[lastIndex], cache.lang)
	for _, segment := range segments[1:] {
		cache.normalizedLines = append(cache.normalizedLines, segment)
		cache.highlightedLines = append(cache.highlightedLines, highlightWriteLine(segment, cache.lang))
	}
	refreshWriteHighlightPrefix(cache)
	return cache
}

func rebuildWriteHighlightCache(rawPath, content string) *writeHighlightCache {
	lang := writeHighlightLanguage(rawPath)
	if lang == "" {
		return nil
	}
	normalized := replaceDisplayTabs(normalizeDisplayText(content))
	normalizedLines := strings.Split(normalized, "\n")
	return &writeHighlightCache{
		rawPath:          rawPath,
		lang:             lang,
		rawContent:       content,
		normalizedLines:  normalizedLines,
		highlightedLines: highlightWriteSource(normalized, lang),
	}
}

func refreshWriteHighlightPrefix(cache *writeHighlightCache) {
	if cache == nil {
		return
	}
	prefixCount := min(writePartialFullHighlightLines, len(cache.normalizedLines))
	if prefixCount <= 0 {
		return
	}
	prefix := strings.Join(cache.normalizedLines[:prefixCount], "\n")
	highlighted := highlightWriteSource(prefix, cache.lang)
	for i := 0; i < prefixCount; i++ {
		if i < len(highlighted) {
			cache.highlightedLines[i] = highlighted[i]
			continue
		}
		cache.highlightedLines[i] = highlightWriteLine(cache.normalizedLines[i], cache.lang)
	}
}

func highlightWriteSource(source, lang string) []string {
	return strings.Split(RenderHighlightedHTML(Highlight(source, HighlightOptions{
		Language:       lang,
		IgnoreIllegals: true,
		Theme:          writeHighlightTheme(),
	}), nil), "\n")
}

func highlightWriteLine(line, lang string) string {
	highlighted := highlightWriteSource(line, lang)
	if len(highlighted) == 0 {
		return ""
	}
	return highlighted[0]
}

func writeHighlightTheme() HighlightTheme {
	return HighlightTheme{
		"comment":     func(text string) string { return tuiThemeFG("syntaxComment", text) },
		"keyword":     func(text string) string { return tuiThemeFG("syntaxKeyword", text) },
		"function":    func(text string) string { return tuiThemeFG("syntaxFunction", text) },
		"variable":    func(text string) string { return tuiThemeFG("syntaxVariable", text) },
		"string":      func(text string) string { return tuiThemeFG("syntaxString", text) },
		"number":      func(text string) string { return tuiThemeFG("syntaxNumber", text) },
		"type":        func(text string) string { return tuiThemeFG("syntaxType", text) },
		"operator":    func(text string) string { return tuiThemeFG("syntaxOperator", text) },
		"punctuation": func(text string) string { return tuiThemeFG("syntaxPunctuation", text) },
	}
}

func writeHighlightLanguage(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".ts", ".tsx", ".mts", ".cts":
		return "typescript"
	default:
		return ""
	}
}

func getWriteHighlightCache(state map[string]any) *writeHighlightCache {
	if state == nil {
		return nil
	}
	cache, _ := state["write.highlightCache"].(*writeHighlightCache)
	return cache
}

func setWriteHighlightCache(state map[string]any, cache *writeHighlightCache) {
	if state == nil {
		return
	}
	if cache == nil {
		delete(state, "write.highlightCache")
		return
	}
	state["write.highlightCache"] = cache
}

func renderWriteToolResult(result FileToolResult, _ ToolRenderResultOptions, context ToolRenderContext) []string {
	if !context.IsError {
		return nil
	}
	return splitNonEmptyLines(fileToolResultText(result))
}

func renderBashToolCall(args any, _ ToolRenderContext) []string {
	command := bashRenderCommand(args)
	if command == "" {
		command = "..."
	}
	return []string{"bash " + command}
}

func renderBashToolResult(result FileToolResult, _ ToolRenderResultOptions, _ ToolRenderContext) []string {
	return trimTrailingEmptyLines(strings.Split(fileToolResultText(result), "\n"))
}

func renderGrepToolCall(args any, _ ToolRenderContext) []string {
	pattern, path, glob, limit := grepRenderArgs(args)
	if path == "" {
		path = "."
	}
	line := fmt.Sprintf("grep /%s/ in %s", pattern, shortenDisplayPath(path))
	if glob != "" {
		line += " (" + glob + ")"
	}
	if limit > 0 {
		line += fmt.Sprintf(" limit %d", limit)
	}
	return []string{line}
}

func renderGrepToolResult(result FileToolResult, options ToolRenderResultOptions, _ ToolRenderContext) []string {
	return renderSearchToolResult(result, options, 15)
}

func renderFindToolCall(args any, _ ToolRenderContext) []string {
	pattern, path, limit := findRenderArgs(args)
	if path == "" {
		path = "."
	}
	line := fmt.Sprintf("find %s in %s", pattern, shortenDisplayPath(path))
	if limit > 0 {
		line += fmt.Sprintf(" (limit %d)", limit)
	}
	return []string{line}
}

func renderFindToolResult(result FileToolResult, options ToolRenderResultOptions, _ ToolRenderContext) []string {
	return renderSearchToolResult(result, options, 20)
}

func renderLsToolCall(args any, context ToolRenderContext) []string {
	path, limit := lsRenderArgs(args)
	pathDisplay := renderToolPath(
		toolPathArgument(args, path, "path"),
		context.CWD,
		toolPathRenderOptions{emptyFallback: "."},
	)
	line := "ls " + pathDisplay
	if limit > 0 {
		line += fmt.Sprintf(" (limit %d)", limit)
	}
	return []string{line}
}

func renderLsToolResult(result FileToolResult, options ToolRenderResultOptions, _ ToolRenderContext) []string {
	return renderSearchToolResult(result, options, 20)
}

func renderSearchToolResult(result FileToolResult, options ToolRenderResultOptions, collapsedLines int) []string {
	text := strings.TrimSpace(fileToolResultText(result))
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if !options.Expanded && collapsedLines > 0 && len(lines) > collapsedLines {
		remaining := len(lines) - collapsedLines
		lines = append(lines[:collapsedLines], fmt.Sprintf("... (%d more lines, Ctrl+O to expand)", remaining))
	}
	lines = appendSearchTruncationWarning(lines, result.Details)
	return lines
}

func appendSearchTruncationWarning(lines []string, details *FileToolDetails) []string {
	if details == nil {
		return lines
	}
	var warnings []string
	if details.MatchLimitReached > 0 {
		warnings = append(warnings, fmt.Sprintf("%d matches limit", details.MatchLimitReached))
	}
	if details.ResultLimitReached > 0 {
		warnings = append(warnings, fmt.Sprintf("%d results limit", details.ResultLimitReached))
	}
	if details.EntryLimitReached > 0 {
		warnings = append(warnings, fmt.Sprintf("%d entries limit", details.EntryLimitReached))
	}
	if details.Truncation != nil && details.Truncation.Truncated {
		warnings = append(warnings, formatDisplayByteSize(defaultReadToolByteLimit)+" limit")
	}
	if details.SearchLinesTruncated {
		warnings = append(warnings, "some lines truncated")
	}
	if len(warnings) == 0 {
		return lines
	}
	return append(lines, "[Truncated: "+strings.Join(warnings, ", ")+"]")
}

type compactReadCall struct {
	kind  string
	label string
}

func compactReadClassification(args any, cwd string) *compactReadCall {
	path, _, _ := readRenderArgs(args)
	if path == "" {
		return nil
	}
	absolutePath := ResolveToCwd(path, cwd)
	fileName := filepath.Base(absolutePath)
	if fileName == "SKILL.md" {
		label := filepath.Base(filepath.Dir(absolutePath))
		if label == "." || label == string(filepath.Separator) || label == "" {
			label = fileName
		}
		return &compactReadCall{kind: "skill", label: label}
	}
	if rel, ok := GetCwdRelativePath(absolutePath, cwd); ok {
		label := filepath.ToSlash(rel)
		if label == "README.md" || strings.HasPrefix(label, "docs/") || strings.HasPrefix(label, "examples/") {
			return &compactReadCall{kind: "docs", label: label}
		}
		if isCompactResourceFileName(fileName) {
			return &compactReadCall{kind: "resource", label: label}
		}
	}
	if isCompactResourceFileName(fileName) {
		return &compactReadCall{kind: "resource", label: filepath.ToSlash(filepath.Clean(absolutePath))}
	}
	return nil
}

func isCompactResourceFileName(fileName string) bool {
	switch fileName {
	case "AGENTS.md", "AGENTS.MD", "CLAUDE.md", "CLAUDE.MD":
		return true
	default:
		return false
	}
}

func formatCompactReadCall(classification compactReadCall, args any) string {
	lineRange := formatReadLineRange(args)
	if classification.kind == "skill" {
		return fmt.Sprintf("[skill] %s%s (Ctrl+O to expand)", classification.label, lineRange)
	}
	return fmt.Sprintf("read %s %s%s (Ctrl+O to expand)", classification.kind, classification.label, lineRange)
}

func formatReadLineRange(args any) string {
	_, offset, limit := readRenderArgs(args)
	if offset == nil && limit == nil {
		return ""
	}
	start := 1
	if offset != nil {
		start = *offset
	}
	if limit == nil {
		return fmt.Sprintf(":%d", start)
	}
	return fmt.Sprintf(":%d-%d", start, start+*limit-1)
}

func normalizeRenderedLines(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(lines))
	for _, line := range lines {
		normalized = append(normalized, strings.Split(line, "\n")...)
	}
	return normalized
}

func trimTrailingEmptyLines(lines []string) []string {
	end := len(lines)
	for end > 0 && lines[end-1] == "" {
		end--
	}
	return lines[:end]
}

func truncateToolExecutionLines(lines []string, width int) []string {
	if width <= 0 {
		return lines
	}
	truncated := make([]string, len(lines))
	for i, line := range lines {
		truncated[i] = gitui.TruncateToWidth(line, width, "")
	}
	return truncated
}

func fileToolResultText(result FileToolResult) string {
	if result.Text != "" {
		return sanitizeToolOutputText(result.Text)
	}
	var parts []string
	for _, part := range result.Content {
		if part.Type == llm.ContentText {
			parts = append(parts, sanitizeToolOutputText(part.Text))
		}
	}
	return strings.Join(parts, "\n")
}

func sanitizeToolOutputText(text string) string {
	text = StripAnsi(text)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.Map(func(r rune) rune {
		switch r {
		case '\t', '\n':
			return r
		}
		if r <= 0x1f {
			return -1
		}
		if r >= 0xfff9 && r <= 0xfffb {
			return -1
		}
		return r
	}, text)
}

func normalizeDisplayText(text string) string {
	return strings.ReplaceAll(text, "\r", "")
}

func replaceDisplayTabs(text string) string {
	return strings.ReplaceAll(text, "\t", "   ")
}

func readRenderArgs(args any) (string, *int, *int) {
	switch typed := args.(type) {
	case ReadToolInput:
		return typed.Path, intPointerIfNonZero(typed.Offset), intPointerIfNonZero(typed.Limit)
	case map[string]any:
		path := ""
		if rawPath := toolPathArgument(typed, "", "file_path", "path"); rawPath != nil {
			path = *rawPath
		}
		return path, intArgPointer(typed["offset"]), intArgPointer(typed["limit"])
	default:
		return "", nil, nil
	}
}

func writeRenderArgs(args any) (string, string, bool) {
	switch typed := args.(type) {
	case WriteToolInput:
		return typed.Path, typed.Content, true
	case map[string]any:
		path := ""
		if rawPath := toolPathArgument(typed, "", "file_path", "path"); rawPath != nil {
			path = *rawPath
		}
		content, ok := typed["content"].(string)
		return path, content, ok
	default:
		return "", "", false
	}
}

func bashRenderCommand(args any) string {
	switch typed := args.(type) {
	case BashToolInput:
		return typed.Command
	case map[string]any:
		command, _ := typed["command"].(string)
		return command
	default:
		return ""
	}
}

func grepRenderArgs(args any) (string, string, string, int) {
	switch typed := args.(type) {
	case GrepToolInput:
		return typed.Pattern, typed.Path, typed.Glob, typed.Limit
	case map[string]any:
		return stringArg(typed["pattern"]), stringArg(typed["path"]), stringArg(typed["glob"]), intArgValue(typed["limit"])
	default:
		return "", "", "", 0
	}
}

func findRenderArgs(args any) (string, string, int) {
	switch typed := args.(type) {
	case FindToolInput:
		return typed.Pattern, typed.Path, 0
	case map[string]any:
		return stringArg(typed["pattern"]), stringArg(typed["path"]), intArgValue(typed["limit"])
	default:
		return "", "", 0
	}
}

func lsRenderArgs(args any) (string, int) {
	switch typed := args.(type) {
	case LsToolInput:
		return typed.Path, 0
	case map[string]any:
		path := ""
		if rawPath := toolPathArgument(typed, "", "path"); rawPath != nil {
			path = *rawPath
		}
		return path, intArgValue(typed["limit"])
	default:
		return "", 0
	}
}

func editRenderPath(args any) string {
	switch typed := args.(type) {
	case EditToolInput:
		return typed.Path
	case map[string]any:
		if rawPath := toolPathArgument(typed, "", "file_path", "path"); rawPath != nil {
			return *rawPath
		}
		return ""
	default:
		return ""
	}
}

func stringArg(value any) string {
	text, _ := value.(string)
	return text
}

func intPointerIfNonZero(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

func intArgPointer(value any) *int {
	switch typed := value.(type) {
	case int:
		return &typed
	case int64:
		v := int(typed)
		return &v
	case float64:
		v := int(typed)
		return &v
	default:
		return nil
	}
}

func shortenDisplayPath(path string) string {
	if path == "" {
		return path
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		cleanPath := filepath.ToSlash(filepath.Clean(path))
		cleanHome := filepath.ToSlash(filepath.Clean(home))
		if cleanPath == cleanHome {
			return "~"
		}
		if strings.HasPrefix(cleanPath, cleanHome+"/") {
			return "~" + strings.TrimPrefix(cleanPath, cleanHome)
		}
	}
	return filepath.ToSlash(path)
}

func toolExecutionEditInput(args any) (EditToolInput, error) {
	switch typed := args.(type) {
	case EditToolInput:
		return typed, nil
	case map[string]any:
		return parseEditToolDefinitionInput(typed)
	default:
		return parseEditToolDefinitionInput(PrepareEditToolArguments(args))
	}
}

func splitNonEmptyLines(text string) []string {
	if text == "" {
		return nil
	}
	rawLines := strings.Split(text, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
