package gillmprovider

import (
	"encoding/json"
	"fmt"
	"strings"
)

type OpenAIResponsesStreamEvent struct {
	Type        string
	OutputIndex int
	Response    *OpenAIResponsesResponseEvent
	Item        *OpenAIResponsesOutputItem
	Part        *OpenAIResponsesOutputContentPart
	Delta       string
	Arguments   string
	Input       string
	ErrorCode   string
	Error       string
}

type OpenAIResponsesResponseEvent struct {
	ID                string
	Status            string
	ServiceTier       string
	Usage             *OpenAIResponsesUsage
	IncompleteDetails *OpenAIResponsesIncompleteDetails
	Error             *OpenAIResponsesError
}

type OpenAIResponsesError struct {
	Code    string
	Message string
}

type OpenAIResponsesUsage struct {
	InputTokens         int
	OutputTokens        int
	TotalTokens         int
	InputTokensDetails  OpenAIResponsesInputTokenDetails
	OutputTokensDetails OpenAIResponsesOutputTokenDetails
}

type OpenAIResponsesInputTokenDetails struct {
	CachedTokens     int
	CacheWriteTokens int
}

type OpenAIResponsesOutputTokenDetails struct {
	ReasoningTokens int
}

type OpenAIResponsesIncompleteDetails struct {
	Reason string
}

type OpenAIResponsesOutputItem struct {
	Type             string                             `json:"type,omitempty"`
	ID               string                             `json:"id,omitempty"`
	CallID           string                             `json:"call_id,omitempty"`
	Name             string                             `json:"name,omitempty"`
	Arguments        string                             `json:"arguments,omitempty"`
	Input            string                             `json:"input,omitempty"`
	Status           string                             `json:"status,omitempty"`
	Content          []OpenAIResponsesOutputContentPart `json:"content,omitempty"`
	Summary          []OpenAIResponsesOutputContentPart `json:"summary,omitempty"`
	Phase            string                             `json:"phase,omitempty"`
	EncryptedContent string                             `json:"encrypted_content,omitempty"`
}

type OpenAIResponsesOutputContentPart struct {
	Type    string `json:"type,omitempty"`
	Text    string `json:"text,omitempty"`
	Refusal string `json:"refusal,omitempty"`
}

type OpenAIResponsesStreamProcessor struct {
	model                      Model
	output                     *Message
	grammarToolInputProperties map[string]string
	outputSlots                map[int]*openAIResponsesOutputSlot
}

type openAIResponsesSlotKind uint8

const (
	openAIResponsesReasoningSlot openAIResponsesSlotKind = iota + 1
	openAIResponsesTextSlot
	openAIResponsesToolSlot
)

type openAIResponsesOutputSlot struct {
	kind               openAIResponsesSlotKind
	index              int
	partialJSON        string
	customInput        *openAIResponsesCustomToolState
	summaryPartStarted bool
}

type openAIResponsesCustomToolState struct {
	property string
	buffer   GrammarToolInputJSONBuffer
}

func NewOpenAIResponsesStreamProcessor(model Model, output *Message) *OpenAIResponsesStreamProcessor {
	return NewOpenAIResponsesStreamProcessorWithOptions(
		model,
		output,
		OpenAIResponsesStreamProcessorOptions{},
	)
}

type OpenAIResponsesStreamProcessorOptions struct {
	GrammarToolInputProperties map[string]string
}

func NewOpenAIResponsesStreamProcessorWithOptions(
	model Model,
	output *Message,
	options OpenAIResponsesStreamProcessorOptions,
) *OpenAIResponsesStreamProcessor {
	if output.API == "" {
		output.API = model.API
	}
	if output.Provider == "" {
		output.Provider = model.Provider
	}
	if output.Model == "" {
		output.Model = model.ID
	}
	if output.StopReason == "" {
		output.StopReason = StopReasonStop
	}
	grammarToolInputProperties := make(map[string]string, len(options.GrammarToolInputProperties))
	for name, property := range options.GrammarToolInputProperties {
		grammarToolInputProperties[name] = property
	}
	return &OpenAIResponsesStreamProcessor{
		model:                      model,
		output:                     output,
		grammarToolInputProperties: grammarToolInputProperties,
		outputSlots:                make(map[int]*openAIResponsesOutputSlot),
	}
}

