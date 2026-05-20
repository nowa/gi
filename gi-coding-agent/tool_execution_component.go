package gicodingagent

import "strings"

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
}

func NewToolExecutionComponent(name, callID string, args any, definition ToolDefinition, cwd string) *ToolExecutionComponent {
	return &ToolExecutionComponent{
		name:       name,
		callID:     callID,
		args:       args,
		definition: definition,
		cwd:        cwd,
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

func (c *ToolExecutionComponent) UpdateResult(result FileToolResult, isError bool) bool {
	copied := result
	c.result = &copied
	c.resultIsError = isError
	return false
}

func (c *ToolExecutionComponent) Render(_ int) []string {
	lines := []string{c.name}
	if c.name == "edit" {
		return c.renderEdit(lines)
	}
	if c.result != nil && c.result.Text != "" {
		lines = append(lines, c.result.Text)
	}
	return lines
}

func (c *ToolExecutionComponent) renderEdit(lines []string) []string {
	if c.result != nil {
		if c.result.Details != nil && c.result.Details.Diff != "" {
			return append(lines, splitNonEmptyLines(c.result.Details.Diff)...)
		}
		if c.resultIsError && c.result.Text != "" {
			return append(lines, c.result.Text)
		}
		return lines
	}
	if c.preflightDiff != nil {
		if c.preflightDiff.Error != "" {
			return append(lines, c.preflightDiff.Error)
		}
		if c.preflightDiff.Diff != "" {
			return append(lines, splitNonEmptyLines(c.preflightDiff.Diff)...)
		}
	}
	if c.argsComplete {
		return append(lines, "Preparing edit preview...")
	}
	return lines
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
