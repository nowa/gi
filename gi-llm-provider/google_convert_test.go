package gillmprovider

import (
	"reflect"
	"testing"
)

func TestSanitizeSchemaForOpenAPIStripsMetaKeysRecursively(t *testing.T) {
	schema := map[string]any{
		"$schema":     "http://json-schema.org/draft-07/schema#",
		"$id":         "urn:bash-tool",
		"$comment":    "demo",
		"$defs":       map[string]any{"commandDef": map[string]any{"type": "string"}},
		"definitions": map[string]any{"legacyDef": map[string]any{"type": "number"}},
		"type":        "object",
		"properties": map[string]any{
			"command": map[string]any{"$schema": "nested", "$id": "urn:nested", "type": "string"},
			"refProp": map[string]any{"$ref": "#/$defs/someDef", "type": "string"},
		},
		"required": []any{"command"},
	}

	got := SanitizeSchemaForOpenAPI(schema)

	want := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{"type": "string"},
			"refProp": map[string]any{"$ref": "#/$defs/someDef", "type": "string"},
		},
		"required": []any{"command"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sanitized = %#v", got)
	}
	if _, ok := schema["$schema"]; !ok {
		t.Fatal("SanitizeSchemaForOpenAPI mutated original map")
	}
}

func TestConvertGoogleToolsReturnsNilForEmptyAndUsesParametersMode(t *testing.T) {
	if got := ConvertGoogleTools(nil, true); got != nil {
		t.Fatalf("empty tools = %#v", got)
	}
	tools := []Tool{{
		Name:        "test_tool",
		Description: "A test tool",
		Parameters:  Object(map[string]Schema{"command": String()}, "command"),
	}}

	result := ConvertGoogleTools(tools, true)

	if len(result) != 1 || len(result[0].FunctionDeclarations) != 1 {
		t.Fatalf("tools = %#v", result)
	}
	declaration := result[0].FunctionDeclarations[0]
	if declaration.Name != "test_tool" || declaration.Parameters == nil || declaration.ParametersJSONSchema != nil {
		t.Fatalf("declaration = %#v", declaration)
	}
}

func TestConvertGoogleMessagesGemini3UnsignedToolCalls(t *testing.T) {
	model := Model{ID: "gemini-3-pro-preview", Provider: "google", API: "google-generative-ai", Reasoning: true, Input: []string{"text"}}
	context := googleToolCallContext(model, "")

	contents := ConvertGoogleMessages(model, context)

	modelTurn := findGoogleRole(contents, "model")
	if modelTurn == nil {
		t.Fatalf("contents = %#v", contents)
	}
	var functionCalls []GooglePart
	for _, part := range modelTurn.Parts {
		if part.FunctionCall != nil {
			functionCalls = append(functionCalls, part)
		}
	}
	if len(functionCalls) != 2 || functionCalls[0].ThoughtSignature != "" || functionCalls[1].ThoughtSignature != "" {
		t.Fatalf("function calls = %#v", functionCalls)
	}
}

func TestConvertGoogleMessagesPreservesValidThoughtSignatureForSameModel(t *testing.T) {
	model := Model{ID: "gemini-3-pro-preview", Provider: "google", API: "google-generative-ai", Reasoning: true, Input: []string{"text"}}

	contents := ConvertGoogleMessages(model, googleToolCallContext(model, "AAAAAAAAAAAAAAAAAAAAAA=="))

	modelTurn := findGoogleRole(contents, "model")
	if modelTurn == nil || len(modelTurn.Parts) < 1 || modelTurn.Parts[0].ThoughtSignature != "AAAAAAAAAAAAAAAAAAAAAA==" {
		t.Fatalf("model turn = %#v", modelTurn)
	}
}

func TestConvertGoogleMessagesImageToolResultRouting(t *testing.T) {
	model2 := Model{ID: "gemini-2.5-flash", Provider: "google", API: "google-generative-ai", Reasoning: true, Input: []string{"text", "image"}}
	contents2 := ConvertGoogleMessages(model2, googleImageToolResultContext(model2))
	if len(contents2) != 5 {
		t.Fatalf("gemini2 contents = %#v", contents2)
	}
	if !allGoogleFunctionResponses(contents2[2].Parts) || contents2[3].Parts[0].Text != "Tool result image:" || contents2[3].Parts[1].InlineData == nil || contents2[4].Parts[0].FunctionResponse == nil {
		t.Fatalf("gemini2 routing = %#v", contents2)
	}

	model3 := Model{ID: "gemini-3-pro-preview", Provider: "google", API: "google-generative-ai", Reasoning: true, Input: []string{"text", "image"}}
	contents3 := ConvertGoogleMessages(model3, googleImageToolResultContext(model3))
	if len(contents3) != 3 {
		t.Fatalf("gemini3 contents = %#v", contents3)
	}
	if len(contents3[2].Parts) != 3 || contents3[2].Parts[1].FunctionResponse == nil || len(contents3[2].Parts[1].FunctionResponse.Parts) != 1 || contents3[2].Parts[1].FunctionResponse.Parts[0].InlineData == nil {
		t.Fatalf("gemini3 routing = %#v", contents3)
	}
}