func (p *OpenAIResponsesStreamProcessor) Process(event OpenAIResponsesStreamEvent) []AssistantMessageEvent {
	switch event.Type {
	case "response.created":
		if event.Response != nil {
			p.output.ResponseID = event.Response.ID
		}
	case "response.output_item.added":
		if event.Item == nil {
			return nil
		}
		_, emitted := p.createOutputSlot(event.OutputIndex, *event.Item)
		return emitted
	case "response.content_part.added":
		if event.Part == nil || event.Part.Type != "output_text" {
			return nil
		}
		if p.outputSlot(event.OutputIndex, openAIResponsesTextSlot) == nil {
			_, emitted := p.createOutputSlot(
				event.OutputIndex,
				OpenAIResponsesOutputItem{Type: "message"},
			)
			return emitted
		}
	case "response.output_text.delta", "response.refusal.delta":
		slot := p.outputSlot(event.OutputIndex, openAIResponsesTextSlot)
		if slot != nil {
			p.output.Content[slot.index].Text += SanitizeSurrogates(event.Delta)
			return []AssistantMessageEvent{{
				Type:         "text_delta",
				ContentIndex: slot.index,
				Delta:        event.Delta,
				Partial:      cloneMessageState(*p.output),
			}}
		}
	case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
		slot := p.outputSlot(event.OutputIndex, openAIResponsesReasoningSlot)
		if slot != nil {
			if event.Type == "response.reasoning_summary_text.delta" && !slot.summaryPartStarted {
				return nil
			}
			p.output.Content[slot.index].Thinking += SanitizeSurrogates(event.Delta)
			return []AssistantMessageEvent{{
				Type:         "thinking_delta",
				ContentIndex: slot.index,
				Delta:        event.Delta,
				Partial:      cloneMessageState(*p.output),
			}}
		}
	case "response.reasoning_summary_part.added":
		if slot := p.outputSlot(event.OutputIndex, openAIResponsesReasoningSlot); slot != nil {
			slot.summaryPartStarted = true
		}
	case "response.reasoning_summary_part.done":
		slot := p.outputSlot(event.OutputIndex, openAIResponsesReasoningSlot)
		if slot != nil {
			if !slot.summaryPartStarted {
				return nil
			}
			p.output.Content[slot.index].Thinking += "\n\n"
			slot.summaryPartStarted = false
			return []AssistantMessageEvent{{
				Type:         "thinking_delta",
				ContentIndex: slot.index,
				Delta:        "\n\n",
				Partial:      cloneMessageState(*p.output),
			}}
		}
	case "response.function_call_arguments.delta":
		slot := p.outputSlot(event.OutputIndex, openAIResponsesToolSlot)
		if slot != nil && slot.customInput == nil {
			slot.partialJSON += event.Delta
			p.output.Content[slot.index].Arguments = parseStreamingJSONObject(slot.partialJSON)
			return []AssistantMessageEvent{{
				Type:         "toolcall_delta",
				ContentIndex: slot.index,
				Delta:        event.Delta,
				Partial:      cloneMessageState(*p.output),
			}}
		}
	case "response.function_call_arguments.done":
		slot := p.outputSlot(event.OutputIndex, openAIResponsesToolSlot)
		if slot != nil && slot.customInput == nil {
			previousPartialJSON := slot.partialJSON
			slot.partialJSON = event.Arguments
			p.output.Content[slot.index].Arguments = parseStreamingJSONObject(event.Arguments)
			if strings.HasPrefix(event.Arguments, previousPartialJSON) {
				delta := event.Arguments[len(previousPartialJSON):]
				if delta != "" {
					return []AssistantMessageEvent{{
						Type:         "toolcall_delta",
						ContentIndex: slot.index,
						Delta:        delta,
						Partial:      cloneMessageState(*p.output),
					}}
				}
			}
		}
	case "response.custom_tool_call_input.delta":
		slot := p.outputSlot(event.OutputIndex, openAIResponsesToolSlot)
		if slot == nil || slot.customInput == nil {
			return nil
		}
		currentInput := p.customToolInput(slot)
		return p.appendCustomToolInput(slot, currentInput+event.Delta, false)
	case "response.custom_tool_call_input.done":
		slot := p.outputSlot(event.OutputIndex, openAIResponsesToolSlot)
		if slot == nil || slot.customInput == nil {
			return nil
		}
		return p.appendCustomToolInput(slot, event.Input, true)
	case "response.output_item.done":
		if event.Item == nil {
			return nil
		}
		slot := p.outputSlots[event.OutputIndex]
		var emitted []AssistantMessageEvent
		if slot == nil {
			slot, emitted = p.createOutputSlot(event.OutputIndex, *event.Item)
		}
		if event.Item.Type == "reasoning" {
			if slot == nil || slot.kind != openAIResponsesReasoningSlot {
				return emitted
			}
			text := openAIResponsesReasoningItemText(*event.Item)
			if text != "" {
				p.output.Content[slot.index].Thinking = text
			}
			if signature := encodeOpenAIResponsesReasoningSignature(*event.Item); signature != "" {
				p.output.Content[slot.index].ThinkingSignature = signature
			}
			delete(p.outputSlots, event.OutputIndex)
			return append(emitted, AssistantMessageEvent{
				Type:         "thinking_end",
				ContentIndex: slot.index,
				Partial:      cloneMessageState(*p.output),
				Content:      p.output.Content[slot.index].Thinking,
			})
		}
		if event.Item.Type == "message" {
			if slot == nil || slot.kind != openAIResponsesTextSlot {
				return emitted
			}
			text := openAIResponsesOutputItemText(*event.Item)
			if text != "" {
				p.output.Content[slot.index].Text = SanitizeSurrogates(text)
			}
			p.output.Content[slot.index].TextSignature = encodeOpenAIResponsesTextSignature(event.Item.ID, event.Item.Phase)
			delete(p.outputSlots, event.OutputIndex)
			return append(emitted, AssistantMessageEvent{
				Type:         "text_end",
				ContentIndex: slot.index,
				Content:      p.output.Content[slot.index].Text,
				Partial:      cloneMessageState(*p.output),
			})
		}
		if event.Item.Type == "function_call" {
			if slot == nil || slot.kind != openAIResponsesToolSlot || slot.customInput != nil {
				return emitted
			}
			arguments := event.Item.Arguments
			if arguments == "" {
				arguments = slot.partialJSON
			}
			p.output.Content[slot.index].ID = event.Item.CallID + "|" + event.Item.ID
			p.output.Content[slot.index].Name = event.Item.Name
			p.output.Content[slot.index].Arguments = parseStreamingJSONObject(arguments)
			toolCall := p.output.Content[slot.index]
			delete(p.outputSlots, event.OutputIndex)
			return append(emitted, AssistantMessageEvent{
				Type:         "toolcall_end",
				ContentIndex: slot.index,
				ToolCall:     toolCall,
				Partial:      cloneMessageState(*p.output),
			})
		}
		if event.Item.Type == "custom_tool_call" {
			if slot == nil || slot.kind != openAIResponsesToolSlot || slot.customInput == nil {
				return emitted
			}
			deltas := p.appendCustomToolInput(slot, event.Item.Input, true)
			emitted = append(emitted, deltas...)
			for _, item := range deltas {
				if item.Type == "error" {
					return emitted
				}
			}
			p.output.Content[slot.index].ID = event.Item.CallID + "|" + event.Item.ID
			p.output.Content[slot.index].Name = event.Item.Name
			toolCall := p.output.Content[slot.index]
			delete(p.outputSlots, event.OutputIndex)
			return append(emitted, AssistantMessageEvent{
				Type:         "toolcall_end",
				ContentIndex: slot.index,
				ToolCall:     toolCall,
				Partial:      cloneMessageState(*p.output),
			})
		}
	case "response.completed", "response.incomplete":
		if event.Response != nil {
			p.output.ResponseID = event.Response.ID
			if event.Response.Usage != nil {
				p.output.Usage = ParseOpenAIResponsesUsage(*event.Response.Usage, p.model)
			}
			p.output.StopReason = mapOpenAIResponsesStatus(event.Response.Status)
			if p.output.StopReason == StopReasonStop && hasOpenAIResponsesToolCall(p.output.Content) {
				p.output.StopReason = StopReasonToolUse
			}
			if event.Error != "" {
				p.output.ErrorMessage = event.Error
			}
		}
		if p.output.StopReason == StopReasonError {
			return p.terminalError(p.output.ErrorMessage)
		}
		return []AssistantMessageEvent{{
			Type:    "done",
			Reason:  p.output.StopReason,
			Message: cloneMessageState(*p.output),
		}}
	case "response.failed":
		return p.terminalError(openAIResponsesFailedMessage(event.Response))
	case "error":
		return p.terminalError(fmt.Sprintf("Error Code %s: %s", event.ErrorCode, event.Error))
	}
	return nil
}

