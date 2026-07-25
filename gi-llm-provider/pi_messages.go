package gillmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	piMessagesAPI                    = "pi-messages"
	maxPiMessagesErrorBodyBytes      = 1 << 20
	maxPiMessagesDiagnosticBodyRunes = 8192
)

// PiMessagesRewriteImpact describes a server-side message rewrite.
type PiMessagesRewriteImpact struct {
	PolicyID            string `json:"policyId"`
	PolicyVersion       int    `json:"policyVersion"`
	Changed             bool   `json:"changed"`
	TokenCountChange    int    `json:"tokenCountChange"`
	MessageCountChange  int    `json:"messageCountChange"`
	SystemPromptChanged bool   `json:"systemPromptChanged"`
}

// PiMessagesEvent is one serialized event from a pi-messages SSE response.
// Go uses one tagged struct for Pi's discriminated event union.
type PiMessagesEvent struct {
	Type             string                   `json:"type"`
	ContentIndex     int                      `json:"contentIndex,omitempty"`
	Delta            string                   `json:"delta,omitempty"`
	Content          string                   `json:"content,omitempty"`
	ContentSignature string                   `json:"contentSignature,omitempty"`
	Redacted         bool                     `json:"redacted,omitempty"`
	ID               string                   `json:"id,omitempty"`
	ToolName         string                   `json:"toolName,omitempty"`
	ToolCall         ContentPart              `json:"toolCall,omitempty"`
	Reason           string                   `json:"reason,omitempty"`
	Usage            Usage                    `json:"usage,omitempty"`
	ErrorMessage     string                   `json:"errorMessage,omitempty"`
	ResponseID       string                   `json:"responseId,omitempty"`
	Rewrite          *PiMessagesRewriteImpact `json:"rewrite,omitempty"`
}

// PiMessagesRequest is the wire request accepted by Radius and compatible
// pi-messages backends.
type PiMessagesRequest struct {
	Model   string                   `json:"model"`
	Context Context                  `json:"context"`
	Options PiMessagesRequestOptions `json:"options"`
}

// PiMessagesRequestOptions contains only options understood by the wire
// protocol. Transport-only controls remain on StreamOptions.
type PiMessagesRequestOptions struct {
	Temperature    *float64 `json:"temperature,omitempty"`
	MaxTokens      int      `json:"maxTokens,omitempty"`
	Reasoning      string   `json:"reasoning,omitempty"`
	CacheRetention string   `json:"cacheRetention,omitempty"`
	SessionID      string   `json:"sessionId,omitempty"`
	ToolChoice     any      `json:"toolChoice,omitempty"`
}

// PiMessagesResponseError preserves structured gateway failure details for
// diagnostics while implementing Go's error contract.
type PiMessagesResponseError struct {
	Message           string
	Code              string
	DiagnosticDetails map[string]any
}

func (e *PiMessagesResponseError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// PiMessagesProvider implements the Radius pi-messages HTTP/SSE protocol.
type PiMessagesProvider struct {
	Client HTTPDoer
}

// NewPiMessagesProvider creates a pi-messages transport with an injectable
// HTTP boundary. A nil client uses the package default.
func NewPiMessagesProvider(client HTTPDoer) PiMessagesProvider {
	return PiMessagesProvider{Client: httpClientOrDefault(client)}
}

func init() {
	RegisterBuiltInAPIProvider(piMessagesAPI, NewPiMessagesProvider(nil))
}

func (p PiMessagesProvider) Stream(
	model Model,
	llmContext Context,
	options StreamOptions,
) (*AssistantMessageEventStream, error) {
	return p.StreamSimple(model, llmContext, options)
}

func (p PiMessagesProvider) StreamSimple(
	model Model,
	llmContext Context,
	options SimpleStreamOptions,
) (*AssistantMessageEventStream, error) {
	stream := NewAssistantMessageEventStream()
	go p.run(stream, model, llmContext, cloneStreamOptions(options))
	return stream, nil
}

func (p PiMessagesProvider) run(
	stream *AssistantMessageEventStream,
	model Model,
	llmContext Context,
	options StreamOptions,
) {
	ctx := contextOrBackground(options.Context)
	if err := p.stream(ctx, stream, model, llmContext, options); err != nil {
		aborted := ctx.Err() != nil ||
			errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded)
		stream.Push(piMessagesErrorEvent(model, err, aborted))
	}
}