func TestGoogleThinkingDetectionAndSignatureRetention(t *testing.T) {
	if !IsGoogleThinkingPart(GooglePart{Thought: true}) {
		t.Fatal("thought=true should be thinking")
	}
	if !IsGoogleThinkingPart(GooglePart{Thought: true, ThoughtSignature: "opaque-signature"}) {
		t.Fatal("thought=true with signature should be thinking")
	}
	if IsGoogleThinkingPart(GooglePart{ThoughtSignature: "opaque-signature"}) {
		t.Fatal("signature alone should not be thinking")
	}
	if IsGoogleThinkingPart(GooglePart{Thought: false, ThoughtSignature: "opaque-signature"}) {
		t.Fatal("thought=false should not be thinking")
	}

	first := RetainGoogleThoughtSignature("", "sig-1")
	if first != "sig-1" {
		t.Fatalf("first = %q", first)
	}
	second := RetainGoogleThoughtSignature(first, "")
	if second != "sig-1" {
		t.Fatalf("second = %q", second)
	}
	third := RetainGoogleThoughtSignature(second, "sig-2")
	if third != "sig-2" {
		t.Fatalf("third = %q", third)
	}
}

func TestGoogleThinkingConfigDisableAndBudgets(t *testing.T) {
	disabled2 := BuildGoogleThinkingConfig(Model{ID: "gemini-2.5-flash", Reasoning: true}, GoogleThinkingOptions{})
	if disabled2 == nil || disabled2.ThinkingBudget == nil || *disabled2.ThinkingBudget != 0 || disabled2.IncludeThoughts != nil {
		t.Fatalf("gemini 2.5 disabled = %#v", disabled2)
	}

	disabledFlash3 := BuildGoogleThinkingConfig(Model{ID: "gemini-3-flash-preview", Reasoning: true}, GoogleThinkingOptions{})
	if disabledFlash3 == nil || disabledFlash3.ThinkingLevel != "MINIMAL" || disabledFlash3.IncludeThoughts != nil {
		t.Fatalf("gemini 3 flash disabled = %#v", disabledFlash3)
	}

	disabledPro31 := BuildGoogleThinkingConfig(Model{ID: "gemini-3.1-pro-preview", Reasoning: true}, GoogleThinkingOptions{})
	if disabledPro31 == nil || disabledPro31.ThinkingLevel != "LOW" || disabledPro31.IncludeThoughts != nil {
		t.Fatalf("gemini 3.1 pro disabled = %#v", disabledPro31)
	}

	enabled25 := BuildGoogleThinkingConfig(Model{ID: "gemini-2.5-flash", Reasoning: true}, GoogleThinkingOptions{Reasoning: "high"})
	if enabled25 == nil || enabled25.IncludeThoughts == nil || !*enabled25.IncludeThoughts || enabled25.ThinkingBudget == nil || *enabled25.ThinkingBudget != 24576 {
		t.Fatalf("gemini 2.5 enabled = %#v", enabled25)
	}

	enabled3 := BuildGoogleThinkingConfig(Model{ID: "gemini-3-pro-preview", Reasoning: true}, GoogleThinkingOptions{Reasoning: "medium"})
	if enabled3 == nil || enabled3.IncludeThoughts == nil || !*enabled3.IncludeThoughts || enabled3.ThinkingLevel != "HIGH" {
		t.Fatalf("gemini 3 enabled = %#v", enabled3)
	}

	custom := BuildGoogleThinkingConfig(Model{ID: "gemini-2.5-pro", Reasoning: true}, GoogleThinkingOptions{Reasoning: "low", CustomBudgets: map[string]int{"low": 4096}})
	if custom == nil || custom.ThinkingBudget == nil || *custom.ThinkingBudget != 4096 {
		t.Fatalf("custom budget = %#v", custom)
	}
}

func googleToolCallContext(model Model, thoughtSignature string) Context {
	firstToolCall := ToolCall("call_1", "bash", map[string]any{"command": "echo hi"})
	firstToolCall.ThoughtSignature = thoughtSignature
	return Context{Messages: []Message{
		UserMessageText("Hi"),
		AssistantMessage([]ContentPart{
			firstToolCall,
			ToolCall("call_2", "bash", map[string]any{"command": "ls -la"}),
		}, "toolUse", model),
	}}
}

func googleImageToolResultContext(model Model) Context {
	return Context{Messages: []Message{
		UserMessageText("read the files"),
		AssistantMessage([]ContentPart{
			ToolCall("call_a", "read", map[string]any{"path": "a.txt"}),
			ToolCall("call_img", "read", map[string]any{"path": "image.png"}),
			ToolCall("call_b", "read", map[string]any{"path": "b.txt"}),
		}, "toolUse", model),
		{Role: RoleToolResult, ToolCallID: "call_a", ToolName: "read", Content: []ContentPart{Text("alpha text")}},
		{Role: RoleToolResult, ToolCallID: "call_img", ToolName: "read", Content: []ContentPart{Image("abc", "image/png")}},
		{Role: RoleToolResult, ToolCallID: "call_b", ToolName: "read", Content: []ContentPart{Text("beta text")}},
	}}
}

func findGoogleRole(contents []GoogleContent, role string) *GoogleContent {
	for i := range contents {
		if contents[i].Role == role {
			return &contents[i]
		}
	}
	return nil
}

func allGoogleFunctionResponses(parts []GooglePart) bool {
	if len(parts) == 0 {
		return false
	}
	for _, part := range parts {
		if part.FunctionResponse == nil {
			return false
		}
	}
	return true
}
