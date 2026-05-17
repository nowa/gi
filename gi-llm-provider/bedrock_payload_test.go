package gillmprovider

import "testing"

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
