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
	PromptTokens       int `json:"prompt_tokens"`
	CompletionTokens   int `json:"completion_tokens"`
	TotalTokens        int `json:"total_tokens"`
	CachedPromptTokens int `json:"-"`
}

type mistralUsageDetails struct {
	CachedTokens      *int `json:"cached_tokens"`
	CachedTokensCamel *int `json:"cachedTokens"`
}

func (d *mistralUsageDetails) cachedTokens() (*int, bool) {
	if d == nil {
		return nil, false
	}
	if d.CachedTokensCamel != nil {
		return d.CachedTokensCamel, true
	}
	if d.CachedTokens != nil {
		return d.CachedTokens, true
	}
	return nil, false
}

func (u *MistralUsage) UnmarshalJSON(data []byte) error {
	var wire struct {
		PromptTokens            *int                 `json:"prompt_tokens"`
		PromptTokensCamel       *int                 `json:"promptTokens"`
		CompletionTokens        *int                 `json:"completion_tokens"`
		CompletionTokensCamel   *int                 `json:"completionTokens"`
		TotalTokens             *int                 `json:"total_tokens"`
		TotalTokensCamel        *int                 `json:"totalTokens"`
		PromptTokensDetails     *mistralUsageDetails `json:"promptTokensDetails"`
		PromptTokensDetailsWire *mistralUsageDetails `json:"prompt_tokens_details"`
		PromptTokenDetails      *mistralUsageDetails `json:"promptTokenDetails"`
		PromptTokenDetailsWire  *mistralUsageDetails `json:"prompt_token_details"`
		NumCachedTokens         *int                 `json:"numCachedTokens"`
		NumCachedTokensWire     *int                 `json:"num_cached_tokens"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	u.PromptTokens = firstInt(wire.PromptTokens, wire.PromptTokensCamel)
	u.CompletionTokens = firstInt(wire.CompletionTokens, wire.CompletionTokensCamel)
	u.TotalTokens = firstInt(wire.TotalTokens, wire.TotalTokensCamel)
	for _, details := range []*mistralUsageDetails{
		wire.PromptTokensDetails,
		wire.PromptTokensDetailsWire,
		wire.PromptTokenDetails,
		wire.PromptTokenDetailsWire,
	} {
		if cached, ok := details.cachedTokens(); ok {
			u.CachedPromptTokens = *cached
			return nil
		}
	}
	u.CachedPromptTokens = firstInt(wire.NumCachedTokens, wire.NumCachedTokensWire)
	return nil
}

func firstInt(values ...*int) int {
	for _, value := range values {
		if value != nil {
			return *value
		}
	}
	return 0
}

type mistralStreamProcessor struct {
	model        Model
	output       *Message
	currentIndex int
	currentType  string
	toolIndex    map[string]int
	toolArgs     map[string]string
	toolOrder    []string
	finished     bool
}

func NewMistralProvider(client HTTPDoer) MistralProvider {
	return MistralProvider{Client: httpClientOrDefault(client)}
}

func init() {
	RegisterBuiltInAPIProvider("mistral-conversations", NewMistralProvider(nil))
}

func (p MistralProvider) Stream(model Model, llmContext Context, options StreamOptions) (*AssistantMessageEventStream, error) {
	return p.stream(model, llmContext, options)
}

func (p MistralProvider) StreamSimple(model Model, llmContext Context, options SimpleStreamOptions) (*AssistantMessageEventStream, error) {
	return p.stream(model, llmContext, prepareSimpleStreamOptions(model, llmContext, options))
}

func (p MistralProvider) stream(model Model, llmContext Context, options StreamOptions) (*AssistantMessageEventStream, error) {
	apiKey := apiKeyOrEnv(model.Provider, options.APIKey, options.Env)
	if apiKey == "" {
		return streamError(model, "missing API key for provider %s", model.Provider), nil
	}
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	payload := any(BuildMistralPayload(model, llmContext, MistralPayloadOptions{
		MaxTokens:      options.MaxTokens,
		Temperature:    options.Temperature,
		Reasoning:      options.Reasoning,
		SessionID:      options.SessionID,
		CacheRetention: options.CacheRetention,
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
	response, err := postSSEWithRetry(
		ctx,
		httpClientForRequest(p.Client, options),
		mistralEndpoint(model.BaseURL),
		headers,
		payload,
		providerRetryOptions(options.MaxRetries, options.MaxRetryDelayMs),
		func(status int, headers map[string]string) error {
			if options.OnResponseStatus == nil {
				return nil
			}
			return options.OnResponseStatus(status, headers, model)
		},
	)
	if err != nil {
		if ctx.Err() != nil {
			return ErrorAssistantStream(AssistantErrorMessage(ctx.Err().Error(), model, true)), nil
		}
		return streamProviderRequestError(model, err), nil
	}
	stream := NewAssistantMessageEventStream()
	go streamMistralBody(model, response.Body, stream)
	return stream, nil
}

func streamMistralBody(model Model, body io.ReadCloser, stream *AssistantMessageEventStream) {
	output := AssistantMessage(nil, StopReasonStop, model)
	stream.Push(AssistantMessageEvent{
		Type:    "start",
		Partial: cloneMessageState(output),
	})
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
			for _, event := range processor.Finish() {
				stream.Push(event)
			}
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
		output.StopReason = StopReasonError
		output.ErrorMessage = err.Error()
		stream.Push(AssistantMessageEvent{
			Type:   "error",
			Reason: StopReasonError,
			Error:  cloneMessageState(output),
		})
		return
	}
	if !terminal {
		for _, event := range processor.Finish() {
			stream.Push(event)
		}
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
		model:        model,
		output:       output,
		currentIndex: -1,
		toolIndex:    map[string]int{},
		toolArgs:     map[string]string{},
	}
}

func (p *mistralStreamProcessor) Process(chunk MistralCompletionChunk) []AssistantMessageEvent {
	if chunk.ID != "" && p.output.ResponseID == "" {
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
	return snapshotPartialEvents(events, *p.output)
}

func (p *mistralStreamProcessor) Result() Message {
	return cloneMessageState(*p.output)
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
	var events []AssistantMessageEvent
	if p.currentIndex < 0 || p.currentType != ContentText {
		events = append(events, p.closeCurrent()...)
		p.output.Content = append(p.output.Content, Text(""))
		p.currentIndex = len(p.output.Content) - 1
		p.currentType = ContentText
		events = append(events, AssistantMessageEvent{
			Type:         "text_start",
			ContentIndex: p.currentIndex,
		})
	}
	delta = SanitizeSurrogates(delta)
	p.output.Content[p.currentIndex].Text += delta
	return append(events, AssistantMessageEvent{
		Type:         "text_delta",
		ContentIndex: p.currentIndex,
		Delta:        delta,
	})
}

func (p *mistralStreamProcessor) appendThinking(delta string) []AssistantMessageEvent {
	var events []AssistantMessageEvent
	if p.currentIndex < 0 || p.currentType != ContentThinking {
		events = append(events, p.closeCurrent()...)
		p.output.Content = append(p.output.Content, Thinking(""))
		p.currentIndex = len(p.output.Content) - 1
		p.currentType = ContentThinking
		events = append(events, AssistantMessageEvent{
			Type:         "thinking_start",
			ContentIndex: p.currentIndex,
		})
	}
	delta = SanitizeSurrogates(delta)
	p.output.Content[p.currentIndex].Thinking += delta
	return append(events, AssistantMessageEvent{
		Type:         "thinking_delta",
		ContentIndex: p.currentIndex,
		Delta:        delta,
	})
}

func (p *mistralStreamProcessor) appendToolCall(call MistralStreamToolCall) []AssistantMessageEvent {
	events := p.closeCurrent()
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
		p.toolOrder = append(p.toolOrder, key)
		events = append(events, AssistantMessageEvent{
			Type:         "toolcall_start",
			ContentIndex: index,
		})
	}
	p.toolArgs[key] += call.Function.Arguments
	p.output.Content[index].Arguments = parseStreamingJSONObject(p.toolArgs[key])
	if p.output.Content[index].Name == "" && call.Function.Name != "" {
		p.output.Content[index].Name = call.Function.Name
	}
	return append(events, AssistantMessageEvent{
		Type:         "toolcall_delta",
		ContentIndex: index,
		Delta:        call.Function.Arguments,
	})
}

func (p *mistralStreamProcessor) Finish() []AssistantMessageEvent {
	if p.finished {
		return nil
	}
	p.finished = true
	events := p.closeCurrent()
	for _, key := range p.toolOrder {
		index := p.toolIndex[key]
		if index < 0 || index >= len(p.output.Content) {
			continue
		}
		p.output.Content[index].Arguments = parseStreamingJSONObject(
			p.toolArgs[key],
		)
		events = append(events, AssistantMessageEvent{
			Type:         "toolcall_end",
			ContentIndex: index,
			ToolCall:     cloneContentPartState(p.output.Content[index]),
		})
	}
	return snapshotPartialEvents(events, *p.output)
}

func (p *mistralStreamProcessor) closeCurrent() []AssistantMessageEvent {
	if p.currentIndex < 0 || p.currentIndex >= len(p.output.Content) {
		return nil
	}
	index := p.currentIndex
	part := p.output.Content[index]
	p.currentIndex = -1
	p.currentType = ""
	switch part.Type {
	case ContentText:
		return []AssistantMessageEvent{{
			Type:         "text_end",
			ContentIndex: index,
			Content:      part.Text,
		}}
	case ContentThinking:
		return []AssistantMessageEvent{{
			Type:         "thinking_end",
			ContentIndex: index,
			Content:      part.Thinking,
		}}
	default:
		return nil
	}
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
	cacheRead := min(raw.PromptTokens, max(0, raw.CachedPromptTokens))
	input := raw.PromptTokens - cacheRead
	total := raw.TotalTokens
	if total == 0 {
		total = raw.PromptTokens + raw.CompletionTokens
	}
	usage := Usage{
		Input:       input,
		Output:      raw.CompletionTokens,
		CacheRead:   cacheRead,
		TotalTokens: total,
	}
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
	if shouldUseMistralPromptCaching(options.SessionID, options.CacheRetention) && headers["x-affinity"] == "" {
		headers["x-affinity"] = options.SessionID
	}
	headers["Authorization"] = "Bearer " + apiKey
	return headers
}

func mistralEndpoint(baseURL string) string {
	return appendEndpoint(baseURL, "https://api.mistral.ai", "/v1/chat/completions")
}
