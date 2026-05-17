package gillmprovider

import (
	"encoding/json"
	"strings"
	"testing"
)

const copilotRawToolCallID = "call_4VnzVawQXPB9MgYib7CiQFEY|I9b95oN1wD/cHXKTw3PpRkL6KkCtzTJhUxMouMWYwHeTo2j3htzfSk7YPx2vifiIM4g3A8XXyOj8q4Bt6SLUG7gqY1E3ELkrkVQNHglRfUmWj84lqxJY+Puieb3VKyX0FB+83TUzn91cDMF/4gzt990IzqVrc+nIb9RRscRD070Du16q1glydVjWR0SBJsE6TbY/esOjFpqplogQqrajm1eI++f3eLi73R6q7hVusY0QbeFySVxABCjhN0lXB04caBe1rzHjYzul6MAXj7uq+0r17VLq+yrtyYhN12wkmFqHeqTyEei6EFPbMy24Nc+IbJlkP0OCg02W+gOnyBFcbi2ctvJFSOhSjt1CqBdqCnnhwUqXjbWiT0wh3DmLScRgTHmGkaI+oAcQQjfic65nxj+TnEkReA=="

func TestConvertOpenAIResponsesMessagesHashesForeignToolItemID(t *testing.T) {
	model := Model{ID: "gpt-5.3-codex", Provider: "openai-codex", API: "openai-codex-responses", Reasoning: true, Input: []string{"text"}}
	assistant := AssistantMessage([]ContentPart{ToolCall(copilotRawToolCallID, "edit", map[string]any{"path": "src/styles/app.css"})}, "toolUse", Model{
		ID:       "gpt-5.3-codex",
		Provider: "github-copilot",
		API:      "openai-responses",
	})
	toolResult := Message{Role: RoleToolResult, ToolCallID: copilotRawToolCallID, ToolName: "edit", Content: []ContentPart{Text("ok")}, Timestamp: NowMillis()}
	context := Context{SystemPrompt: "You are concise.", Messages: []Message{UserMessageText("Use the tool."), assistant, toolResult}}

	input := ConvertOpenAIResponsesMessages(model, context, ConvertOpenAIResponsesOptions{
		AllowedToolCallProviders: map[string]bool{"openai": true, "openai-codex": true, "opencode": true},
	})

	if len(input) < 4 || input[0].Role != "developer" {
		t.Fatalf("input = %#v", input)
	}
	var functionCall OpenAIResponsesInputItem
	for _, item := range input {
		if item.Type == "function_call" {
			functionCall = item
			break
		}
	}
	if functionCall.Type == "" {
		t.Fatalf("missing function call in %#v", input)
	}
	expectedItemID := "fc_" + shortHash(strings.Split(copilotRawToolCallID, "|")[1])
	if functionCall.ID != expectedItemID || len(functionCall.ID) > 64 || functionCall.CallID != "call_4VnzVawQXPB9MgYib7CiQFEY" {
		t.Fatalf("function call = %#v expected id %s", functionCall, expectedItemID)
	}
	var arguments map[string]any
	if err := json.Unmarshal([]byte(functionCall.Arguments), &arguments); err != nil {
		t.Fatal(err)
	}
	if arguments["path"] != "src/styles/app.css" {
		t.Fatalf("arguments = %#v", arguments)
	}
}

func TestConvertOpenAIResponsesMessagesHandlesEmptyAndImageToolOutputs(t *testing.T) {
	model := Model{ID: "gpt-4o", Provider: "openai", API: "openai-responses", Input: []string{"text", "image"}}
	messages := []Message{
		UserMessageText("hello"),
		Message{Role: RoleToolResult, ToolCallID: "call-1|fc-1", ToolName: "screenshot", Content: []ContentPart{Text("see"), Image("abc", "image/png")}},
	}

	input := ConvertOpenAIResponsesMessages(model, Context{Messages: messages}, ConvertOpenAIResponsesOptions{})

	last := input[len(input)-1]
	parts, ok := last.Output.([]OpenAIResponsesContentPart)
	if !ok || len(parts) != 2 || parts[0].Type != "input_text" || parts[1].Type != "input_image" {
		t.Fatalf("tool output = %#v", last.Output)
	}
	if last.CallID != "call-1" {
		t.Fatalf("call id = %q", last.CallID)
	}
}
