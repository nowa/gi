package gillmprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
)

type GoogleProvider struct {
	Client HTTPDoer
}

type GooglePayloadOptions struct {
	MaxTokens       int
	Temperature     *float64
	Reasoning       string
	ThinkingBudgets map[string]int
	ToolChoice      any
}

type GooglePayload struct {
	Model             string               `json:"-"`
	Contents          []GoogleContent      `json:"contents"`
	Tools             []GoogleToolGroup    `json:"tools,omitempty"`
	ToolConfig        *GoogleToolConfig    `json:"toolConfig,omitempty"`
	SystemInstruction *GoogleSystemContent `json:"systemInstruction,omitempty"`
	Config            GoogleGenerateConfig `json:"generationConfig,omitempty"`
}

type GoogleGenerateConfig struct {
	Temperature     *float64              `json:"temperature,omitempty"`
	MaxOutputTokens int                   `json:"maxOutputTokens,omitempty"`
	ThinkingConfig  *GoogleThinkingConfig `json:"thinkingConfig,omitempty"`
}

type GoogleSystemContent struct {
	Parts []GooglePart `json:"parts"`
}

type GoogleStreamChunk struct {
	ResponseID    string               `json:"responseId"`
	Candidates    []GoogleCandidate    `json:"candidates"`
	UsageMetadata *GoogleUsageMetadata `json:"usageMetadata,omitempty"`
}

type GoogleCandidate struct {
	Content      GoogleContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type GoogleUsageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount"`
	TotalTokenCount         int `json:"totalTokenCount"`
}

type googleStreamProcessor struct {
	model        Model
	output       *Message
	currentIndex int
	currentType  string
	toolCounter  int
}

func NewGoogleProvider(client HTTPDoer) GoogleProvider {
	return GoogleProvider{Client: httpClientOrDefault(client)}
}

func init() {
	RegisterBuiltInAPIProvider("google-generative-ai", NewGoogleProvider(nil))
}

func (p GoogleProvider) Stream(model Model, llmContext Context, options StreamOptions) (*AssistantMessageEventStream, error) {
	return p.stream(model, llmContext, options)
}

func (p GoogleProvider) StreamSimple(model Model, llmContext Context, options SimpleStreamOptions) (*AssistantMessageEventStream, error) {
	return p.stream(model, llmContext, prepareSimpleStreamOptions(model, llmContext, options))
}