func (p PiMessagesProvider) stream(
	ctx context.Context,
	stream *AssistantMessageEventStream,
	model Model,
	llmContext Context,
	options StreamOptions,
) error {
	apiKey := apiKeyOrEnv(model.Provider, options.APIKey, options.Env)
	if apiKey == "" {
		return fmt.Errorf("No API key provided for provider %q", model.Provider)
	}

	requestURL, err := piMessagesEndpoint(model.BaseURL, options.Debug)
	if err != nil {
		return err
	}
	payload := any(PiMessagesRequest{
		Model:   model.ID,
		Context: llmContext,
		Options: PiMessagesRequestOptions{
			Temperature:    options.Temperature,
			MaxTokens:      options.MaxTokens,
			Reasoning:      options.Reasoning,
			CacheRetention: piMessagesCacheRetention(options),
			SessionID:      options.SessionID,
			ToolChoice:     cloneCredentialMetadataValue(options.ToolChoice),
		},
	})
	if options.OnPayload != nil {
		next, replace, err := options.OnPayload(payload, model)
		if err != nil {
			return err
		}
		if replace {
			payload = next
		}
	}

	headers := mergeHeadersCaseInsensitive(
		map[string]string{"Authorization": "Bearer " + apiKey},
		options.Headers,
	)
	headers = applyHeaderRemovals(headers, options.HeaderRemovals)
	response, err := postJSONWithAccept(
		ctx,
		httpClientForRequest(p.Client, options),
		requestURL.String(),
		headers,
		payload,
		"text/event-stream",
	)
	if err != nil {
		return err
	}
	if options.OnResponseStatus != nil {
		if err := options.OnResponseStatus(
			response.StatusCode,
			lowercaseResponseHeaders(response.Header),
			model,
		); err != nil {
			if response.Body != nil {
				response.Body.Close()
			}
			return err
		}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return readPiMessagesResponseError(model, requestURL, response)
	}
	if response.Body == nil {
		return fmt.Errorf("%s response has no body", model.Provider)
	}

	converter := newPiMessagesEventConverter(model)
	terminal := false
	err = dispatchSSEUntil(response.Body, func(data string) (bool, error) {
		var wireEvent PiMessagesEvent
		if err := json.Unmarshal([]byte(data), &wireEvent); err != nil {
			return false, fmt.Errorf("decode pi-messages event: %w", err)
		}
		event, err := converter.convert(wireEvent)
		if err != nil {
			return false, err
		}
		stream.Push(event)
		terminal = event.Type == "done" || event.Type == "error"
		return terminal, nil
	})
	if err != nil {
		return err
	}
	if !terminal {
		return fmt.Errorf("%s stream ended without a terminal event", model.Provider)
	}
	return nil
}

func piMessagesEndpoint(baseURL string, debug bool) (*url.URL, error) {
	endpoint, err := url.Parse(strings.TrimRight(baseURL, "/") + "/messages")
	if err != nil {
		return nil, fmt.Errorf("parse pi-messages endpoint: %w", err)
	}
	if endpoint.Scheme == "" || endpoint.Host == "" {
		return nil, fmt.Errorf("invalid pi-messages base URL %q", baseURL)
	}
	if debug {
		query := endpoint.Query()
		query.Set("debug", "1")
		endpoint.RawQuery = query.Encode()
	}
	return endpoint, nil
}

func piMessagesCacheRetention(options StreamOptions) string {
	if options.CacheRetention != "" {
		return options.CacheRetention
	}
	if GetProviderEnvValue("PI_CACHE_RETENTION", options.Env) == "long" {
		return "long"
	}
	return ""
}

func lowercaseResponseHeaders(headers http.Header) map[string]string {
	result := make(map[string]string, len(headers))
	for name, values := range headers {
		if len(values) > 0 {
			result[strings.ToLower(name)] = values[0]
		}
	}
	return result
}

func readPiMessagesResponseError(
	model Model,
	requestURL *url.URL,
	response *http.Response,
) error {
	if response.Body == nil {
		return newPiMessagesResponseError(model, requestURL, response, "")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(
		response.Body,
		maxPiMessagesErrorBodyBytes+1,
	))
	if err != nil {
		return fmt.Errorf("read pi-messages error response: %w", err)
	}
	if len(body) > maxPiMessagesErrorBodyBytes {
		body = append(body[:maxPiMessagesErrorBodyBytes], []byte("…")...)
	}
	return newPiMessagesResponseError(
		model,
		requestURL,
		response,
		string(body),
	)
}

