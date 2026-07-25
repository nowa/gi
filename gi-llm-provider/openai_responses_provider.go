package gillmprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

type OpenAIResponsesProvider struct {
	Client HTTPDoer
}

func NewOpenAIResponsesProvider(client HTTPDoer) OpenAIResponsesProvider {
	return OpenAIResponsesProvider{Client: httpClientOrDefault(client)}
}

func init() {
	RegisterBuiltInAPIProvider("openai-responses", NewOpenAIResponsesProvider(nil))
}

func (p OpenAIResponsesProvider) Stream(model Model, llmContext Context, options StreamOptions) (*AssistantMessageEventStream, error) {
	return p.stream(model, llmContext, options)
}

func (p OpenAIResponsesProvider) StreamSimple(model Model, llmContext Context, options SimpleStreamOptions) (*AssistantMessageEventStream, error) {
	return p.stream(model, llmContext, prepareSimpleStreamOptions(model, llmContext, options))
}

func (p OpenAIResponsesProvider) stream(model Model, llmContext Context, options StreamOptions) (*AssistantMessageEventStream, error) {
	auth := resolveProviderRequestAuth(
		model.Provider,
		options.APIKey,
		options.Env,
		options.Headers,
		"authorization",
		"cf-aig-authorization",
	)
	if !auth.Configured() {
		return streamError(model, "missing API key for provider %s", model.Provider), nil
	}
	apiKey := auth.APIKey
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	options.CacheRetention = resolveCacheRetentionWithEnv(options.CacheRetention, options.Env)
	reasoning := ""
	if options.Reasoning != "" {
		reasoning = ClampThinkingLevel(model, options.Reasoning)
		if reasoning == "off" {
			reasoning = ""
		}
	}
	payload, sampling, err := buildOpenAIResponsesPayloadChecked(model, llmContext, OpenAIResponsesPayloadOptions{
		Temperature:      options.Temperature,
		MaxTokens:        options.MaxTokens,
		CacheRetention:   options.CacheRetention,
		SessionID:        options.SessionID,
		ReasoningEffort:  reasoning,
		ReasoningSummary: "",
	})
	if err != nil {
		return streamError(model, "%s", err.Error()), nil
	}
	payloadAny := any(payload)
	if options.OnPayload != nil {
		next, replace, err := options.OnPayload(payloadAny, model)
		if err != nil {
			return streamError(model, "%s", err.Error()), nil
		}
		if replace {
			payloadAny = next
		}
	}
	headers := BuildOpenAIResponsesHeaders(model, OpenAIResponsesPayloadOptions{
		SessionID:      options.SessionID,
		CacheRetention: options.CacheRetention,
		Headers:        options.Headers,
	})
	if apiKey != "" {
		setHeaderCaseInsensitive(headers, "Authorization", "Bearer "+apiKey)
	}
	headers = applyHeaderRemovals(headers, options.HeaderRemovals)

	baseURL := model.BaseURL
	if IsCloudflareProvider(model.Provider) {
		var err error
		baseURL, err = ResolveCloudflareBaseURL(model)
		if err != nil {
			return streamError(model, "%s", err.Error()), nil
		}
	}
	response, err := postSSEWithRetry(
		ctx,
		httpClientForRequest(p.Client, options),
		responsesEndpoint(baseURL),
		headers,
		payloadAny,
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
		return streamProviderRequestError(model, err, "OpenAI API error"), nil
	}

	stream := NewAssistantMessageEventStream()
	go streamOpenAIResponsesBody(model, response.Body, stream, sampling.GrammarToolInputProperties)
	return stream, nil
}

