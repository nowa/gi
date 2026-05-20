package gicodingagent

import (
	"fmt"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func CreateReadToolDefinition(cwd string, operations ...FileToolOperations) ToolDefinition {
	tool := NewReadTool(cwd, operations...)
	return ToolDefinition{
		Name:        "read",
		Description: "Read file contents",
		Parameters: llm.Object(map[string]llm.Schema{
			"path":   llm.String(),
			"offset": llm.Integer(),
			"limit":  llm.Integer(),
		}, "path"),
		PrepareArguments: PrepareReadToolArguments,
		Execute: func(callID string, args any) (FileToolResult, error) {
			input, err := parseReadToolDefinitionInput(args)
			if err != nil {
				return FileToolResult{}, err
			}
			return tool.Execute(callID, input)
		},
		RenderCall:   renderReadToolCall,
		RenderResult: renderReadToolResult,
	}
}

func CreateWriteToolDefinition(cwd string, operations ...FileToolOperations) ToolDefinition {
	tool := NewWriteTool(cwd, operations...)
	return ToolDefinition{
		Name:        "write",
		Description: "Create or overwrite files",
		Parameters: llm.Object(map[string]llm.Schema{
			"path":    llm.String(),
			"content": llm.String(),
		}, "path", "content"),
		PrepareArguments: PrepareWriteToolArguments,
		Execute: func(callID string, args any) (FileToolResult, error) {
			input, err := parseWriteToolDefinitionInput(args)
			if err != nil {
				return FileToolResult{}, err
			}
			return tool.Execute(callID, input)
		},
		RenderCall:   renderWriteToolCall,
		RenderResult: renderWriteToolResult,
	}
}

func PrepareReadToolArguments(args any) any {
	values, ok := args.(map[string]any)
	if !ok || values == nil {
		return args
	}
	legacy, hasLegacy := values["file_path"]
	if !hasLegacy {
		return args
	}
	prepared := cloneSettingsMap(values)
	if _, hasPath := prepared["path"]; !hasPath {
		prepared["path"] = legacy
	}
	delete(prepared, "file_path")
	return prepared
}

func PrepareWriteToolArguments(args any) any {
	values, ok := args.(map[string]any)
	if !ok || values == nil {
		return args
	}
	legacy, hasLegacy := values["file_path"]
	if !hasLegacy {
		return args
	}
	prepared := cloneSettingsMap(values)
	if _, hasPath := prepared["path"]; !hasPath {
		prepared["path"] = legacy
	}
	delete(prepared, "file_path")
	return prepared
}

func parseReadToolDefinitionInput(args any) (ReadToolInput, error) {
	switch typed := args.(type) {
	case ReadToolInput:
		return typed, nil
	case map[string]any:
		prepared := PrepareReadToolArguments(typed)
		values, ok := prepared.(map[string]any)
		if !ok {
			return ReadToolInput{}, fmt.Errorf("read arguments must be an object")
		}
		path, _ := values["path"].(string)
		return ReadToolInput{
			Path:   path,
			Offset: intArgValue(values["offset"]),
			Limit:  intArgValue(values["limit"]),
		}, nil
	default:
		return ReadToolInput{}, fmt.Errorf("read arguments must be an object")
	}
}

func parseWriteToolDefinitionInput(args any) (WriteToolInput, error) {
	switch typed := args.(type) {
	case WriteToolInput:
		return typed, nil
	case map[string]any:
		prepared := PrepareWriteToolArguments(typed)
		values, ok := prepared.(map[string]any)
		if !ok {
			return WriteToolInput{}, fmt.Errorf("write arguments must be an object")
		}
		path, _ := values["path"].(string)
		content, _ := values["content"].(string)
		return WriteToolInput{Path: path, Content: content}, nil
	default:
		return WriteToolInput{}, fmt.Errorf("write arguments must be an object")
	}
}

func intArgValue(value any) int {
	if pointer := intArgPointer(value); pointer != nil {
		return *pointer
	}
	return 0
}
