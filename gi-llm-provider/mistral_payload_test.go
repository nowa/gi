package gillmprovider

import "testing"

func TestBuildMistralPayloadReasoningModeSelection(t *testing.T) {
	tests := []struct {
		name                string
		model               Model
		reasoning           string
		wantPromptMode      string
		wantReasoningEffort string
	}{
		{
			name:                "small 4 uses reasoning effort",
			model:               MustGetModel("mistral", "mistral-small-2603"),
			reasoning:           "medium",
			wantReasoningEffort: "high",
		},
		{
			name:  "small 4 omits controls when thinking is off",
			model: MustGetModel("mistral", "mistral-small-2603"),
		},
		{
			name:           "magistral uses prompt mode",
			model:          MustGetModel("mistral", "magistral-medium-latest"),
			reasoning:      "medium",
			wantPromptMode: "reasoning",
		},
		{
			name:                "medium 3.5 uses reasoning effort",
			model:               MustGetModel("mistral", "mistral-medium-3.5"),
			reasoning:           "medium",
			wantReasoningEffort: "high",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload := BuildMistralPayload(tc.model, Context{Messages: []Message{UserMessageText("Hello")}}, MistralPayloadOptions{Reasoning: tc.reasoning})
			if payload.PromptMode != tc.wantPromptMode {
				t.Fatalf("prompt mode = %q, want %q", payload.PromptMode, tc.wantPromptMode)
			}
			if payload.ReasoningEffort != tc.wantReasoningEffort {
				t.Fatalf("reasoning effort = %q, want %q", payload.ReasoningEffort, tc.wantReasoningEffort)
			}
		})
	}
}

func TestConvertMistralToolsSerializesPlainJSONSchema(t *testing.T) {
	payload := BuildMistralPayload(
		MustGetModel("mistral", "devstral-medium-latest"),
		Context{
			Messages: []Message{UserMessageText("Hi")},
			Tools: []Tool{{
				Name:        "inspect_schema",
				Description: "Inspect the schema",
				Parameters: Object(map[string]Schema{
					"nested": Object(map[string]Schema{
						"value": String(),
					}, "value"),
				}, "nested"),
			}},
		},
		MistralPayloadOptions{},
	)

	if len(payload.Tools) != 1 {
		t.Fatalf("tools = %#v", payload.Tools)
	}
	parameters, ok := payload.Tools[0].Function.Parameters.(map[string]any)
	if !ok {
		t.Fatalf("parameters = %#v", payload.Tools[0].Function.Parameters)
	}
	properties, ok := parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %#v", parameters["properties"])
	}
	nested, ok := properties["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested = %#v", properties["nested"])
	}
	if _, ok := nested["properties"].(map[string]any); !ok {
		t.Fatalf("nested properties = %#v", nested["properties"])
	}
}
