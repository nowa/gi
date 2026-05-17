package gillmprovider

import "testing"

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
	if len(ended.Message.Content) != 1 || ended.Message.Content[0].Arguments["path"] != "README.md" {
		t.Fatalf("toolcall_end = %#v", ended.Message.Content)
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
