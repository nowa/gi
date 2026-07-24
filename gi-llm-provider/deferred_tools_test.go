package gillmprovider

import (
	"encoding/json"
	"strings"
	"testing"
)

func deferredTestTool(name string) Tool {
	return Tool{
		Name:        name,
		Description: "The " + name + " tool",
		Parameters:  Object(map[string]Schema{"value": String()}, "value"),
	}
}

func deferredTestContext(tools []Tool, addedNames []string) Context {
	return Context{
		Messages: []Message{
			{Role: RoleUser, Content: []ContentPart{Text("Hello")}, Timestamp: 1},
			{
				Role:       RoleAssistant,
				Content:    []ContentPart{ToolCall("call_1", "base_tool", nil)},
				StopReason: StopReasonToolUse,
				Timestamp:  2,
			},
			{
				Role:           RoleToolResult,
				ToolCallID:     "call_1",
				ToolName:       "base_tool",
				Content:        []ContentPart{Text("done")},
				AddedToolNames: addedNames,
				Timestamp:      3,
			},
		},
		Tools: tools,
	}
}

func TestSplitDeferredTools(t *testing.T) {
	t.Run("does not resurrect a marked tool missing from Context.tools", func(t *testing.T) {
		context := deferredTestContext([]Tool{deferredTestTool("base_tool")}, []string{"late_tool"})
		split := SplitDeferredTools(context, true, nil)
		if len(split.Immediate) != 1 || split.Immediate[0].Name != "base_tool" || len(split.Deferred) != 0 {
			t.Fatalf("split = %#v", split)
		}
	})

	t.Run("keeps a tool immediate when it was used before its marker", func(t *testing.T) {
		context := deferredTestContext(
			[]Tool{deferredTestTool("base_tool"), deferredTestTool("late_tool")},
			[]string{"late_tool"},
		)
		context.Messages[1].Content = []ContentPart{ToolCall("call_1", "late_tool", nil)}

		split := SplitDeferredTools(context, true, nil)
		if len(split.Immediate) != 2 || len(split.Deferred) != 0 {
			t.Fatalf("split = %#v", split)
		}
	})

	t.Run("matches OAuth-canonicalized markers to active tools", func(t *testing.T) {
		context := deferredTestContext(
			[]Tool{deferredTestTool("base_tool"), deferredTestTool("read")},
			[]string{"Read"},
		)

		split := SplitDeferredTools(context, true, strings.ToLower)
		if len(split.Immediate) != 1 || split.Immediate[0].Name != "base_tool" || split.Deferred["read"].Name != "read" {
			t.Fatalf("split = %#v", split)
		}
	})

	t.Run("deduplicates active tools after OAuth canonicalization", func(t *testing.T) {
		context := Context{
			Messages: []Message{{Role: RoleUser, Content: []ContentPart{Text("Hello")}, Timestamp: 1}},
			Tools: []Tool{
				deferredTestTool("read"),
				{Name: "Read", Description: "Canonical definition", Parameters: Object(nil)},
			},
		}

		split := SplitDeferredTools(context, false, strings.ToLower)
		if len(split.Immediate) != 1 ||
			split.Immediate[0].Name != "Read" ||
			split.Immediate[0].Description != "Canonical definition" {
			t.Fatalf("split = %#v", split)
		}
	})
}

func TestAnthropicDeferredToolReferences(t *testing.T) {
	model := Model{
		ID:       "claude-opus-4-6",
		API:      "anthropic-messages",
		Provider: "anthropic",
		Input:    []string{"text", "image"},
	}
	context := deferredTestContext(
		[]Tool{deferredTestTool("base_tool"), deferredTestTool("late_tool")},
		[]string{"late_tool"},
	)

	payload, err := BuildAnthropicPayloadChecked(model, context, AnthropicPayloadOptions{CacheRetention: "none"})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Tools) != 2 ||
		payload.Tools[0].Name != "base_tool" ||
		payload.Tools[0].DeferLoading ||
		payload.Tools[1].Name != "late_tool" ||
		!payload.Tools[1].DeferLoading {
		t.Fatalf("tools = %#v", payload.Tools)
	}
	blocks := anthropicDeferredResultBlocks(t, payload)
	if len(blocks) != 2 ||
		blocks[0].Type != "tool_result" ||
		blocks[1].Type != "text" ||
		blocks[1].Text != "done" {
		t.Fatalf("tool result blocks = %#v", blocks)
	}
	references, ok := blocks[0].Content.([]AnthropicContentBlock)
	if !ok || len(references) != 1 ||
		references[0].Type != "tool_reference" ||
		references[0].ToolName != "late_tool" {
		t.Fatalf("tool references = %#v", blocks[0].Content)
	}
}

