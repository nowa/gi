package gillmprovider

import "testing"

func TestProcessOpenAICompletionsChunksIgnoresNilAndCapturesResponseID(t *testing.T) {
	stop := "stop"
	model := Model{ID: "gpt-4o-mini", Provider: "openai", API: "openai-completions"}

	response := ProcessOpenAICompletionsChunks(model, []*OpenAIChatCompletionChunk{
		nil,
		{ID: "chatcmpl-test", Choices: []OpenAIChatCompletionChoice{{Delta: OpenAIChatDelta{Content: "OK"}}}},
		{ID: "chatcmpl-test", Choices: []OpenAIChatCompletionChoice{{FinishReason: &stop}}, Usage: &OpenAIChatUsage{PromptTokens: 3, CompletionTokens: 1}},
	})

	if response.StopReason != StopReasonStop || response.ErrorMessage != "" {
		t.Fatalf("stop = %q error = %q", response.StopReason, response.ErrorMessage)
	}
	if response.ResponseID != "chatcmpl-test" {
		t.Fatalf("response id = %q", response.ResponseID)
	}
	if len(response.Content) != 1 || response.Content[0].Text != "OK" {
		t.Fatalf("content = %#v", response.Content)
	}
	if response.Usage.TotalTokens != 4 {
		t.Fatalf("usage = %#v", response.Usage)
	}
}

func TestProcessOpenAICompletionsChunksMapsErrorsAndMissingFinishReason(t *testing.T) {
	networkError := "network_error"
	model := Model{ID: "glm-5.1", Provider: "zai", API: "openai-completions"}

	response := ProcessOpenAICompletionsChunks(model, []*OpenAIChatCompletionChunk{
		{Choices: []OpenAIChatCompletionChoice{{Delta: OpenAIChatDelta{Content: "partial"}}}},
		{Choices: []OpenAIChatCompletionChoice{{FinishReason: &networkError}}},
	})
	if response.StopReason != StopReasonError || response.ErrorMessage != "Provider finish_reason: network_error" {
		t.Fatalf("response = %#v", response)
	}

	response = ProcessOpenAICompletionsChunks(model, []*OpenAIChatCompletionChunk{
		{ID: "chatcmpl-truncated", Choices: []OpenAIChatCompletionChoice{{Delta: OpenAIChatDelta{Content: "partial answer"}}}},
		{ID: "chatcmpl-truncated", Choices: []OpenAIChatCompletionChoice{{Delta: OpenAIChatDelta{Content: " again"}}}},
	})
	if response.StopReason != StopReasonError || response.ErrorMessage != "Stream ended without finish_reason" {
		t.Fatalf("response = %#v", response)
	}
}

func TestProcessOpenAICompletionsChunksCoalescesToolDeltasByIndex(t *testing.T) {
	toolCalls := "tool_calls"
	index := 0
	model := Model{ID: "gpt-4o-mini", Provider: "openai", API: "openai-completions"}

	response := ProcessOpenAICompletionsChunks(model, []*OpenAIChatCompletionChunk{
		{ID: "chatcmpl-kimi-bad-stream", Choices: []OpenAIChatCompletionChoice{{Delta: OpenAIChatDelta{ToolCalls: []OpenAIChatToolCallDelta{{
			Index: &index,
			ID:    "functions.read:0",
			Type:  "function",
			Function: OpenAIChatToolCallFunctionDelta{
				Name: "read",
			},
		}}}}}},
		{ID: "chatcmpl-kimi-bad-stream", Choices: []OpenAIChatCompletionChoice{{Delta: OpenAIChatDelta{ToolCalls: []OpenAIChatToolCallDelta{{
			Index:    &index,
			ID:       "chatcmpl-tool-a",
			Type:     "function",
			Function: OpenAIChatToolCallFunctionDelta{Arguments: `{"path":"README`},
		}}}}}},
		{ID: "chatcmpl-kimi-bad-stream", Choices: []OpenAIChatCompletionChoice{{Delta: OpenAIChatDelta{ToolCalls: []OpenAIChatToolCallDelta{{
			Index:    &index,
			ID:       "chatcmpl-tool-b",
			Type:     "function",
			Function: OpenAIChatToolCallFunctionDelta{Arguments: `.md"}`},
		}}}, FinishReason: &toolCalls}}},
	})

	if response.StopReason != StopReasonToolUse {
		t.Fatalf("stop reason = %q", response.StopReason)
	}
	if len(response.Content) != 1 || response.Content[0].Type != ContentToolCall {
		t.Fatalf("content = %#v", response.Content)
	}
	call := response.Content[0]
	if call.ID != "functions.read:0" || call.Name != "read" || call.Arguments["path"] != "README.md" {
		t.Fatalf("tool call = %#v", call)
	}
}

