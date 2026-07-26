package gillmprovider

import (
	"encoding/json"
	"testing"
)

func TestMessageMarshalJSONUsesRoleSpecificUsageContract(t *testing.T) {
	tests := []struct {
		name      string
		message   Message
		wantUsage bool
	}{
		{
			name: "user omits zero usage",
			message: Message{
				Role:      RoleUser,
				Content:   []ContentPart{Text("hello")},
				Timestamp: 1,
			},
		},
		{
			name: "user omits accidental nonzero usage",
			message: Message{
				Role:      RoleUser,
				Content:   []ContentPart{Text("hello")},
				Timestamp: 1,
				Usage:     Usage{Input: 10},
			},
		},
		{
			name: "assistant includes empty usage",
			message: Message{
				Role:       RoleAssistant,
				Content:    []ContentPart{Text("hello")},
				Usage:      EmptyUsage(),
				StopReason: StopReasonStop,
				Timestamp:  1,
			},
			wantUsage: true,
		},
		{
			name: "tool result omits absent usage",
			message: Message{
				Role:       RoleToolResult,
				ToolCallID: "call-1",
				ToolName:   "lookup",
				Content:    []ContentPart{Text("ok")},
				Timestamp:  1,
			},
		},
		{
			name: "tool result includes reported usage",
			message: Message{
				Role:       RoleToolResult,
				ToolCallID: "call-1",
				ToolName:   "lookup",
				Content:    []ContentPart{Text("ok")},
				Timestamp:  1,
				Usage:      Usage{Input: 10},
			},
			wantUsage: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(test.message)
			if err != nil {
				t.Fatal(err)
			}
			var object map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &object); err != nil {
				t.Fatal(err)
			}
			_, hasUsage := object["usage"]
			if hasUsage != test.wantUsage {
				t.Fatalf("usage present = %t, want %t: %s", hasUsage, test.wantUsage, encoded)
			}
		})
	}
}
