package gillmprovider

import (
	"strings"
	"testing"
)

func contextEstimateUsage(total int) Usage {
	return Usage{Input: total, TotalTokens: total}
}

func contextEstimateAssistant(timestamp int64, total int) Message {
	return Message{
		Role:       RoleAssistant,
		Content:    []ContentPart{Text("kept")},
		Usage:      contextEstimateUsage(total),
		StopReason: StopReasonStop,
		Timestamp:  timestamp,
	}
}

func TestContextTokenEstimation(t *testing.T) {
	t.Run("ignores stale assistant usage after a newer message is inserted before it", func(t *testing.T) {
		context := Context{
			SystemPrompt: "system",
			Messages: []Message{
				{Role: RoleUser, Content: []ContentPart{Text("summary")}, Timestamp: 200},
				contextEstimateAssistant(100, 9500),
				{Role: RoleUser, Content: []ContentPart{Text(strings.Repeat("x", 4000))}, Timestamp: 300},
			},
		}

		estimate := EstimateContextTokens(context)
		if estimate.Tokens != 1005 ||
			estimate.UsageTokens != 0 ||
			estimate.TrailingTokens != 1005 ||
			estimate.LastUsageIndex != nil {
			t.Fatalf("estimate = %#v", estimate)
		}
	})

	t.Run("uses assistant usage again after a response to the inserted context", func(t *testing.T) {
		context := Context{
			Messages: []Message{
				{Role: RoleUser, Content: []ContentPart{Text("summary")}, Timestamp: 200},
				contextEstimateAssistant(100, 9500),
				{Role: RoleUser, Content: []ContentPart{Text("new prompt")}, Timestamp: 300},
				contextEstimateAssistant(400, 2000),
				{Role: RoleUser, Content: []ContentPart{Text("tail")}, Timestamp: 500},
			},
		}

		estimate := EstimateContextTokens(context)
		if estimate.Tokens != 2001 ||
			estimate.UsageTokens != 2000 ||
			estimate.TrailingTokens != 1 ||
			estimate.LastUsageIndex == nil ||
			*estimate.LastUsageIndex != 3 {
			t.Fatalf("estimate = %#v", estimate)
		}
	})

	t.Run("counts definitions marked after the latest usage checkpoint", func(t *testing.T) {
		assistant := contextEstimateAssistant(100, 100)
		tool := Tool{
			Name:        "late_tool",
			Description: strings.Repeat("x", 4000),
			Parameters:  Object(map[string]Schema{"value": String()}, "value"),
		}
		plain := EstimateContextTokens(Context{
			Messages: []Message{
				assistant,
				{Role: RoleUser, Content: []ContentPart{Text("tail")}, Timestamp: 200},
			},
		})
		marked := EstimateContextTokens(Context{
			Messages: []Message{
				assistant,
				{
					Role:           RoleToolResult,
					Content:        []ContentPart{Text("done")},
					AddedToolNames: []string{"late_tool"},
					Timestamp:      200,
				},
			},
			Tools: []Tool{tool},
		})

		if marked.Tokens <= plain.Tokens+500 || marked.TrailingTokens <= plain.TrailingTokens+500 {
			t.Fatalf("plain = %#v, marked = %#v", plain, marked)
		}
	})
}

func TestEstimateTextAndImageContentUsesUTF16AndFixedImageCost(t *testing.T) {
	if got := EstimateTextTokens("🙂🙂"); got != 1 {
		t.Fatalf("emoji estimate = %d, want 1", got)
	}
	if got := EstimateTextAndImageContentTokens([]ContentPart{Image("data", "image/png")}); got != 1200 {
		t.Fatalf("image estimate = %d, want 1200", got)
	}
}
