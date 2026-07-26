package gillmprovider

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildBedrockAdditionalModelRequestFieldsThinking(t *testing.T) {
	base := MustGetModel("amazon-bedrock", "global.anthropic.claude-opus-4-6-v1")
	opus47 := base
	opus47.ID = "global.anthropic.claude-opus-4-7-v1"
	opus47.Name = "Claude Opus 4.7 (Global)"
	opus5 := MustGetModel(
		"amazon-bedrock",
		"global.anthropic.claude-opus-5",
	)

	tests := []struct {
		name         string
		model        Model
		options      BedrockPayloadOptions
		wantType     string
		wantDisplay  bool
		wantEffort   string
		wantBeta     bool
		wantBudget   int
		wantGovCloud bool
	}{
		{
			name:        "adaptive opus 4.7 high",
			model:       opus47,
			options:     BedrockPayloadOptions{Reasoning: "high"},
			wantType:    "adaptive",
			wantDisplay: true,
			wantEffort:  "high",
		},
		{
			name:        "adaptive opus 4.7 xhigh",
			model:       opus47,
			options:     BedrockPayloadOptions{Reasoning: "xhigh"},
			wantType:    "adaptive",
			wantDisplay: true,
			wantEffort:  "xhigh",
		},
		{
			name:        "adaptive opus 5 high",
			model:       opus5,
			options:     BedrockPayloadOptions{Reasoning: "high"},
			wantType:    "adaptive",
			wantDisplay: true,
			wantEffort:  "high",
		},
		{
			name:        "adaptive opus 5 xhigh",
			model:       opus5,
			options:     BedrockPayloadOptions{Reasoning: "xhigh"},
			wantType:    "adaptive",
			wantDisplay: true,
			wantEffort:  "xhigh",
		},
		{
			name:       "govcloud fixed budget omits display",
			model:      Model{ID: "us-gov.anthropic.claude-sonnet-4-5-20250929-v1:0", Name: "Claude Sonnet 4.5", Provider: "amazon-bedrock", API: "bedrock-converse-stream", Reasoning: true},
			options:    BedrockPayloadOptions{Reasoning: "high"},
			wantType:   "enabled",
			wantBeta:   true,
			wantBudget: 16384,
		},
		{
			name:         "govcloud adaptive omits display",
			model:        opus47,
			options:      BedrockPayloadOptions{Reasoning: "high", Region: "us-gov-west-1"},
			wantType:     "adaptive",
			wantEffort:   "high",
			wantGovCloud: true,
		},
		{
			name:        "application profile uses model name",
			model:       Model{ID: "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/my-profile", Name: "Claude Opus 4.6", Provider: "amazon-bedrock", API: "bedrock-converse-stream", Reasoning: true},
			options:     BedrockPayloadOptions{Reasoning: "high"},
			wantType:    "adaptive",
			wantDisplay: true,
			wantEffort:  "high",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fields := BuildBedrockAdditionalModelRequestFields(tc.model, tc.options)
			thinking, ok := fields["thinking"].(map[string]any)
			if !ok {
				t.Fatalf("thinking = %#v", fields["thinking"])
			}
			if thinking["type"] != tc.wantType {
				t.Fatalf("type = %#v, want %q", thinking["type"], tc.wantType)
			}
			_, hasDisplay := thinking["display"]
			if hasDisplay != tc.wantDisplay {
				t.Fatalf("display presence = %v, want %v", hasDisplay, tc.wantDisplay)
			}
			if tc.wantEffort != "" {
				outputConfig, ok := fields["output_config"].(map[string]any)
				if !ok || outputConfig["effort"] != tc.wantEffort {
					t.Fatalf("output config = %#v", fields["output_config"])
				}
			}
			if tc.wantBudget != 0 && thinking["budget_tokens"] != tc.wantBudget {
				t.Fatalf("budget = %#v, want %d", thinking["budget_tokens"], tc.wantBudget)
			}
			_, hasBeta := fields["anthropic_beta"]
			if hasBeta != tc.wantBeta {
				t.Fatalf("anthropic beta presence = %v, want %v", hasBeta, tc.wantBeta)
			}
		})
	}
}

func TestBedrockCatalogExposesOpus5ThroughInferenceProfileOnly(
	t *testing.T,
) {
	if _, ok := GetModel(
		"amazon-bedrock",
		"global.anthropic.claude-opus-5",
	); !ok {
		t.Fatal("global Claude Opus 5 inference profile is missing")
	}
	if _, ok := GetModel(
		"amazon-bedrock",
		"anthropic.claude-opus-5",
	); ok {
		t.Fatal("Claude Opus 5 base model must not be exposed")
	}
}

