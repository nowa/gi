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

func TestAssistantMessageEventMarshalJSONUsesVariantContract(t *testing.T) {
	model := Model{ID: "model", API: "api", Provider: "provider"}
	message := AssistantMessage([]ContentPart{Text("hello")}, StopReasonStop, model)
	tests := []struct {
		name      string
		event     AssistantMessageEvent
		wantKeys  []string
		rejectKey string
	}{
		{
			name:      "zero content index remains present",
			event:     AssistantMessageEvent{Type: "text_start", ContentIndex: 0, Partial: message},
			wantKeys:  []string{"type", "contentIndex", "partial"},
			rejectKey: "message",
		},
		{
			name:      "done carries only final message",
			event:     AssistantMessageEvent{Type: "done", Reason: StopReasonStop, Message: message},
			wantKeys:  []string{"type", "reason", "message"},
			rejectKey: "partial",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(test.event)
			if err != nil {
				t.Fatal(err)
			}
			var object map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &object); err != nil {
				t.Fatal(err)
			}
			for _, key := range test.wantKeys {
				if _, ok := object[key]; !ok {
					t.Fatalf("%s missing from %s", key, encoded)
				}
			}
			if _, ok := object[test.rejectKey]; ok {
				t.Fatalf("%s unexpectedly present in %s", test.rejectKey, encoded)
			}
		})
	}
}

func TestPiMessagesEventMarshalJSONUsesVariantContract(t *testing.T) {
	tests := []struct {
		name      string
		event     PiMessagesEvent
		wantKeys  []string
		rejectKey string
	}{
		{
			name:      "start omits inactive zero-value state",
			event:     PiMessagesEvent{Type: "start"},
			wantKeys:  []string{"type"},
			rejectKey: "usage",
		},
		{
			name:      "zero content index remains present",
			event:     PiMessagesEvent{Type: "text_start", ContentIndex: 0},
			wantKeys:  []string{"type", "contentIndex"},
			rejectKey: "usage",
		},
		{
			name:      "done always carries usage",
			event:     PiMessagesEvent{Type: "done", Reason: StopReasonStop, Usage: EmptyUsage()},
			wantKeys:  []string{"type", "reason", "usage"},
			rejectKey: "delta",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(test.event)
			if err != nil {
				t.Fatal(err)
			}
			var object map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &object); err != nil {
				t.Fatal(err)
			}
			for _, key := range test.wantKeys {
				if _, ok := object[key]; !ok {
					t.Fatalf("%s missing from %s", key, encoded)
				}
			}
			if _, ok := object[test.rejectKey]; ok {
				t.Fatalf("%s unexpectedly present in %s", test.rejectKey, encoded)
			}
		})
	}
}
