package gillmprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

type MistralProvider struct {
	Client HTTPDoer
}

type MistralStreamEvent struct {
	Data MistralCompletionChunk `json:"data"`
}

type MistralCompletionChunk struct {
	ID      string                    `json:"id"`
	Choices []MistralCompletionChoice `json:"choices"`
	Usage   *MistralUsage             `json:"usage,omitempty"`
}

type MistralCompletionChoice struct {
	Delta        MistralDelta `json:"delta"`
	FinishReason string       `json:"finish_reason"`
}

type MistralDelta struct {
	Content   json.RawMessage         `json:"content,omitempty"`
	ToolCalls []MistralStreamToolCall `json:"tool_calls,omitempty"`
}

type MistralStreamToolCall struct {
	Index    *int                    `json:"index,omitempty"`
	ID       string                  `json:"id,omitempty"`
	Type     string                  `json:"type,omitempty"`
	Function MistralToolCallFunction `json:"function"`
}

type MistralUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type mistralStreamProcessor struct {
	model     Model
	output    *Message
	textIndex int
	toolIndex map[string]int
	toolArgs  map[string]string
}

func NewMistralProvider(client HTTPDoer) MistralProvider {
	return MistralProvider{Client: httpClientOrDefault(client)}
}

func init() {
	RegisterAPIProvider("mistral-conversations", NewMistralProvider(nil))
}

func (p MistralProvider) Stream(model Model, llmContext Context, options StreamOptions) (*AssistantMessageEventStream, error) {
	return p.StreamSimple(model, llmContext, options)
}

func (p MistralProvider) StreamSimple(model Model, llmContext Context, options SimpleStreamOptions) (*AssistantMessageEventStream, error) {
	apiKey := apiKeyOrEnv(model.Provider, options.APIKey)
	if apiKey == "" {
		return streamError(model, "missing API key for provider %s", model.Provider), nil
	}
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	payload := any(BuildMistralPayload(model, llmContext, MistralPayloadOptions{
		MaxTokens:   options.MaxTokens,
		Temperature: options.Temperature,
		Reasoning:   options.Reasoning,
	}))
	if options.OnPayload != nil {
		next, replace, err := options.OnPayload(payload, model)
		if err != nil {
			return streamError(model, "%s", err.Error()), nil
		}
		if replace {
			payload = next
		}
	}
	headers := mistralHeaders(model, options, apiKey)
	response, err := postSSE(ctx, httpClientOrDefault(p.Client), mistralEndpoint(model.BaseURL), headers, payload)
	if err != nil {
		return streamError(model, "request failed: %v", err), nil
	}
	if options.OnResponseStatus != nil {
		if err := options.OnResponseStatus(response.StatusCode, responseHeaders(response.Header), model); err != nil {
			response.Body.Close()
			return streamError(model, "%s", err.Error()), nil
		}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return responseErrorStream(model, response), nil
	}
	stream := NewAssistantMessageEventStream()
	go streamMistralBody(model, response.Body, stream)
	return stream, nil
}

func streamMistralBody(model Model, body io.ReadCloser, stream *AssistantMessageEventStream) {
	output := AssistantMessage(nil, StopReasonStop, model)
	stream.Push(AssistantMessageEvent{Type: "start", Partial: output})
	processor := newMistralStreamProcessor(model, &output)
	terminal := false
	err := dispatchSSEUntil(body, func(data string) (bool, error) {
		chunk, err := DecodeMistralCompletionChunk([]byte(data))
		if err != nil {
			return false, err
		}
		emitted := processor.Process(chunk)
		for _, event := range emitted {
			stream.Push(event)
		}
		if hasMistralFinishReason(chunk) {
			terminal = true
			message := processor.Result()
			if message.StopReason == StopReasonError {
				stream.Push(AssistantMessageEvent{Type: "error", Reason: message.StopReason, Error: message})
			} else {
				stream.Push(AssistantMessageEvent{Type: "done", Reason: message.StopReason, Message: message})
			}
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		stream.Push(AssistantMessageEvent{Type: "error", Reason: StopReasonError, Error: AssistantErrorMessage(err.Error(), model, false)})
		return
	}
	if !terminal {
		message := processor.Result()
		if message.StopReason == StopReasonError {
			stream.Push(AssistantMessageEvent{Type: "error", Reason: message.StopReason, Error: message})
		} else {
			stream.Push(AssistantMessageEvent{Type: "done", Reason: message.StopReason, Message: message})
		}
	}
}

func newMistralStreamProcessor(model Model, output *Message) *mistralStreamProcessor {
	return &mistralStreamProcessor{
		model:     model,
		output:    output,
		textIndex: -1,
		toolIndex: map[string]int{},
		toolArgs:  map[string]string{},
	}
}

func (p *mistralStreamProcessor) Process(chunk MistralCompletionChunk) []AssistantMessageEvent {
	if chunk.ID != "" {
		p.output.ResponseID = chunk.ID
	}
	if chunk.Usage != nil {
		p.output.Usage = ParseMistralUsage(*chunk.Usage, p.model)
	}
	var events []AssistantMessageEvent
	for _, choice := range chunk.Choices {
		if len(choice.Delta.Content) > 0 && string(choice.Delta.Content) != "null" {
			events = append(events, p.appendContent(choice.Delta.Content)...)
		}
		for _, call := range choice.Delta.ToolCalls {
			events = append(events, p.appendToolCall(call)...)
		}
		if choice.FinishReason != "" {
			p.output.StopReason = mapMistralFinishReason(choice.FinishReason)
		}
	}
	return events
}

func (p *mistralStreamProcessor) Result() Message {
	for key, index := range p.toolIndex {
		if index >= 0 && index < len(p.output.Content) {
			p.output.Content[index].Arguments = parseStreamingJSONObject(p.toolArgs[key])
		}
	}
	return *p.output
}

func (p *mistralStreamProcessor) appendContent(raw json.RawMessage) []AssistantMessageEvent {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return p.appendText(text)
	}
	var parts []MistralContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil
	}
	var events []AssistantMessageEvent
	for _, part := range parts {
		switch part.Type {
		case "text":
			events = append(events, p.appendText(part.Text)...)
		case "thinking":
			thinking := ""
			for _, block := range part.Thinking {
				thinking += block.Text
			}
			if thinking != "" {
				events = append(events, p.appendThinking(thinking)...)
			}
		}
	}
	return events
}

