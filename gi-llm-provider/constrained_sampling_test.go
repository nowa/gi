package gillmprovider

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func constrainedSamplingTestModel() Model {
	return Model{
		ID:       "gpt-test",
		Name:     "GPT Test",
		API:      "openai-responses",
		Provider: "openai",
		Input:    []string{"text", "image"},
	}
}

func constrainedSamplingTestTool(config *ConstrainedSamplingConfig) Tool {
	return Tool{
		Name:                "sample_tool",
		Description:         "Sample tool",
		Parameters:          Object(map[string]Schema{"payload": String()}, "payload"),
		ConstrainedSampling: config,
	}
}

func TestConstrainedToolSampling(t *testing.T) {
	t.Run("converts supported constraints and falls back when unsupported", func(t *testing.T) {
		jsonSchemaTool := constrainedSamplingTestTool(&ConstrainedSamplingConfig{
			Type:   ConstrainedSamplingJSONSchema,
			Strict: ConstrainedSamplingPrefer,
		})
		converted, err := ConvertOpenAIResponsesToolsChecked([]Tool{jsonSchemaTool}, OpenAIResponsesToolOptions{
			SupportsStrictMode: ptrBool(true),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(converted) != 1 || converted[0].Strict == nil || !*converted[0].Strict {
			t.Fatalf("JSON-schema tool = %#v", converted)
		}

		required := jsonSchemaTool
		required.ConstrainedSampling = &ConstrainedSamplingConfig{
			Type:   ConstrainedSamplingJSONSchema,
			Strict: ConstrainedSamplingRequire,
		}
		if _, err := ConvertOpenAIResponsesToolsChecked(
			[]Tool{required},
			OpenAIResponsesToolOptions{SupportsStrictMode: ptrBool(false)},
		); err == nil || !strings.Contains(err.Error(), `tool "sample_tool" requires JSON-schema constrained sampling`) {
			t.Fatalf("required strict error = %v", err)
		}

		grammarTool := constrainedSamplingTestTool(&ConstrainedSamplingConfig{
			Type: ConstrainedSamplingGrammar,
			Variants: GrammarVariants{
				OpenAILark: "start: /[a-z]+/",
			},
		})
		converted, err = ConvertOpenAIResponsesToolsChecked([]Tool{grammarTool}, OpenAIResponsesToolOptions{
			SupportsOpenAIGrammarTools: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(converted) != 1 ||
			converted[0].Type != "custom" ||
			converted[0].Format == nil ||
			converted[0].Format.Syntax != GrammarFormatLark ||
			converted[0].Format.Definition != "start: /[a-z]+/" {
			t.Fatalf("grammar tool = %#v", converted)
		}

		missingVariant := grammarTool
		missingVariant.ConstrainedSampling = &ConstrainedSamplingConfig{Type: ConstrainedSamplingGrammar}
		if _, err := ConvertOpenAIResponsesToolsChecked(
			[]Tool{missingVariant},
			OpenAIResponsesToolOptions{SupportsOpenAIGrammarTools: true},
		); err == nil || !strings.Contains(err.Error(), "no supported grammar variant was provided") {
			t.Fatalf("missing grammar error = %v", err)
		}

		fallback, err := ConvertOpenAIResponsesToolsChecked(
			[]Tool{grammarTool},
			OpenAIResponsesToolOptions{SupportsStrictMode: ptrBool(false)},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(fallback) != 1 || fallback[0].Type != "function" || fallback[0].Strict != nil {
			t.Fatalf("fallback = %#v", fallback)
		}

		plain, err := ConvertOpenAIResponsesToolsChecked(
			[]Tool{constrainedSamplingTestTool(nil)},
			OpenAIResponsesToolOptions{},
		)
		if err != nil {
			t.Fatal(err)
		}
		disabled, err := ConvertOpenAIResponsesToolsChecked(
			[]Tool{constrainedSamplingTestTool(nil)},
			OpenAIResponsesToolOptions{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(disabled, plain) {
			t.Fatalf("disabled = %#v, plain = %#v", disabled, plain)
		}
	})

	t.Run("applies Anthropic strict-tool compatibility", func(t *testing.T) {
		strictTool := constrainedSamplingTestTool(&ConstrainedSamplingConfig{
			Type:   ConstrainedSamplingJSONSchema,
			Strict: ConstrainedSamplingPrefer,
		})
		converted, err := ConvertAnthropicToolsChecked([]Tool{strictTool}, AnthropicToolOptions{
			SupportsStrictTools: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(converted) != 1 ||
			converted[0].Strict == nil ||
			!*converted[0].Strict ||
			converted[0].InputSchema["type"] != "object" {
			t.Fatalf("Anthropic strict tool = %#v", converted)
		}

		required := strictTool
		required.ConstrainedSampling = &ConstrainedSamplingConfig{
			Type:   ConstrainedSamplingJSONSchema,
			Strict: ConstrainedSamplingRequire,
		}
		if _, err := ConvertAnthropicToolsChecked(
			[]Tool{required},
			AnthropicToolOptions{},
		); err == nil || !strings.Contains(err.Error(), `tool "sample_tool" requires JSON-schema constrained sampling`) {
			t.Fatalf("required Anthropic strict error = %v", err)
		}
	})

	t.Run("replays grammar calls as custom Responses items", func(t *testing.T) {
		toolCall := ToolCall("call_1|ctc_1", "sample_tool", map[string]any{})
		context := Context{
			Messages: []Message{
				{
					Role:       RoleAssistant,
					Content:    []ContentPart{toolCall},
					API:        "openai-responses",
					Provider:   "openai",
					Model:      "gpt-test",
					StopReason: StopReasonToolUse,
				},
				{
					Role:       RoleToolResult,
					ToolCallID: "call_1|ctc_1",
					ToolName:   "sample_tool",
					Content:    []ContentPart{Text("done")},
				},
			},
		}
		options := ConvertOpenAIResponsesOptions{
			GrammarToolInputProperties: map[string]string{"sample_tool": "payload"},
		}
		for _, invalidArguments := range []map[string]any{{}, {"payload": 42}} {
			context.Messages[0].Content[0].Arguments = invalidArguments
			if _, err := ConvertOpenAIResponsesMessagesChecked(
				constrainedSamplingTestModel(),
				context,
				options,
			); err == nil || !strings.Contains(err.Error(), `requires argument "payload" to be a string`) {
				t.Fatalf("invalid arguments %#v error = %v", invalidArguments, err)
			}
		}

		context.Messages[0].Content[0].Arguments = map[string]any{"payload": "abc"}
		items, err := ConvertOpenAIResponsesMessagesChecked(
			constrainedSamplingTestModel(),
			context,
			options,
		)
		if err != nil {
			t.Fatal(err)
		}
		if !containsOpenAIResponsesItem(items, OpenAIResponsesInputItem{
			Type:   "custom_tool_call",
			ID:     "ctc_1",
			CallID: "call_1",
			Name:   "sample_tool",
			Input:  "abc",
		}) {
			t.Fatalf("custom call not found: %#v", items)
		}
		if !containsOpenAIResponsesItem(items, OpenAIResponsesInputItem{
			Type:   "custom_tool_call_output",
			CallID: "call_1",
			Output: "done",
		}) {
			t.Fatalf("custom output not found: %#v", items)
		}
	})

	t.Run("keeps grammar input JSON deltas append-only", func(t *testing.T) {
		buffer := GrammarToolInputJSONBuffer{}
		first, ok, err := AppendGrammarToolInputJSONDelta(&buffer, "payload", `a"`, false)
		if err != nil || !ok {
			t.Fatalf("first = %q, %v, %v", first, ok, err)
		}
		second, ok, err := AppendGrammarToolInputJSONDelta(&buffer, "payload", "a\"\nb", true)
		if err != nil || !ok {
			t.Fatalf("second = %q, %v, %v", second, ok, err)
		}
		var value map[string]string
		if err := json.Unmarshal([]byte(first+second), &value); err != nil {
			t.Fatal(err)
		}
		if value["payload"] != "a\"\nb" {
			t.Fatalf("value = %#v", value)
		}
		if delta, ok, err := AppendGrammarToolInputJSONDelta(&buffer, "payload", "a\"\nb", true); err != nil || ok || delta != "" {
			t.Fatalf("idempotent close = %q, %v, %v", delta, ok, err)
		}
		if _, _, err := AppendGrammarToolInputJSONDelta(&buffer, "payload", "changed", true); err == nil ||
			!strings.Contains(err.Error(), `changed after it was closed`) {
			t.Fatalf("closed mutation error = %v", err)
		}
	})

	t.Run("streams custom Responses tool calls as string arguments", func(t *testing.T) {
		model := constrainedSamplingTestModel()
		output := AssistantMessage(nil, StopReasonStop, model)
		processor := NewOpenAIResponsesStreamProcessorWithOptions(
			model,
			&output,
			OpenAIResponsesStreamProcessorOptions{
				GrammarToolInputProperties: map[string]string{"sample_tool": "payload"},
			},
		)
		events := []OpenAIResponsesStreamEvent{
			{
				Type: "response.output_item.added",
				Item: &OpenAIResponsesOutputItem{
					Type:   "custom_tool_call",
					CallID: "call_1",
					ID:     "ctc_1",
					Name:   "sample_tool",
				},
			},
			{Type: "response.custom_tool_call_input.delta", Delta: "ab"},
			{Type: "response.custom_tool_call_input.done", Input: "abc"},
			{
				Type: "response.output_item.done",
				Item: &OpenAIResponsesOutputItem{
					Type:   "custom_tool_call",
					CallID: "call_1",
					ID:     "ctc_1",
					Name:   "sample_tool",
					Input:  "abc",
				},
			},
			{
				Type: "response.completed",
				Response: &OpenAIResponsesResponseEvent{
					Status: "completed",
					Usage:  &OpenAIResponsesUsage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
				},
			},
		}
		var deltas strings.Builder
		for _, event := range events {
			for _, emitted := range processor.Process(event) {
				if emitted.Type == "toolcall_delta" {
					deltas.WriteString(emitted.Delta)
				}
			}
		}

		if output.StopReason != StopReasonToolUse ||
			len(output.Content) != 1 ||
			output.Content[0].Type != ContentToolCall ||
			output.Content[0].ID != "call_1|ctc_1" ||
			output.Content[0].Arguments["payload"] != "abc" {
			t.Fatalf("output = %#v", output)
		}
		var value map[string]string
		if err := json.Unmarshal([]byte(deltas.String()), &value); err != nil {
			t.Fatalf("deltas = %q: %v", deltas.String(), err)
		}
		if value["payload"] != "abc" {
			t.Fatalf("delta value = %#v", value)
		}
	})
}

func TestOpenAIResponsesBuildersShareConstrainedSamplingState(t *testing.T) {
	tool := constrainedSamplingTestTool(&ConstrainedSamplingConfig{
		Type: ConstrainedSamplingGrammar,
		Variants: GrammarVariants{
			OpenAIRegex: `[a-z]+`,
		},
	})
	testBuilder := func(
		t *testing.T,
		model Model,
		build func(Context) ([]OpenAIResponsesTool, []OpenAIResponsesInputItem, error),
	) {
		t.Helper()
		model.Compat.SupportsOpenAIGrammarTools = ptrBool(true)
		context := Context{
			Tools: []Tool{tool},
			Messages: []Message{
				{
					Role:       RoleAssistant,
					Content:    []ContentPart{ToolCall("call_1|ctc_1", tool.Name, map[string]any{"payload": "abc"})},
					API:        model.API,
					Provider:   model.Provider,
					Model:      model.ID,
					StopReason: StopReasonToolUse,
				},
				{
					Role:       RoleToolResult,
					ToolCallID: "call_1|ctc_1",
					ToolName:   tool.Name,
					Content:    []ContentPart{Text("done")},
				},
			},
		}
		tools, input, err := build(context)
		if err != nil {
			t.Fatal(err)
		}
		if len(tools) != 1 ||
			tools[0].Type != "custom" ||
			tools[0].Format == nil ||
			tools[0].Format.Syntax != GrammarFormatRegex {
			t.Fatalf("tools = %#v", tools)
		}
		if !containsOpenAIResponsesItem(input, OpenAIResponsesInputItem{
			Type:   "custom_tool_call",
			ID:     "ctc_1",
			CallID: "call_1",
			Name:   tool.Name,
			Input:  "abc",
		}) {
			t.Fatalf("custom replay not found: %#v", input)
		}
		if !containsOpenAIResponsesItem(input, OpenAIResponsesInputItem{
			Type:   "custom_tool_call_output",
			CallID: "call_1",
			Output: "done",
		}) {
			t.Fatalf("custom output not found: %#v", input)
		}
	}

	t.Run("OpenAI Responses", func(t *testing.T) {
		model := constrainedSamplingTestModel()
		model.Compat.SupportsOpenAIGrammarTools = ptrBool(true)
		testBuilder(t, model, func(context Context) ([]OpenAIResponsesTool, []OpenAIResponsesInputItem, error) {
			payload, err := BuildOpenAIResponsesPayloadChecked(model, context, OpenAIResponsesPayloadOptions{})
			return payload.Tools, payload.Input, err
		})
	})
	t.Run("Azure OpenAI Responses", func(t *testing.T) {
		model := constrainedSamplingTestModel()
		model.API = "azure-openai-responses"
		model.Provider = "azure-openai-responses"
		model.Compat.SupportsOpenAIGrammarTools = ptrBool(true)
		testBuilder(t, model, func(context Context) ([]OpenAIResponsesTool, []OpenAIResponsesInputItem, error) {
			payload, err := BuildAzureOpenAIResponsesPayloadChecked(
				model,
				context,
				AzureOpenAIResponsesPayloadOptions{},
			)
			return payload.Tools, payload.Input, err
		})
	})
	t.Run("OpenAI Codex Responses", func(t *testing.T) {
		model := constrainedSamplingTestModel()
		model.API = "openai-codex-responses"
		model.Provider = "openai-codex"
		model.Compat.SupportsOpenAIGrammarTools = ptrBool(true)
		testBuilder(t, model, func(context Context) ([]OpenAIResponsesTool, []OpenAIResponsesInputItem, error) {
			payload, err := BuildOpenAICodexResponsesPayloadChecked(
				model,
				context,
				OpenAICodexResponsesPayloadOptions{},
			)
			return payload.Tools, payload.Input, err
		})
	})
}

func TestOpenAIResponsesToolStrictWireStates(t *testing.T) {
	plainTool := constrainedSamplingTestTool(nil)
	legacy := ConvertOpenAIResponsesTools([]Tool{plainTool}, true)
	if len(legacy) != 1 || legacy[0].Strict == nil || !*legacy[0].Strict {
		t.Fatalf("legacy strict tools = %#v", legacy)
	}

	tools, err := ConvertOpenAIResponsesToolsChecked([]Tool{plainTool}, OpenAIResponsesToolOptions{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(tools[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"strict":false`) {
		t.Fatalf("default tool = %s", raw)
	}

	tools, err = ConvertOpenAIResponsesToolsChecked(
		[]Tool{plainTool},
		OpenAIResponsesToolOptions{SupportsStrictMode: ptrBool(false)},
	)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(tools[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"strict"`) {
		t.Fatalf("unsupported strict tool = %s", raw)
	}

	model := constrainedSamplingTestModel()
	model.API = "openai-codex-responses"
	model.Provider = "openai-codex"
	payload, err := BuildOpenAICodexResponsesPayloadChecked(
		model,
		Context{Tools: []Tool{plainTool}},
		OpenAICodexResponsesPayloadOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"strict":null`) {
		t.Fatalf("Codex payload = %s", raw)
	}
}

func containsOpenAIResponsesItem(items []OpenAIResponsesInputItem, want OpenAIResponsesInputItem) bool {
	for _, item := range items {
		if reflect.DeepEqual(item, want) {
			return true
		}
	}
	return false
}