func TestAnthropicDeferredToolResultSiblingOrdering(t *testing.T) {
	model := Model{
		ID:       "claude-opus-4-6",
		API:      "anthropic-messages",
		Provider: "anthropic",
		Input:    []string{"text", "image"},
	}
	context := deferredTestContext(
		[]Tool{deferredTestTool("base_tool"), deferredTestTool("late_tool")},
		[]string{"late_tool"},
	)
	context.Messages[1].Content = []ContentPart{
		ToolCall("call_1", "base_tool", nil),
		ToolCall("call_2", "base_tool", nil),
	}
	context.Messages[2].Content = []ContentPart{
		Text("work completed"),
		Image("aW1hZ2U=", "image/png"),
	}
	context.Messages = append(context.Messages, Message{
		Role:       RoleToolResult,
		ToolCallID: "call_2",
		ToolName:   "base_tool",
		Content:    []ContentPart{Text("second result")},
		Timestamp:  4,
	})

	payload, err := BuildAnthropicPayloadChecked(model, context, AnthropicPayloadOptions{CacheRetention: "none"})
	if err != nil {
		t.Fatal(err)
	}
	blocks := anthropicDeferredResultBlocks(t, payload)
	if len(blocks) != 4 {
		t.Fatalf("blocks = %#v", blocks)
	}
	if blocks[0].Type != "tool_result" ||
		blocks[1].Type != "tool_result" ||
		blocks[1].Content != "second result" ||
		blocks[2].Type != "text" ||
		blocks[2].Text != "work completed" ||
		blocks[3].Type != "image" ||
		blocks[3].Source == nil ||
		blocks[3].Source.Data != "aW1hZ2U=" {
		t.Fatalf("blocks = %#v", blocks)
	}
}

