package gillmprovider

import "encoding/json"

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
	Type      string
	ID        string
	CallID    string
	Name      string
	Arguments string
	Status    string
	Content   []OpenAIResponsesOutputContentPart
	Phase     string
}

type OpenAIResponsesOutputContentPart struct {
	Type    string
	Text    string
	Refusal string
}

type OpenAIResponsesStreamProcessor struct {
	model       Model
	output      *Message
	currentTool *openAIResponsesToolState
	textIndex   int
}

type openAIResponsesToolState struct {
	index       int
	partialJSON string
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
		if event.Item.Type == "message" {
			p.output.Content = append(p.output.Content, Text(""))
			p.textIndex = len(p.output.Content) - 1
			return []AssistantMessageEvent{{Type: "text_start", Partial: *p.output}}
		}
		if event.Item.Type == "function_call" {
			args := parseStreamingJSONObject(event.Item.Arguments)
			p.output.Content = append(p.output.Content, ToolCall(event.Item.CallID+"|"+event.Item.ID, event.Item.Name, args))
			p.currentTool = &openAIResponsesToolState{index: len(p.output.Content) - 1, partialJSON: event.Item.Arguments}
			return []AssistantMessageEvent{{Type: "toolcall_start", Partial: *p.output}}
		}
	case "response.content_part.added":
		if event.Part != nil && event.Part.Type == "output_text" && p.textIndex < 0 {
			p.output.Content = append(p.output.Content, Text(SanitizeSurrogates(event.Part.Text)))
			p.textIndex = len(p.output.Content) - 1
			return []AssistantMessageEvent{{Type: "text_start", Partial: *p.output}}
		}
	case "response.output_text.delta", "response.refusal.delta":
		if p.textIndex >= 0 && p.textIndex < len(p.output.Content) {
			p.output.Content[p.textIndex].Text += SanitizeSurrogates(event.Delta)
			return []AssistantMessageEvent{{Type: "text_delta", Partial: *p.output}}
		}
	case "response.function_call_arguments.delta":
		if p.currentTool != nil {
			p.currentTool.partialJSON += event.Delta
			if p.currentTool.index >= 0 && p.currentTool.index < len(p.output.Content) {
				p.output.Content[p.currentTool.index].Arguments = parseStreamingJSONObject(p.currentTool.partialJSON)
			}
			return []AssistantMessageEvent{{Type: "toolcall_delta", Partial: *p.output}}
		}
	case "response.function_call_arguments.done":
		if p.currentTool != nil {
			p.currentTool.partialJSON = event.Arguments
			if p.currentTool.index >= 0 && p.currentTool.index < len(p.output.Content) {
				p.output.Content[p.currentTool.index].Arguments = parseStreamingJSONObject(event.Arguments)
			}
			return []AssistantMessageEvent{{Type: "toolcall_delta", Partial: *p.output}}
		}
	case "response.output_item.done":
		if event.Item == nil {
			return nil
		}
		if event.Item.Type == "message" {
			text := openAIResponsesOutputItemText(*event.Item)
			if p.textIndex < 0 {
				p.output.Content = append(p.output.Content, Text(text))
				p.textIndex = len(p.output.Content) - 1
			} else if text != "" {
				p.output.Content[p.textIndex].Text = SanitizeSurrogates(text)
			}
			p.textIndex = -1
			return []AssistantMessageEvent{{Type: "text_end", Partial: *p.output}}
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
			return []AssistantMessageEvent{{Type: "toolcall_end", Partial: *p.output, Message: Message{Content: []ContentPart{toolCall}}}}
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
	if data == "" {
		return map[string]any{}
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(data), &result); err != nil || result == nil {
		return map[string]any{}
	}
	return result
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