func streamOpenAIResponsesBody(
	model Model,
	body io.ReadCloser,
	stream *AssistantMessageEventStream,
	grammarToolInputProperties map[string]string,
) {
	output := AssistantMessage(nil, StopReasonStop, model)
	stream.Push(AssistantMessageEvent{Type: "start", Partial: output})
	processor := NewOpenAIResponsesStreamProcessorWithOptions(
		model,
		&output,
		OpenAIResponsesStreamProcessorOptions{
			GrammarToolInputProperties: grammarToolInputProperties,
		},
	)
	terminal := false
	err := dispatchSSEUntil(body, func(data string) (bool, error) {
		event, err := DecodeOpenAIResponsesSSEEvent([]byte(data))
		if err != nil {
			return false, err
		}
		for _, emitted := range processor.Process(event) {
			if emitted.Type == "done" || emitted.Type == "error" {
				terminal = true
				stream.Push(emitted)
				return true, nil
			}
			stream.Push(emitted)
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

func DecodeOpenAIResponsesSSEEvent(data []byte) (OpenAIResponsesStreamEvent, error) {
	var raw openAIResponsesRawEvent
	if err := json.Unmarshal(data, &raw); err != nil {
		return OpenAIResponsesStreamEvent{}, fmt.Errorf("decode OpenAI Responses SSE event: %w", err)
	}
	event := OpenAIResponsesStreamEvent{
		Type:        raw.Type,
		OutputIndex: raw.OutputIndex,
		Delta:       raw.Delta,
		Arguments:   raw.Arguments,
		Input:       raw.Input,
	}
	if raw.Error != nil {
		event.Error = raw.Error.Message
	}
	if raw.Response != nil {
		event.Response = &OpenAIResponsesResponseEvent{
			ID:          raw.Response.ID,
			Status:      raw.Response.Status,
			ServiceTier: raw.Response.ServiceTier,
		}
		if raw.Response.Usage != nil {
			event.Response.Usage = &OpenAIResponsesUsage{
				InputTokens:  raw.Response.Usage.InputTokens,
				OutputTokens: raw.Response.Usage.OutputTokens,
				TotalTokens:  raw.Response.Usage.TotalTokens,
				InputTokensDetails: OpenAIResponsesInputTokenDetails{
					CachedTokens:     raw.Response.Usage.InputTokensDetails.CachedTokens,
					CacheWriteTokens: raw.Response.Usage.InputTokensDetails.CacheWriteTokens,
				},
				OutputTokensDetails: OpenAIResponsesOutputTokenDetails{
					ReasoningTokens: raw.Response.Usage.OutputTokensDetails.ReasoningTokens,
				},
			}
		}
		if raw.Response.IncompleteDetails != nil {
			event.Response.IncompleteDetails = &OpenAIResponsesIncompleteDetails{Reason: raw.Response.IncompleteDetails.Reason}
		}
	}
	if raw.Item != nil {
		item := &OpenAIResponsesOutputItem{
			Type:             raw.Item.Type,
			ID:               raw.Item.ID,
			CallID:           raw.Item.CallID,
			Name:             raw.Item.Name,
			Arguments:        raw.Item.Arguments,
			Input:            raw.Item.Input,
			Status:           raw.Item.Status,
			Phase:            raw.Item.Phase,
			EncryptedContent: raw.Item.EncryptedContent,
		}
		for _, part := range raw.Item.Content {
			item.Content = append(item.Content, OpenAIResponsesOutputContentPart{Type: part.Type, Text: part.Text, Refusal: part.Refusal})
		}
		for _, part := range raw.Item.Summary {
			item.Summary = append(item.Summary, OpenAIResponsesOutputContentPart{Type: part.Type, Text: part.Text, Refusal: part.Refusal})
		}
		event.Item = item
	}
	if raw.Part != nil {
		event.Part = &OpenAIResponsesOutputContentPart{Type: raw.Part.Type, Text: raw.Part.Text, Refusal: raw.Part.Refusal}
	}
	return event, nil
}

type openAIResponsesRawEvent struct {
	Type        string                         `json:"type"`
	OutputIndex int                            `json:"output_index"`
	Response    *openAIResponsesRawResponse    `json:"response"`
	Item        *openAIResponsesRawOutputItem  `json:"item"`
	Part        *openAIResponsesRawContentPart `json:"part"`
	Delta       string                         `json:"delta"`
	Arguments   string                         `json:"arguments"`
	Input       string                         `json:"input"`
	Error       *openAIResponsesRawError       `json:"error"`
}

type openAIResponsesRawError struct {
	Message string `json:"message"`
}

type openAIResponsesRawResponse struct {
	ID                string                               `json:"id"`
	Status            string                               `json:"status"`
	ServiceTier       string                               `json:"service_tier"`
	Usage             *openAIResponsesRawUsage             `json:"usage"`
	IncompleteDetails *openAIResponsesRawIncompleteDetails `json:"incomplete_details"`
}

type openAIResponsesRawUsage struct {
	InputTokens         int                                   `json:"input_tokens"`
	OutputTokens        int                                   `json:"output_tokens"`
	TotalTokens         int                                   `json:"total_tokens"`
	InputTokensDetails  openAIResponsesRawInputTokensDetails  `json:"input_tokens_details"`
	OutputTokensDetails openAIResponsesRawOutputTokensDetails `json:"output_tokens_details"`
}

type openAIResponsesRawInputTokensDetails struct {
	CachedTokens     int `json:"cached_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
}

type openAIResponsesRawOutputTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

type openAIResponsesRawIncompleteDetails struct {
	Reason string `json:"reason"`
}

type openAIResponsesRawOutputItem struct {
	Type             string                          `json:"type"`
	ID               string                          `json:"id"`
	CallID           string                          `json:"call_id"`
	Name             string                          `json:"name"`
	Arguments        string                          `json:"arguments"`
	Input            string                          `json:"input"`
	Status           string                          `json:"status"`
	Content          []openAIResponsesRawContentPart `json:"content"`
	Summary          []openAIResponsesRawContentPart `json:"summary"`
	Phase            string                          `json:"phase"`
	EncryptedContent string                          `json:"encrypted_content"`
}

type openAIResponsesRawContentPart struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Refusal string `json:"refusal"`
}
