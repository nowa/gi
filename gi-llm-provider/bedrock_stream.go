package gillmprovider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/smithy-go"
)

type BedrockConverseStreamTransport func(context.Context, BedrockConverseStreamRequest) (<-chan BedrockConverseStreamEvent, error)

type BedrockConverseStreamProvider struct {
	Transport BedrockConverseStreamTransport
}

type BedrockConverseStreamRequest struct {
	Model           Model
	Payload         BedrockPayload
	ClientConfig    BedrockClientConfig
	MaxTokens       int
	Temperature     *float64
	CacheRetention  string
	RequestMetadata map[string]string
	Headers         map[string]string
	OnResponse      func(status int, headers map[string]string) error
}

type BedrockConverseStreamEvent struct {
	MessageStart      *BedrockMessageStartEvent
	ContentBlockStart *BedrockContentBlockStartEvent
	ContentBlockDelta *BedrockContentBlockDeltaEvent
	ContentBlockStop  *BedrockContentBlockStopEvent
	MessageStop       *BedrockMessageStopEvent
	Metadata          *BedrockMetadataEvent
	Error             error
}

type BedrockMessageStartEvent struct {
	Role string
}

type BedrockContentBlockStartEvent struct {
	ContentBlockIndex int
	ToolUse           *BedrockToolUseBlock
}

type BedrockContentBlockDeltaEvent struct {
	ContentBlockIndex int
	Text              string
	ToolUseInput      string
	ReasoningContent  *BedrockReasoningContent
}

type BedrockContentBlockStopEvent struct {
	ContentBlockIndex int
}

type BedrockMessageStopEvent struct {
	StopReason string
}

type BedrockMetadataEvent struct {
	Usage BedrockUsage
}

type BedrockUsage struct {
	InputTokens          int
	OutputTokens         int
	CacheReadInputTokens int
	CacheWriteTokens     int
	TotalTokens          int
}

type bedrockStreamBlock struct {
	ContentPart
	index       int
	partialJSON string
}

func init() {
	RegisterBuiltInAPIProvider(
		"bedrock-converse-stream",
		NewBedrockConverseStreamProvider(NewAWSBedrockConverseStreamTransport()),
	)
}

func NewBedrockConverseStreamProvider(transport BedrockConverseStreamTransport) BedrockConverseStreamProvider {
	return BedrockConverseStreamProvider{Transport: transport}
}

func (p BedrockConverseStreamProvider) Stream(model Model, llmContext Context, options StreamOptions) (*AssistantMessageEventStream, error) {
	return p.stream(model, llmContext, options, false)
}

func (p BedrockConverseStreamProvider) StreamSimple(model Model, llmContext Context, options SimpleStreamOptions) (*AssistantMessageEventStream, error) {
	return p.stream(
		model,
		llmContext,
		prepareSimpleStreamOptions(model, llmContext, options),
		true,
	)
}

