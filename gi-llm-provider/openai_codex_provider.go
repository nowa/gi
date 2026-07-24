package gillmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAICodexResponsesProvider struct {
	Client          HTTPDoer
	WebSocketDialer OpenAICodexWebSocketDialer
	Now             func() time.Time
	Sleep           func(context.Context, time.Duration) error
}

func NewOpenAICodexResponsesProvider(client HTTPDoer) OpenAICodexResponsesProvider {
	return OpenAICodexResponsesProvider{
		Client:          httpClientOrDefault(client),
		WebSocketDialer: gorillaOpenAICodexWebSocketDialer{},
		Now:             time.Now,
		Sleep:           sleepContext,
	}
}

type openAICodexPreparedRequest struct {
	payload          any
	sseHeaders       map[string]string
	webSocketHeaders map[string]string
	sampling         OpenAIResponsesSamplingState
	cacheSessionID   string
	codexSessionID   string
}

func init() {
	RegisterBuiltInAPIProvider("openai-codex-responses", NewOpenAICodexResponsesProvider(nil))
	RegisterSessionResourceCleanup(func(sessionID string) error {
		CloseOpenAICodexWebSocketSessions(sessionID)
		return nil
	})
}

func (p OpenAICodexResponsesProvider) Stream(model Model, llmContext Context, options StreamOptions) (*AssistantMessageEventStream, error) {
	return p.StreamSimple(model, llmContext, options)
}

func (p OpenAICodexResponsesProvider) StreamSimple(model Model, llmContext Context, options SimpleStreamOptions) (*AssistantMessageEventStream, error) {
	apiKey := apiKeyOrEnv(model.Provider, options.APIKey)
	if apiKey == "" {
		return streamError(model, "missing API key for provider %s", model.Provider), nil
	}
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	execution, err := prepareOpenAICodexExecutionOptions(options)
	if err != nil {
		return streamError(model, "%s", err.Error()), nil
	}
	request, err := p.buildRequest(model, llmContext, options, apiKey)
	if err != nil {
		return streamError(model, "%s", err.Error()), nil
	}
	if execution.transport != "sse" {
		if isOpenAICodexWebSocketSSEFallbackActive(request.cacheSessionID) {
			recordOpenAICodexWebSocketSSEFallback(request.cacheSessionID)
		} else {
			body, err := newOpenAICodexWebSocketRequestBody(request.payload)
			if err != nil {
				return streamError(model, "%s", err.Error()), nil
			}
			stream := NewAssistantMessageEventStream()
			go p.streamOpenAICodexWebSocketWithFallback(
				ctx,
				model,
				options,
				request,
				body,
				stream,
				execution,
			)
			return stream, nil
		}
	}

	response, err := p.postWithRetry(
		ctx,
		model,
		options,
		execution,
		request.sseHeaders,
		request.payload,
	)
	if err != nil {
		if ctx.Err() != nil {
			return ErrorAssistantStream(AssistantErrorMessage(ctx.Err().Error(), model, true)), nil
		}
		return streamError(model, "%s", err.Error()), nil
	}

	stream := NewAssistantMessageEventStream()
	serviceTier := metadataString(options.Metadata, "service_tier")
	go streamOpenAICodexResponsesBody(
		model,
		response.Body,
		stream,
		serviceTier,
		nil,
		request.sampling.GrammarToolInputProperties,
	)
	return stream, nil
}

