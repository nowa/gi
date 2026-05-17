package gillmprovider

import (
	"reflect"
	"strings"
	"testing"
)

func TestSanitizeSurrogatesKeepsValidUTF8AndDropsInvalidSequences(t *testing.T) {
	valid := "Hello 🙈 你好"
	if got := SanitizeSurrogates(valid); got != valid {
		t.Fatalf("valid sanitized = %q", got)
	}
	invalidHighSurrogateBytes := string([]byte{0xed, 0xa0, 0xbd})
	got := SanitizeSurrogates("before " + invalidHighSurrogateBytes + " after")
	if got != "before  after" {
		t.Fatalf("invalid surrogate sanitized = %q", got)
	}
}

func TestTransformMessagesDowngradesImagesForTextOnlyModels(t *testing.T) {
	model := Model{ID: "text", Provider: "openai", API: "openai-completions", Input: []string{"text"}}
	messages := []Message{
		{Role: RoleUser, Content: []ContentPart{Text("a"), Image("xxx", "image/png"), Image("yyy", "image/png"), Text("b")}},
		{Role: RoleToolResult, ToolCallID: "call-1", ToolName: "tool", Content: []ContentPart{Image("zzz", "image/png"), Text("result")}},
	}

	transformed := TransformMessages(messages, model, nil)

	if got := contentTexts(transformed[0].Content); !reflect.DeepEqual(got, []string{"a", NonVisionUserImagePlaceholder, "b"}) {
		t.Fatalf("user content = %#v", got)
	}
	if got := contentTexts(transformed[1].Content); !reflect.DeepEqual(got, []string{NonVisionToolImagePlaceholder, "result"}) {
		t.Fatalf("tool content = %#v", got)
	}
	if messages[0].Content[1].Type != ContentImage {
		t.Fatal("TransformMessages mutated input message")
	}
}

func TestTransformMessagesKeepsSameModelThinkingAndDropsCrossModelOpaqueThinking(t *testing.T) {
	target := Model{ID: "target", Provider: "anthropic", API: "anthropic-messages", Input: []string{"text"}}
	sameModel := AssistantMessage([]ContentPart{
		Thinking("signed"),
		{Type: ContentThinking, Thinking: "", ThinkingSignature: "sig"},
		{Type: ContentThinking, Thinking: "secret", Redacted: true},
	}, StopReasonStop, target)
	crossModel := sameModel
	crossModel.Provider = "openai"
	crossModel.API = "openai-responses"
	crossModel.Model = "source"

	same := TransformMessages([]Message{sameModel}, target, nil)
	cross := TransformMessages([]Message{crossModel}, target, nil)

	if len(same) != 1 || len(same[0].Content) != 3 || same[0].Content[1].ThinkingSignature != "sig" {
		t.Fatalf("same-model content = %#v", same)
	}
	if got := cross[0].Content; len(got) != 1 || got[0].Type != ContentText || got[0].Text != "signed" {
		t.Fatalf("cross-model content = %#v", got)
	}
}

func TestTransformMessagesNormalizesToolCallIDsAndToolResults(t *testing.T) {
	target := Model{ID: "gpt-5", Provider: "openrouter", API: "openai-completions", Input: []string{"text"}}
	source := Model{ID: "gpt-5.2-codex", Provider: "github-copilot", API: "openai-responses", Input: []string{"text"}}
	longID := "call_abc|very/long+item=="
	assistant := AssistantMessage([]ContentPart{ToolCall(longID, "echo", map[string]any{"message": "hello"})}, "toolUse", source)
	toolResult := Message{Role: RoleToolResult, ToolCallID: longID, ToolName: "echo", Content: []ContentPart{Text("hello")}, Timestamp: NowMillis()}

	transformed := TransformMessages([]Message{assistant, toolResult}, target, func(id string, model Model, source Message) string {
		return NormalizeToolCallIDForOpenAICompletions(id, model)
	})

	if len(transformed) != 2 {
		t.Fatalf("transformed length = %d: %#v", len(transformed), transformed)
	}
	normalizedID := transformed[0].Content[0].ID
	if normalizedID != "call_abc" {
		t.Fatalf("normalized id = %q", normalizedID)
	}
	if transformed[1].ToolCallID != normalizedID {
		t.Fatalf("tool result id = %q want %q", transformed[1].ToolCallID, normalizedID)
	}
	if transformed[0].Content[0].ThoughtSignature != "" {
		t.Fatalf("thought signature should be stripped: %#v", transformed[0].Content[0])
	}
}

func TestTransformMessagesInsertsSyntheticToolResultsForOrphanedCalls(t *testing.T) {
	target := Model{ID: "model", Provider: "anthropic", API: "anthropic-messages", Input: []string{"text"}}
	assistant := AssistantMessage([]ContentPart{ToolCall("call-1", "missing", map[string]any{})}, "toolUse", target)
	user := UserMessageText("next")

	transformed := TransformMessages([]Message{assistant, user}, target, nil)

	if len(transformed) != 3 {
		t.Fatalf("transformed = %#v", transformed)
	}
	if transformed[1].Role != RoleToolResult || transformed[1].ToolCallID != "call-1" || !transformed[1].IsError || transformed[1].Content[0].Text != "No result provided" {
		t.Fatalf("synthetic tool result = %#v", transformed[1])
	}
}

func TestTransformMessagesSkipsErroredAssistantMessages(t *testing.T) {
	target := Model{ID: "model", Provider: "anthropic", API: "anthropic-messages", Input: []string{"text"}}
	errored := AssistantErrorMessage("boom", target, false)
	ok := UserMessageText("continue")

	transformed := TransformMessages([]Message{errored, ok}, target, nil)

	if len(transformed) != 1 || transformed[0].Role != RoleUser {
		t.Fatalf("transformed = %#v", transformed)
	}
}

func TestNormalizeToolCallIDHelpers(t *testing.T) {
	if got := NormalizeToolCallIDForAnthropic("call/with+bad=chars"); got != "call_with_bad_chars" {
		t.Fatalf("anthropic id = %q", got)
	}
	openAI := Model{Provider: "openai"}
	if got := NormalizeToolCallIDForOpenAICompletions(strings.Repeat("a", 45), openAI); len(got) != 40 {
		t.Fatalf("openai completion id len = %d", len(got))
	}
	source := AssistantMessage([]ContentPart{ToolCall("call_1|foreign/item+id", "echo", nil)}, "toolUse", Model{ID: "source", Provider: "github-copilot", API: "openai-responses"})
	target := Model{ID: "target", Provider: "openai-codex", API: "openai-codex-responses"}
	got := NormalizeToolCallIDForOpenAIResponses("call_1|foreign/item+id", target, source, map[string]bool{"openai-codex": true})
	if !strings.HasPrefix(got, "call_1|fc_") || len(got) > len("call_1|")+64 {
		t.Fatalf("responses id = %q", got)
	}
	if got != "call_1|fc_lwbr2h1p12rcb" {
		t.Fatalf("responses id hash = %q", got)
	}
}

func contentTexts(content []ContentPart) []string {
	var texts []string
	for _, part := range content {
		if part.Type == ContentText {
			texts = append(texts, part.Text)
		}
	}
	return texts
}