func (p BedrockConverseStreamProvider) stream(
	model Model,
	llmContext Context,
	options StreamOptions,
	simple bool,
) (*AssistantMessageEventStream, error) {
	transport := p.Transport
	if transport == nil {
		return streamError(model, "Bedrock Converse Stream transport is not configured"), nil
	}
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	reasoning := ""
	if options.Reasoning != "" {
		reasoning = ClampThinkingLevel(model, options.Reasoning)
		if reasoning == "off" {
			reasoning = ""
		}
	}
	if simple &&
		reasoning != "" &&
		IsAnthropicClaudeBedrockModel(model) &&
		!SupportsBedrockAdaptiveThinking(model) {
		allocation := AdjustMaxTokensForThinking(
			options.MaxTokens,
			model.MaxTokens,
			reasoning,
			options.ThinkingBudgets,
		)
		allocation = clampThinkingTokenAllocationToContext(model, llmContext, allocation)
		options.MaxTokens = allocation.MaxTokens
		if options.ThinkingBudgets == nil {
			options.ThinkingBudgets = map[string]int{}
		}
		options.ThinkingBudgets[normalizeThinkingBudgetLevel(reasoning)] = allocation.ThinkingBudget
	}
	cacheRetention := firstNonEmpty(
		options.CacheRetention,
		metadataString(options.Metadata, "cache_retention"),
	)
	cacheRetention = resolveCacheRetentionWithEnv(cacheRetention, options.Env)
	toolChoice := options.ToolChoice
	if toolChoice == nil {
		toolChoice = metadataValue(options.Metadata, "tool_choice")
	}
	var interleavedThinking *bool
	if configured, ok := metadataValue(
		options.Metadata,
		"interleaved_thinking",
	).(bool); ok {
		interleavedThinking = &configured
	}
	payloadOptions := BedrockPayloadOptions{
		Reasoning:           reasoning,
		Region:              metadataString(options.Metadata, "region"),
		ThinkingBudgets:     options.ThinkingBudgets,
		ThinkingDisplay:     metadataString(options.Metadata, "thinking_display"),
		InterleavedThinking: interleavedThinking,
		CacheRetention:      cacheRetention,
		ForcePromptCache: GetProviderEnvValue(
			"AWS_BEDROCK_FORCE_CACHE",
			options.Env,
		) == "1",
		ToolChoice: toolChoice,
	}
	payload, err := BuildBedrockPayloadChecked(model, llmContext, payloadOptions)
	if err != nil {
		return streamError(model, "%s", err.Error()), nil
	}
	maxTokens := options.MaxTokens
	if maxTokens == 0 && IsAnthropicClaudeBedrockModel(model) {
		maxTokens = model.MaxTokens
	}
	clientOptions := BedrockClientOptions{
		Region:      payloadOptions.Region,
		Profile:     metadataString(options.Metadata, "profile"),
		APIKey:      options.APIKey,
		BearerToken: metadataString(options.Metadata, "bearer_token"),
		Env:         options.Env,
	}
	clientConfig, err := ResolveBedrockClientConfigChecked(model, clientOptions)
	if err != nil {
		return streamError(model, "%s", err.Error()), nil
	}
	request := BedrockConverseStreamRequest{
		Model:           model,
		Payload:         payload,
		ClientConfig:    clientConfig,
		MaxTokens:       maxTokens,
		Temperature:     options.Temperature,
		CacheRetention:  payloadOptions.CacheRetention,
		RequestMetadata: metadataStringMap(options.Metadata, "request_metadata"),
		Headers: applyHeaderRemovals(
			mergeHeadersCaseInsensitive(model.Headers, options.Headers),
			options.HeaderRemovals,
		),
	}
	if options.OnResponseStatus != nil {
		request.OnResponse = func(status int, headers map[string]string) error {
			return options.OnResponseStatus(status, headers, model)
		}
	}
	if options.OnPayload != nil {
		next, replace, err := options.OnPayload(request.Payload, model)
		if err != nil {
			return streamError(model, "%s", err.Error()), nil
		}
		if replace {
			payload, ok := next.(BedrockPayload)
			if !ok {
				return streamError(model, "Bedrock onPayload must return BedrockPayload"), nil
			}
			request.Payload = payload
		}
	}
	streamContext, cancelStream := context.WithCancel(ctx)
	events, err := transport(streamContext, request)
	if err != nil {
		cancelStream()
		return streamError(model, "%s", formatBedrockStreamError(err)), nil
	}
	stream := NewAssistantMessageEventStream()
	go func() {
		defer cancelStream()
		streamBedrockConverseEvents(streamContext, model, events, stream)
	}()
	return stream, nil
}

