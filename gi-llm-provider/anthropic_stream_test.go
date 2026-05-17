package gillmprovider

import "testing"

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
