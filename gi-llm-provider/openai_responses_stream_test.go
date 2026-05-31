package gillmprovider

import (
	"strings"
	"testing"
)

func TestProcessOpenAIResponsesStreamCleansToolCallScratchState(t *testing.T) {
	model := Model{
		ID:       "gpt-5-mini",
		Name:     "GPT-5 Mini",
		API:      "openai-responses",
		Provider: "openai",
		BaseURL:  "https://api.openai.com/v1",
	}
	output := AssistantMessage(nil, StopReasonStop, model)
	args := `{"path":"README.md","content":"updated"}`

	emitted := ProcessOpenAIResponsesStreamEvents(model, &output, []OpenAIResponsesStreamEvent{
		{
			Type: "response.output_item.added",
			Item: &OpenAIResponsesOutputItem{Type: "function_call", ID: "fc_test", CallID: "call_test", Name: "edit"},
		},
		{Type: "response.function_call_arguments.delta", Delta: `{"path":"README.md"`},
		{Type: "response.function_call_arguments.delta", Delta: `,"content":"updated"}`},
		{Type: "response.function_call_arguments.done", Arguments: args},
		{
			Type: "response.output_item.done",
			Item: &OpenAIResponsesOutputItem{Type: "function_call", ID: "fc_test", CallID: "call_test", Name: "edit", Arguments: args},
		},
	})

	if len(output.Content) != 1 || output.Content[0].Type != ContentToolCall {
		t.Fatalf("content = %#v", output.Content)
	}
	if output.Content[0].Arguments["path"] != "README.md" || output.Content[0].Arguments["content"] != "updated" {
		t.Fatalf("arguments = %#v", output.Content[0].Arguments)
	}
	var ended *AssistantMessageEvent
	for i := range emitted {
		if emitted[i].Type == "toolcall_end" {
			ended = &emitted[i]
		}
	}
	if ended == nil {
		t.Fatalf("events = %#v", emitted)
	}
	if ended.ContentIndex != 0 || ended.ToolCall.Arguments["path"] != "README.md" {
		t.Fatalf("toolcall_end = %#v", *ended)
	}
}

func TestProcessOpenAIResponsesStreamEmitsPiLifecycleEventFields(t *testing.T) {
	model := Model{ID: "gpt-5-mini", Provider: "openai", API: "openai-responses"}
	output := AssistantMessage(nil, StopReasonStop, model)

	events := ProcessOpenAIResponsesStreamEvents(model, &output, []OpenAIResponsesStreamEvent{
		{Type: "response.output_item.added", Item: &OpenAIResponsesOutputItem{Type: "reasoning", ID: "rs_1"}},
		{Type: "response.reasoning_summary_part.added", Part: &OpenAIResponsesOutputContentPart{Type: "summary_text"}},
		{Type: "response.reasoning_summary_text.delta", Delta: "Need"},
		{Type: "response.reasoning_summary_part.done"},
		{Type: "response.output_item.done", Item: &OpenAIResponsesOutputItem{Type: "reasoning", ID: "rs_1", Summary: []OpenAIResponsesOutputContentPart{{Type: "summary_text", Text: "Need"}}}},
		{Type: "response.output_item.added", Item: &OpenAIResponsesOutputItem{Type: "message", ID: "msg_1", Phase: "final_answer"}},
		{Type: "response.output_text.delta", Delta: "OK"},
		{Type: "response.output_item.done", Item: &OpenAIResponsesOutputItem{Type: "message", ID: "msg_1", Phase: "final_answer", Content: []OpenAIResponsesOutputContentPart{{Type: "output_text", Text: "OK"}}}},
		{Type: "response.output_item.added", Item: &OpenAIResponsesOutputItem{Type: "function_call", ID: "fc_1", CallID: "call_1", Name: "read", Arguments: ""}},
		{Type: "response.function_call_arguments.delta", Delta: `{"path":"README`},
		{Type: "response.function_call_arguments.done", Arguments: `{"path":"README.md"}`},
		{Type: "response.output_item.done", Item: &OpenAIResponsesOutputItem{Type: "function_call", ID: "fc_1", CallID: "call_1", Name: "read", Arguments: `{"path":"README.md"}`}},
	})

	wantTypes := []string{
		"thinking_start", "thinking_delta", "thinking_delta", "thinking_end",
		"text_start", "text_delta", "text_end",
		"toolcall_start", "toolcall_delta", "toolcall_delta", "toolcall_end",
	}
	if got := assistantEventTypes(events); !stringSlicesEqual(got, wantTypes) {
		t.Fatalf("event types = %#v, want %#v", got, wantTypes)
	}
	if events[0].ContentIndex != 0 || events[1].ContentIndex != 0 || events[1].Delta != "Need" || events[2].Delta != "\n\n" || events[3].Content != "Need" {
		t.Fatalf("thinking events = %#v", events[:4])
	}
	if events[4].ContentIndex != 1 || events[5].ContentIndex != 1 || events[5].Delta != "OK" || events[6].Content != "OK" {
		t.Fatalf("text events = %#v", events[4:7])
	}
	if events[7].ContentIndex != 2 || events[8].Delta != `{"path":"README` || events[9].Delta != `.md"}` || events[10].ToolCall.Arguments["path"] != "README.md" {
		t.Fatalf("tool events = %#v", events[7:])
	}
}