func TestAnthropicDeferredToolCompatibilityAndNormalization(t *testing.T) {
	context := deferredTestContext(
		[]Tool{deferredTestTool("base_tool"), deferredTestTool("read")},
		[]string{"Read"},
	)
	model := Model{
		ID:       "claude-opus-4-6",
		API:      "anthropic-messages",
		Provider: "anthropic",
		Input:    []string{"text"},
	}
	payload, err := BuildAnthropicPayloadChecked(
		model,
		context,
		AnthropicPayloadOptions{IsOAuthToken: true, CacheRetention: "none"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Tools) != 2 ||
		payload.Tools[1].Name != "Read" ||
		!payload.Tools[1].DeferLoading {
		t.Fatalf("OAuth tools = %#v", payload.Tools)
	}
	references := anthropicDeferredResultBlocks(t, payload)[0].Content.([]AnthropicContentBlock)
	if len(references) != 1 || references[0].ToolName != "Read" {
		t.Fatalf("OAuth references = %#v", references)
	}

	t.Run("unsupported models keep the normal list", func(t *testing.T) {
		unsupported := model
		unsupported.ID = "claude-haiku-4-5"
		plainContext := deferredTestContext(
			[]Tool{deferredTestTool("base_tool"), deferredTestTool("late_tool")},
			[]string{"late_tool"},
		)
		plain, err := BuildAnthropicPayloadChecked(
			unsupported,
			plainContext,
			AnthropicPayloadOptions{CacheRetention: "none"},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(plain.Tools) != 2 || plain.Tools[1].DeferLoading {
			t.Fatalf("tools = %#v", plain.Tools)
		}
	})

	t.Run("an explicit compatibility override enables references", func(t *testing.T) {
		proxy := model
		proxy.Provider = "anthropic-proxy"
		proxy.Compat.SupportsToolReferences = ptrBool(true)
		proxyContext := deferredTestContext(
			[]Tool{deferredTestTool("base_tool"), deferredTestTool("late_tool")},
			[]string{"late_tool"},
		)
		proxyPayload, err := BuildAnthropicPayloadChecked(
			proxy,
			proxyContext,
			AnthropicPayloadOptions{CacheRetention: "none"},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(proxyPayload.Tools) != 2 || !proxyPayload.Tools[1].DeferLoading {
			t.Fatalf("tools = %#v", proxyPayload.Tools)
		}
	})

	t.Run("all deferred tools fall back to immediate", func(t *testing.T) {
		allDeferredContext := deferredTestContext(
			[]Tool{deferredTestTool("late_tool")},
			[]string{"late_tool"},
		)
		allDeferred, err := BuildAnthropicPayloadChecked(
			model,
			allDeferredContext,
			AnthropicPayloadOptions{CacheRetention: "none"},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(allDeferred.Tools) != 1 || allDeferred.Tools[0].DeferLoading {
			t.Fatalf("tools = %#v", allDeferred.Tools)
		}
		content := anthropicDeferredResultBlocks(t, allDeferred)[0].Content
		if references, ok := content.([]AnthropicContentBlock); ok {
			for _, reference := range references {
				if reference.Type == "tool_reference" {
					t.Fatalf("unexpected reference in %#v", content)
				}
			}
		}
	})
}

func TestKimiDeferredToolsUseSystemMessages(t *testing.T) {
	model := Model{
		ID:       "deferred-tools-model",
		API:      "openai-completions",
		Provider: "moonshotai",
		Input:    []string{"text"},
		Compat:   ModelCompat{DeferredToolsMode: "kimi"},
	}
	context := deferredTestContext(
		[]Tool{
			deferredTestTool("base_tool"),
			deferredTestTool("late_tool"),
			deferredTestTool("later_tool"),
		},
		[]string{"late_tool"},
	)
	context.Messages = append(context.Messages, Message{
		Role:           RoleToolResult,
		ToolCallID:     "call_2",
		ToolName:       "base_tool",
		Content:        []ContentPart{Text("second")},
		AddedToolNames: []string{"later_tool"},
		Timestamp:      4,
	})
	context.Messages = append(context.Messages, UserMessageText("next"))

	payload := BuildOpenAICompletionsPayload(model, context, OpenAICompletionsPayloadOptions{})
	if len(payload.Tools) != 1 || payload.Tools[0].Function.Name != "base_tool" {
		t.Fatalf("top-level tools = %#v", payload.Tools)
	}
	roles := make([]string, 0, len(payload.Messages))
	for _, message := range payload.Messages {
		roles = append(roles, message.Role)
	}
	if strings.Join(roles, ",") != "user,assistant,tool,tool,system,user" {
		t.Fatalf("roles = %#v", roles)
	}
	systemTools := payload.Messages[4].Tools
	if len(systemTools) != 2 ||
		systemTools[0].Function.Name != "late_tool" ||
		systemTools[1].Function.Name != "later_tool" {
		t.Fatalf("system tools = %#v", systemTools)
	}

	model.Compat.DeferredToolsMode = ""
	plain := BuildOpenAICompletionsPayload(model, context, OpenAICompletionsPayloadOptions{})
	if len(plain.Tools) != 3 {
		t.Fatalf("plain tools = %#v", plain.Tools)
	}
	for _, message := range plain.Messages {
		if len(message.Tools) > 0 {
			t.Fatalf("unexpected system tool message = %#v", message)
		}
	}
}

func TestOpenAIResponsesDeferredToolsUseClientToolSearch(t *testing.T) {
	model := Model{
		ID:       "gpt-5.4",
		API:      "openai-responses",
		Provider: "openai",
		Input:    []string{"text"},
		Compat: ModelCompat{
			SupportsStrictMode: ptrBool(true),
			SupportsToolSearch: ptrBool(true),
		},
	}
	context := deferredTestContext(
		[]Tool{deferredTestTool("base_tool"), deferredTestTool("late_tool")},
		[]string{"late_tool"},
	)

	payload, err := BuildOpenAIResponsesPayloadChecked(model, context, OpenAIResponsesPayloadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Tools) != 1 || payload.Tools[0].Name != "base_tool" {
		t.Fatalf("top-level tools = %#v", payload.Tools)
	}
	searchCall, searchOutput := findOpenAIToolSearchItems(t, payload.Input)
	if searchCall.Execution != "client" ||
		searchCall.Status != "completed" ||
		searchCall.ToolSearchArguments == nil ||
		searchCall.ToolSearchArguments.Query != "late_tool" ||
		searchCall.ToolSearchArguments.Limit != 1 {
		t.Fatalf("search call = %#v", searchCall)
	}
	if searchOutput.CallID != searchCall.CallID ||
		len(searchOutput.Tools) != 1 ||
		searchOutput.Tools[0].Name != "late_tool" ||
		!searchOutput.Tools[0].DeferLoading {
		t.Fatalf("search output = %#v", searchOutput)
	}
	raw, err := json.Marshal(searchCall)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	arguments, ok := wire["arguments"].(map[string]any)
	if !ok || arguments["query"] != "late_tool" || arguments["limit"] != float64(1) {
		t.Fatalf("wire arguments = %#v", wire["arguments"])
	}
	var roundTrip OpenAIResponsesInputItem
	if err := json.Unmarshal(raw, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if roundTrip.ToolSearchArguments == nil ||
		roundTrip.ToolSearchArguments.Query != "late_tool" ||
		roundTrip.ToolSearchArguments.Limit != 1 {
		t.Fatalf("round-trip search call = %#v", roundTrip)
	}

	model.Compat.SupportsToolSearch = ptrBool(false)
	plain, err := BuildOpenAIResponsesPayloadChecked(model, context, OpenAIResponsesPayloadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plain.Tools) != 2 {
		t.Fatalf("plain tools = %#v", plain.Tools)
	}
	for _, item := range plain.Input {
		if item.Type == "tool_search_call" || item.Type == "tool_search_output" {
			t.Fatalf("unexpected tool search item = %#v", item)
		}
	}
}

func TestOpenAICodexDeferredToolsHonorModelCompatibility(t *testing.T) {
	context := deferredTestContext(
		[]Tool{deferredTestTool("base_tool"), deferredTestTool("late_tool")},
		[]string{"late_tool"},
	)
	model := Model{
		ID:       "gpt-5.4",
		API:      "openai-codex-responses",
		Provider: "openai-codex",
		Input:    []string{"text"},
		Compat: ModelCompat{
			SupportsStrictMode: ptrBool(true),
			SupportsToolSearch: ptrBool(true),
		},
	}
	payload, err := BuildOpenAICodexResponsesPayloadChecked(model, context, OpenAICodexResponsesPayloadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Tools) != 1 || payload.Tools[0].Name != "base_tool" {
		t.Fatalf("top-level tools = %#v", payload.Tools)
	}
	_, searchOutput := findOpenAIToolSearchItems(t, payload.Input)
	if len(searchOutput.Tools) != 1 ||
		!searchOutput.Tools[0].DeferLoading ||
		!searchOutput.Tools[0].StrictNull {
		t.Fatalf("search output tools = %#v", searchOutput.Tools)
	}

	model.Compat.SupportsToolSearch = ptrBool(false)
	plain, err := BuildOpenAICodexResponsesPayloadChecked(model, context, OpenAICodexResponsesPayloadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plain.Tools) != 2 {
		t.Fatalf("plain tools = %#v", plain.Tools)
	}
}

func anthropicDeferredResultBlocks(t *testing.T, payload AnthropicPayload) []AnthropicContentBlock {
	t.Helper()
	for _, message := range payload.Messages {
		blocks, ok := message.Content.([]AnthropicContentBlock)
		if !ok {
			continue
		}
		for _, block := range blocks {
			if block.Type == "tool_result" {
				return blocks
			}
		}
	}
	t.Fatal("no Anthropic tool result found")
	return nil
}

func findOpenAIToolSearchItems(
	t *testing.T,
	items []OpenAIResponsesInputItem,
) (OpenAIResponsesInputItem, OpenAIResponsesInputItem) {
	t.Helper()
	var searchCall OpenAIResponsesInputItem
	var searchOutput OpenAIResponsesInputItem
	for _, item := range items {
		switch item.Type {
		case "tool_search_call":
			searchCall = item
		case "tool_search_output":
			searchOutput = item
		}
	}
	if searchCall.Type == "" || searchOutput.Type == "" {
		t.Fatalf("tool search items not found in %#v", items)
	}
	return searchCall, searchOutput
}