func TestBuildBedrockPayloadInjectsCachePointsFromModelName(t *testing.T) {
	model := Model{
		ID:        "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/my-profile",
		Name:      "Claude Sonnet 4.6",
		Provider:  "amazon-bedrock",
		API:       "bedrock-converse-stream",
		Reasoning: true,
		Input:     []string{"text"},
	}

	payload := BuildBedrockPayload(model, Context{
		SystemPrompt: "You are helpful.",
		Messages:     []Message{UserMessageText("Hello")},
	}, BedrockPayloadOptions{})

	if len(payload.System) != 2 || payload.System[1].CachePoint == nil {
		t.Fatalf("system = %#v", payload.System)
	}
	last := payload.Messages[len(payload.Messages)-1]
	if len(last.Content) == 0 || last.Content[len(last.Content)-1].CachePoint == nil {
		t.Fatalf("last message = %#v", last)
	}
}

func TestBuildBedrockPayloadFallsBackToFixedBudgetByModelName(t *testing.T) {
	model := Model{
		ID:        "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/my-profile",
		Name:      "Claude Sonnet 4.5",
		Provider:  "amazon-bedrock",
		API:       "bedrock-converse-stream",
		Reasoning: true,
		Input:     []string{"text"},
	}

	payload := BuildBedrockPayload(model, Context{Messages: []Message{UserMessageText("Hello")}}, BedrockPayloadOptions{Reasoning: "high"})
	thinking := payload.AdditionalModelRequestFields["thinking"].(map[string]any)
	if thinking["type"] != "enabled" || thinking["budget_tokens"] == nil {
		t.Fatalf("thinking = %#v", thinking)
	}
	if _, ok := payload.AdditionalModelRequestFields["anthropic_beta"]; !ok {
		t.Fatalf("anthropic beta missing: %#v", payload.AdditionalModelRequestFields)
	}
}

func TestBuildBedrockPayloadConvertsToolConfigPiStyle(t *testing.T) {
	model := Model{ID: "bedrock-test", Provider: "amazon-bedrock", API: "bedrock-converse-stream"}
	context := Context{
		Messages: []Message{UserMessageText("Use the tool.")},
		Tools: []Tool{{
			Name:        "read_file",
			Description: "Read a file",
			Parameters: Schema{
				Type: "object",
				Properties: map[string]Schema{
					"path": {Type: "string"},
				},
				Required: []string{"path"},
			},
		}},
	}

	payload := BuildBedrockPayload(model, context, BedrockPayloadOptions{})
	if payload.ToolConfig == nil || len(payload.ToolConfig.Tools) != 1 {
		t.Fatalf("tool config = %#v", payload.ToolConfig)
	}
	spec := payload.ToolConfig.Tools[0].ToolSpec
	if spec.Name != "read_file" || spec.Description != "Read a file" {
		t.Fatalf("tool spec = %#v", spec)
	}
	wantSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
		},
		"required": []any{"path"},
	}
	if !reflect.DeepEqual(spec.InputSchema.JSON, wantSchema) {
		t.Fatalf("input schema = %#v, want %#v", spec.InputSchema.JSON, wantSchema)
	}
	if payload.ToolConfig.ToolChoice != nil {
		t.Fatalf("default tool choice = %#v", payload.ToolConfig.ToolChoice)
	}

	tests := []struct {
		name       string
		choice     any
		wantConfig bool
		wantChoice any
	}{
		{name: "auto", choice: "auto", wantConfig: true, wantChoice: map[string]any{"auto": map[string]any{}}},
		{name: "any", choice: "any", wantConfig: true, wantChoice: map[string]any{"any": map[string]any{}}},
		{name: "none", choice: "none"},
		{name: "named tool object", choice: BedrockNamedToolChoice{Type: "tool", Name: "read_file"}, wantConfig: true, wantChoice: map[string]any{"tool": map[string]any{"name": "read_file"}}},
		{name: "named tool map", choice: map[string]any{"type": "tool", "name": "read_file"}, wantConfig: true, wantChoice: map[string]any{"tool": map[string]any{"name": "read_file"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload := BuildBedrockPayload(model, context, BedrockPayloadOptions{ToolChoice: tc.choice})
			if !tc.wantConfig {
				if payload.ToolConfig != nil {
					t.Fatalf("tool config = %#v, want nil", payload.ToolConfig)
				}
				return
			}
			if payload.ToolConfig == nil {
				t.Fatal("tool config missing")
			}
			if !reflect.DeepEqual(payload.ToolConfig.ToolChoice, tc.wantChoice) {
				t.Fatalf("tool choice = %#v, want %#v", payload.ToolConfig.ToolChoice, tc.wantChoice)
			}
		})
	}
}