func streamBedrockConverseEvents(ctx context.Context, model Model, events <-chan BedrockConverseStreamEvent, stream *AssistantMessageEventStream) {
	output := AssistantMessage(nil, StopReasonStop, model)
	processor := NewBedrockConverseStreamProcessor(model, &output)
	terminal := false
	for {
		select {
		case <-ctx.Done():
			stream.Push(AssistantMessageEvent{Type: "error", Reason: StopReasonAborted, Error: AssistantErrorMessage(ctx.Err().Error(), model, true)})
			return
		case event, ok := <-events:
			if !ok {
				if !terminal {
					if output.StopReason == StopReasonError ||
						output.StopReason == StopReasonAborted {
						if output.ErrorMessage == "" {
							output.ErrorMessage = "Bedrock stream ended with an unknown error"
						}
						stream.Push(AssistantMessageEvent{
							Type:   "error",
							Reason: output.StopReason,
							Error:  cloneMessageState(output),
						})
					} else {
						stream.Push(AssistantMessageEvent{
							Type:    "done",
							Reason:  output.StopReason,
							Message: cloneMessageState(output),
						})
					}
				}
				return
			}
			for _, emitted := range processor.Process(event) {
				stream.Push(emitted)
				if emitted.Type == "done" || emitted.Type == "error" {
					terminal = true
					return
				}
			}
		}
	}
}

type BedrockConverseStreamProcessor struct {
	model  Model
	output *Message
	blocks []bedrockStreamBlock
}

func NewBedrockConverseStreamProcessor(model Model, output *Message) *BedrockConverseStreamProcessor {
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
	return &BedrockConverseStreamProcessor{model: model, output: output}
}

func (p *BedrockConverseStreamProcessor) Process(event BedrockConverseStreamEvent) []AssistantMessageEvent {
	events := p.process(event)
	for index := range events {
		switch events[index].Type {
		case "done":
			events[index].Message = cloneMessageState(events[index].Message)
		case "error":
			events[index].Error = cloneMessageState(events[index].Error)
		default:
			events[index].Partial = cloneMessageState(*p.output)
			events[index].ToolCall = cloneContentPartState(
				events[index].ToolCall,
			)
		}
	}
	return events
}

func (p *BedrockConverseStreamProcessor) process(event BedrockConverseStreamEvent) []AssistantMessageEvent {
	if event.Error != nil {
		p.output.StopReason = StopReasonError
		p.output.ErrorMessage = formatBedrockStreamError(event.Error)
		return []AssistantMessageEvent{{Type: "error", Reason: StopReasonError, Error: *p.output}}
	}
	if event.MessageStart != nil {
		if event.MessageStart.Role != "" && event.MessageStart.Role != "assistant" {
			p.output.StopReason = StopReasonError
			p.output.ErrorMessage = fmt.Sprintf("Unexpected assistant message start but got %s message start instead", event.MessageStart.Role)
			return []AssistantMessageEvent{{Type: "error", Reason: StopReasonError, Error: *p.output}}
		}
		return []AssistantMessageEvent{{Type: "start", Partial: *p.output}}
	}
	if event.ContentBlockStart != nil {
		return p.processContentBlockStart(*event.ContentBlockStart)
	}
	if event.ContentBlockDelta != nil {
		return p.processContentBlockDelta(*event.ContentBlockDelta)
	}
	if event.ContentBlockStop != nil {
		return p.processContentBlockStop(*event.ContentBlockStop)
	}
	if event.MessageStop != nil {
		p.output.StopReason = mapBedrockStopReason(event.MessageStop.StopReason)
		if p.output.StopReason == StopReasonError {
			p.output.ErrorMessage = event.MessageStop.StopReason
		}
		return nil
	}
	if event.Metadata != nil {
		p.processMetadata(*event.Metadata)
		return nil
	}
	return nil
}

func (p *BedrockConverseStreamProcessor) processContentBlockStart(event BedrockContentBlockStartEvent) []AssistantMessageEvent {
	if event.ToolUse == nil {
		return nil
	}
	part := ToolCall(event.ToolUse.ToolUseID, event.ToolUse.Name, event.ToolUse.Input)
	block := bedrockStreamBlock{ContentPart: part, index: event.ContentBlockIndex}
	p.blocks = append(p.blocks, block)
	p.output.Content = append(p.output.Content, part)
	return []AssistantMessageEvent{{Type: "toolcall_start", ContentIndex: len(p.output.Content) - 1, Partial: *p.output}}
}

