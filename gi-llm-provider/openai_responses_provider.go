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
	RegisterAPIProvider("openai-responses", NewOpenAIResponsesProvider(nil))
}

func (p OpenAIResponsesProvider) Stream(model Model, llmContext Context, options StreamOptions) (*AssistantMessageEventStream, error) {
	return p.StreamSimple(model, llmContext, options)
}

func (p OpenAIResponsesProvider) StreamSimple(model Model, llmContext Context, options SimpleStreamOptions) (*AssistantMessageEventStream, error) {
	apiKey := apiKeyOrEnv(model.Provider, options.APIKey)
	if apiKey == "" {
		return streamError(model, "missing API key for provider %s", model.Provider), nil
	}
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	payload, err := BuildOpenAIResponsesPayloadChecked(model, llmContext, OpenAIResponsesPayloadOptions{
		Temperature:      options.Temperature,
		MaxTokens:        options.MaxTokens,
		CacheRetention:   options.CacheRetention,
		SessionID:        options.SessionID,
		ReasoningEffort:  options.Reasoning,
		ReasoningSummary: "",
	})
	if err != nil {
		return streamError(model, "%s", err.Error()), nil
	}
	headers := BuildOpenAIResponsesHeaders(model, OpenAIResponsesPayloadOptions{
		SessionID:      options.SessionID,
		CacheRetention: options.CacheRetention,
		Headers:        options.Headers,
	})
	headers["Authorization"] = "Bearer " + apiKey

	response, err := postSSE(ctx, httpClientOrDefault(p.Client), responsesEndpoint(model.BaseURL), headers, payload)
	if err != nil {
		return streamError(model, "request failed: %v", err), nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return responseErrorStream(model, response), nil
	}

	stream := NewAssistantMessageEventStream()
	go streamOpenAIResponsesBody(model, response.Body, stream)
	return stream, nil
}

func streamOpenAIResponsesBody(model Model, body io.ReadCloser, stream *AssistantMessageEventStream) {
	output := AssistantMessage(nil, StopReasonStop, model)
	stream.Push(AssistantMessageEvent{Type: "start", Partial: output})
	processor := NewOpenAIResponsesStreamProcessor(model, &output)
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
		Type:      raw.Type,
		Delta:     raw.Delta,
		Arguments: raw.Arguments,
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
			}
		}
		if raw.Response.IncompleteDetails != nil {
			event.Response.IncompleteDetails = &OpenAIResponsesIncompleteDetails{Reason: raw.Response.IncompleteDetails.Reason}
		}
	}
	if raw.Item != nil {
		item := &OpenAIResponsesOutputItem{
			Type:      raw.Item.Type,
			ID:        raw.Item.ID,
			CallID:    raw.Item.CallID,
			Name:      raw.Item.Name,
			Arguments: raw.Item.Arguments,
			Status:    raw.Item.Status,
			Phase:     raw.Item.Phase,
		}
		for _, part := range raw.Item.Content {
			item.Content = append(item.Content, OpenAIResponsesOutputContentPart{Type: part.Type, Text: part.Text, Refusal: part.Refusal})
		}
		event.Item = item
	}
	if raw.Part != nil {
		event.Part = &OpenAIResponsesOutputContentPart{Type: raw.Part.Type, Text: raw.Part.Text, Refusal: raw.Part.Refusal}
	}
	return event, nil
}

type openAIResponsesRawEvent struct {
	Type      string                         `json:"type"`
	Response  *openAIResponsesRawResponse    `json:"response"`
	Item      *openAIResponsesRawOutputItem  `json:"item"`
	Part      *openAIResponsesRawContentPart `json:"part"`
	Delta     string                         `json:"delta"`
	Arguments string                         `json:"arguments"`
	Error     *openAIResponsesRawError       `json:"error"`
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
	InputTokens        int                                  `json:"input_tokens"`
	OutputTokens       int                                  `json:"output_tokens"`
	TotalTokens        int                                  `json:"total_tokens"`
	InputTokensDetails openAIResponsesRawInputTokensDetails `json:"input_tokens_details"`
}

type openAIResponsesRawInputTokensDetails struct {
	CachedTokens     int `json:"cached_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
}

type openAIResponsesRawIncompleteDetails struct {
	Reason string `json:"reason"`
}

type openAIResponsesRawOutputItem struct {
	Type      string                          `json:"type"`
	ID        string                          `json:"id"`
	CallID    string                          `json:"call_id"`
	Name      string                          `json:"name"`
	Arguments string                          `json:"arguments"`
	Status    string                          `json:"status"`
	Content   []openAIResponsesRawContentPart `json:"content"`
	Phase     string                          `json:"phase"`
}

type openAIResponsesRawContentPart struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Refusal string `json:"refusal"`
}
