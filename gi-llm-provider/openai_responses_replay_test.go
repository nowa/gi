package gillmprovider

import "testing"

func TestOpenAIResponsesReasoningReplaySkipsAbortedReasoningOnlyHistory(t *testing.T) {
	model := MustGetModel("openai", "gpt-5-mini")
	userMessage := UserMessageText("Use the double_number tool to double 21.")
	corruptedAssistant := AssistantMessage([]ContentPart{{
		Type:              ContentThinking,
		Thinking:          "",
		ThinkingSignature: `{"type":"reasoning","id":"rs_aborted"}`,
	}}, StopReasonAborted, model)
	followUp := UserMessageText("Say hello to confirm you can continue.")

	items := ConvertOpenAIResponsesMessages(model, Context{
		SystemPrompt: "You are a helpful assistant.",
		Messages:     []Message{userMessage, corruptedAssistant, followUp},
		Tools:        []Tool{doubleNumberTool()},
	}, ConvertOpenAIResponsesOptions{})

	for _, item := range items {
		if item.Type == "reasoning" || item.Type == "function_call" || item.Type == "function_call_output" {
			t.Fatalf("aborted reasoning-only history leaked into payload: %#v", items)
		}
	}
	if countResponseUserItems(items) != 2 {
		t.Fatalf("items = %#v", items)
	}
}

func TestOpenAIResponsesReasoningReplaySameProviderDifferentModelHandoff(t *testing.T) {
	modelA := MustGetModel("openai", "gpt-5-mini")
	modelB := MustGetModel("openai", "gpt-5.2-codex")
	userMessage := UserMessageText("Use the double_number tool to double 21.")
	assistant := AssistantMessage([]ContentPart{
		{Type: ContentThinking, Thinking: "Need to call the tool.", ThinkingSignature: `{"type":"reasoning","id":"rs_pair"}`},
		ToolCall("call_pair|fc_pair", "double_number", map[string]any{"value": 21}),
	}, "toolUse", modelA)
	toolResult := Message{Role: RoleToolResult, ToolCallID: "call_pair|fc_pair", ToolName: "double_number", Content: []ContentPart{Text("42")}, Timestamp: NowMillis()}
	followUp := UserMessageText("What was the result? Answer with just the number.")

	items := ConvertOpenAIResponsesMessages(modelB, Context{
		SystemPrompt: "You are a helpful assistant.",
		Messages:     []Message{userMessage, assistant, toolResult, followUp},
		Tools:        []Tool{doubleNumberTool()},
	}, ConvertOpenAIResponsesOptions{})

	functionCall := findResponseItem(items, "function_call")
	if functionCall == nil || functionCall.CallID != "call_pair" || functionCall.ID != "" {
		t.Fatalf("function call should omit same-provider different-model item id: %#v in %#v", functionCall, items)
	}
	output := findResponseItem(items, "function_call_output")
	if output == nil || output.CallID != "call_pair" || output.Output != "42" {
		t.Fatalf("function output = %#v in %#v", output, items)
	}
}

func TestOpenAIResponsesReasoningReplayCrossProviderHandoff(t *testing.T) {
	anthropic := MustGetModel("anthropic", "claude-sonnet-4-5")
	codex := MustGetModel("openai", "gpt-5.2-codex")
	userMessage := UserMessageText("Use the double_number tool to double 21.")
	assistant := AssistantMessage([]ContentPart{
		Thinking("Need to call the tool."),
		ToolCall("toolu_12345", "double_number", map[string]any{"value": 21}),
	}, "toolUse", anthropic)
	toolResult := Message{Role: RoleToolResult, ToolCallID: "toolu_12345", ToolName: "double_number", Content: []ContentPart{Text("42")}, Timestamp: NowMillis()}
	followUp := UserMessageText("What was the result? Answer with just the number.")

	items := ConvertOpenAIResponsesMessages(codex, Context{
		SystemPrompt: "You are a helpful assistant.",
		Messages:     []Message{userMessage, assistant, toolResult, followUp},
		Tools:        []Tool{doubleNumberTool()},
	}, ConvertOpenAIResponsesOptions{})

	functionCall := findResponseItem(items, "function_call")
	if functionCall == nil || functionCall.CallID != "toolu_12345" {
		t.Fatalf("function call = %#v in %#v", functionCall, items)
	}
	output := findResponseItem(items, "function_call_output")
	if output == nil || output.CallID != "toolu_12345" || output.Output != "42" {
		t.Fatalf("function output = %#v in %#v", output, items)
	}
}

func doubleNumberTool() Tool {
	return Tool{
		Name:        "double_number",
		Description: "Doubles a number and returns the result.",
		Parameters:  Object(map[string]Schema{"value": Number()}, "value"),
	}
}

func countResponseUserItems(items []OpenAIResponsesInputItem) int {
	count := 0
	for _, item := range items {
		if item.Role == "user" {
			count++
		}
	}
	return count
}

func findResponseItem(items []OpenAIResponsesInputItem, typ string) *OpenAIResponsesInputItem {
	for i := range items {
		if items[i].Type == typ {
			return &items[i]
		}
	}
	return nil
}
