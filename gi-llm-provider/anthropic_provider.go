package gillmprovider

import (
	"context"
	"io"
	"strings"
)

type AnthropicMessagesProvider struct {
	Client HTTPDoer
}

func NewAnthropicMessagesProvider(client HTTPDoer) AnthropicMessagesProvider {
	return AnthropicMessagesProvider{Client: httpClientOrDefault(client)}
}

func init() {
	RegisterBuiltInAPIProvider("anthropic-messages", NewAnthropicMessagesProvider(nil))
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
	isOAuthToken := model.Provider == "anthropic" && isAnthropicOAuthToken(apiKey)
	payloadOptions := AnthropicPayloadOptions{
		MaxTokens:       options.MaxTokens,
		Temperature:     options.Temperature,
		CacheRetention:  options.CacheRetention,
		SessionID:       options.SessionID,
		Reasoning:       options.Reasoning,
		ThinkingBudgets: options.ThinkingBudgets,
		Metadata:        options.Metadata,
		Headers:         options.Headers,
		IsOAuthToken:    isOAuthToken,
	}
	payload := BuildAnthropicPayload(model, llmContext, payloadOptions)
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
	headers := BuildAnthropicRequestHeaders(model, llmContext, payloadOptions)
	if model.Provider == "github-copilot" {
		headers["Authorization"] = "Bearer " + apiKey
	} else if isOAuthToken {
		headers["Authorization"] = "Bearer " + apiKey
		headers["anthropic-version"] = "2023-06-01"
		applyAnthropicOAuthHeaders(headers)
	} else {
		headers["x-api-key"] = apiKey
		headers["anthropic-version"] = "2023-06-01"
	}

	baseURL, err := ResolveCloudflareBaseURL(model)
	if err != nil {
		return streamError(model, "%s", err.Error()), nil
	}
	response, err := postSSE(ctx, httpClientOrDefault(p.Client), anthropicMessagesEndpoint(baseURL), headers, payloadAny)
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
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	if strings.HasSuffix(baseURL, "/messages") {
		return baseURL
	}
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL + "/messages"
	}
	if baseURL == "https://api.anthropic.com" {
		return baseURL + "/v1/messages"
	}
	return baseURL + "/messages"
}

func isAnthropicOAuthToken(apiKey string) bool {
	return strings.Contains(apiKey, "sk-ant-oat")
}

func applyAnthropicOAuthHeaders(headers map[string]string) {
	delete(headers, "x-api-key")
	headers["anthropic-beta"] = mergeAnthropicBeta(headers["anthropic-beta"], "claude-code-20250219", "oauth-2025-04-20")
	headers["user-agent"] = "claude-cli/2.1.75"
	headers["x-app"] = "cli"
}

func mergeAnthropicBeta(existing string, required ...string) string {
	seen := map[string]bool{}
	var result []string
	for _, value := range required {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	for _, value := range strings.Split(existing, ",") {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return strings.Join(result, ",")
}
