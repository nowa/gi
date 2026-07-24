package gillmprovider

import "testing"

func TestProcessAnthropicSSEEventsPreservesPartialUsageAndCacheBreakdowns(t *testing.T) {
	model := MustGetModel("anthropic", "claude-opus-4-8")

	t.Run("prices the 1h portion at 2x input and the rest at the 5m rate", func(t *testing.T) {
		result, err := ProcessAnthropicSSEEvents(model, []AnthropicSSEEvent{
			{Event: "message_start", Data: `{"type":"message_start","message":{"id":"msg_cache","usage":{"input_tokens":100,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":1000000,"cache_creation":{"ephemeral_5m_input_tokens":600000,"ephemeral_1h_input_tokens":400000}}}}`},
			{Event: "message_delta", Data: `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":100,"output_tokens":5,"cache_read_input_tokens":0,"cache_creation_input_tokens":1000000}}`},
			{Event: "message_stop", Data: `{"type":"message_stop"}`},
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.Usage.CacheWrite != 1_000_000 || result.Usage.CacheWrite1h != 400_000 {
			t.Fatalf("usage = %#v", result.Usage)
		}
		if result.Usage.Cost.CacheWrite != 7.75 {
			t.Fatalf("cache write cost = %v, want 7.75", result.Usage.Cost.CacheWrite)
		}
	})

	t.Run("falls back to the 5m rate when no breakdown is reported", func(t *testing.T) {
		result, err := ProcessAnthropicSSEEvents(model, []AnthropicSSEEvent{
			{Event: "message_start", Data: `{"type":"message_start","message":{"id":"msg_cache","usage":{"input_tokens":100,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":1000000}}}`},
			{Event: "message_delta", Data: `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":100,"output_tokens":5,"cache_read_input_tokens":0,"cache_creation_input_tokens":1000000}}`},
			{Event: "message_stop", Data: `{"type":"message_stop"}`},
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.Usage.CacheWrite1h != 0 || result.Usage.Cost.CacheWrite != 6.25 {
			t.Fatalf("usage = %#v", result.Usage)
		}
	})

	t.Run("treats message delta without usage as a no-op for usage accumulation", func(t *testing.T) {
		result, err := ProcessAnthropicSSEEvents(model, []AnthropicSSEEvent{
			{Event: "message_start", Data: `{"type":"message_start","message":{"id":"msg_partial","usage":{"input_tokens":12,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`},
			{Event: "content_block_start", Data: `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
			{Event: "content_block_delta", Data: `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`},
			{Event: "content_block_stop", Data: `{"type":"content_block_stop","index":0}`},
			{Event: "message_delta", Data: `{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`},
			{Event: "message_stop", Data: `{"type":"message_stop"}`},
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.StopReason != StopReasonStop || result.Usage.Input != 12 || result.Usage.TotalTokens != 12 {
			t.Fatalf("result = %#v", result)
		}
	})

	t.Run("keeps reported reasoning as an output token subset", func(t *testing.T) {
		result, err := ProcessAnthropicSSEEvents(model, []AnthropicSSEEvent{
			{Event: "message_start", Data: `{"type":"message_start","message":{"id":"msg_reasoning","usage":{"input_tokens":10,"output_tokens":0}}}`},
			{Event: "message_delta", Data: `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7,"output_tokens_details":{"thinking_tokens":3}}}`},
			{Event: "message_stop", Data: `{"type":"message_stop"}`},
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.Usage.Reasoning == nil || *result.Usage.Reasoning != 3 || result.Usage.Output != 7 || result.Usage.TotalTokens != 17 {
			t.Fatalf("usage = %#v", result.Usage)
		}
	})
}

func TestProcessAnthropicSSEEventsRepairsMalformedJSON(t *testing.T) {
	model := MustGetModel("anthropic", "claude-sonnet-4-5")
	malformedToolJSONDelta := `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"A\H\",\"text\":\"col1	col2\"}"}}`

	result, err := ProcessAnthropicSSEEvents(model, []AnthropicSSEEvent{
		{Event: "message_start", Data: `{"type":"message_start","message":{"id":"msg_test","usage":{"input_tokens":12,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`},
		{Event: "content_block_start", Data: `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_test","name":"edit","input":{}}}`},
		{Event: "content_block_delta", Data: malformedToolJSONDelta},
		{Event: "content_block_stop", Data: `{"type":"content_block_stop","index":0}`},
		{Event: "message_delta", Data: `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":12,"output_tokens":5,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`},
		{Event: "message_stop", Data: `{"type":"message_stop"}`},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != StopReasonToolUse || result.ErrorMessage != "" {
		t.Fatalf("result = %#v", result)
	}
	if len(result.Content) != 1 || result.Content[0].Type != ContentToolCall {
		t.Fatalf("content = %#v", result.Content)
	}
	args := result.Content[0].Arguments
	if args["path"] != `A\H` || args["text"] != "col1\tcol2" {
		t.Fatalf("args = %#v", args)
	}
}

func TestProcessAnthropicSSEEventsIgnoresUnknownEventsAfterStop(t *testing.T) {
	model := MustGetModel("anthropic", "claude-sonnet-4-5")
	result, err := ProcessAnthropicSSEEvents(model, []AnthropicSSEEvent{
		{Event: "message_start", Data: `{"type":"message_start","message":{"id":"msg_test","usage":{"input_tokens":12,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`},
		{Event: "content_block_start", Data: `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		{Event: "content_block_delta", Data: `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`},
		{Event: "content_block_stop", Data: `{"type":"content_block_stop","index":0}`},
		{Event: "message_delta", Data: `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":12,"output_tokens":5,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`},
		{Event: "message_stop", Data: `{"type":"message_stop"}`},
		{Event: "done", Data: "[DONE]"},
		{Event: "proxy.stats", Data: "not json"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != StopReasonStop || len(result.Content) != 1 || result.Content[0].Text != "Hello" {
		t.Fatalf("result = %#v", result)
	}
	if result.ResponseID != "msg_test" || result.Usage.TotalTokens != 17 {
		t.Fatalf("metadata = id:%q usage:%#v", result.ResponseID, result.Usage)
	}
}

func TestProcessAnthropicSSEEventsPreservesThinkingAndTextIndexMapping(t *testing.T) {
	model := MustGetModel("anthropic", "claude-haiku-4-5")
	result, err := ProcessAnthropicSSEEvents(model, []AnthropicSSEEvent{
		{Event: "message_start", Data: `{"type":"message_start","message":{"id":"msg_think","usage":{"input_tokens":12,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`},
		{Event: "content_block_start", Data: `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`},
		{Event: "content_block_delta", Data: `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Need answer."}}`},
		{Event: "content_block_delta", Data: `{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig_1"}}`},
		{Event: "content_block_stop", Data: `{"type":"content_block_stop","index":0}`},
		{Event: "content_block_start", Data: `{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`},
		{Event: "content_block_delta", Data: `{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"OK"}}`},
		{Event: "content_block_stop", Data: `{"type":"content_block_stop","index":1}`},
		{Event: "message_delta", Data: `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":12,"output_tokens":5,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`},
		{Event: "message_stop", Data: `{"type":"message_stop"}`},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %#v", result.Content)
	}
	if result.Content[0].Type != ContentThinking || result.Content[0].Thinking != "Need answer." || result.Content[0].ThinkingSignature != "sig_1" {
		t.Fatalf("thinking = %#v", result.Content[0])
	}
	if result.Content[1].Type != ContentText || result.Content[1].Text != "OK" {
		t.Fatalf("text = %#v", result.Content[1])
	}
}