func TestProcessOpenAICompletionsChunksMixedContentThinkingToolsAndUsage(t *testing.T) {
	toolCalls := "tool_calls"
	readIndex := 0
	grepIndex := 1
	model := Model{ID: "gpt-4o-mini", Provider: "openai", API: "openai-completions"}

	chunks := []*OpenAIChatCompletionChunk{
		{
			ID: "chatcmpl-mixed",
			Choices: []OpenAIChatCompletionChoice{{
				Delta: OpenAIChatDelta{
					Content:          "answer 1",
					ReasoningContent: "think 1",
					ToolCalls: []OpenAIChatToolCallDelta{
						{Index: &readIndex, ID: "tc_read_initial", Type: "function", Function: OpenAIChatToolCallFunctionDelta{Name: "read", Arguments: `{"path":"README`}},
						{Index: &grepIndex, ID: "tc_grep_initial", Type: "function", Function: OpenAIChatToolCallFunctionDelta{Name: "grep", Arguments: `{"pattern":"TODO`}},
						{ID: "tc_list_no_index", Type: "function", Function: OpenAIChatToolCallFunctionDelta{Name: "list", Arguments: `{"path":"packages`}},
						{ID: "tc_write_no_index", Type: "function", Function: OpenAIChatToolCallFunctionDelta{Name: "write", Arguments: `{"path":"out`}},
					},
				},
			}},
		},
		{
			ID: "chatcmpl-mixed",
			Choices: []OpenAIChatCompletionChoice{{
				Delta: OpenAIChatDelta{
					Content: " answer 2",
					ToolCalls: []OpenAIChatToolCallDelta{
						{Index: &grepIndex, ID: "tc_grep_changed", Type: "function", Function: OpenAIChatToolCallFunctionDelta{Arguments: `","path":"src`}},
						{ID: "tc_write_no_index", Type: "function", Function: OpenAIChatToolCallFunctionDelta{Arguments: `.txt","content":"ok"}`}},
						{ID: "tc_list_no_index", Type: "function", Function: OpenAIChatToolCallFunctionDelta{Arguments: `/ai"}`}},
					},
				},
			}},
		},
		{
			ID: "chatcmpl-mixed",
			Choices: []OpenAIChatCompletionChoice{{
				Delta: OpenAIChatDelta{
					Content:          "\n",
					ReasoningContent: " think 2",
					ToolCalls: []OpenAIChatToolCallDelta{
						{Index: &readIndex, ID: "tc_read_changed", Type: "function", Function: OpenAIChatToolCallFunctionDelta{Arguments: `.md"}`}},
						{Index: &grepIndex, Type: "function", Function: OpenAIChatToolCallFunctionDelta{Arguments: `"}`}},
					},
				},
				FinishReason: &toolCalls,
			}},
			Usage: &OpenAIChatUsage{
				PromptTokens:     10,
				CompletionTokens: 8,
				CompletionTokenDetails: OpenAIChatCompletionTokenDetails{
					ReasoningTokens: 2,
				},
			},
		},
	}
	response := ProcessOpenAICompletionsChunks(model, chunks)

	if response.StopReason != StopReasonToolUse {
		t.Fatalf("stop = %q", response.StopReason)
	}
	if len(response.Content) != 6 {
		t.Fatalf("content = %#v", response.Content)
	}
	if response.Content[0].Text != "answer 1 answer 2\n" {
		t.Fatalf("text = %#v", response.Content[0])
	}
	if response.Content[1].Thinking != "think 1 think 2" || response.Content[1].ThinkingSignature != "reasoning_content" {
		t.Fatalf("thinking = %#v", response.Content[1])
	}
	if response.Content[2].ID != "tc_read_initial" || response.Content[2].Arguments["path"] != "README.md" {
		t.Fatalf("read = %#v", response.Content[2])
	}
	if response.Content[3].ID != "tc_grep_initial" || response.Content[3].Arguments["pattern"] != "TODO" || response.Content[3].Arguments["path"] != "src" {
		t.Fatalf("grep = %#v", response.Content[3])
	}
	if response.Content[4].ID != "tc_list_no_index" || response.Content[4].Arguments["path"] != "packages/ai" {
		t.Fatalf("list = %#v", response.Content[4])
	}
	if response.Content[5].ID != "tc_write_no_index" || response.Content[5].Arguments["path"] != "out.txt" || response.Content[5].Arguments["content"] != "ok" {
		t.Fatalf("write = %#v", response.Content[5])
	}
	if response.Usage.Output != 8 || response.Usage.TotalTokens != 18 {
		t.Fatalf("usage = %#v", response.Usage)
	}
}

func TestParseOpenAIChatUsagePreservesCacheReadWrite(t *testing.T) {
	usage := ParseOpenAIChatUsage(OpenAIChatUsage{
		PromptTokens:     100,
		CompletionTokens: 5,
		PromptTokensDetails: OpenAIChatPromptTokenDetails{
			CachedTokens:     50,
			CacheWriteTokens: 30,
		},
	}, Model{})

	if usage.Input != 20 || usage.CacheRead != 50 || usage.CacheWrite != 30 || usage.TotalTokens != 105 {
		t.Fatalf("usage = %#v", usage)
	}
}
