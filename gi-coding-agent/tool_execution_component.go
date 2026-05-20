package gicodingagent

import (
	"fmt"
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
	IsError          bool
	PreflightDiff    *EditDiffResult
	ExecutionStarted bool
}

type ToolExecutionComponent struct {
	name          string
	callID        string
	args          any
	definition    ToolDefinition
	cwd           string
	argsComplete  bool
	preflightDiff *EditDiffResult
	result        *FileToolResult
	resultIsError bool
	expanded      bool
	rendererState map[string]any
}

func NewToolExecutionComponent(name, callID string, args any, definition ToolDefinition, cwd string) *ToolExecutionComponent {
	return &ToolExecutionComponent{
		name:          name,
		callID:        callID,
		args:          args,
		definition:    definition,
		cwd:           cwd,
		rendererState: map[string]any{},
	}
}

func (c *ToolExecutionComponent) Invalidate() {}

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

func (c *ToolExecutionComponent) UpdateResult(result FileToolResult, isError bool) bool {
	copied := result
	c.result = &copied
	c.resultIsError = isError
	return false
}

func (c *ToolExecutionComponent) Render(width int) []string {
	var lines []string
	context := c.renderContext()
	if renderer := c.callRenderer(); renderer != nil {
		lines = append(lines, normalizeRenderedLines(renderer(c.args, context))...)
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
		lines = append(lines, normalizeRenderedLines(renderer(*c.result, ToolRenderResultOptions{
			Expanded:  c.expanded,
			IsPartial: false,
		}, context))...)
	} else {
		lines = append(lines, c.renderResultFallback()...)
	}
	return truncateToolExecutionLines(lines, width)
}

func (c *ToolExecutionComponent) renderContext() ToolRenderContext {
	return ToolRenderContext{
		Args:          c.args,
		ToolCallID:    c.callID,
		State:         c.rendererState,
		CWD:           c.cwd,
		ArgsComplete:  c.argsComplete,
		IsPartial:     c.result == nil,
		Expanded:      c.expanded,
		IsError:       c.resultIsError,
		PreflightDiff: c.preflightDiff,
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

func (c *ToolExecutionComponent) renderCallFallback() []string {
	return []string{c.name}
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
	if !options.Expanded && len(lines) > 10 {
		remaining := len(lines) - 10
		lines = append(lines[:10], fmt.Sprintf("... (%d more lines, Ctrl+O to expand)", remaining))
	}
	return lines
}

func renderWriteToolCall(args any, context ToolRenderContext) []string {
	path, content, hasContent := writeRenderArgs(args)
	if path == "" {
		path = "..."
	}
	lines := []string{strings.TrimSpace("write " + shortenDisplayPath(path))}
	if hasContent {
		contentLines := trimTrailingEmptyLines(strings.Split(content, "\n"))
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
		if fileName == "AGENTS.md" {
			return &compactReadCall{kind: "resource", label: label}
		}
	}
	if fileName == "AGENTS.md" {
		return &compactReadCall{kind: "resource", label: filepath.ToSlash(filepath.Clean(absolutePath))}
	}
	return nil
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
		return result.Text
	}
	var parts []string
	for _, part := range result.Content {
		if part.Type == llm.ContentText {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "\n")
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
