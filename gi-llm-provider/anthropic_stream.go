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
		Type        string                `json:"type"`
		Text        string                `json:"text"`
		Thinking    string                `json:"thinking"`
		Signature   string                `json:"signature"`
		PartialJSON string                `json:"partial_json"`
		StopReason  string                `json:"stop_reason"`
		StopDetails *anthropicStopDetails `json:"stop_details"`
	} `json:"delta"`
	Usage *anthropicWireUsage `json:"usage"`
}

type anthropicStopDetails struct {
	Type        string `json:"type"`
	Category    string `json:"category"`
	Explanation string `json:"explanation"`
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
	processor := NewAnthropicStreamProcessor(model)
	for _, sse := range events {
		if _, err := processor.Process(sse); err != nil {
			return processor.Message(), err
		}
	}
	return processor.Finish()
}

// AnthropicStreamProcessor owns the single mutable partial-message state used
// for both incremental events and the terminal result.
type AnthropicStreamProcessor struct {
	model                    Model
	output                   Message
	partialJSONByEventIndex  map[int]string
	contentIndexByEventIndex map[int]int
	sawMessageStart          bool
	sawMessageStop           bool
}

func NewAnthropicStreamProcessor(model Model) *AnthropicStreamProcessor {
	return &AnthropicStreamProcessor{
		model:                    model,
		output:                   AssistantMessage([]ContentPart{}, StopReasonStop, model),
		partialJSONByEventIndex:  map[int]string{},
		contentIndexByEventIndex: map[int]int{},
	}
}

func (p *AnthropicStreamProcessor) Message() Message {
	return cloneMessageState(p.output)
}

func (p *AnthropicStreamProcessor) Process(sse AnthropicSSEEvent) ([]AssistantMessageEvent, error) {
	if sse.Event == "error" {
		return nil, errors.New(sse.Data)
	}
	if sse.Event == "done" || sse.Data == "[DONE]" || !isAnthropicMessageSSEEvent(sse.Event) {
		return nil, nil
	}

	var event rawAnthropicEvent
	if err := UnmarshalJSONWithRepair([]byte(sse.Data), &event); err != nil {
		return nil, err
	}
	switch event.Type {
	case "message_start":
		p.sawMessageStart = true
		p.output.ResponseID = event.Message.ID
		p.output.Usage = mergeAnthropicWireUsage(p.output.Usage, event.Message.Usage, p.model)
	case "message_stop":
		p.sawMessageStop = true
	case "content_block_start":
		return p.startContentBlock(event), nil
	case "content_block_delta":
		return p.applyContentBlockDelta(event), nil
	case "content_block_stop":
		return p.stopContentBlock(event), nil
	case "message_delta":
		if event.Delta.StopReason != "" {
			stopReason, errorMessage, err := mapAnthropicStopReason(
				event.Delta.StopReason,
				event.Delta.StopDetails,
			)
			if err != nil {
				return nil, err
			}
			p.output.StopReason = stopReason
			p.output.ErrorMessage = errorMessage
		}
		p.output.Usage = mergeAnthropicWireUsage(p.output.Usage, event.Usage, p.model)
	}
	return nil, nil
}

func (p *AnthropicStreamProcessor) startContentBlock(event rawAnthropicEvent) []AssistantMessageEvent {
	var eventType string
	switch event.ContentBlock.Type {
	case "text":
		p.output.Content = append(p.output.Content, Text(""))
		eventType = "text_start"
	case "thinking":
		p.output.Content = append(p.output.Content, Thinking(""))
		eventType = "thinking_start"
	case "redacted_thinking":
		p.output.Content = append(p.output.Content, ContentPart{
			Type:              ContentThinking,
			Thinking:          "[Reasoning redacted]",
			ThinkingSignature: event.ContentBlock.Data,
			Redacted:          true,
		})
		eventType = "thinking_start"
	case "tool_use":
		p.output.Content = append(
			p.output.Content,
			ToolCall(event.ContentBlock.ID, event.ContentBlock.Name, event.ContentBlock.Input),
		)
		p.partialJSONByEventIndex[event.Index] = ""
		eventType = "toolcall_start"
	default:
		return nil
	}
	contentIndex := len(p.output.Content) - 1
	p.contentIndexByEventIndex[event.Index] = contentIndex
	return []AssistantMessageEvent{{
		Type:         eventType,
		ContentIndex: contentIndex,
		Partial:      cloneMessageState(p.output),
	}}
}

