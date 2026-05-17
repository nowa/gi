package gillmprovider

import "testing"

func TestConvertOpenAICompletionsMessagesBatchesToolResultImages(t *testing.T) {
	model := Model{ID: "gpt-4o-mini", Provider: "openai", API: "openai-completions", Input: []string{"text", "image"}}
	assistant := AssistantMessage([]ContentPart{
		ToolCall("tool-1", "read", map[string]any{"path": "img-1.png"}),
		ToolCall("tool-2", "read", map[string]any{"path": "img-2.png"}),
	}, "toolUse", model)
	context := Context{Messages: []Message{
		UserMessageText("Read the images"),
		assistant,
		{Role: RoleToolResult, ToolCallID: "tool-1", ToolName: "read", Content: []ContentPart{Text("Read image file [image/png]"), Image("ZmFrZQ==", "image/png")}},
		{Role: RoleToolResult, ToolCallID: "tool-2", ToolName: "read", Content: []ContentPart{Text("Read image file [image/png]"), Image("ZmFrZQ==", "image/png")}},
	}}

	messages := ConvertOpenAICompletionsMessages(model, context, OpenAICompletionsCompat{})

	if len(messages) != 5 {
		t.Fatalf("messages = %#v", messages)
	}
	roles := []string{messages[0].Role, messages[1].Role, messages[2].Role, messages[3].Role, messages[4].Role}
	wantRoles := []string{"user", "assistant", "tool", "tool", "user"}
	for i := range roles {
		if roles[i] != wantRoles[i] {
			t.Fatalf("roles = %#v", roles)
		}
	}
	content, ok := messages[4].Content.([]OpenAIChatContentPart)
	if !ok || len(content) != 3 || content[0].Type != "text" || content[1].Type != "image_url" || content[2].Type != "image_url" {
		t.Fatalf("image batch = %#v", messages[4].Content)
	}
}

func TestConvertOpenAICompletionsMessagesNormalizesPipeToolIDs(t *testing.T) {
	model := Model{ID: "gpt-4o-mini", Provider: "openai", API: "openai-completions", Input: []string{"text"}}
	source := Model{ID: "gpt-5.2-codex", Provider: "github-copilot", API: "openai-responses"}
	assistant := AssistantMessage([]ContentPart{ToolCall("call_xxx|very/long+item==", "echo", map[string]any{"message": "hello"})}, "toolUse", source)
	toolResult := Message{Role: RoleToolResult, ToolCallID: "call_xxx|very/long+item==", ToolName: "echo", Content: []ContentPart{Text("hello")}}

	messages := ConvertOpenAICompletionsMessages(model, Context{Messages: []Message{assistant, toolResult}}, OpenAICompletionsCompat{})

	if len(messages) != 2 || len(messages[0].ToolCalls) != 1 {
		t.Fatalf("messages = %#v", messages)
	}
	if messages[0].ToolCalls[0].ID != "call_xxx" || messages[1].ToolCallID != "call_xxx" {
		t.Fatalf("tool ids = %#v / %q", messages[0].ToolCalls[0], messages[1].ToolCallID)
	}
}

func TestShouldSendOpenAICompletionsTools(t *testing.T) {
	if ShouldSendOpenAICompletionsTools(Context{Tools: []Tool{}}) {
		t.Fatal("empty tools without history should be omitted")
	}
	if !ShouldSendOpenAICompletionsTools(Context{Tools: []Tool{{Name: "echo"}}}) {
		t.Fatal("non-empty tools should be sent")
	}
	if !ShouldSendOpenAICompletionsTools(Context{Messages: []Message{
		AssistantMessage([]ContentPart{ToolCall("call-1", "echo", nil)}, "toolUse", Model{}),
	}}) {
		t.Fatal("tool history should preserve empty tools field")
	}
	if !ShouldSendOpenAICompletionsTools(Context{Messages: []Message{
		{Role: RoleToolResult, ToolCallID: "call-1", ToolName: "echo", Content: []ContentPart{Text("ok")}},
	}}) {
		t.Fatal("tool results should preserve empty tools field")
	}
}

func TestConvertOpenAICompletionsMessagesOmitsEmptyAssistantMessages(t *testing.T) {
	model := Model{ID: "model", Provider: "openai", API: "openai-completions", Input: []string{"text"}}
	emptyAssistant := AssistantMessage(nil, StopReasonStop, model)

	messages := ConvertOpenAICompletionsMessages(model, Context{Messages: []Message{emptyAssistant, UserMessageText("next")}}, OpenAICompletionsCompat{})

	if len(messages) != 1 || messages[0].Role != "user" {
		t.Fatalf("messages = %#v", messages)
	}
}
