package gillmprovider

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

func TestStreamContractsWithFauxProvider(t *testing.T) {
	t.Run("basic text generation can continue from prior assistant turn", func(t *testing.T) {
		registration := RegisterFauxProvider()
		defer registration.Unregister()
		registration.SetResponses([]FauxResponseStep{
			{Message: FauxAssistantText("Hello test successful")},
			{Message: FauxAssistantText("Goodbye test successful")},
		})
		llmContext := Context{
			SystemPrompt: "You are a helpful assistant. Be concise.",
			Messages:     []Message{UserMessageText("Reply with exactly: 'Hello test successful'")},
		}

		first, err := Complete(context.Background(), registration.MustModel(), llmContext, StreamOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if first.Role != RoleAssistant || first.Usage.Input <= 0 || first.Usage.Output <= 0 || !messageTextContains(first, "Hello test successful") {
			t.Fatalf("first response = %#v", first)
		}

		llmContext.Messages = append(llmContext.Messages, first, UserMessageText("Now say 'Goodbye test successful'"))
		second, err := Complete(context.Background(), registration.MustModel(), llmContext, StreamOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if second.Role != RoleAssistant || second.Usage.Input <= 0 || second.Usage.Output <= 0 || !messageTextContains(second, "Goodbye test successful") {
			t.Fatalf("second response = %#v", second)
		}
	})

	t.Run("tool call stream emits start delta end and returns toolUse", func(t *testing.T) {
		registration := RegisterFauxProvider()
		defer registration.Unregister()
		registration.SetResponses([]FauxResponseStep{{Message: FauxAssistantMessage([]ContentPart{
			FauxToolCall("math_operation", map[string]any{"a": 15, "b": 27, "operation": "add"}, "call_math"),
		}, "toolUse")}})

		stream, err := Stream(registration.MustModel(), Context{Messages: []Message{UserMessageText("calculate 15 + 27")}}, StreamOptions{})
		if err != nil {
			t.Fatal(err)
		}
		events := collectAssistantEvents(stream)
		assertEventTypes(t, events, []string{"toolcall_start", "toolcall_delta", "toolcall_end", "done"})

		message, err := stream.Result(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if message.StopReason != "toolUse" || len(message.Content) != 1 || message.Content[0].Name != "math_operation" {
			t.Fatalf("message = %#v", message)
		}
		if message.Content[0].Arguments["a"] != 15 || message.Content[0].Arguments["b"] != 27 {
			t.Fatalf("tool arguments = %#v", message.Content[0].Arguments)
		}
	})

	t.Run("multi-turn tool loop can feed results back into context", func(t *testing.T) {
		registration := RegisterFauxProvider(WithFauxModels(FauxModelDefinition{ID: "faux-thinker", Reasoning: true}))
		defer registration.Unregister()
		registration.SetResponses([]FauxResponseStep{
			{Message: FauxAssistantMessage([]ContentPart{
				FauxThinking("Need two calculator calls."),
				FauxToolCall("math_operation", map[string]any{"a": 42, "b": 17, "operation": "multiply"}, "call_mul"),
				FauxToolCall("math_operation", map[string]any{"a": 453, "b": 434, "operation": "add"}, "call_add"),
			}, "toolUse")},
			{Message: FauxAssistantText("The results are 714 and 887.")},
		})

		llmContext := Context{
			SystemPrompt: "You can use tools.",
			Messages:     []Message{UserMessageText("calculate 42 * 17 and 453 + 434")},
		}
		seenThinking := false
		seenToolCall := false
		var allText strings.Builder

		for turn := 0; turn < 5; turn++ {
			response, err := Complete(context.Background(), registration.MustModel("faux-thinker"), llmContext, StreamOptions{})
			if err != nil {
				t.Fatal(err)
			}
			llmContext.Messages = append(llmContext.Messages, response)

			var results []Message
			for _, part := range response.Content {
				switch part.Type {
				case ContentText:
					allText.WriteString(part.Text)
				case ContentThinking:
					seenThinking = true
				case ContentToolCall:
					seenToolCall = true
					results = append(results, evaluateMathToolCall(part))
				}
			}
			llmContext.Messages = append(llmContext.Messages, results...)
			if response.StopReason == StopReasonStop {
				break
			}
		}

		if !seenThinking || !seenToolCall {
			t.Fatalf("seen thinking=%v tool=%v", seenThinking, seenToolCall)
		}
		text := allText.String()
		if !strings.Contains(text, "714") || !strings.Contains(text, "887") {
			t.Fatalf("final text = %q", text)
		}
	})
}

func assertEventTypes(t *testing.T, events []AssistantMessageEvent, wants []string) {
	t.Helper()
	var got []string
	for _, event := range events {
		got = append(got, event.Type)
	}
	for _, want := range wants {
		if !containsString(got, want) {
			t.Fatalf("event types = %#v, missing %s", got, want)
		}
	}
}

func messageTextContains(message Message, want string) bool {
	for _, part := range message.Content {
		if part.Type == ContentText && strings.Contains(part.Text, want) {
			return true
		}
	}
	return false
}

func evaluateMathToolCall(part ContentPart) Message {
	a, _ := part.Arguments["a"].(int)
	b, _ := part.Arguments["b"].(int)
	result := 0
	switch part.Arguments["operation"] {
	case "add":
		result = a + b
	case "multiply":
		result = a * b
	}
	return Message{
		Role:       RoleToolResult,
		ToolCallID: part.ID,
		ToolName:   part.Name,
		Content:    []ContentPart{Text(strconv.Itoa(result))},
		Timestamp:  NowMillis(),
	}
}
