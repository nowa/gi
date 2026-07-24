package gillmprovider

import (
	"encoding/json"
	"errors"
	"strings"
)

type AnthropicSSEEvent struct {
	Event string
	Data  string
}

type rawAnthropicEvent struct {
	Type    string `json:"type"`
	Index   int    `json:"index"`
	Message struct {
		ID    string              `json:"id"`
		Usage *anthropicWireUsage `json:"usage"`
	} `json:"message"`
	ContentBlock struct {
		Type  string         `json:"type"`
		Text  string         `json:"text"`
		ID    string         `json:"id"`
		Name  string         `json:"name"`
		Input map[string]any `json:"input"`
		Data  string         `json:"data"`
	} `json:"content_block"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
		Signature   string `json:"signature"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	Usage *anthropicWireUsage `json:"usage"`
}

type AnthropicRawUsage struct {
	InputTokens              int                          `json:"input_tokens"`
	OutputTokens             int                          `json:"output_tokens"`
	CacheReadInputTokens     int                          `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int                          `json:"cache_creation_input_tokens"`
	CacheCreation            AnthropicCacheCreationUsage  `json:"cache_creation"`
	OutputTokensDetails      AnthropicOutputTokensDetails `json:"output_tokens_details"`
}

type AnthropicCacheCreationUsage struct {
	Ephemeral5mInputTokens int `json:"ephemeral_5m_input_tokens"`
	Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens"`
}

type AnthropicOutputTokensDetails struct {
	ThinkingTokens *int `json:"thinking_tokens"`
}

type anthropicWireUsage struct {
	InputTokens              *int                              `json:"input_tokens"`
	OutputTokens             *int                              `json:"output_tokens"`
	CacheReadInputTokens     *int                              `json:"cache_read_input_tokens"`
	CacheCreationInputTokens *int                              `json:"cache_creation_input_tokens"`
	CacheCreation            *anthropicWireCacheCreation       `json:"cache_creation"`
	OutputTokensDetails      *anthropicWireOutputTokensDetails `json:"output_tokens_details"`
}

type anthropicWireCacheCreation struct {
	Ephemeral5mInputTokens *int `json:"ephemeral_5m_input_tokens"`
	Ephemeral1hInputTokens *int `json:"ephemeral_1h_input_tokens"`
}

type anthropicWireOutputTokensDetails struct {
	ThinkingTokens *int `json:"thinking_tokens"`
}

func ProcessAnthropicSSEEvents(model Model, events []AnthropicSSEEvent) (Message, error) {
	output := AssistantMessage(nil, StopReasonStop, model)
	partialJSONByIndex := map[int]string{}
	contentIndexByEventIndex := map[int]int{}
	stopped := false
	for _, sse := range events {
		if stopped {
			continue
		}
		if sse.Event == "message_stop" {
			stopped = true
			continue
		}
		if sse.Event == "done" || sse.Data == "[DONE]" || !isAnthropicMessageSSEEvent(sse.Event) {
			continue
		}
		var event rawAnthropicEvent
		if err := UnmarshalJSONWithRepair([]byte(sse.Data), &event); err != nil {
			return output, err
		}
		switch event.Type {
		case "message_start":
			output.ResponseID = event.Message.ID
			output.Usage = mergeAnthropicWireUsage(output.Usage, event.Message.Usage, model)
		case "content_block_start":
			switch event.ContentBlock.Type {
			case "text":
				output.Content = append(output.Content, Text(event.ContentBlock.Text))
				contentIndexByEventIndex[event.Index] = len(output.Content) - 1
			case "thinking":
				output.Content = append(output.Content, Thinking(""))
				contentIndexByEventIndex[event.Index] = len(output.Content) - 1
			case "redacted_thinking":
				output.Content = append(output.Content, ContentPart{
					Type:              ContentThinking,
					Thinking:          "[Reasoning redacted]",
					ThinkingSignature: event.ContentBlock.Data,
					Redacted:          true,
				})
				contentIndexByEventIndex[event.Index] = len(output.Content) - 1
			case "tool_use":
				output.Content = append(output.Content, ToolCall(event.ContentBlock.ID, event.ContentBlock.Name, event.ContentBlock.Input))
				contentIndexByEventIndex[event.Index] = len(output.Content) - 1
				partialJSONByIndex[event.Index] = ""
			}
		case "content_block_delta":
			contentIndex, ok := contentIndexByEventIndex[event.Index]
			if !ok || contentIndex < 0 || contentIndex >= len(output.Content) {
				continue
			}
			block := &output.Content[contentIndex]
			switch event.Delta.Type {
			case "text_delta":
				block.Text += SanitizeSurrogates(event.Delta.Text)
			case "thinking_delta":
				block.Thinking += SanitizeSurrogates(event.Delta.Thinking)
			case "signature_delta":
				block.ThinkingSignature += event.Delta.Signature
			case "input_json_delta":
				partialJSONByIndex[event.Index] += event.Delta.PartialJSON
				block.Arguments = parseJSONRepairObject(partialJSONByIndex[event.Index])
			}
		case "content_block_stop":
			contentIndex, ok := contentIndexByEventIndex[event.Index]
			if ok && contentIndex >= 0 && contentIndex < len(output.Content) {
				if partial := partialJSONByIndex[event.Index]; partial != "" {
					output.Content[contentIndex].Arguments = parseJSONRepairObject(partial)
				}
			}
		case "message_delta":
			if event.Delta.StopReason != "" {
				output.StopReason = mapAnthropicStopReason(event.Delta.StopReason)
			}
			output.Usage = mergeAnthropicWireUsage(output.Usage, event.Usage, model)
		}
	}
	return output, nil
}