func (p *mistralStreamProcessor) appendText(delta string) []AssistantMessageEvent {
	if p.textIndex < 0 || p.textIndex >= len(p.output.Content) || p.output.Content[p.textIndex].Type != ContentText {
		p.output.Content = append(p.output.Content, Text(""))
		p.textIndex = len(p.output.Content) - 1
	}
	p.output.Content[p.textIndex].Text += SanitizeSurrogates(delta)
	return []AssistantMessageEvent{{Type: "text_delta", Partial: *p.output}}
}

func (p *mistralStreamProcessor) appendThinking(delta string) []AssistantMessageEvent {
	part := Thinking(SanitizeSurrogates(delta))
	part.ThinkingSignature = "mistral_thinking"
	p.output.Content = append(p.output.Content, part)
	return []AssistantMessageEvent{{Type: "thinking_delta", Partial: *p.output}}
}

func (p *mistralStreamProcessor) appendToolCall(call MistralStreamToolCall) []AssistantMessageEvent {
	key := mistralToolCallKey(call)
	index, ok := p.toolIndex[key]
	if !ok {
		id := call.ID
		if id == "" {
			id = key
		}
		p.output.Content = append(p.output.Content, ToolCall(id, call.Function.Name, nil))
		index = len(p.output.Content) - 1
		p.toolIndex[key] = index
	}
	p.toolArgs[key] += call.Function.Arguments
	p.output.Content[index].Arguments = parseStreamingJSONObject(p.toolArgs[key])
	if p.output.Content[index].Name == "" && call.Function.Name != "" {
		p.output.Content[index].Name = call.Function.Name
	}
	return []AssistantMessageEvent{{Type: "toolcall_delta", Partial: *p.output}}
}

func DecodeMistralCompletionChunk(data []byte) (MistralCompletionChunk, error) {
	var wrapped MistralStreamEvent
	if err := json.Unmarshal(data, &wrapped); err == nil && (wrapped.Data.ID != "" || len(wrapped.Data.Choices) > 0 || wrapped.Data.Usage != nil) {
		return wrapped.Data, nil
	}
	var chunk MistralCompletionChunk
	if err := json.Unmarshal(data, &chunk); err != nil {
		return MistralCompletionChunk{}, fmt.Errorf("decode Mistral SSE event: %w", err)
	}
	return chunk, nil
}

func ParseMistralUsage(raw MistralUsage, model Model) Usage {
	total := raw.TotalTokens
	if total == 0 {
		total = raw.PromptTokens + raw.CompletionTokens
	}
	usage := Usage{Input: raw.PromptTokens, Output: raw.CompletionTokens, TotalTokens: total}
	usage.Cost = CalculateCost(model, usage)
	return usage
}

func hasMistralFinishReason(chunk MistralCompletionChunk) bool {
	for _, choice := range chunk.Choices {
		if choice.FinishReason != "" {
			return true
		}
	}
	return false
}

func mapMistralFinishReason(reason string) string {
	switch reason {
	case "", "stop":
		return StopReasonStop
	case "length", "model_length":
		return StopReasonLength
	case "tool_calls":
		return StopReasonToolUse
	case "error":
		return StopReasonError
	default:
		return StopReasonStop
	}
}

func mistralToolCallKey(call MistralStreamToolCall) string {
	if call.ID != "" {
		return "id:" + call.ID
	}
	if call.Index != nil {
		return fmt.Sprintf("index:%d", *call.Index)
	}
	return "anonymous"
}

func mistralHeaders(model Model, options SimpleStreamOptions, apiKey string) map[string]string {
	headers := map[string]string{}
	for key, value := range model.Headers {
		headers[key] = value
	}
	for key, value := range options.Headers {
		headers[key] = value
	}
	if options.SessionID != "" && headers["x-affinity"] == "" {
		headers["x-affinity"] = options.SessionID
	}
	headers["Authorization"] = "Bearer " + apiKey
	return headers
}

func mistralEndpoint(baseURL string) string {
	return appendEndpoint(baseURL, "https://api.mistral.ai", "/v1/chat/completions")
}
