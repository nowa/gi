package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	core "github.com/nowa/gi/gi-agent-core"
	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	harnessenv "github.com/nowa/gi/gi-agent-core/harness/env"
	llm "github.com/nowa/gi/gi-llm-provider"
)

type EditToolDetails struct {
	Diff             string `json:"diff"`
	Patch            string `json:"patch"`
	FirstChangedLine int    `json:"firstChangedLine,omitempty"`
}

func CreateEditTool() agentharness.AgentHarnessTool {
	editItemSchema := llm.Object(map[string]llm.Schema{
		"oldText": {
			Type:        "string",
			Description: "Exact text for one targeted replacement. It must be unique in the original file and must not overlap with any other edits[].oldText in the same call.",
		},
		"newText": {
			Type:        "string",
			Description: "Replacement text for this targeted edit.",
		},
	}, "oldText", "newText")
	return agentharness.AgentHarnessTool{
		Name:        "edit",
		Label:       "edit",
		Description: "Edit a single file using exact text replacement. Every edits[].oldText must match a unique, non-overlapping region of the original file. If two changes affect the same block or nearby lines, merge them into one edit instead of emitting overlapping edits. Do not include large unchanged regions just to connect distant changes.",
		Parameters: llm.Object(map[string]llm.Schema{
			"path": {
				Type:        "string",
				Description: "Path to the file to edit (relative or absolute)",
			},
			"edits": {
				Type:        "array",
				Description: "One or more targeted replacements. Each edit is matched against the original file, not incrementally.",
				Items:       &editItemSchema,
			},
		}, "path", "edits"),
		PrepareArguments: PrepareEditArguments,
		Execute: func(ctx context.Context, _ string, params map[string]any, _ core.AgentToolUpdateCallback, contextValue any) (core.AgentToolResult, error) {
			provider, mutations, err := executionContext(contextValue)
			if err != nil {
				return core.AgentToolResult{}, err
			}
			path, err := requiredString(params, "path")
			if err != nil {
				return core.AgentToolResult{}, err
			}
			edits, err := parseEdits(params["edits"])
			if err != nil {
				return core.AgentToolResult{}, err
			}
			env := provider.ExecutionEnvironment()
			absolutePath := ResolveToolPath(env, path)
			var result core.AgentToolResult
			err = mutations.With(ctx, env, absolutePath, func() error {
				if err := contextError(ctx); err != nil {
					return err
				}
				info, err := env.FileInfo(ctx, absolutePath)
				if err != nil {
					return editAccessError(path, err)
				}
				if info.Kind != harnessenv.FileKindFile && info.Kind != harnessenv.FileKindSymlink {
					return fmt.Errorf("Could not edit file: %s. Path is not a file.", path)
				}
				content, err := env.ReadTextFile(ctx, absolutePath)
				if err != nil {
					return editAccessError(path, err)
				}
				if err := contextError(ctx); err != nil {
					return err
				}
				bom, content := StripBOM(content)
				lineEnding := DetectLineEnding(content)
				applied, err := ApplyEditsToNormalizedContent(NormalizeToLF(content), edits, path)
				if err != nil {
					return err
				}
				if err := contextError(ctx); err != nil {
					return err
				}
				finalContent := bom + RestoreLineEndings(applied.NewContent, lineEnding)
				if err := env.WriteFile(ctx, absolutePath, []byte(finalContent)); err != nil {
					return editAccessError(path, err)
				}
				if err := contextError(ctx); err != nil {
					return err
				}
				text := fmt.Sprintf("Successfully replaced %d block(s) in %s.", len(edits), path)
				result = core.AgentToolResult{
					Content: []llm.ContentPart{llm.Text(text)},
					Details: &EditToolDetails{
						Diff:             applied.Diff,
						Patch:            applied.Patch,
						FirstChangedLine: applied.FirstChangedLine,
					},
				}
				return nil
			})
			return result, err
		},
	}
}

func PrepareEditArguments(input any) (map[string]any, error) {
	args, ok := input.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("edit arguments must be an object")
	}
	prepared := make(map[string]any, len(args))
	for key, value := range args {
		prepared[key] = value
	}
	if encoded, ok := prepared["edits"].(string); ok {
		var decoded []any
		if json.Unmarshal([]byte(encoded), &decoded) == nil {
			prepared["edits"] = decoded
		}
	}
	oldText, hasOldText := prepared["oldText"].(string)
	newText, hasNewText := prepared["newText"].(string)
	if hasOldText && hasNewText {
		edits, _ := prepared["edits"].([]any)
		edits = append(append([]any(nil), edits...), map[string]any{
			"oldText": oldText,
			"newText": newText,
		})
		prepared["edits"] = edits
		delete(prepared, "oldText")
		delete(prepared, "newText")
	}
	return prepared, nil
}

func parseEdits(value any) ([]Edit, error) {
	if typed, ok := value.([]Edit); ok {
		if len(typed) == 0 {
			return nil, errors.New("Edit tool input is invalid. edits must contain at least one replacement.")
		}
		return append([]Edit(nil), typed...), nil
	}
	raw, ok := value.([]any)
	if !ok || len(raw) == 0 {
		return nil, errors.New("Edit tool input is invalid. edits must contain at least one replacement.")
	}
	edits := make([]Edit, 0, len(raw))
	for index, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("edits[%d] must be an object", index)
		}
		oldText, ok := entry["oldText"].(string)
		if !ok {
			return nil, fmt.Errorf("edits[%d].oldText must be a string", index)
		}
		newText, ok := entry["newText"].(string)
		if !ok {
			return nil, fmt.Errorf("edits[%d].newText must be a string", index)
		}
		edits = append(edits, Edit{OldText: oldText, NewText: newText})
	}
	return edits, nil
}

func editAccessError(path string, err error) error {
	var fileError *harnessenv.FileError
	if errors.As(err, &fileError) {
		return &editToolAccessError{
			message: fmt.Sprintf("Could not edit file: %s. Error code: %s.", path, fileError.Code),
			cause:   err,
		}
	}
	return &editToolAccessError{
		message: fmt.Sprintf("Could not edit file: %s. Error: %s.", path, err.Error()),
		cause:   err,
	}
}

type editToolAccessError struct {
	message string
	cause   error
}

func (e *editToolAccessError) Error() string {
	return e.message
}

func (e *editToolAccessError) Unwrap() error {
	return e.cause
}
