package gicodingagent

import (
	"fmt"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func CreateBashToolDefinition(cwd string, options ...BashToolOptions) ToolDefinition {
	tool := NewBashTool(cwd, options...)
	return ToolDefinition{
		Name:        "bash",
		Description: "Execute bash commands",
		Parameters: llm.Object(map[string]llm.Schema{
			"command": llm.String(),
			"timeout": llm.Integer(),
		}, "command"),
		Execute: func(callID string, args any) (FileToolResult, error) {
			input, err := parseBashToolDefinitionInput(args)
			if err != nil {
				return FileToolResult{}, err
			}
			return tool.Execute(callID, input)
		},
		RenderCall:   renderBashToolCall,
		RenderResult: renderBashToolResult,
	}
}

func parseBashToolDefinitionInput(args any) (BashToolInput, error) {
	switch typed := args.(type) {
	case BashToolInput:
		return typed, nil
	case map[string]any:
		command, _ := typed["command"].(string)
		return BashToolInput{Command: command, Timeout: intArgValue(typed["timeout"])}, nil
	default:
		return BashToolInput{}, fmt.Errorf("bash arguments must be an object")
	}
}