func (p OpenAICodexResponsesProvider) buildRequest(
	model Model,
	llmContext Context,
	options SimpleStreamOptions,
	apiKey string,
) (openAICodexPreparedRequest, error) {
	cacheSessionID := strings.TrimSpace(options.SessionID)
	if strings.EqualFold(strings.TrimSpace(options.CacheRetention), "none") {
		cacheSessionID = ""
	}
	codexSessionID := ClampOpenAIPromptCacheKey(cacheSessionID)
	reasoning := ""
	if options.Reasoning != "" {
		reasoning = ClampThinkingLevel(model, options.Reasoning)
		if reasoning == "off" {
			reasoning = ""
		}
	}
	codexPayload, sampling, err := buildOpenAICodexResponsesPayload(model, llmContext, OpenAICodexResponsesPayloadOptions{
		Temperature:      options.Temperature,
		SessionID:        codexSessionID,
		ReasoningEffort:  reasoning,
		ReasoningSummary: metadataString(options.Metadata, "reasoning_summary"),
		ServiceTier:      metadataString(options.Metadata, "service_tier"),
		TextVerbosity:    metadataString(options.Metadata, "text_verbosity"),
	})
	if err != nil {
		return openAICodexPreparedRequest{}, err
	}
	payload := any(codexPayload)
	if options.OnPayload != nil {
		next, replace, err := options.OnPayload(payload, model)
		if err != nil {
			return openAICodexPreparedRequest{}, err
		}
		if replace {
			payload = next
		}
	}
	sseHeaders, err := BuildOpenAICodexSSEHeaders(model.Headers, options.Headers, apiKey, codexSessionID)
	if err != nil {
		return openAICodexPreparedRequest{}, err
	}
	sseHeaders = applyHeaderRemovals(sseHeaders, options.HeaderRemovals)
	requestID := codexSessionID
	if requestID == "" {
		requestID = UUIDv7()
	}
	webSocketHeaders, err := BuildOpenAICodexWebSocketHeaders(
		model.Headers,
		options.Headers,
		apiKey,
		requestID,
	)
	if err != nil {
		return openAICodexPreparedRequest{}, err
	}
	webSocketHeaders = applyHeaderRemovals(webSocketHeaders, options.HeaderRemovals)
	return openAICodexPreparedRequest{
		payload:          payload,
		sseHeaders:       sseHeaders,
		webSocketHeaders: webSocketHeaders,
		sampling:         sampling,
		cacheSessionID:   cacheSessionID,
		codexSessionID:   codexSessionID,
	}, nil
}