func TestBuildBedrockPayloadUsesRequiredEmptyTextPlaceholders(t *testing.T) {
	model := Model{
		ID:       "anthropic.claude-sonnet-4-5",
		Provider: "amazon-bedrock",
		API:      "bedrock-converse-stream",
	}
	for _, test := range []struct {
		name string
		text string
	}{
		{name: "replaces blank user string content with a placeholder", text: " \t "},
		{
			name: "replaces user content emptied by surrogate sanitization with a placeholder",
			text: string([]byte{0xed, 0xa0, 0xbd}),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := BuildBedrockPayload(
				model,
				Context{Messages: []Message{UserMessageText(test.text)}},
				BedrockPayloadOptions{},
			)
			if len(payload.Messages) != 1 ||
				len(payload.Messages[0].Content) < 1 ||
				payload.Messages[0].Content[0].Text != bedrockEmptyTextPlaceholder {
				t.Fatalf("user message = %#v", payload.Messages)
			}
		})
	}

	payload := BuildBedrockPayload(model, Context{Messages: []Message{{
		Role:       RoleToolResult,
		ToolCallID: "tool-1",
		Content:    []ContentPart{{Type: ContentText, Text: "\n"}},
	}}}, BedrockPayloadOptions{})
	toolResult := payload.Messages[0].Content[0].ToolResult
	if toolResult == nil ||
		len(toolResult.Content) != 1 ||
		toolResult.Content[0].Text != bedrockEmptyTextPlaceholder {
		t.Fatalf("tool result = %#v", toolResult)
	}
}

func TestBuildBedrockPayloadCheckedValidatesStrictTools(t *testing.T) {
	model := Model{
		ID:       "anthropic.claude-sonnet-5",
		Provider: "amazon-bedrock",
		API:      "bedrock-converse-stream",
		Compat:   ModelCompat{SupportsStrictMode: ptrBool(false)},
	}
	tool := Tool{
		Name:       "lookup",
		Parameters: Object(map[string]Schema{}),
		ConstrainedSampling: &ConstrainedSamplingConfig{
			Type:   ConstrainedSamplingJSONSchema,
			Strict: ConstrainedSamplingRequire,
		},
	}
	_, err := BuildBedrockPayloadChecked(
		model,
		Context{Tools: []Tool{tool}},
		BedrockPayloadOptions{},
	)
	if err == nil || !strings.Contains(err.Error(), "strict tools are unsupported") {
		t.Fatalf("error = %v", err)
	}

	model.Compat.SupportsStrictMode = ptrBool(true)
	payload, err := BuildBedrockPayloadChecked(
		model,
		Context{Tools: []Tool{tool}},
		BedrockPayloadOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if payload.ToolConfig == nil ||
		payload.ToolConfig.Tools[0].ToolSpec.Strict == nil ||
		!*payload.ToolConfig.Tools[0].ToolSpec.Strict {
		t.Fatalf("tool config = %#v", payload.ToolConfig)
	}
}

func TestBedrockThinkingSupportsPiV082Models(t *testing.T) {
	for _, name := range []string{
		"Claude Opus 4.8",
		"Claude Sonnet 5",
		"Claude Fable 5",
	} {
		t.Run(name, func(t *testing.T) {
			model := Model{
				ID:        "application-profile",
				Name:      name,
				Provider:  "amazon-bedrock",
				API:       "bedrock-converse-stream",
				Reasoning: true,
			}
			if !SupportsBedrockAdaptiveThinking(model) {
				t.Fatalf("%q should support adaptive thinking", name)
			}
			if got := MapBedrockThinkingEffort(model, "xhigh"); got != "xhigh" {
				t.Fatalf("xhigh effort = %q", got)
			}
		})
	}
}

func TestBuildBedrockPayloadSnapshotsToolArguments(t *testing.T) {
	arguments := map[string]any{
		"path": "README.md",
		"nested": map[string]any{
			"line": 1,
		},
	}
	payload := BuildBedrockPayload(
		Model{ID: "model", API: "bedrock-converse-stream"},
		Context{Messages: []Message{{
			Role: RoleAssistant,
			Content: []ContentPart{ToolCall(
				"tool-1",
				"read_file",
				arguments,
			)},
		}}},
		BedrockPayloadOptions{},
	)
	arguments["path"] = "changed"
	arguments["nested"].(map[string]any)["line"] = 2

	input := payload.Messages[0].Content[0].ToolUse.Input
	if input["path"] != "README.md" ||
		input["nested"].(map[string]any)["line"] != 1 {
		t.Fatalf("payload input changed with caller state: %#v", input)
	}
}
