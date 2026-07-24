package gillmprovider

import (
	"strings"
	"testing"
)

func deferredTestTool(name string) Tool {
	return Tool{
		Name:        name,
		Description: "The " + name + " tool",
		Parameters:  Object(map[string]Schema{"value": String()}, "value"),
	}
}

func deferredTestContext(tools []Tool, addedNames []string) Context {
	return Context{
		Messages: []Message{
			{Role: RoleUser, Content: []ContentPart{Text("Hello")}, Timestamp: 1},
			{
				Role:       RoleAssistant,
				Content:    []ContentPart{ToolCall("call_1", "base_tool", nil)},
				StopReason: StopReasonToolUse,
				Timestamp:  2,
			},
			{
				Role:           RoleToolResult,
				ToolCallID:     "call_1",
				ToolName:       "base_tool",
				Content:        []ContentPart{Text("done")},
				AddedToolNames: addedNames,
				Timestamp:      3,
			},
		},
		Tools: tools,
	}
}

func TestSplitDeferredTools(t *testing.T) {
	t.Run("does not resurrect a marked tool missing from Context.tools", func(t *testing.T) {
		context := deferredTestContext([]Tool{deferredTestTool("base_tool")}, []string{"late_tool"})
		split := SplitDeferredTools(context, true, nil)
		if len(split.Immediate) != 1 || split.Immediate[0].Name != "base_tool" || len(split.Deferred) != 0 {
			t.Fatalf("split = %#v", split)
		}
	})

	t.Run("keeps a tool immediate when it was used before its marker", func(t *testing.T) {
		context := deferredTestContext(
			[]Tool{deferredTestTool("base_tool"), deferredTestTool("late_tool")},
			[]string{"late_tool"},
		)
		context.Messages[1].Content = []ContentPart{ToolCall("call_1", "late_tool", nil)}

		split := SplitDeferredTools(context, true, nil)
		if len(split.Immediate) != 2 || len(split.Deferred) != 0 {
			t.Fatalf("split = %#v", split)
		}
	})

	t.Run("matches OAuth-canonicalized markers to active tools", func(t *testing.T) {
		context := deferredTestContext(
			[]Tool{deferredTestTool("base_tool"), deferredTestTool("read")},
			[]string{"Read"},
		)

		split := SplitDeferredTools(context, true, strings.ToLower)
		if len(split.Immediate) != 1 || split.Immediate[0].Name != "base_tool" || split.Deferred["read"].Name != "read" {
			t.Fatalf("split = %#v", split)
		}
	})

	t.Run("deduplicates active tools after OAuth canonicalization", func(t *testing.T) {
		context := Context{
			Messages: []Message{{Role: RoleUser, Content: []ContentPart{Text("Hello")}, Timestamp: 1}},
			Tools: []Tool{
				deferredTestTool("read"),
				{Name: "Read", Description: "Canonical definition", Parameters: Object(nil)},
			},
		}

		split := SplitDeferredTools(context, false, strings.ToLower)
		if len(split.Immediate) != 1 ||
			split.Immediate[0].Name != "Read" ||
			split.Immediate[0].Description != "Canonical definition" {
			t.Fatalf("split = %#v", split)
		}
	})
}
