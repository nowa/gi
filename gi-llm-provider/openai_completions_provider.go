package gillmprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type OpenAICompletionsProvider struct {
	Client HTTPDoer
}

func NewOpenAICompletionsProvider(client HTTPDoer) OpenAICompletionsProvider {
	return OpenAICompletionsProvider{Client: httpClientOrDefault(client)}
}

func init() {
	RegisterBuiltInAPIProvider("openai-completions", NewOpenAICompletionsProvider(nil))
}

func (p OpenAICompletionsProvider) Stream(model Model, llmContext Context, options StreamOptions) (*AssistantMessageEventStream, error) {
	return p.StreamSimple(model, llmContext, options)
}

func (p OpenAICompletionsProvider) StreamSimple(model Model, llmContext Context, options SimpleStreamOptions) (*AssistantMessageEventStream, error) {
	apiKey := apiKeyOrEnv(model.Provider, options.APIKey)
	if apiKey == "" && !hasCloudflareAIGatewayAuthorization(model, options.Headers) {
		return streamError(model, "missing API key for provider %s", model.Provider), nil
	}
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	reasoning := ""
	if options.Reasoning != "" {
		reasoning = ClampThinkingLevel(model, options.Reasoning)
	}
	payload, requestState, err := buildOpenAICompletionsPayloadChecked(model, llmContext, OpenAICompletionsPayloadOptions{
		MaxTokens:      options.MaxTokens,
		Temperature:    options.Temperature,
		CacheRetention: options.CacheRetention,
		SessionID:      options.SessionID,
		Reasoning:      reasoning,
		Headers:        options.Headers,
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
	headers := BuildOpenAICompletionsHeaders(model, OpenAICompletionsPayloadOptions{
		CacheRetention: options.CacheRetention,
		SessionID:      options.SessionID,
		Headers:        options.Headers,
	})
	if apiKey != "" {
		headers["Authorization"] = "Bearer " + apiKey
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
		httpClientOrDefault(p.Client),
		chatCompletionsEndpoint(baseURL),
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
		return streamProviderRequestError(model, err), nil
	}
	stream := NewAssistantMessageEventStream()
	go streamOpenAICompletionsBody(
		model,
		response.Body,
		stream,
		requestState.GrammarToolInputProperties,
	)
	return stream, nil
}

func streamOpenAICompletionsBody(
	model Model,
	body io.ReadCloser,
	stream *AssistantMessageEventStream,
	grammarToolInputProperties map[string]string,
) {
	output := AssistantMessage(nil, StopReasonStop, model)
	stream.Push(AssistantMessageEvent{Type: "start", Partial: output})
	processor := NewOpenAICompletionsStreamProcessorWithOptions(
		model,
		&output,
		OpenAICompletionsStreamProcessorOptions{
			GrammarToolInputProperties: grammarToolInputProperties,
		},
	)
	terminal := false
	err := dispatchSSEUntil(body, func(data string) (bool, error) {
		chunk, err := DecodeOpenAIChatCompletionChunk([]byte(data))
		if err != nil {
			return false, err
		}
		for _, emitted := range processor.Process(&chunk) {
			stream.Push(emitted)
		}
		if hasOpenAIChatFinishReason(chunk) {
			message := output
			terminal = true
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

func DecodeOpenAIChatCompletionChunk(data []byte) (OpenAIChatCompletionChunk, error) {
	var chunk OpenAIChatCompletionChunk
	if err := json.Unmarshal(data, &chunk); err != nil {
		return OpenAIChatCompletionChunk{}, fmt.Errorf("decode OpenAI chat completion chunk: %w", err)
	}
	return chunk, nil
}

func hasOpenAIChatFinishReason(chunk OpenAIChatCompletionChunk) bool {
	for _, choice := range chunk.Choices {
		if choice.FinishReason != nil {
			return true
		}
	}
	return false
}

func chatCompletionsEndpoint(baseURL string) string {
	return appendEndpoint(strings.TrimSpace(baseURL), "https://api.openai.com/v1", "/chat/completions")
}