func newPiMessagesResponseError(
	model Model,
	requestURL *url.URL,
	response *http.Response,
	body string,
) *PiMessagesResponseError {
	var parsed struct {
		Error map[string]any `json:"error"`
	}
	hasStructuredError := json.Unmarshal([]byte(body), &parsed) == nil &&
		parsed.Error != nil
	message, _ := parsed.Error["message"].(string)
	code, _ := parsed.Error["code"].(string)
	suffix := message
	if suffix == "" {
		suffix = body
	}
	codeSuffix := ""
	if code != "" {
		codeSuffix = " (" + code + ")"
	}
	statusText := http.StatusText(response.StatusCode)
	errorMessage := fmt.Sprintf(
		"%d %s: %s%s",
		response.StatusCode,
		statusText,
		suffix,
		codeSuffix,
	)
	details := map[string]any{
		"version":     1,
		"provider":    model.Provider,
		"model":       model.ID,
		"url":         requestURL.String(),
		"status":      response.StatusCode,
		"statusText":  statusText,
		"timestampMs": NowMillis(),
	}
	if hasStructuredError {
		details["error"] = cloneCredentialMetadata(parsed.Error)
	} else {
		details["body"] = truncatePiMessagesDiagnosticString(body)
	}
	return &PiMessagesResponseError{
		Message:           errorMessage,
		Code:              code,
		DiagnosticDetails: details,
	}
}

func truncatePiMessagesDiagnosticString(value string) string {
	runes := []rune(value)
	if len(runes) <= maxPiMessagesDiagnosticBodyRunes {
		return value
	}
	return string(runes[:maxPiMessagesDiagnosticBodyRunes]) + "…"
}

func piMessagesErrorEvent(
	model Model,
	err error,
	aborted bool,
) AssistantMessageEvent {
	reason := StopReasonError
	if aborted {
		reason = StopReasonAborted
	}
	message := AssistantMessage(nil, reason, model)
	message.ErrorMessage = FormatThrownValue(err)
	if !aborted {
		var responseError *PiMessagesResponseError
		if errors.As(err, &responseError) {
			AppendAssistantMessageDiagnostic(
				&message,
				NewAssistantMessageDiagnostic(
					"pi_messages_response_failure",
					responseError,
					cloneCredentialMetadata(responseError.DiagnosticDetails),
				),
			)
		}
	}
	return AssistantMessageEvent{
		Type:   "error",
		Reason: reason,
		Error:  message,
	}
}

type piMessagesEventConverter struct {
	partial  Message
	toolJSON map[int]string
}

func newPiMessagesEventConverter(model Model) *piMessagesEventConverter {
	return &piMessagesEventConverter{
		partial:  AssistantMessage(nil, StopReasonStop, model),
		toolJSON: map[int]string{},
	}
}

func (c *piMessagesEventConverter) convert(
	wire PiMessagesEvent,
) (AssistantMessageEvent, error) {
	switch wire.Type {
	case "done":
		c.partial.StopReason = wire.Reason
		c.partial.Usage = wire.Usage
		c.partial.ResponseID = wire.ResponseID
		appendPiMessagesRewriteDiagnostic(&c.partial, wire.Rewrite)
		message := clonePiMessagesMessage(c.partial)
		return AssistantMessageEvent{
			Type:    "done",
			Reason:  wire.Reason,
			Message: message,
		}, nil
	case "error":
		c.partial.StopReason = wire.Reason
		c.partial.Usage = wire.Usage
		c.partial.ErrorMessage = wire.ErrorMessage
		c.partial.ResponseID = wire.ResponseID
		appendPiMessagesRewriteDiagnostic(&c.partial, wire.Rewrite)
		message := clonePiMessagesMessage(c.partial)
		return AssistantMessageEvent{
			Type:   "error",
			Reason: wire.Reason,
			Error:  message,
		}, nil
	case "start":
	case "text_start":
		if err := c.setContent(wire.ContentIndex, Text("")); err != nil {
			return AssistantMessageEvent{}, err
		}
	case "text_delta":
		part, err := c.content(wire.ContentIndex, ContentText)
		if err != nil {
			return AssistantMessageEvent{}, err
		}
		part.Text += wire.Delta
	case "text_end":
		part, err := c.content(wire.ContentIndex, ContentText)
		if err != nil {
			return AssistantMessageEvent{}, err
		}
		part.Text = wire.Content
		part.TextSignature = wire.ContentSignature
	case "thinking_start":
		if err := c.setContent(wire.ContentIndex, Thinking("")); err != nil {
			return AssistantMessageEvent{}, err
		}
	case "thinking_delta":
		part, err := c.content(wire.ContentIndex, ContentThinking)
		if err != nil {
			return AssistantMessageEvent{}, err
		}
		part.Thinking += wire.Delta
	case "thinking_end":
		part, err := c.content(wire.ContentIndex, ContentThinking)
		if err != nil {
			return AssistantMessageEvent{}, err
		}
		part.Thinking = wire.Content
		part.ThinkingSignature = wire.ContentSignature
		part.Redacted = wire.Redacted
	case "toolcall_start":
		if err := c.setContent(
			wire.ContentIndex,
			ToolCall(wire.ID, wire.ToolName, nil),
		); err != nil {
			return AssistantMessageEvent{}, err
		}
		c.toolJSON[wire.ContentIndex] = ""
	case "toolcall_delta":
		part, err := c.content(wire.ContentIndex, ContentToolCall)
		if err != nil {
			return AssistantMessageEvent{}, err
		}
		raw := c.toolJSON[wire.ContentIndex] + wire.Delta
		c.toolJSON[wire.ContentIndex] = raw
		part.Arguments = parseStreamingJSONObject(raw)
	case "toolcall_end":
		part, err := c.content(wire.ContentIndex, ContentToolCall)
		if err != nil {
			return AssistantMessageEvent{}, err
		}
		mergePiMessagesToolCall(part, wire.ToolCall)
		delete(c.toolJSON, wire.ContentIndex)
		partial := clonePiMessagesMessage(c.partial)
		return AssistantMessageEvent{
			Type:         "toolcall_end",
			ContentIndex: wire.ContentIndex,
			ToolCall:     clonePiMessagesContentPart(*part),
			Partial:      partial,
		}, nil
	default:
		return AssistantMessageEvent{}, fmt.Errorf(
			"unsupported pi-messages event type %q",
			wire.Type,
		)
	}

	return AssistantMessageEvent{
		Type:         wire.Type,
		ContentIndex: wire.ContentIndex,
		Delta:        wire.Delta,
		Content:      wire.Content,
		Partial:      clonePiMessagesMessage(c.partial),
	}, nil
}