func (p *AnthropicStreamProcessor) applyContentBlockDelta(event rawAnthropicEvent) []AssistantMessageEvent {
	contentIndex, ok := p.contentIndexByEventIndex[event.Index]
	if !ok || contentIndex < 0 || contentIndex >= len(p.output.Content) {
		return nil
	}
	block := &p.output.Content[contentIndex]
	switch event.Delta.Type {
	case "text_delta":
		delta := SanitizeSurrogates(event.Delta.Text)
		block.Text += delta
		return []AssistantMessageEvent{{
			Type:         "text_delta",
			ContentIndex: contentIndex,
			Delta:        delta,
			Partial:      cloneMessageState(p.output),
		}}
	case "thinking_delta":
		delta := SanitizeSurrogates(event.Delta.Thinking)
		block.Thinking += delta
		return []AssistantMessageEvent{{
			Type:         "thinking_delta",
			ContentIndex: contentIndex,
			Delta:        delta,
			Partial:      cloneMessageState(p.output),
		}}
	case "signature_delta":
		block.ThinkingSignature += event.Delta.Signature
	case "input_json_delta":
		p.partialJSONByEventIndex[event.Index] += event.Delta.PartialJSON
		block.Arguments = parseJSONRepairObject(p.partialJSONByEventIndex[event.Index])
		return []AssistantMessageEvent{{
			Type:         "toolcall_delta",
			ContentIndex: contentIndex,
			Delta:        event.Delta.PartialJSON,
			Partial:      cloneMessageState(p.output),
		}}
	}
	return nil
}

func (p *AnthropicStreamProcessor) stopContentBlock(event rawAnthropicEvent) []AssistantMessageEvent {
	contentIndex, ok := p.contentIndexByEventIndex[event.Index]
	if !ok || contentIndex < 0 || contentIndex >= len(p.output.Content) {
		return nil
	}
	block := &p.output.Content[contentIndex]
	switch block.Type {
	case ContentText:
		return []AssistantMessageEvent{{
			Type:         "text_end",
			ContentIndex: contentIndex,
			Content:      block.Text,
			Partial:      cloneMessageState(p.output),
		}}
	case ContentThinking:
		return []AssistantMessageEvent{{
			Type:         "thinking_end",
			ContentIndex: contentIndex,
			Content:      block.Thinking,
			Partial:      cloneMessageState(p.output),
		}}
	case ContentToolCall:
		if partial := p.partialJSONByEventIndex[event.Index]; partial != "" {
			block.Arguments = parseJSONRepairObject(partial)
		}
		delete(p.partialJSONByEventIndex, event.Index)
		return []AssistantMessageEvent{{
			Type:         "toolcall_end",
			ContentIndex: contentIndex,
			ToolCall:     *block,
			Partial:      cloneMessageState(p.output),
		}}
	default:
		return nil
	}
}

func (p *AnthropicStreamProcessor) Finish() (Message, error) {
	if p.sawMessageStart && !p.sawMessageStop {
		return cloneMessageState(p.output), errors.New("Anthropic stream ended before message_stop")
	}
	return cloneMessageState(p.output), nil
}

func (p *AnthropicStreamProcessor) Fail(err error, aborted bool) Message {
	p.output.StopReason = StopReasonError
	if aborted {
		p.output.StopReason = StopReasonAborted
	}
	p.output.ErrorMessage = err.Error()
	return cloneMessageState(p.output)
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

func mapAnthropicStopReason(
	reason string,
	details *anthropicStopDetails,
) (stopReason, errorMessage string, err error) {
	switch reason {
	case "end_turn", "stop_sequence":
		return StopReasonStop, "", nil
	case "max_tokens":
		return StopReasonLength, "", nil
	case "tool_use":
		return StopReasonToolUse, "", nil
	case "pause_turn":
		return StopReasonStop, "", nil
	case "refusal":
		message := "The model refused to complete the request"
		if details != nil && details.Explanation != "" {
			message = details.Explanation
		}
		return StopReasonError, message, nil
	case "sensitive":
		return StopReasonError, "", nil
	default:
		return "", "", errors.New("Unhandled stop reason: " + reason)
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