func (p OpenAICodexResponsesProvider) postWithRetry(
	ctx context.Context,
	model Model,
	options SimpleStreamOptions,
	execution openAICodexExecutionOptions,
	headers map[string]string,
	payload any,
) (*http.Response, error) {
	client := httpClientOrDefault(p.Client)
	endpoint := ResolveOpenAICodexURL(model.BaseURL)
	var lastErr error
	for attempt := 0; attempt <= execution.maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		response, err := postOpenAICodexSSEWithHeaderTimeout(
			ctx,
			client,
			endpoint,
			headers,
			payload,
			execution.sseResponseHeaderTimeout,
		)
		if err != nil {
			if attempt < execution.maxRetries {
				if err := p.wait(ctx, OpenAICodexRetryDelay(0, nil, attempt, p.now())); err != nil {
					return nil, err
				}
				continue
			}
			return nil, err
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
		if attempt < execution.maxRetries && IsOpenAICodexRetryable(response.StatusCode, bodyText) {
			delay, serverRequested := openAICodexRetryDelay(
				headerMap,
				attempt,
				p.now(),
			)
			if serverRequested {
				delay, err = validateOpenAICodexRetryDelay(delay, execution.maxRetryDelay)
				if err != nil {
					return nil, err
				}
			}
			if err := p.wait(ctx, delay); err != nil {
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

type openAICodexCancelReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (body *openAICodexCancelReadCloser) Close() error {
	err := body.ReadCloser.Close()
	body.cancel()
	return err
}

func postOpenAICodexSSEWithHeaderTimeout(
	ctx context.Context,
	client HTTPDoer,
	endpoint string,
	headers map[string]string,
	payload any,
	timeout time.Duration,
) (*http.Response, error) {
	if timeout <= 0 {
		return postSSE(ctx, client, endpoint, headers, payload)
	}

	requestContext, cancel := context.WithCancel(ctx)
	timeoutDone := make(chan struct{})
	timer := time.AfterFunc(timeout, func() {
		close(timeoutDone)
		cancel()
	})
	response, err := postSSE(requestContext, client, endpoint, headers, payload)
	if !timer.Stop() {
		<-timeoutDone
	}
	timedOut := false
	select {
	case <-timeoutDone:
		timedOut = true
	default:
	}
	if timedOut {
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		cancel()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf(
			"Codex SSE response headers timed out after %dms",
			timeout/time.Millisecond,
		)
	}
	if err != nil {
		cancel()
		return nil, err
	}
	response.Body = &openAICodexCancelReadCloser{
		ReadCloser: response.Body,
		cancel:     cancel,
	}
	return response, nil
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

func (p OpenAICodexResponsesProvider) streamOpenAICodexWebSocketWithFallback(
	ctx context.Context,
	model Model,
	options SimpleStreamOptions,
	request openAICodexPreparedRequest,
	body openAICodexWebSocketRequestBody,
	stream *AssistantMessageEventStream,
	execution openAICodexExecutionOptions,
) {
	output := AssistantMessage(nil, StopReasonStop, model)
	startEmitted := false
	retriedConnectionLimit := false
	retriedMissingContinuation := false

	for {
		started, err := p.streamOpenAICodexWebSocket(
			ctx,
			model,
			options,
			request,
			body,
			stream,
			execution,
			&output,
			&startEmitted,
		)
		if err == nil {
			return
		}
		var apiError *openAICodexAPIError
		connectionLimitBeforeStart := false
		if errors.As(err, &apiError) {
			connectionLimitBeforeStart = apiError.code == openAICodexWebSocketConnectionLimitCode && !started
			switch {
			case apiError.code == openAICodexPreviousResponseNotFoundCode &&
				!retriedMissingContinuation:
				retriedMissingContinuation = true
				continue
			case connectionLimitBeforeStart &&
				!retriedConnectionLimit:
				retriedConnectionLimit = true
				continue
			}
		}
		if ctx.Err() != nil {
			pushOpenAICodexWebSocketError(stream, &output, ctx.Err(), true)
			return
		}
		if isOpenAICodexNonTransportError(err) && !connectionLimitBeforeStart {
			pushOpenAICodexWebSocketError(stream, &output, err, false)
			return
		}

		diagnostic := newOpenAICodexTransportDiagnostic(
			execution.transport,
			err,
			started,
			request.payload,
		)
		output.Diagnostics = append(output.Diagnostics, diagnostic)
		recordOpenAICodexWebSocketFailure(request.cacheSessionID, err)
		if started {
			pushOpenAICodexWebSocketError(stream, &output, err, false)
			return
		}

		recordOpenAICodexWebSocketSSEFallback(request.cacheSessionID)
		response, fallbackErr := p.postWithRetry(
			ctx,
			model,
			options,
			execution,
			request.sseHeaders,
			request.payload,
		)
		if fallbackErr != nil {
			message := AssistantErrorMessage(fallbackErr.Error(), model, ctx.Err() != nil)
			message.Diagnostics = append(message.Diagnostics, diagnostic)
			stream.Push(AssistantMessageEvent{
				Type:   "error",
				Reason: StopReasonError,
				Error:  message,
			})
			return
		}
		streamOpenAICodexResponsesBody(
			model,
			response.Body,
			stream,
			metadataString(options.Metadata, "service_tier"),
			[]AssistantMessageDiagnostic{diagnostic},
			request.sampling.GrammarToolInputProperties,
		)
		return
	}
}

func (p OpenAICodexResponsesProvider) streamOpenAICodexWebSocket(
	ctx context.Context,
	model Model,
	options SimpleStreamOptions,
	request openAICodexPreparedRequest,
	body openAICodexWebSocketRequestBody,
	stream *AssistantMessageEventStream,
	execution openAICodexExecutionOptions,
	output *Message,
	startEmitted *bool,
) (started bool, resultErr error) {
	lease, err := acquireOpenAICodexWebSocket(
		ctx,
		p.WebSocketDialer,
		ResolveOpenAICodexWebSocketURL(model.BaseURL),
		request.webSocketHeaders,
		request.cacheSessionID,
		p.now(),
		execution.webSocketConnectTimeout,
	)
	if err != nil {
		return false, err
	}
	keepConnection := false
	defer func() {
		if resultErr != nil {
			clearOpenAICodexWebSocketContinuation(lease.session)
		}
		lease.release(keepConnection)
	}()

	useCachedContext := execution.transport == "websocket-cached" ||
		execution.transport == "auto"
	plan := buildOpenAICodexWebSocketRequestPlan(lease.session, body, useCachedContext)
	recordOpenAICodexWebSocketRequest(
		request.cacheSessionID,
		lease.reused,
		useCachedContext,
		body.store,
		plan,
	)
	frame, err := plan.frame()
	if err != nil {
		return false, fmt.Errorf("encode Codex WebSocket frame: %w", err)
	}
	if err := lease.connection.Write(ctx, frame); err != nil {
		return false, err
	}

	processor := NewOpenAIResponsesStreamProcessorWithOptions(
		model,
		output,
		OpenAIResponsesStreamProcessorOptions{
			GrammarToolInputProperties: request.sampling.GrammarToolInputProperties,
		},
	)
	for {
		payload, err := lease.connection.Read(ctx, execution.webSocketIdleTimeout)
		if err != nil {
			return started, err
		}
		event, err := decodeOpenAICodexWebSocketEvent(payload)
		if err != nil {
			return started, err
		}
		if event.Type == "" {
			continue
		}
		normalized, ok := normalizeOpenAICodexEvent(event)
		if !ok {
			continue
		}
		if !started {
			started = true
			if !*startEmitted {
				*startEmitted = true
				stream.Push(AssistantMessageEvent{Type: "start", Partial: *output})
			}
		}

		emitted := processOpenAICodexNormalizedEvent(
			processor,
			output,
			normalized,
			metadataString(options.Metadata, "service_tier"),
			model,
		)
		terminal := false
		for _, emittedEvent := range emitted {
			if emittedEvent.Type == "done" || emittedEvent.Type == "error" {
				terminal = true
				break
			}
		}
		if terminal && useCachedContext && lease.session != nil && output.ResponseID != "" {
			responseItems, err := openAICodexWebSocketResponseItems(
				model,
				*output,
				request.sampling.GrammarToolInputProperties,
			)
			if err != nil {
				return started, err
			}
			updateOpenAICodexWebSocketContinuation(
				lease.session,
				body,
				output.ResponseID,
				responseItems,
			)
		}
		for _, emittedEvent := range emitted {
			stream.Push(emittedEvent)
		}
		if terminal {
			keepConnection = ctx.Err() == nil
			return started, nil
		}
	}
}

const (
	openAICodexWebSocketConnectionLimitCode = "websocket_connection_limit_reached"
	openAICodexPreviousResponseNotFoundCode = "previous_response_not_found"
)

type openAICodexAPIError struct {
	code    string
	message string
	payload json.RawMessage
}

func (e *openAICodexAPIError) Error() string {
	detail := strings.TrimSpace(e.message)
	if detail == "" {
		detail = strings.TrimSpace(e.code)
	}
	if detail == "" {
		detail = "Codex error"
	}
	return detail
}

type openAICodexProtocolError struct {
	message string
	payload json.RawMessage
}

func (e *openAICodexProtocolError) Error() string {
	return e.message
}

func isOpenAICodexNonTransportError(err error) bool {
	var apiError *openAICodexAPIError
	var protocolError *openAICodexProtocolError
	return errors.As(err, &apiError) || errors.As(err, &protocolError)
}

func decodeOpenAICodexWebSocketEvent(payload []byte) (OpenAIResponsesStreamEvent, error) {
	var envelope struct {
		Type    string `json:"type"`
		Code    string `json:"code"`
		Message string `json:"message"`
		Error   *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Response *struct {
			Error *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		} `json:"response"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return OpenAIResponsesStreamEvent{}, &openAICodexProtocolError{
			message: fmt.Sprintf("invalid Codex WebSocket JSON: %v", err),
			payload: append(json.RawMessage(nil), payload...),
		}
	}
	switch envelope.Type {
	case "error":
		code, message := envelope.Code, envelope.Message
		if envelope.Error != nil {
			if code == "" {
				code = envelope.Error.Code
			}
			if message == "" {
				message = envelope.Error.Message
			}
		}
		detail := strings.TrimSpace(message)
		if detail == "" {
			detail = strings.TrimSpace(code)
		}
		if detail == "" {
			detail = strings.TrimSpace(string(payload))
		}
		return OpenAIResponsesStreamEvent{}, &openAICodexAPIError{
			code:    code,
			message: "Codex error: " + detail,
			payload: append(json.RawMessage(nil), payload...),
		}
	case "response.failed":
		var code, message string
		if envelope.Response != nil && envelope.Response.Error != nil {
			code = envelope.Response.Error.Code
			message = envelope.Response.Error.Message
		}
		if message == "" {
			message = "Codex response failed"
		}
		return OpenAIResponsesStreamEvent{}, &openAICodexAPIError{
			code:    code,
			message: message,
			payload: append(json.RawMessage(nil), payload...),
		}
	}
	event, err := DecodeOpenAIResponsesSSEEvent(payload)
	if err != nil {
		return OpenAIResponsesStreamEvent{}, &openAICodexProtocolError{
			message: err.Error(),
			payload: append(json.RawMessage(nil), payload...),
		}
	}
	return event, nil
}

func openAICodexWebSocketResponseItems(
	model Model,
	output Message,
	grammarToolInputProperties map[string]string,
) ([]json.RawMessage, error) {
	items, err := ConvertOpenAIResponsesMessagesChecked(
		model,
		Context{Messages: []Message{output}},
		ConvertOpenAIResponsesOptions{
			IncludeSystemPrompt:        ptrBool(false),
			GrammarToolInputProperties: grammarToolInputProperties,
		},
	)
	if err != nil {
		return nil, err
	}
	result := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		if item.Type == "function_call_output" || item.Type == "custom_tool_call_output" {
			continue
		}
		raw, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		raw, err = canonicalOpenAICodexJSON(raw)
		if err != nil {
			return nil, err
		}
		result = append(result, raw)
	}
	return result, nil
}

func normalizeOpenAICodexTransport(transport string) string {
	transport = strings.TrimSpace(strings.ToLower(transport))
	if transport == "" {
		return "auto"
	}
	return transport
}

func newOpenAICodexTransportDiagnostic(
	transport string,
	err error,
	started bool,
	payload any,
) AssistantMessageDiagnostic {
	requestBytes := 0
	if raw, marshalErr := json.Marshal(payload); marshalErr == nil {
		requestBytes = len(raw)
	}
	fallbackTransport := any(nil)
	phase := "after_message_stream_start"
	if !started {
		fallbackTransport = "sse"
		phase = "before_message_stream_start"
	}
	return NewAssistantMessageDiagnostic("provider_transport_failure", err, map[string]any{
		"configuredTransport": transport,
		"fallbackTransport":   fallbackTransport,
		"eventsEmitted":       started,
		"phase":               phase,
		"requestBytes":        requestBytes,
	})
}

func pushOpenAICodexWebSocketError(
	stream *AssistantMessageEventStream,
	output *Message,
	err error,
	aborted bool,
) {
	if output == nil {
		return
	}
	reason := StopReasonError
	if aborted {
		reason = StopReasonAborted
	}
	output.StopReason = reason
	output.ErrorMessage = err.Error()
	stream.Push(AssistantMessageEvent{
		Type:    "error",
		Reason:  reason,
		Error:   *output,
		Partial: *output,
	})
}

func streamOpenAICodexResponsesBody(
	model Model,
	body io.ReadCloser,
	stream *AssistantMessageEventStream,
	requestServiceTier string,
	diagnostics []AssistantMessageDiagnostic,
	grammarToolInputProperties map[string]string,
) {
	output := AssistantMessage(nil, StopReasonStop, model)
	output.Diagnostics = append(output.Diagnostics, diagnostics...)
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
		normalized, ok := normalizeOpenAICodexEvent(event)
		if !ok {
			if event.Type == "error" {
				return false, fmt.Errorf("codex error: %s", event.Error)
			}
			return false, nil
		}
		emitted := processOpenAICodexNormalizedEvent(
			processor,
			&output,
			normalized,
			requestServiceTier,
			model,
		)
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

func processOpenAICodexNormalizedEvent(
	processor *OpenAIResponsesStreamProcessor,
	output *Message,
	event OpenAIResponsesStreamEvent,
	requestServiceTier string,
	model Model,
) []AssistantMessageEvent {
	emitted := processor.Process(event)
	if event.Type != "response.completed" || event.Response == nil {
		return emitted
	}
	tier := ResolveOpenAICodexServiceTier(event.Response.ServiceTier, requestServiceTier)
	ApplyOpenAICodexServiceTierPricing(&output.Usage, tier, model)
	for index := range emitted {
		emitted[index].Reason = output.StopReason
		if emitted[index].Type == "error" {
			emitted[index].Error = *output
		} else {
			emitted[index].Message = *output
		}
		emitted[index].Partial = *output
	}
	return emitted
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
