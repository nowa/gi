package gicodingagent

import (
	"encoding/json"
	"fmt"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type ToolDefinition struct {
	Name             string
	Description      string
	Parameters       llm.Schema
	PrepareArguments func(args any) any
	Execute          func(callID string, args any) (FileToolResult, error)
	RenderCall       ToolCallRenderer
	RenderResult     ToolResultRenderer
}

func CreateEditToolDefinition(cwd string, operations ...FileToolOperations) ToolDefinition {
	tool := NewEditTool(cwd, operations...)
	editSchema := llm.Object(map[string]llm.Schema{
		"oldText": llm.String(),
		"newText": llm.String(),
	}, "oldText", "newText")
	return ToolDefinition{
		Name:        "edit",
		Description: "Make surgical edits to files",
		Parameters: llm.Object(map[string]llm.Schema{
			"path":  llm.String(),
			"edits": {Type: "array", Items: &editSchema},
		}, "path", "edits"),
		PrepareArguments: PrepareEditToolArguments,
		Execute: func(callID string, args any) (FileToolResult, error) {
			input, err := parseEditToolDefinitionInput(args)
			if err != nil {
				return FileToolResult{}, err
			}
			return tool.Execute(callID, input)
		},
		RenderCall:   renderEditToolCall,
		RenderResult: renderEditToolResult,
	}
}

func PrepareEditToolArguments(args any) any {
	values, ok := args.(map[string]any)
	if !ok || values == nil {
		return args
	}
	prepared := values
	copied := false
	copyForWrite := func() {
		if copied {
			return
		}
		prepared = cloneSettingsMap(values)
		copied = true
	}

	if editsText, ok := values["edits"].(string); ok {
		var parsed []map[string]any
		if json.Unmarshal([]byte(editsText), &parsed) == nil {
			copyForWrite()
			edits := make([]any, len(parsed))
			for i, edit := range parsed {
				edits[i] = edit
			}
			prepared["edits"] = edits
		}
	}

	oldText, hasOldText := values["oldText"]
	newText, hasNewText := values["newText"]
	if hasOldText || hasNewText {
		copyForWrite()
		edits := normalizePreparedEditList(prepared["edits"])
		edits = append(edits, map[string]any{"oldText": oldText, "newText": newText})
		prepared["edits"] = edits
		delete(prepared, "oldText")
		delete(prepared, "newText")
	}
	return prepared
}

func normalizePreparedEditList(value any) []any {
	switch edits := value.(type) {
	case []any:
		return append([]any(nil), edits...)
	case []Edit:
		values := make([]any, len(edits))
		for i, edit := range edits {
			values[i] = map[string]any{"oldText": edit.OldText, "newText": edit.NewText}
		}
		return values
	case []map[string]any:
		values := make([]any, len(edits))
		for i, edit := range edits {
			values[i] = edit
		}
		return values
	default:
		return nil
	}
}

func parseEditToolDefinitionInput(args any) (EditToolInput, error) {
	values, ok := args.(map[string]any)
	if !ok {
		return EditToolInput{}, fmt.Errorf("edit arguments must be an object")
	}
	path, _ := values["path"].(string)
	edits, err := parseEditList(values["edits"])
	if err != nil {
		return EditToolInput{}, err
	}
	return EditToolInput{Path: path, Edits: edits}, nil
}

func parseEditList(value any) ([]Edit, error) {
	switch edits := value.(type) {
	case []Edit:
		return edits, nil
	case []any:
		result := make([]Edit, 0, len(edits))
		for _, item := range edits {
			edit, err := parseEditItem(item)
			if err != nil {
				return nil, err
			}
			result = append(result, edit)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("edits must be an array")
	}
}

func parseEditItem(value any) (Edit, error) {
	switch edit := value.(type) {
	case Edit:
		return edit, nil
	case map[string]any:
		oldText, _ := edit["oldText"].(string)
		newText, _ := edit["newText"].(string)
		return Edit{OldText: oldText, NewText: newText}, nil
	default:
		return Edit{}, fmt.Errorf("edit entries must be objects")
	}
}
