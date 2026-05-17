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
}

type GooglePayload struct {
	Model    string               `json:"-"`
	Contents []GoogleContent      `json:"contents"`
	Tools    []GoogleToolGroup    `json:"tools,omitempty"`
	Config   GoogleGenerateConfig `json:"generationConfig,omitempty"`
}

type GoogleGenerateConfig struct {
	Temperature       *float64              `json:"temperature,omitempty"`
	MaxOutputTokens   int                   `json:"maxOutputTokens,omitempty"`
	SystemInstruction *GoogleSystemContent  `json:"systemInstruction,omitempty"`
	ThinkingConfig    *GoogleThinkingConfig `json:"thinkingConfig,omitempty"`
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
	RegisterAPIProvider("google-generative-ai", NewGoogleProvider(nil))
}

func (p GoogleProvider) Stream(model Model, llmContext Context, options StreamOptions) (*AssistantMessageEventStream, error) {
	return p.StreamSimple(model, llmContext, options)
}

func (p GoogleProvider) StreamSimple(model Model, llmContext Context, options SimpleStreamOptions) (*AssistantMessageEventStream, error) {
	apiKey := apiKeyOrEnv(model.Provider, options.APIKey)
	if apiKey == "" {
		return streamError(model, "missing API key for provider %s", model.Provider), nil
	}
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	payload := any(BuildGooglePayload(model, llmContext, GooglePayloadOptions{
		MaxTokens:       options.MaxTokens,
		Temperature:     options.Temperature,
		Reasoning:       options.Reasoning,
		ThinkingBudgets: options.ThinkingBudgets,
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
	response, err := postSSE(ctx, httpClientOrDefault(p.Client), googleStreamEndpoint(model.BaseURL, model.ID, apiKey), googleHeaders(model, options), payload)
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
	go streamGoogleBody(model, response.Body, stream)
	return stream, nil
}

func BuildGooglePayload(model Model, llmContext Context, options GooglePayloadOptions) GooglePayload {
	payload := GooglePayload{
		Model:    model.ID,
		Contents: ConvertGoogleMessages(model, llmContext),
		Tools:    ConvertGoogleTools(llmContext.Tools, false),
	}
	if options.MaxTokens > 0 {
		payload.Config.MaxOutputTokens = options.MaxTokens
	}
	if options.Temperature != nil {
		payload.Config.Temperature = options.Temperature
	}
	if llmContext.SystemPrompt != "" {
		payload.Config.SystemInstruction = &GoogleSystemContent{Parts: []GooglePart{{Text: SanitizeSurrogates(llmContext.SystemPrompt)}}}
	}
	payload.Config.ThinkingConfig = BuildGoogleThinkingConfig(model, GoogleThinkingOptions{
		Reasoning:     options.Reasoning,
		CustomBudgets: options.ThinkingBudgets,
	})
	return payload
}

func streamGoogleBody(model Model, body io.ReadCloser, stream *AssistantMessageEventStream) {
	output := AssistantMessage(nil, StopReasonStop, model)
	stream.Push(AssistantMessageEvent{Type: "start", Partial: output})
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
			terminal = true
			message := output
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
		stream.Push(AssistantMessageEvent{Type: "done", Reason: output.StopReason, Message: output})
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
	return events
}

func (p *googleStreamProcessor) appendText(part GooglePart) []AssistantMessageEvent {
	if p.currentIndex < 0 || p.currentType != ContentText {
		p.output.Content = append(p.output.Content, Text(""))
		p.currentIndex = len(p.output.Content) - 1
		p.currentType = ContentText
	}
	p.output.Content[p.currentIndex].Text += SanitizeSurrogates(part.Text)
	p.output.Content[p.currentIndex].TextSignature = RetainGoogleThoughtSignature(p.output.Content[p.currentIndex].TextSignature, part.ThoughtSignature)
	return []AssistantMessageEvent{{Type: "text_delta", Partial: *p.output}}
}

func (p *googleStreamProcessor) appendThinking(part GooglePart) []AssistantMessageEvent {
	if p.currentIndex < 0 || p.currentType != ContentThinking {
		p.output.Content = append(p.output.Content, Thinking(""))
		p.currentIndex = len(p.output.Content) - 1
		p.currentType = ContentThinking
	}
	p.output.Content[p.currentIndex].Thinking += SanitizeSurrogates(part.Text)
	p.output.Content[p.currentIndex].ThinkingSignature = RetainGoogleThoughtSignature(p.output.Content[p.currentIndex].ThinkingSignature, part.ThoughtSignature)
	return []AssistantMessageEvent{{Type: "thinking_delta", Partial: *p.output}}
}

func (p *googleStreamProcessor) appendToolCall(part GooglePart) []AssistantMessageEvent {
	call := part.FunctionCall
	id := call.ID
	if id == "" || hasContentPartID(p.output.Content, id) {
		p.toolCounter++
		id = fmt.Sprintf("%s_%d", call.Name, p.toolCounter)
	}
	toolCall := ToolCall(id, call.Name, call.Args)
	toolCall.ThoughtSignature = part.ThoughtSignature
	p.output.Content = append(p.output.Content, toolCall)
	p.currentIndex = -1
	p.currentType = ""
	return []AssistantMessageEvent{
		{Type: "toolcall_start", Partial: *p.output},
		{Type: "toolcall_delta", Partial: *p.output},
		{Type: "toolcall_end", Partial: *p.output, Message: Message{Content: []ContentPart{toolCall}}},
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