func (p GoogleProvider) stream(model Model, llmContext Context, options StreamOptions) (*AssistantMessageEventStream, error) {
	apiKey := apiKeyOrEnv(model.Provider, options.APIKey, options.Env)
	if apiKey == "" {
		return streamError(model, "missing API key for provider %s", model.Provider), nil
	}
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	builtPayload, err := BuildGooglePayloadChecked(model, llmContext, GooglePayloadOptions{
		MaxTokens:       options.MaxTokens,
		Temperature:     options.Temperature,
		Reasoning:       options.Reasoning,
		ThinkingBudgets: options.ThinkingBudgets,
		ToolChoice:      options.ToolChoice,
	})
	if err != nil {
		return streamError(model, "%s", err.Error()), nil
	}
	payload := any(builtPayload)
	if options.OnPayload != nil {
		next, replace, err := options.OnPayload(payload, model)
		if err != nil {
			return streamError(model, "%s", err.Error()), nil
		}
		if replace {
			payload = next
		}
	}
	response, err := postSSEWithRetry(
		ctx,
		httpClientForRequest(p.Client, options),
		googleStreamEndpoint(model.BaseURL, model.ID, apiKey),
		googleHeaders(model, options),
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
	go streamGoogleBody(model, response.Body, stream)
	return stream, nil
}

func BuildGooglePayload(model Model, llmContext Context, options GooglePayloadOptions) GooglePayload {
	payload, _ := BuildGooglePayloadChecked(model, llmContext, options)
	return payload
}

// BuildGooglePayloadChecked resolves request-wide compatibility state before
// serializing the payload. Providers should use this form so invalid strict
// sampling requirements fail before any network request is attempted.
func BuildGooglePayloadChecked(
	model Model,
	llmContext Context,
	options GooglePayloadOptions,
) (GooglePayload, error) {
	payload := GooglePayload{
		Model:    model.ID,
		Contents: ConvertGoogleMessages(model, llmContext),
		Tools:    ConvertGoogleTools(llmContext.Tools, false),
	}
	if len(llmContext.Tools) > 0 {
		mode, err := ResolveGoogleFunctionCallingMode(
			llmContext.Tools,
			options.ToolChoice,
			SupportsGoogleStrictToolSampling(model.ID),
		)
		if err != nil {
			return GooglePayload{}, err
		}
		if mode != "" {
			payload.ToolConfig = &GoogleToolConfig{
				FunctionCallingConfig: GoogleFunctionCallingConfig{Mode: mode},
			}
		}
	}
	if options.MaxTokens > 0 {
		payload.Config.MaxOutputTokens = options.MaxTokens
	}
	if options.Temperature != nil {
		payload.Config.Temperature = options.Temperature
	}
	if llmContext.SystemPrompt != "" {
		payload.SystemInstruction = &GoogleSystemContent{Parts: []GooglePart{{Text: SanitizeSurrogates(llmContext.SystemPrompt)}}}
	}
	payload.Config.ThinkingConfig = BuildGoogleThinkingConfig(model, GoogleThinkingOptions{
		Reasoning:     options.Reasoning,
		CustomBudgets: options.ThinkingBudgets,
	})
	return payload, nil
}

func streamGoogleBody(model Model, body io.ReadCloser, stream *AssistantMessageEventStream) {
	output := AssistantMessage(nil, StopReasonStop, model)
	stream.Push(AssistantMessageEvent{
		Type:    "start",
		Partial: cloneMessageState(output),
	})
	processor := newGoogleStreamProcessor(model, &output)
	terminal := false
	err := dispatchSSEUntil(body, func(data string) (bool, error) {
		chunk, err := DecodeGoogleStreamChunk([]byte(data))
		if err != nil {
			return false, err
		}
		for _, event := range processor.Process(chunk) {
			stream.Push(event)
		}
		if hasGoogleFinishReason(chunk) {
			for _, event := range processor.Finish() {
				stream.Push(event)
			}
			terminal = true
			message := cloneMessageState(output)
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
		stream.Push(AssistantMessageEvent{
			Type:    "done",
			Reason:  output.StopReason,
			Message: cloneMessageState(output),
		})
	}
}

func newGoogleStreamProcessor(model Model, output *Message) *googleStreamProcessor {
	return &googleStreamProcessor{model: model, output: output, currentIndex: -1}
}

func (p *googleStreamProcessor) Process(chunk GoogleStreamChunk) []AssistantMessageEvent {
	if chunk.ResponseID != "" && p.output.ResponseID == "" {
		p.output.ResponseID = chunk.ResponseID
	}
	if chunk.UsageMetadata != nil {
		p.output.Usage = ParseGoogleUsage(*chunk.UsageMetadata, p.model)
	}
	var events []AssistantMessageEvent
	for _, candidate := range chunk.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				if IsGoogleThinkingPart(part) {
					events = append(events, p.appendThinking(part)...)
				} else {
					events = append(events, p.appendText(part)...)
				}
			}
			if part.FunctionCall != nil {
				events = append(events, p.appendToolCall(part)...)
			}
		}
		if candidate.FinishReason != "" {
			p.output.StopReason = MapGoogleFinishReason(candidate.FinishReason)
			if hasGoogleToolCall(p.output.Content) {
				p.output.StopReason = StopReasonToolUse
			}
		}
	}
	return snapshotPartialEvents(events, *p.output)
}

func (p *googleStreamProcessor) appendText(part GooglePart) []AssistantMessageEvent {
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
	delta := SanitizeSurrogates(part.Text)
	current := &p.output.Content[p.currentIndex]
	current.Text += delta
	current.TextSignature = RetainGoogleThoughtSignature(
		current.TextSignature,
		part.ThoughtSignature,
	)
	return append(events, AssistantMessageEvent{
		Type:         "text_delta",
		ContentIndex: p.currentIndex,
		Delta:        delta,
	})
}