func (p *OpenAIResponsesStreamProcessor) terminalError(message string) []AssistantMessageEvent {
	p.output.StopReason = StopReasonError
	p.output.ErrorMessage = message
	return []AssistantMessageEvent{{
		Type:   "error",
		Reason: StopReasonError,
		Error:  cloneMessageState(*p.output),
	}}
}

func openAIResponsesFailedMessage(response *OpenAIResponsesResponseEvent) string {
	if response != nil && response.Error != nil {
		code := response.Error.Code
		if code == "" {
			code = "unknown"
		}
		message := response.Error.Message
		if message == "" {
			message = "no message"
		}
		return code + ": " + message
	}
	if response != nil && response.IncompleteDetails != nil &&
		response.IncompleteDetails.Reason != "" {
		return "incomplete: " + response.IncompleteDetails.Reason
	}
	return "Unknown error (no error details in response)"
}

func (p *OpenAIResponsesStreamProcessor) createOutputSlot(
	outputIndex int,
	item OpenAIResponsesOutputItem,
) (*openAIResponsesOutputSlot, []AssistantMessageEvent) {
	if slot := p.outputSlots[outputIndex]; slot != nil {
		return slot, nil
	}

	slot := &openAIResponsesOutputSlot{index: len(p.output.Content)}
	var eventType string
	switch item.Type {
	case "reasoning":
		slot.kind = openAIResponsesReasoningSlot
		p.output.Content = append(p.output.Content, Thinking(""))
		eventType = "thinking_start"
	case "message":
		slot.kind = openAIResponsesTextSlot
		p.output.Content = append(p.output.Content, Text(""))
		eventType = "text_start"
	case "function_call":
		slot.kind = openAIResponsesToolSlot
		slot.partialJSON = item.Arguments
		p.output.Content = append(
			p.output.Content,
			ToolCall(
				item.CallID+"|"+item.ID,
				item.Name,
				parseStreamingJSONObject(item.Arguments),
			),
		)
		eventType = "toolcall_start"
	case "custom_tool_call":
		inputProperty := p.grammarToolInputProperties[item.Name]
		if inputProperty == "" {
			inputProperty = "input"
		}
		slot.kind = openAIResponsesToolSlot
		slot.customInput = &openAIResponsesCustomToolState{property: inputProperty}
		p.output.Content = append(
			p.output.Content,
			ToolCall(
				item.CallID+"|"+item.ID,
				item.Name,
				map[string]any{inputProperty: item.Input},
			),
		)
		eventType = "toolcall_start"
	default:
		return nil, nil
	}
	p.outputSlots[outputIndex] = slot
	return slot, []AssistantMessageEvent{{
		Type:         eventType,
		ContentIndex: slot.index,
		Partial:      cloneMessageState(*p.output),
	}}
}

