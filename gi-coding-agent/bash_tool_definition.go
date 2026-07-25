package gicodingagent

import (
	"encoding/json"
	"fmt"
	"strconv"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func CreateBashToolDefinition(cwd string, options ...BashToolOptions) ToolDefinition {
	tool := NewBashTool(cwd, options...)
	return ToolDefinition{
		Name:        "bash",
		Description: "Execute bash commands",
		Parameters: llm.Object(map[string]llm.Schema{
			"command": llm.String(),
			"timeout": llm.Number(),
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
		timeout, timeoutSet, err := optionalBashTimeout(typed)
		if err != nil {
			return BashToolInput{}, err
		}
		return BashToolInput{
			Command:    command,
			Timeout:    timeout,
			timeoutSet: timeoutSet,
		}, nil
	default:
		return BashToolInput{}, fmt.Errorf("bash arguments must be an object")
	}
}

func optionalBashTimeout(input map[string]any) (float64, bool, error) {
	value, exists := input["timeout"]
	if !exists || value == nil {
		return 0, false, nil
	}
	switch typed := value.(type) {
	case float64:
		return typed, true, nil
	case float32:
		return float64(typed), true, nil
	case int:
		return float64(typed), true, nil
	case int64:
		return float64(typed), true, nil
	case int32:
		return float64(typed), true, nil
	case json.Number:
		number, err := strconv.ParseFloat(string(typed), 64)
		if err != nil {
			return 0, false, fmt.Errorf("timeout must be a number")
		}
		return number, true, nil
	default:
		return 0, false, fmt.Errorf("timeout must be a number")
	}
}
