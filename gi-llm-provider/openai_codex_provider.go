package gillmprovider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultOpenAICodexMaxRetries = 3

type OpenAICodexResponsesProvider struct {
	Client HTTPDoer
	Now    func() time.Time
	Sleep  func(context.Context, time.Duration) error
}

func NewOpenAICodexResponsesProvider(client HTTPDoer) OpenAICodexResponsesProvider {
	return OpenAICodexResponsesProvider{
		Client: httpClientOrDefault(client),
		Now:    time.Now,
		Sleep:  sleepContext,
	}
}

func init() {
	RegisterAPIProvider("openai-codex-responses", NewOpenAICodexResponsesProvider(nil))
}

func (p OpenAICodexResponsesProvider) Stream(model Model, llmContext Context, options StreamOptions) (*AssistantMessageEventStream, error) {
	return p.StreamSimple(model, llmContext, options)
}

func (p OpenAICodexResponsesProvider) StreamSimple(model Model, llmContext Context, options SimpleStreamOptions) (*AssistantMessageEventStream, error) {
	if strings.EqualFold(options.Transport, "websocket") {
		return streamError(model, "openai codex websocket transport is not implemented"), nil
	}
	apiKey := apiKeyOrEnv(model.Provider, options.APIKey)
	if apiKey == "" {
		return streamError(model, "missing API key for provider %s", model.Provider), nil
	}
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	payload, headers, err := p.buildRequest(model, llmContext, options, apiKey)
	if err != nil {
		return streamError(model, "%s", err.Error()), nil
	}
	response, err := p.postWithRetry(ctx, model, options, headers, payload)
	if err != nil {
		if ctx.Err() != nil {
			return ErrorAssistantStream(AssistantErrorMessage(ctx.Err().Error(), model, true)), nil
		}
		return streamError(model, "%s", err.Error()), nil
	}

	stream := NewAssistantMessageEventStream()
	serviceTier := metadataString(options.Metadata, "service_tier")
	go streamOpenAICodexResponsesBody(model, response.Body, stream, serviceTier)
	return stream, nil
}

func (p OpenAICodexResponsesProvider) buildRequest(model Model, llmContext Context, options SimpleStreamOptions, apiKey string) (any, map[string]string, error) {
	reasoning := ""
	if options.Reasoning != "" {
		reasoning = ClampThinkingLevel(model, options.Reasoning)
		if reasoning == "off" {
			reasoning = ""
		}
	}
	payload := any(BuildOpenAICodexResponsesPayload(model, llmContext, OpenAICodexResponsesPayloadOptions{
		Temperature:      options.Temperature,
		SessionID:        options.SessionID,
		ReasoningEffort:  reasoning,
		ReasoningSummary: metadataString(options.Metadata, "reasoning_summary"),
		ServiceTier:      metadataString(options.Metadata, "service_tier"),
		TextVerbosity:    metadataString(options.Metadata, "text_verbosity"),
	}))
	if options.OnPayload != nil {
		next, replace, err := options.OnPayload(payload, model)
		if err != nil {
			return nil, nil, err
		}
		if replace {
			payload = next
		}
	}
	headers, err := BuildOpenAICodexSSEHeaders(model.Headers, options.Headers, apiKey, options.SessionID)
	if err != nil {
		return nil, nil, err
	}
	return payload, headers, nil
}

func (p OpenAICodexResponsesProvider) postWithRetry(ctx context.Context, model Model, options SimpleStreamOptions, headers map[string]string, payload any) (*http.Response, error) {
	client := httpClientOrDefault(p.Client)
	maxRetries := options.MaxRetries
	if maxRetries == 0 {
		maxRetries = defaultOpenAICodexMaxRetries
	}
	if maxRetries < 0 {
		maxRetries = 0
	}
	endpoint := ResolveOpenAICodexURL(model.BaseURL)
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		response, err := postSSE(ctx, client, endpoint, headers, payload)
		if err != nil {
			lastErr = err
			if attempt < maxRetries {
				if err := p.wait(ctx, OpenAICodexRetryDelay(0, nil, attempt, p.now())); err != nil {
					return nil, err
				}
				continue
			}
			return nil, fmt.Errorf("request failed: %w", err)
		}
		headerMap := responseHeaders(response.Header)
		if options.OnResponseStatus != nil {
			if err := options.OnResponseStatus(response.StatusCode, headerMap, model); err != nil {
				response.Body.Close()
				return nil, err
			}
		}
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return response, nil
		}
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		response.Body.Close()
		bodyText := strings.TrimSpace(string(body))
		if attempt < maxRetries && IsOpenAICodexRetryable(response.StatusCode, bodyText) {
			if err := p.wait(ctx, OpenAICodexRetryDelay(response.StatusCode, headerMap, attempt, p.now())); err != nil {
				return nil, err
			}
			continue
		}
		if bodyText == "" {
			bodyText = response.Status
		}
		lastErr = fmt.Errorf("provider returned HTTP %d: %s", response.StatusCode, bodyText)
		break
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("request failed after retries")
	}
	return nil, lastErr
}

func (p OpenAICodexResponsesProvider) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p OpenAICodexResponsesProvider) wait(ctx context.Context, delay time.Duration) error {
	if p.Sleep != nil {
		return p.Sleep(ctx, delay)
	}
	return sleepContext(ctx, delay)
}

func streamOpenAICodexResponsesBody(model Model, body io.ReadCloser, stream *AssistantMessageEventStream, requestServiceTier string) {
	output := AssistantMessage(nil, StopReasonStop, model)
	stream.Push(AssistantMessageEvent{Type: "start", Partial: output})
	processor := NewOpenAIResponsesStreamProcessor(model, &output)
	terminal := false
	err := dispatchSSEUntil(body, func(data string) (bool, error) {
		event, err := DecodeOpenAIResponsesSSEEvent([]byte(data))
		if err != nil {
			return false, err
		}
		normalized, ok := normalizeOpenAICodexEvent(event)
		if !ok {
			if event.Type == "error" {
				return false, fmt.Errorf("codex error: %s", event.Error)
			}
			return false, nil
		}
		emitted := processor.Process(normalized)
		if normalized.Type == "response.completed" && normalized.Response != nil {
			tier := ResolveOpenAICodexServiceTier(normalized.Response.ServiceTier, requestServiceTier)
			ApplyOpenAICodexServiceTierPricing(&output.Usage, tier, model)
			for i := range emitted {
				emitted[i].Reason = output.StopReason
				if emitted[i].Type == "error" {
					emitted[i].Error = output
				} else {
					emitted[i].Message = output
				}
				emitted[i].Partial = output
			}
		}
		for _, event := range emitted {
			stream.Push(event)
			if event.Type == "done" || event.Type == "error" {
				terminal = true
				return true, nil
			}
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

func metadataString(metadata map[string]any, key string) string {
	value, ok := metadata[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return text
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