func (p *OpenAIResponsesStreamProcessor) outputSlot(
	outputIndex int,
	kind openAIResponsesSlotKind,
) *openAIResponsesOutputSlot {
	slot := p.outputSlots[outputIndex]
	if slot == nil || slot.kind != kind {
		return nil
	}
	if slot.index < 0 || slot.index >= len(p.output.Content) {
		return nil
	}
	return slot
}

func (p *OpenAIResponsesStreamProcessor) customToolInput(slot *openAIResponsesOutputSlot) string {
	if slot == nil ||
		slot.customInput == nil ||
		slot.index < 0 ||
		slot.index >= len(p.output.Content) {
		return ""
	}
	value, _ := p.output.Content[slot.index].Arguments[slot.customInput.property].(string)
	return value
}

func (p *OpenAIResponsesStreamProcessor) appendCustomToolInput(
	slot *openAIResponsesOutputSlot,
	nextInput string,
	closeInput bool,
) []AssistantMessageEvent {
	if slot == nil ||
		slot.customInput == nil ||
		slot.index < 0 ||
		slot.index >= len(p.output.Content) {
		return nil
	}
	custom := slot.customInput
	delta, ok, err := AppendGrammarToolInputJSONDelta(
		&custom.buffer,
		custom.property,
		nextInput,
		closeInput,
	)
	if err != nil {
		p.output.StopReason = StopReasonError
		p.output.ErrorMessage = err.Error()
		return []AssistantMessageEvent{{
			Type:   "error",
			Reason: StopReasonError,
			Error:  *p.output,
		}}
	}
	p.output.Content[slot.index].Arguments = map[string]any{custom.property: nextInput}
	if !ok {
		return nil
	}
	return []AssistantMessageEvent{{
		Type:         "toolcall_delta",
		ContentIndex: slot.index,
		Delta:        delta,
		Partial:      cloneMessageState(*p.output),
	}}
}

