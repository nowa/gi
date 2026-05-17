package gillmprovider

import (
	"sort"
	"strings"
	"testing"
)

func TestAnthropicMessagesE2ECompatibilityContracts(t *testing.T) {
	cases := anthropicMessagesCatalogCases()
	if len(cases) == 0 {
		t.Fatal("expected anthropic-messages models in catalog")
	}
	contextValue := Context{
		SystemPrompt: "You are a concise assistant.",
		Messages:     []Message{UserMessageText("Call echo_value with value set to compat.")},
		Tools: []Tool{{
			Name:        "echo_value",
			Description: "Echo a string value.",
			Parameters:  Object(map[string]Schema{"value": String()}, "value"),
		}},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			compat := ResolveAnthropicCompat(tc.Model)
			payload := BuildAnthropicPayload(tc.Model, contextValue, AnthropicPayloadOptions{CacheRetention: "none"})
			if len(payload.Tools) != 1 {
				t.Fatalf("tools = %#v", payload.Tools)
			}
			headers := BuildAnthropicHeaders(tc.Model, contextValue, AnthropicPayloadOptions{CacheRetention: "none"})
			hasFineGrainedBeta := strings.Contains(headers["anthropic-beta"], fineGrainedToolStreamingBeta)
			if compat.SupportsEagerToolInputStreaming {
				if payload.Tools[0].EagerInputStreaming == nil || !*payload.Tools[0].EagerInputStreaming || hasFineGrainedBeta {
					t.Fatalf("eager compat payload=%#v headers=%#v", payload.Tools[0], headers)
				}
			} else {
				if payload.Tools[0].EagerInputStreaming != nil || !hasFineGrainedBeta {
					t.Fatalf("non-eager compat payload=%#v headers=%#v", payload.Tools[0], headers)
				}
			}

			forced := tc.Model
			forced.Compat.SupportsEagerToolInputStreaming = ptrBool(true)
			forcedPayload := BuildAnthropicPayload(forced, contextValue, AnthropicPayloadOptions{CacheRetention: "none"})
			if forcedPayload.Tools[0].EagerInputStreaming == nil || !*forcedPayload.Tools[0].EagerInputStreaming {
				t.Fatalf("forced eager payload = %#v", forcedPayload.Tools[0])
			}
		})
	}
}

func TestAnthropicMessagesLongCacheRetentionE2EContract(t *testing.T) {
	for _, tc := range selectOneAnthropicMessagesCasePerProvider(anthropicMessagesCatalogCases()) {
		t.Run(tc.Name, func(t *testing.T) {
			model := tc.Model
			model.Compat.SupportsLongCacheRetention = ptrBool(true)
			payload := BuildAnthropicPayload(model, Context{
				SystemPrompt: "You are a concise assistant.",
				Messages:     []Message{UserMessageText("Reply with exactly: long cache retention accepted")},
			}, AnthropicPayloadOptions{CacheRetention: "long"})

			if len(payload.System) == 0 || payload.System[0].CacheControl == nil || payload.System[0].CacheControl.TTL != "1h" {
				t.Fatalf("system cache control = %#v", payload.System)
			}
			userBlocks, ok := payload.Messages[len(payload.Messages)-1].Content.([]AnthropicContentBlock)
			if !ok || len(userBlocks) == 0 || userBlocks[len(userBlocks)-1].CacheControl == nil || userBlocks[len(userBlocks)-1].CacheControl.TTL != "1h" {
				t.Fatalf("last user cache control = %#v", payload.Messages)
			}
		})
	}
}

func TestAnthropicOpus47SmokePayloadContract(t *testing.T) {
	model := MustGetModel("anthropic", "claude-opus-4-7")
	payload := BuildAnthropicPayload(model, Context{
		SystemPrompt: "You are a precise assistant.",
		Messages:     []Message{UserMessageText("Compute a deterministic arithmetic answer.")},
	}, AnthropicPayloadOptions{Reasoning: "high", MaxTokens: 1024})

	if payload.Thinking["type"] != "adaptive" || payload.Thinking["display"] != "summarized" {
		t.Fatalf("thinking = %#v", payload.Thinking)
	}
	if payload.OutputConfig["effort"] != "high" {
		t.Fatalf("output_config = %#v", payload.OutputConfig)
	}
	if payload.MaxTokens != 1024 {
		t.Fatalf("max tokens = %d", payload.MaxTokens)
	}
}

type anthropicMessagesCatalogCase struct {
	Name     string
	Provider string
	Model    Model
}

func anthropicMessagesCatalogCases() []anthropicMessagesCatalogCase {
	var cases []anthropicMessagesCatalogCase
	for _, provider := range GetProviders() {
		for _, model := range GetModels(provider) {
			if model.API == "anthropic-messages" {
				cases = append(cases, anthropicMessagesCatalogCase{
					Name:     provider + "/" + model.ID,
					Provider: provider,
					Model:    model,
				})
			}
		}
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].Name < cases[j].Name })
	return cases
}

func selectOneAnthropicMessagesCasePerProvider(cases []anthropicMessagesCatalogCase) []anthropicMessagesCatalogCase {
	byProvider := map[string][]anthropicMessagesCatalogCase{}
	for _, tc := range cases {
		byProvider[tc.Provider] = append(byProvider[tc.Provider], tc)
	}
	var selected []anthropicMessagesCatalogCase
	for _, providerCases := range byProvider {
		sort.Slice(providerCases, func(i, j int) bool {
			left := anthropicProbePriority(providerCases[i].Model)
			right := anthropicProbePriority(providerCases[j].Model)
			if left == right {
				return providerCases[i].Model.ID < providerCases[j].Model.ID
			}
			return left < right
		})
		selected = append(selected, providerCases[0])
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].Name < selected[j].Name })
	return selected
}

func anthropicProbePriority(model Model) float64 {
	id := strings.ToLower(model.ID)
	priority := model.Cost.Input + model.Cost.Output
	switch {
	case strings.Contains(id, "haiku") && (strings.Contains(id, "4-5") || strings.Contains(id, "4.5")):
		priority -= 1000
	case strings.Contains(id, "sonnet") && (strings.Contains(id, "4-") || strings.Contains(id, "4.")):
		priority -= 750
	case strings.Contains(id, "claude") && (strings.Contains(id, "4-") || strings.Contains(id, "4.")):
		priority -= 500
	}
	return priority
}