func (p *BedrockConverseStreamProcessor) processContentBlockDelta(event BedrockContentBlockDeltaEvent) []AssistantMessageEvent {
	index := p.blockIndex(event.ContentBlockIndex)
	if event.Text != "" {
		if index < 0 {
			part := Text("")
			p.blocks = append(p.blocks, bedrockStreamBlock{ContentPart: part, index: event.ContentBlockIndex})
			p.output.Content = append(p.output.Content, part)
			index = len(p.blocks) - 1
			start := AssistantMessageEvent{Type: "text_start", ContentIndex: index, Partial: *p.output}
			delta := p.appendTextDelta(index, event.Text)
			return []AssistantMessageEvent{start, delta}
		}
		if p.blocks[index].Type == ContentText {
			return []AssistantMessageEvent{p.appendTextDelta(index, event.Text)}
		}
	}
	if event.ToolUseInput != "" && index >= 0 && p.blocks[index].Type == ContentToolCall {
		p.blocks[index].partialJSON += event.ToolUseInput
		p.blocks[index].Arguments = parseStreamingJSONObject(p.blocks[index].partialJSON)
		p.output.Content[index].Arguments = p.blocks[index].Arguments
		return []AssistantMessageEvent{{Type: "toolcall_delta", ContentIndex: index, Delta: event.ToolUseInput, Partial: *p.output}}
	}
	if event.ReasoningContent != nil {
		if index < 0 {
			part := Thinking("")
			p.blocks = append(p.blocks, bedrockStreamBlock{ContentPart: part, index: event.ContentBlockIndex})
			p.output.Content = append(p.output.Content, part)
			index = len(p.blocks) - 1
			start := AssistantMessageEvent{Type: "thinking_start", ContentIndex: index, Partial: *p.output}
			deltas := p.appendThinkingDelta(index, *event.ReasoningContent)
			return append([]AssistantMessageEvent{start}, deltas...)
		}
		if p.blocks[index].Type == ContentThinking {
			return p.appendThinkingDelta(index, *event.ReasoningContent)
		}
	}
	return nil
}

func (p *BedrockConverseStreamProcessor) appendTextDelta(index int, delta string) AssistantMessageEvent {
	p.blocks[index].Text += SanitizeSurrogates(delta)
	p.output.Content[index].Text = p.blocks[index].Text
	return AssistantMessageEvent{Type: "text_delta", ContentIndex: index, Delta: delta, Partial: *p.output}
}

func (p *BedrockConverseStreamProcessor) appendThinkingDelta(index int, delta BedrockReasoningContent) []AssistantMessageEvent {
	var events []AssistantMessageEvent
	if delta.Text != "" {
		p.blocks[index].Thinking += SanitizeSurrogates(delta.Text)
		p.output.Content[index].Thinking = p.blocks[index].Thinking
		events = append(events, AssistantMessageEvent{Type: "thinking_delta", ContentIndex: index, Delta: delta.Text, Partial: *p.output})
	}
	if delta.Signature != "" {
		p.blocks[index].ThinkingSignature += delta.Signature
		p.output.Content[index].ThinkingSignature = p.blocks[index].ThinkingSignature
	}
	return events
}

func (p *BedrockConverseStreamProcessor) processContentBlockStop(event BedrockContentBlockStopEvent) []AssistantMessageEvent {
	index := p.blockIndex(event.ContentBlockIndex)
	if index < 0 {
		return nil
	}
	block := p.blocks[index]
	switch block.Type {
	case ContentText:
		return []AssistantMessageEvent{{Type: "text_end", ContentIndex: index, Content: block.Text, Partial: *p.output}}
	case ContentThinking:
		return []AssistantMessageEvent{{Type: "thinking_end", ContentIndex: index, Content: block.Thinking, Partial: *p.output}}
	case ContentToolCall:
		block.Arguments = parseStreamingJSONObject(block.partialJSON)
		p.blocks[index].Arguments = block.Arguments
		p.output.Content[index].Arguments = block.Arguments
		return []AssistantMessageEvent{{Type: "toolcall_end", ContentIndex: index, ToolCall: p.output.Content[index], Partial: *p.output}}
	default:
		return nil
	}
}

