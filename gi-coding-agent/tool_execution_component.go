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
		return truncateToolExecutionLines(lines, width)
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
	return truncateToolExecutionLines(lines, width)
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
	lines := []string{strings.TrimSpace("edit " + path)}
	if context.PreflightDiff != nil {
		if context.PreflightDiff.Error != "" {
			return append(lines, context.PreflightDiff.Error)
		}
		if context.PreflightDiff.Diff != "" {
			return append(lines, splitNonEmptyLines(context.PreflightDiff.Diff)...)
		}
	}
	if context.ArgsComplete && !context.IsError && err == nil {
		return append(lines, "Preparing edit preview...")
	}
	return lines
}

func renderEditToolResult(result FileToolResult, _ ToolRenderResultOptions, context ToolRenderContext) []string {
	if result.Details != nil && result.Details.Diff != "" && !context.IsError {
		return splitNonEmptyLines(result.Details.Diff)
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
	if path == "" {
		path = "..."
	}
	return []string{strings.TrimSpace("read " + shortenDisplayPath(path))}
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
	switch truncation.TruncatedBy {
	case "lines":
		return append(lines, fmt.Sprintf("[Truncated: showing %d of %d lines (%d line limit)]", truncation.OutputLines, truncation.TotalLines, defaultReadToolLineLimit))
	case "bytes":
		return append(lines, fmt.Sprintf("[Truncated: %d lines shown (%s limit)]", truncation.OutputLines, formatDisplayByteSize(defaultReadToolByteLimit)))
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

func renderWriteToolCall(args any, context ToolRenderContext) []string {
	path, content, hasContent := writeRenderArgs(args)
	if path == "" {
		path = "..."
	}
	lines := []string{strings.TrimSpace("write " + shortenDisplayPath(path))}
	if hasContent {
		contentLines := trimTrailingEmptyLines(strings.Split(normalizeDisplayText(content), "\n"))
		for i := range contentLines {
			contentLines[i] = replaceDisplayTabs(contentLines[i])
		}
		if !context.Expanded && len(contentLines) > 10 {
			remaining := len(contentLines) - 10
			contentLines = append(contentLines[:10], fmt.Sprintf("... (%d more lines, Ctrl+O to expand)", remaining))
		}
		lines = append(lines, contentLines...)
	}
	return lines
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

func renderLsToolCall(args any, _ ToolRenderContext) []string {
	path, limit := lsRenderArgs(args)
	if path == "" {
		path = "."
	}
	line := "ls " + shortenDisplayPath(path)
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
	text = strings.ReplaceAll(text, "\r", "")
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
		path, _ := typed["path"].(string)
		if legacy, ok := typed["file_path"].(string); ok && legacy != "" {
			path = legacy
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
		path, _ := typed["path"].(string)
		if legacy, ok := typed["file_path"].(string); ok && legacy != "" {
			path = legacy
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
		return stringArg(typed["path"]), intArgValue(typed["limit"])
	default:
		return "", 0
	}
}

func editRenderPath(args any) string {
	switch typed := args.(type) {
	case EditToolInput:
		return typed.Path
	case map[string]any:
		path, _ := typed["path"].(string)
		return path
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