func (c *piMessagesEventConverter) setContent(
	index int,
	part ContentPart,
) error {
	if index < 0 {
		return fmt.Errorf("invalid pi-messages content index %d", index)
	}
	for len(c.partial.Content) <= index {
		c.partial.Content = append(c.partial.Content, ContentPart{})
	}
	c.partial.Content[index] = part
	return nil
}

func (c *piMessagesEventConverter) content(
	index int,
	wantType string,
) (*ContentPart, error) {
	if index < 0 || index >= len(c.partial.Content) {
		return nil, fmt.Errorf("pi-messages content index %d was not started", index)
	}
	part := &c.partial.Content[index]
	if part.Type != wantType {
		return nil, fmt.Errorf(
			"pi-messages content index %d has type %q, want %q",
			index,
			part.Type,
			wantType,
		)
	}
	return part, nil
}

func mergePiMessagesToolCall(target *ContentPart, source ContentPart) {
	if source.Type != "" {
		target.Type = source.Type
	}
	if source.ID != "" {
		target.ID = source.ID
	}
	if source.Name != "" {
		target.Name = source.Name
	}
	if source.Arguments != nil {
		target.Arguments = cloneCredentialMetadata(source.Arguments)
	}
	if source.ThoughtSignature != "" {
		target.ThoughtSignature = source.ThoughtSignature
	}
}

func appendPiMessagesRewriteDiagnostic(
	message *Message,
	rewrite *PiMessagesRewriteImpact,
) {
	if rewrite == nil {
		return
	}
	AppendAssistantMessageDiagnostic(message, AssistantMessageDiagnostic{
		Type:      "pi_messages_rewrite",
		Timestamp: NowMillis(),
		Details: map[string]any{
			"policyId":            rewrite.PolicyID,
			"policyVersion":       rewrite.PolicyVersion,
			"changed":             rewrite.Changed,
			"tokenCountChange":    rewrite.TokenCountChange,
			"messageCountChange":  rewrite.MessageCountChange,
			"systemPromptChanged": rewrite.SystemPromptChanged,
		},
	})
}

func clonePiMessagesMessage(message Message) Message {
	cloned := message
	cloned.Content = make([]ContentPart, len(message.Content))
	for index, part := range message.Content {
		cloned.Content[index] = clonePiMessagesContentPart(part)
	}
	cloned.Diagnostics = make(
		[]AssistantMessageDiagnostic,
		len(message.Diagnostics),
	)
	for index, diagnostic := range message.Diagnostics {
		cloned.Diagnostics[index] = diagnostic
		if diagnostic.Error != nil {
			errorInfo := *diagnostic.Error
			errorInfo.Code = cloneCredentialMetadataValue(diagnostic.Error.Code)
			cloned.Diagnostics[index].Error = &errorInfo
		}
		cloned.Diagnostics[index].Details = cloneCredentialMetadata(
			diagnostic.Details,
		)
	}
	cloned.Details = cloneCredentialMetadataValue(message.Details)
	return cloned
}

func clonePiMessagesContentPart(part ContentPart) ContentPart {
	cloned := part
	cloned.Arguments = cloneCredentialMetadata(part.Arguments)
	return cloned
}