func TestParseStreamingJSONObjectRepairsPartialObjectLikePi(t *testing.T) {
	got := parseStreamingJSONObject(`{"path":"README.md","content":"upd`)
	if got["path"] != "README.md" || got["content"] != "upd" {
		t.Fatalf("partial object = %#v", got)
	}
	got = parseStreamingJSONObject(`{"items":[{"name":"one"}]`)
	items, ok := got["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("partial nested object = %#v", got)
	}
}

func TestConvertOpenAIResponsesMessagesKeepsToolResultImagesInsideFunctionOutput(t *testing.T) {
	model := Model{ID: "gpt-5-mini", Provider: "openai", API: "openai-responses", Input: []string{"text", "image"}}
	assistant := AssistantMessage([]ContentPart{ToolCall("call_1|fc_1", "get_image", nil)}, "toolUse", model)
	context := Context{Messages: []Message{
		UserMessageText("Get an image"),
		assistant,
		{
			Role:       RoleToolResult,
			ToolCallID: "call_1|fc_1",
			ToolName:   "get_image",
			Content: []ContentPart{
				Text("A red circle with a diameter of 100 pixels."),
				Image("ZmFrZQ==", "image/png"),
			},
		},
	}}

	items := ConvertOpenAIResponsesMessages(model, context, ConvertOpenAIResponsesOptions{})
	index := -1
	for i, item := range items {
		if item.Type == "function_call_output" {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatalf("items = %#v", items)
	}
	outputParts, ok := items[index].Output.([]OpenAIResponsesContentPart)
	if !ok {
		t.Fatalf("output = %#v", items[index].Output)
	}
	seenText, seenImage := false, false
	for _, part := range outputParts {
		if part.Type == "input_text" && part.Text != "" {
			seenText = true
		}
		if part.Type == "input_image" && part.ImageURL == "data:image/png;base64,ZmFrZQ==" {
			seenImage = true
		}
	}
	if !seenText || !seenImage {
		t.Fatalf("output parts = %#v", outputParts)
	}
	for _, item := range items[index+1:] {
		if item.Role == "user" {
			t.Fatalf("tool image leaked into later user message: %#v", items)
		}
	}
}

func TestProcessOpenAIResponsesStreamCapturesResponseID(t *testing.T) {
	model := Model{ID: "gpt-5-mini", Provider: "openai", API: "openai-responses"}
	output := AssistantMessage(nil, StopReasonStop, model)

	ProcessOpenAIResponsesStreamEvents(model, &output, []OpenAIResponsesStreamEvent{
		{Type: "response.created", Response: &OpenAIResponsesResponseEvent{ID: "resp_123"}},
		{Type: "response.completed", Response: &OpenAIResponsesResponseEvent{ID: "resp_456"}},
	})

	if output.ResponseID != "resp_456" {
		t.Fatalf("response id = %q", output.ResponseID)
	}
}

func TestProcessOpenAIResponsesStreamCapturesReasoningLikePi(t *testing.T) {
	model := Model{ID: "gpt-5-mini", Provider: "openai", API: "openai-responses"}
	output := AssistantMessage(nil, StopReasonStop, model)

	events := ProcessOpenAIResponsesStreamEvents(model, &output, []OpenAIResponsesStreamEvent{
		{Type: "response.output_item.added", Item: &OpenAIResponsesOutputItem{Type: "reasoning", ID: "rs_1", Status: "in_progress"}},
		{Type: "response.reasoning_summary_part.added", Part: &OpenAIResponsesOutputContentPart{Type: "summary_text"}},
		{Type: "response.reasoning_summary_text.delta", Delta: "Need the answer"},
		{Type: "response.reasoning_summary_part.done"},
		{Type: "response.output_item.done", Item: &OpenAIResponsesOutputItem{
			Type:             "reasoning",
			ID:               "rs_1",
			Status:           "completed",
			EncryptedContent: "opaque",
			Summary:          []OpenAIResponsesOutputContentPart{{Type: "summary_text", Text: "Need the answer"}},
		}},
		{Type: "response.output_item.added", Item: &OpenAIResponsesOutputItem{Type: "message", ID: "msg_1", Status: "in_progress", Phase: "final_answer"}},
		{Type: "response.output_text.delta", Delta: "OK"},
		{Type: "response.output_item.done", Item: &OpenAIResponsesOutputItem{
			Type:    "message",
			ID:      "msg_1",
			Status:  "completed",
			Phase:   "final_answer",
			Content: []OpenAIResponsesOutputContentPart{{Type: "output_text", Text: "OK"}},
		}},
	})

	if len(output.Content) != 2 || output.Content[0].Type != ContentThinking || output.Content[0].Thinking != "Need the answer" {
		t.Fatalf("content = %#v", output.Content)
	}
	if output.Content[0].ThinkingSignature == "" || !strings.Contains(output.Content[0].ThinkingSignature, `"type":"reasoning"`) || !strings.Contains(output.Content[0].ThinkingSignature, `"encrypted_content":"opaque"`) {
		t.Fatalf("thinking signature = %q", output.Content[0].ThinkingSignature)
	}
	if output.Content[1].Text != "OK" || output.Content[1].TextSignature == "" || !strings.Contains(output.Content[1].TextSignature, `"id":"msg_1"`) {
		t.Fatalf("text content = %#v", output.Content[1])
	}
	if !containsAssistantEvent(events, "thinking_start") || !containsAssistantEvent(events, "thinking_delta") || !containsAssistantEvent(events, "thinking_end") {
		t.Fatalf("events = %#v", events)
	}
}

func TestProcessOpenAIResponsesStreamIgnoresBareReasoningSummaryDeltaLikePi(t *testing.T) {
	model := Model{ID: "gpt-5-mini", Provider: "openai", API: "openai-responses"}
	output := AssistantMessage(nil, StopReasonStop, model)

	events := ProcessOpenAIResponsesStreamEvents(model, &output, []OpenAIResponsesStreamEvent{
		{Type: "response.output_item.added", Item: &OpenAIResponsesOutputItem{Type: "reasoning", ID: "rs_1", Status: "in_progress"}},
		{Type: "response.reasoning_summary_text.delta", Delta: "Hidden summary"},
		{Type: "response.reasoning_summary_part.done"},
		{Type: "response.output_item.done", Item: &OpenAIResponsesOutputItem{
			Type:             "reasoning",
			ID:               "rs_1",
			Status:           "completed",
			EncryptedContent: "opaque",
		}},
	})

	if len(output.Content) != 1 || output.Content[0].Type != ContentThinking || output.Content[0].Thinking != "" {
		t.Fatalf("content = %#v", output.Content)
	}
	if containsAssistantEvent(events, "thinking_delta") {
		t.Fatalf("bare summary delta should not emit thinking_delta: %#v", events)
	}
}
