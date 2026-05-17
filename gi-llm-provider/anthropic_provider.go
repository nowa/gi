package gillmprovider

import (
	"context"
	"io"
)

type AnthropicMessagesProvider struct {
	Client HTTPDoer
}

func NewAnthropicMessagesProvider(client HTTPDoer) AnthropicMessagesProvider {
	return AnthropicMessagesProvider{Client: httpClientOrDefault(client)}
}

func init() {
	RegisterAPIProvider("anthropic-messages", NewAnthropicMessagesProvider(nil))
}

func (p AnthropicMessagesProvider) Stream(model Model, llmContext Context, options StreamOptions) (*AssistantMessageEventStream, error) {
	return p.StreamSimple(model, llmContext, options)
}

func (p AnthropicMessagesProvider) StreamSimple(model Model, llmContext Context, options SimpleStreamOptions) (*AssistantMessageEventStream, error) {
	apiKey := apiKeyOrEnv(model.Provider, options.APIKey)
	if apiKey == "" {
		return streamError(model, "missing API key for provider %s", model.Provider), nil
	}
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	payloadOptions := AnthropicPayloadOptions{
		MaxTokens:       options.MaxTokens,
		Temperature:     options.Temperature,
		CacheRetention:  options.CacheRetention,
		SessionID:       options.SessionID,
		Reasoning:       options.Reasoning,
		ThinkingBudgets: options.ThinkingBudgets,
		Metadata:        options.Metadata,
	}
	payload := BuildAnthropicPayload(model, llmContext, payloadOptions)
	headers := BuildAnthropicRequestHeaders(model, llmContext, payloadOptions)
	if model.Provider == "github-copilot" {
		headers["Authorization"] = "Bearer " + apiKey
	} else {
		headers["x-api-key"] = apiKey
		headers["anthropic-version"] = "2023-06-01"
	}

	response, err := postSSE(ctx, httpClientOrDefault(p.Client), anthropicMessagesEndpoint(model.BaseURL), headers, payload)
	if err != nil {
		return streamError(model, "request failed: %v", err), nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return responseErrorStream(model, response), nil
	}

	stream := NewAssistantMessageEventStream()
	go streamAnthropicMessagesBody(model, response.Body, stream)
	return stream, nil
}

func streamAnthropicMessagesBody(model Model, body io.ReadCloser, stream *AssistantMessageEventStream) {
	output := AssistantMessage(nil, StopReasonStop, model)
	stream.Push(AssistantMessageEvent{Type: "start", Partial: output})
	var events []AnthropicSSEEvent
	terminal := false
	err := dispatchNamedSSE(body, func(eventName, data string) error {
		events = append(events, AnthropicSSEEvent{Event: eventName, Data: data})
		if eventName == "message_stop" {
			message, err := ProcessAnthropicSSEEvents(model, events)
			if err != nil {
				return err
			}
			terminal = true
			if message.StopReason == StopReasonError {
				stream.Push(AssistantMessageEvent{Type: "error", Reason: message.StopReason, Error: message})
			} else {
				stream.Push(AssistantMessageEvent{Type: "done", Reason: message.StopReason, Message: message})
			}
		}
		return nil
	})
	if err != nil {
		stream.Push(AssistantMessageEvent{Type: "error", Reason: StopReasonError, Error: AssistantErrorMessage(err.Error(), model, false)})
		return
	}
	if !terminal {
		message, err := ProcessAnthropicSSEEvents(model, events)
		if err != nil {
			stream.Push(AssistantMessageEvent{Type: "error", Reason: StopReasonError, Error: AssistantErrorMessage(err.Error(), model, false)})
			return
		}
		stream.Push(AssistantMessageEvent{Type: "done", Reason: message.StopReason, Message: message})
	}
}

func anthropicMessagesEndpoint(baseURL string) string {
	return appendEndpoint(baseURL, "https://api.anthropic.com/v1", "/messages")
}