func (p *BedrockConverseStreamProcessor) processMetadata(event BedrockMetadataEvent) {
	usage := event.Usage
	p.output.Usage.Input = usage.InputTokens
	p.output.Usage.Output = usage.OutputTokens
	p.output.Usage.CacheRead = usage.CacheReadInputTokens
	p.output.Usage.CacheWrite = usage.CacheWriteTokens
	p.output.Usage.TotalTokens = usage.TotalTokens
	if p.output.Usage.TotalTokens == 0 {
		p.output.Usage.TotalTokens = p.output.Usage.Input + p.output.Usage.Output
	}
	p.output.Usage.Cost = CalculateCost(p.model, p.output.Usage)
}

func (p *BedrockConverseStreamProcessor) blockIndex(contentBlockIndex int) int {
	for index, block := range p.blocks {
		if block.index == contentBlockIndex {
			return index
		}
	}
	return -1
}

func ProcessBedrockConverseStreamEvents(model Model, output *Message, events []BedrockConverseStreamEvent) []AssistantMessageEvent {
	processor := NewBedrockConverseStreamProcessor(model, output)
	var emitted []AssistantMessageEvent
	for _, event := range events {
		emitted = append(emitted, processor.Process(event)...)
	}
	return emitted
}

func mapBedrockStopReason(reason string) string {
	switch reason {
	case "end_turn", "stop_sequence":
		return StopReasonStop
	case "max_tokens", "model_context_window_exceeded":
		return StopReasonLength
	case "tool_use":
		return StopReasonToolUse
	default:
		return StopReasonError
	}
}

func formatBedrockStreamError(err error) string {
	if err == nil {
		return ""
	}
	normalized := NormalizeProviderError(err)
	core := normalized.Message
	if !normalized.MessageCarriesBody &&
		normalized.StatusCode != 0 &&
		normalized.Body != "" {
		core = fmt.Sprintf("%d: %s", normalized.StatusCode, normalized.Body)
	}

	code := ""
	var apiError smithy.APIError
	if errors.As(err, &apiError) {
		code = apiError.ErrorCode()
		if message := strings.TrimSpace(apiError.ErrorMessage()); message != "" {
			core = message
		}
	}
	prefixes := map[string]string{
		"InternalServerException":     "Internal server error",
		"ModelStreamErrorException":   "Model stream error",
		"ValidationException":         "Validation error",
		"ThrottlingException":         "Throttling error",
		"ServiceUnavailableException": "Service unavailable",
	}
	if prefix := prefixes[code]; prefix != "" {
		core = prefix + ": " + core
	} else if code != "" {
		core = code + ": " + core
	}
	if strings.Contains(strings.ToLower(core), "data retention mode") {
		core += " See https://docs.aws.amazon.com/bedrock/latest/userguide/data-retention.html for supported data retention modes."
	}
	return core
}

func metadataStringMap(metadata map[string]any, key string) map[string]string {
	value, ok := metadata[key]
	if !ok {
		return nil
	}
	raw, ok := value.(map[string]string)
	if ok {
		copy := make(map[string]string, len(raw))
		for key, value := range raw {
			copy[key] = value
		}
		return copy
	}
	generic, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	result := map[string]string{}
	for key, value := range generic {
		if text, ok := value.(string); ok {
			result[key] = text
		}
	}
	return result
}

func metadataValue(metadata map[string]any, key string) any {
	if metadata == nil {
		return nil
	}
	return metadata[key]
}
