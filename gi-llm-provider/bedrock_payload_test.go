package gillmprovider

import (
	"reflect"
	"testing"
)

func TestBuildBedrockAdditionalModelRequestFieldsThinking(t *testing.T) {
	base := MustGetModel("amazon-bedrock", "global.anthropic.claude-opus-4-6-v1")
	opus47 := base
	opus47.ID = "global.anthropic.claude-opus-4-7-v1"
	opus47.Name = "Claude Opus 4.7 (Global)"

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