func UnmarshalJSONWithRepair(data []byte, target any) error {
	if err := json.Unmarshal(data, target); err == nil {
		return nil
	}
	repaired := RepairJSON(string(data))
	if repaired == string(data) {
		return json.Unmarshal(data, target)
	}
	return json.Unmarshal([]byte(repaired), target)
}

func RepairJSON(data string) string {
	var b strings.Builder
	inString := false
	for i := 0; i < len(data); i++ {
		ch := data[i]
		if !inString {
			b.WriteByte(ch)
			if ch == '"' {
				inString = true
			}
			continue
		}
		switch ch {
		case '"':
			b.WriteByte(ch)
			inString = false
		case '\\':
			if i+1 >= len(data) {
				b.WriteString(`\\`)
				continue
			}
			next := data[i+1]
			if isValidJSONEscape(next) {
				b.WriteByte(ch)
				b.WriteByte(next)
				i++
			} else {
				b.WriteString(`\\`)
			}
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if ch <= 0x1f {
				b.WriteString(`\u00`)
				const hex = "0123456789abcdef"
				b.WriteByte(hex[ch>>4])
				b.WriteByte(hex[ch&0xf])
			} else {
				b.WriteByte(ch)
			}
		}
	}
	return b.String()
}

func isValidJSONEscape(ch byte) bool {
	switch ch {
	case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
		return true
	case 'u':
		return true
	default:
		return false
	}
}

func parseJSONRepairObject(data string) map[string]any {
	result := map[string]any{}
	if strings.TrimSpace(data) == "" {
		return result
	}
	if err := UnmarshalJSONWithRepair([]byte(data), &result); err != nil {
		return map[string]any{}
	}
	return result
}

func isAnthropicMessageSSEEvent(event string) bool {
	switch event {
	case "message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop":
		return true
	default:
		return false
	}
}

func mapAnthropicStopReason(reason string) string {
	switch reason {
	case "end_turn", "stop_sequence":
		return StopReasonStop
	case "max_tokens":
		return StopReasonLength
	case "tool_use":
		return StopReasonToolUse
	default:
		if reason == "" {
			return StopReasonStop
		}
		return StopReasonError
	}
}

func usageFromAnthropicRaw(raw AnthropicRawUsage, model Model) Usage {
	usage := Usage{
		Input:        raw.InputTokens,
		Output:       raw.OutputTokens,
		CacheRead:    raw.CacheReadInputTokens,
		CacheWrite:   raw.CacheCreationInputTokens,
		CacheWrite1h: raw.CacheCreation.Ephemeral1hInputTokens,
		TotalTokens:  raw.InputTokens + raw.OutputTokens + raw.CacheReadInputTokens + raw.CacheCreationInputTokens,
	}
	if raw.OutputTokensDetails.ThinkingTokens != nil {
		usage.Reasoning = ptrInt(*raw.OutputTokensDetails.ThinkingTokens)
	}
	usage.Cost = CalculateCost(model, usage)
	return usage
}

func mergeAnthropicWireUsage(current Usage, raw *anthropicWireUsage, model Model) Usage {
	if raw == nil {
		return current
	}
	if raw.InputTokens != nil {
		current.Input = *raw.InputTokens
	}
	if raw.OutputTokens != nil {
		current.Output = *raw.OutputTokens
	}
	if raw.CacheReadInputTokens != nil {
		current.CacheRead = *raw.CacheReadInputTokens
	}
	if raw.CacheCreationInputTokens != nil {
		current.CacheWrite = *raw.CacheCreationInputTokens
	}
	if raw.CacheCreation != nil && raw.CacheCreation.Ephemeral1hInputTokens != nil {
		current.CacheWrite1h = *raw.CacheCreation.Ephemeral1hInputTokens
	}
	if raw.OutputTokensDetails != nil && raw.OutputTokensDetails.ThinkingTokens != nil {
		current.Reasoning = ptrInt(*raw.OutputTokensDetails.ThinkingTokens)
	}
	current.TotalTokens = current.Input + current.Output + current.CacheRead + current.CacheWrite
	current.Cost = CalculateCost(model, current)
	return current
}

func IsMalformedJSONError(err error) bool {
	var syntax *json.SyntaxError
	return errors.As(err, &syntax)
}
