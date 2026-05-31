package gillmprovider

import (
	"encoding/json"
	"strings"
)

type OpenAIResponsesStreamEvent struct {
	Type      string
	Response  *OpenAIResponsesResponseEvent
	Item      *OpenAIResponsesOutputItem
	Part      *OpenAIResponsesOutputContentPart
	Delta     string
	Arguments string
	Error     string
}

type OpenAIResponsesResponseEvent struct {
	ID                string
	Status            string
	ServiceTier       string
	Usage             *OpenAIResponsesUsage
	IncompleteDetails *OpenAIResponsesIncompleteDetails
}

type OpenAIResponsesUsage struct {
	InputTokens        int
	OutputTokens       int
	TotalTokens        int
	InputTokensDetails OpenAIResponsesInputTokenDetails
}

type OpenAIResponsesInputTokenDetails struct {
	CachedTokens     int
	CacheWriteTokens int
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
	model            Model
	output           *Message
	currentTool      *openAIResponsesToolState
	currentReasoning *openAIResponsesReasoningState
	textIndex        int
}

type openAIResponsesToolState struct {
	index       int
	partialJSON string
}

type openAIResponsesReasoningState struct {
	index              int
	summaryPartStarted bool
}

func NewOpenAIResponsesStreamProcessor(model Model, output *Message) *OpenAIResponsesStreamProcessor {
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
	return &OpenAIResponsesStreamProcessor{model: model, output: output, textIndex: -1}
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
		if event.Item.Type == "reasoning" {
			part := Thinking("")
			p.output.Content = append(p.output.Content, part)
			p.currentReasoning = &openAIResponsesReasoningState{index: len(p.output.Content) - 1}
			return []AssistantMessageEvent{{Type: "thinking_start", ContentIndex: p.currentReasoning.index, Partial: *p.output}}
		}
		if event.Item.Type == "message" {
			p.output.Content = append(p.output.Content, Text(""))
			p.textIndex = len(p.output.Content) - 1
			return []AssistantMessageEvent{{Type: "text_start", ContentIndex: p.textIndex, Partial: *p.output}}
		}
		if event.Item.Type == "function_call" {
			args := parseStreamingJSONObject(event.Item.Arguments)
			p.output.Content = append(p.output.Content, ToolCall(event.Item.CallID+"|"+event.Item.ID, event.Item.Name, args))
			p.currentTool = &openAIResponsesToolState{index: len(p.output.Content) - 1, partialJSON: event.Item.Arguments}
			return []AssistantMessageEvent{{Type: "toolcall_start", ContentIndex: p.currentTool.index, Partial: *p.output}}
		}
	case "response.content_part.added":
		if event.Part != nil && event.Part.Type == "output_text" && p.textIndex < 0 {
			p.output.Content = append(p.output.Content, Text(SanitizeSurrogates(event.Part.Text)))
			p.textIndex = len(p.output.Content) - 1
			return []AssistantMessageEvent{{Type: "text_start", ContentIndex: p.textIndex, Partial: *p.output}}
		}
	case "response.output_text.delta", "response.refusal.delta":
		if p.textIndex >= 0 && p.textIndex < len(p.output.Content) {
			p.output.Content[p.textIndex].Text += SanitizeSurrogates(event.Delta)
			return []AssistantMessageEvent{{Type: "text_delta", ContentIndex: p.textIndex, Delta: event.Delta, Partial: *p.output}}
		}
	case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
		if p.currentReasoning != nil && p.currentReasoning.index >= 0 && p.currentReasoning.index < len(p.output.Content) {
			if event.Type == "response.reasoning_summary_text.delta" && !p.currentReasoning.summaryPartStarted {
				return nil
			}
			p.output.Content[p.currentReasoning.index].Thinking += SanitizeSurrogates(event.Delta)
			return []AssistantMessageEvent{{Type: "thinking_delta", ContentIndex: p.currentReasoning.index, Delta: event.Delta, Partial: *p.output}}
		}
	case "response.reasoning_summary_part.added":
		if p.currentReasoning != nil {
			p.currentReasoning.summaryPartStarted = true
		}
	case "response.reasoning_summary_part.done":
		if p.currentReasoning != nil && p.currentReasoning.index >= 0 && p.currentReasoning.index < len(p.output.Content) {
			if !p.currentReasoning.summaryPartStarted {
				return nil
			}
			p.output.Content[p.currentReasoning.index].Thinking += "\n\n"
			p.currentReasoning.summaryPartStarted = false
			return []AssistantMessageEvent{{Type: "thinking_delta", ContentIndex: p.currentReasoning.index, Delta: "\n\n", Partial: *p.output}}
		}
	case "response.function_call_arguments.delta":
		if p.currentTool != nil {
			p.currentTool.partialJSON += event.Delta
			if p.currentTool.index >= 0 && p.currentTool.index < len(p.output.Content) {
				p.output.Content[p.currentTool.index].Arguments = parseStreamingJSONObject(p.currentTool.partialJSON)
			}
			return []AssistantMessageEvent{{Type: "toolcall_delta", ContentIndex: p.currentTool.index, Delta: event.Delta, Partial: *p.output}}
		}
	case "response.function_call_arguments.done":
		if p.currentTool != nil {
			previousPartialJSON := p.currentTool.partialJSON
			p.currentTool.partialJSON = event.Arguments
			if p.currentTool.index >= 0 && p.currentTool.index < len(p.output.Content) {
				p.output.Content[p.currentTool.index].Arguments = parseStreamingJSONObject(event.Arguments)
			}
			if strings.HasPrefix(event.Arguments, previousPartialJSON) {
				delta := event.Arguments[len(previousPartialJSON):]
				if delta != "" {
					return []AssistantMessageEvent{{Type: "toolcall_delta", ContentIndex: p.currentTool.index, Delta: delta, Partial: *p.output}}
				}
			}
		}
	case "response.output_item.done":
		if event.Item == nil {
			return nil
		}
		if event.Item.Type == "reasoning" {
			index := -1
			if p.currentReasoning != nil {
				index = p.currentReasoning.index
			}
			if index < 0 || index >= len(p.output.Content) {
				p.output.Content = append(p.output.Content, Thinking(""))
				index = len(p.output.Content) - 1
			}
			text := openAIResponsesReasoningItemText(*event.Item)
			if text != "" {
				p.output.Content[index].Thinking = text
			}
			if signature := encodeOpenAIResponsesReasoningSignature(*event.Item); signature != "" {
				p.output.Content[index].ThinkingSignature = signature
			}
			p.currentReasoning = nil
			return []AssistantMessageEvent{{Type: "thinking_end", ContentIndex: index, Partial: *p.output, Content: p.output.Content[index].Thinking}}
		}
		if event.Item.Type == "message" {
			text := openAIResponsesOutputItemText(*event.Item)
			if p.textIndex < 0 {
				p.output.Content = append(p.output.Content, Text(text))
				p.textIndex = len(p.output.Content) - 1
			} else if text != "" {
				p.output.Content[p.textIndex].Text = SanitizeSurrogates(text)
			}
			if p.textIndex >= 0 && p.textIndex < len(p.output.Content) {
				p.output.Content[p.textIndex].TextSignature = encodeOpenAIResponsesTextSignature(event.Item.ID, event.Item.Phase)
			}
			index := p.textIndex
			p.textIndex = -1
			if index >= 0 && index < len(p.output.Content) {
				return []AssistantMessageEvent{{Type: "text_end", ContentIndex: index, Content: p.output.Content[index].Text, Partial: *p.output}}
			}
			return nil
		}
		if event.Item.Type == "function_call" {
			index := -1
			if p.currentTool != nil {
				index = p.currentTool.index
			}
			if index < 0 || index >= len(p.output.Content) {
				p.output.Content = append(p.output.Content, ToolCall(event.Item.CallID+"|"+event.Item.ID, event.Item.Name, nil))
				index = len(p.output.Content) - 1
			}
			p.output.Content[index].ID = event.Item.CallID + "|" + event.Item.ID
			p.output.Content[index].Name = event.Item.Name
			p.output.Content[index].Arguments = parseStreamingJSONObject(event.Item.Arguments)
			toolCall := p.output.Content[index]
			p.currentTool = nil
			return []AssistantMessageEvent{{Type: "toolcall_end", ContentIndex: index, ToolCall: toolCall, Partial: *p.output}}
		}
	case "response.completed", "response.incomplete", "response.failed":
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
			return []AssistantMessageEvent{{Type: "error", Reason: p.output.StopReason, Error: *p.output}}
		}
		return []AssistantMessageEvent{{Type: "done", Reason: p.output.StopReason, Message: *p.output}}
	}
	return nil
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