func ProcessOpenAIResponsesStreamEvents(model Model, output *Message, events []OpenAIResponsesStreamEvent) []AssistantMessageEvent {
	processor := NewOpenAIResponsesStreamProcessor(model, output)
	var emitted []AssistantMessageEvent
	for _, event := range events {
		emitted = append(emitted, processor.Process(event)...)
	}
	return emitted
}

func parseStreamingJSONObject(data string) map[string]any {
	if strings.TrimSpace(data) == "" {
		return map[string]any{}
	}
	var result map[string]any
	if err := UnmarshalJSONWithRepair([]byte(data), &result); err == nil && result != nil {
		return result
	}
	if repaired := completePartialJSONObject(data); repaired != "" {
		result = nil
		if err := UnmarshalJSONWithRepair([]byte(repaired), &result); err == nil && result != nil {
			return result
		}
	}
	return map[string]any{}
}

func completePartialJSONObject(data string) string {
	text := strings.TrimSpace(data)
	if text == "" {
		return ""
	}
	var builder strings.Builder
	closers := make([]byte, 0, 4)
	inString := false
	escaped := false
	for i := 0; i < len(text); i++ {
		ch := text[i]
		builder.WriteByte(ch)
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			closers = append(closers, '}')
		case '[':
			closers = append(closers, ']')
		case '}', ']':
			if len(closers) > 0 && closers[len(closers)-1] == ch {
				closers = closers[:len(closers)-1]
			}
		}
	}
	if inString {
		if escaped {
			builder.WriteString(`\`)
		}
		builder.WriteByte('"')
	}
	for len(closers) > 0 {
		last := len(closers) - 1
		builder.WriteByte(closers[last])
		closers = closers[:last]
	}
	return builder.String()
}

func ParseOpenAIResponsesUsage(raw OpenAIResponsesUsage, model Model) Usage {
	cacheRead := raw.InputTokensDetails.CachedTokens
	cacheWrite := raw.InputTokensDetails.CacheWriteTokens
	input := raw.InputTokens - cacheRead - cacheWrite
	if input < 0 {
		input = 0
	}
	total := raw.TotalTokens
	if total == 0 {
		total = input + raw.OutputTokens + cacheRead + cacheWrite
	}
	usage := Usage{
		Input:       input,
		Output:      raw.OutputTokens,
		CacheRead:   cacheRead,
		CacheWrite:  cacheWrite,
		Reasoning:   ptrInt(raw.OutputTokensDetails.ReasoningTokens),
		TotalTokens: total,
	}
	usage.Cost = CalculateCost(model, usage)
	return usage
}

func openAIResponsesOutputItemText(item OpenAIResponsesOutputItem) string {
	text := ""
	for _, part := range item.Content {
		switch part.Type {
		case "output_text":
			text += part.Text
		case "refusal":
			text += part.Refusal
		}
	}
	return SanitizeSurrogates(text)
}

func openAIResponsesReasoningItemText(item OpenAIResponsesOutputItem) string {
	if text := openAIResponsesJoinedPartText(item.Summary); text != "" {
		return text
	}
	if text := openAIResponsesJoinedPartText(item.Content); text != "" {
		return text
	}
	return ""
}

func openAIResponsesJoinedPartText(parts []OpenAIResponsesOutputContentPart) string {
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part.Text) != "" {
			values = append(values, part.Text)
		} else if strings.TrimSpace(part.Refusal) != "" {
			values = append(values, part.Refusal)
		}
	}
	return SanitizeSurrogates(strings.Join(values, "\n\n"))
}

func encodeOpenAIResponsesReasoningSignature(item OpenAIResponsesOutputItem) string {
	if item.Type == "" {
		item.Type = "reasoning"
	}
	raw, err := json.Marshal(item)
	if err != nil {
		return ""
	}
	return string(raw)
}

func encodeOpenAIResponsesTextSignature(id, phase string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	value := struct {
		Version int    `json:"v"`
		ID      string `json:"id"`
		Phase   string `json:"phase,omitempty"`
	}{Version: 1, ID: id}
	if phase == "commentary" || phase == "final_answer" {
		value.Phase = phase
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return id
	}
	return string(raw)
}

func mapOpenAIResponsesStatus(status string) string {
	switch status {
	case "", "completed", "in_progress", "queued":
		return StopReasonStop
	case "incomplete":
		return StopReasonLength
	case "failed", "cancelled":
		return StopReasonError
	default:
		return StopReasonError
	}
}

func hasOpenAIResponsesToolCall(content []ContentPart) bool {
	for _, part := range content {
		if part.Type == ContentToolCall {
			return true
		}
	}
	return false
}