func (p *googleStreamProcessor) appendThinking(part GooglePart) []AssistantMessageEvent {
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
	delta := SanitizeSurrogates(part.Text)
	current := &p.output.Content[p.currentIndex]
	current.Thinking += delta
	current.ThinkingSignature = RetainGoogleThoughtSignature(
		current.ThinkingSignature,
		part.ThoughtSignature,
	)
	return append(events, AssistantMessageEvent{
		Type:         "thinking_delta",
		ContentIndex: p.currentIndex,
		Delta:        delta,
	})
}

func (p *googleStreamProcessor) appendToolCall(part GooglePart) []AssistantMessageEvent {
	events := p.closeCurrent()
	call := part.FunctionCall
	id := call.ID
	if id == "" || hasContentPartID(p.output.Content, id) {
		p.toolCounter++
		id = fmt.Sprintf("%s_%d", call.Name, p.toolCounter)
	}
	toolCall := ToolCall(id, call.Name, call.Args)
	toolCall.ThoughtSignature = part.ThoughtSignature
	p.output.Content = append(p.output.Content, toolCall)
	index := len(p.output.Content) - 1
	p.currentIndex = -1
	p.currentType = ""
	arguments, _ := json.Marshal(toolCall.Arguments)
	return append(events,
		AssistantMessageEvent{
			Type:         "toolcall_start",
			ContentIndex: index,
		},
		AssistantMessageEvent{
			Type:         "toolcall_delta",
			ContentIndex: index,
			Delta:        string(arguments),
		},
		AssistantMessageEvent{
			Type:         "toolcall_end",
			ContentIndex: index,
			ToolCall:     cloneContentPartState(toolCall),
		},
	)
}

// Finish closes the active text or thinking block exactly once.
func (p *googleStreamProcessor) Finish() []AssistantMessageEvent {
	return snapshotPartialEvents(p.closeCurrent(), *p.output)
}

func (p *googleStreamProcessor) closeCurrent() []AssistantMessageEvent {
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

func DecodeGoogleStreamChunk(data []byte) (GoogleStreamChunk, error) {
	var chunk GoogleStreamChunk
	if err := json.Unmarshal(data, &chunk); err != nil {
		return GoogleStreamChunk{}, fmt.Errorf("decode Google stream chunk: %w", err)
	}
	return chunk, nil
}

func ParseGoogleUsage(raw GoogleUsageMetadata, model Model) Usage {
	input := raw.PromptTokenCount - raw.CachedContentTokenCount
	if input < 0 {
		input = 0
	}
	output := raw.CandidatesTokenCount + raw.ThoughtsTokenCount
	total := raw.TotalTokenCount
	if total == 0 {
		total = input + output + raw.CachedContentTokenCount
	}
	usage := Usage{
		Input:       input,
		Output:      output,
		CacheRead:   raw.CachedContentTokenCount,
		Reasoning:   ptrInt(raw.ThoughtsTokenCount),
		TotalTokens: total,
	}
	usage.Cost = CalculateCost(model, usage)
	return usage
}

func MapGoogleFinishReason(reason string) string {
	switch reason {
	case "", "STOP":
		return StopReasonStop
	case "MAX_TOKENS":
		return StopReasonLength
	default:
		return StopReasonError
	}
}

func hasGoogleFinishReason(chunk GoogleStreamChunk) bool {
	for _, candidate := range chunk.Candidates {
		if candidate.FinishReason != "" {
			return true
		}
	}
	return false
}

func hasGoogleToolCall(content []ContentPart) bool {
	for _, part := range content {
		if part.Type == ContentToolCall {
			return true
		}
	}
	return false
}

func hasContentPartID(content []ContentPart, id string) bool {
	for _, part := range content {
		if part.ID == id {
			return true
		}
	}
	return false
}

func googleHeaders(model Model, options SimpleStreamOptions) map[string]string {
	headers := map[string]string{}
	for key, value := range model.Headers {
		headers[key] = value
	}
	for key, value := range options.Headers {
		headers[key] = value
	}
	return headers
}

func googleStreamEndpoint(baseURL, modelID, apiKey string) string {
	raw := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if raw == "" {
		raw = "https://generativelanguage.googleapis.com/v1beta"
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/models/" + url.PathEscape(modelID) + ":streamGenerateContent"
	query := parsed.Query()
	query.Set("alt", "sse")
	if apiKey != "" {
		query.Set("key", apiKey)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
