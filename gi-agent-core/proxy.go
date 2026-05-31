package giagentcore

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type ProxyStreamOptions struct {
	llm.SimpleStreamOptions
	AuthToken  string
	ProxyURL   string
	HTTPClient *http.Client
}

type ProxyAssistantMessageEvent struct {
	Type             string    `json:"type"`
	ContentIndex     int       `json:"contentIndex,omitempty"`
	Delta            string    `json:"delta,omitempty"`
	ContentSignature string    `json:"contentSignature,omitempty"`
	ID               string    `json:"id,omitempty"`
	ToolName         string    `json:"toolName,omitempty"`
	Reason           string    `json:"reason,omitempty"`
	ErrorMessage     string    `json:"errorMessage,omitempty"`
	Usage            llm.Usage `json:"usage,omitempty"`
}

type proxySerializableStreamOptions struct {
	Temperature     *float64          `json:"temperature,omitempty"`
	MaxTokens       int               `json:"maxTokens,omitempty"`
	Reasoning       string            `json:"reasoning,omitempty"`
	CacheRetention  string            `json:"cacheRetention,omitempty"`
	SessionID       string            `json:"sessionId,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	Metadata        map[string]any    `json:"metadata,omitempty"`
	Transport       string            `json:"transport,omitempty"`
	ThinkingBudgets map[string]int    `json:"thinkingBudgets,omitempty"`
	MaxRetryDelayMs int               `json:"maxRetryDelayMs,omitempty"`
}

func StreamProxy(model llm.Model, llmContext llm.Context, options ProxyStreamOptions) *llm.AssistantMessageEventStream {
	stream := llm.NewAssistantMessageEventStream()
	go runProxyStream(stream, model, llmContext, options)
	return stream
}

func NewProxyStreamFn(proxyURL, authToken string, baseOptions llm.SimpleStreamOptions, client *http.Client) StreamFn {
	return func(model llm.Model, llmContext llm.Context, options llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error) {
		merged := baseOptions
		mergeProxyStreamOptions(&merged, options)
		return StreamProxy(model, llmContext, ProxyStreamOptions{
			SimpleStreamOptions: merged,
			AuthToken:           authToken,
			ProxyURL:            proxyURL,
			HTTPClient:          client,
		}), nil
	}
}

func runProxyStream(stream *llm.AssistantMessageEventStream, model llm.Model, llmContext llm.Context, options ProxyStreamOptions) {
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	partial := llm.AssistantMessage(nil, llm.StopReasonStop, model)
	partial.Timestamp = llm.NowMillis()

	if strings.TrimSpace(options.ProxyURL) == "" {
		pushProxyStreamError(stream, partial, "missing proxy URL", ctx)
		return
	}
	if strings.TrimSpace(options.AuthToken) == "" {
		pushProxyStreamError(stream, partial, "missing proxy auth token", ctx)
		return
	}

	body, err := json.Marshal(map[string]any{
		"model":   model,
		"context": llmContext,
		"options": buildProxyRequestOptions(options.SimpleStreamOptions),
	})
	if err != nil {
		pushProxyStreamError(stream, partial, err.Error(), ctx)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(options.ProxyURL, "/")+"/api/stream", bytes.NewReader(body))
	if err != nil {
		pushProxyStreamError(stream, partial, err.Error(), ctx)
		return
	}
	req.Header.Set("Authorization", "Bearer "+options.AuthToken)
	req.Header.Set("Content-Type", "application/json")

	client := options.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		pushProxyStreamError(stream, partial, err.Error(), ctx)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		pushProxyHTTPError(stream, partial, resp, ctx)
		return
	}

	partialToolJSON := map[int]string{}
	if err := readProxySSE(resp.Body, func(proxyEvent ProxyAssistantMessageEvent) error {
		event, ok, err := processProxyEvent(proxyEvent, &partial, partialToolJSON)
		if err != nil {
			return err
		}
		if ok {
			stream.Push(event)
		}
		return nil
	}); err != nil {
		pushProxyStreamError(stream, partial, err.Error(), ctx)
		return
	}
	stream.End(partial)
}

func buildProxyRequestOptions(options llm.SimpleStreamOptions) proxySerializableStreamOptions {
	return proxySerializableStreamOptions{
		Temperature:     options.Temperature,
		MaxTokens:       options.MaxTokens,
		Reasoning:       options.Reasoning,
		CacheRetention:  options.CacheRetention,
		SessionID:       options.SessionID,
		Headers:         options.Headers,
		Metadata:        options.Metadata,
		Transport:       options.Transport,
		ThinkingBudgets: options.ThinkingBudgets,
		MaxRetryDelayMs: options.MaxRetryDelayMs,
	}
}

func mergeProxyStreamOptions(target *llm.SimpleStreamOptions, update llm.SimpleStreamOptions) {
	if update.Context != nil {
		target.Context = update.Context
	}
	if update.Temperature != nil {
		target.Temperature = update.Temperature
	}
	if update.MaxTokens != 0 {
		target.MaxTokens = update.MaxTokens
	}
	if update.APIKey != "" {
		target.APIKey = update.APIKey
	}
	if update.Transport != "" {
		target.Transport = update.Transport
	}
	if update.CacheRetention != "" {
		target.CacheRetention = update.CacheRetention
	}
	if update.SessionID != "" {
		target.SessionID = update.SessionID
	}
	if update.Reasoning != "" {
		target.Reasoning = update.Reasoning
	}
	if update.ThinkingBudgets != nil {
		target.ThinkingBudgets = update.ThinkingBudgets
	}
	if update.Headers != nil {
		target.Headers = update.Headers
	}
	if update.TimeoutMillis != 0 {
		target.TimeoutMillis = update.TimeoutMillis
	}
	if update.MaxRetries != 0 {
		target.MaxRetries = update.MaxRetries
	}
	if update.MaxRetryDelayMs != 0 {
		target.MaxRetryDelayMs = update.MaxRetryDelayMs
	}
	if update.Metadata != nil {
		target.Metadata = update.Metadata
	}
	if update.OnPayload != nil {
		target.OnPayload = update.OnPayload
	}
	if update.OnResponseStatus != nil {
		target.OnResponseStatus = update.OnResponseStatus
	}
}

func readProxySSE(reader io.Reader, handle func(ProxyAssistantMessageEvent) error) error {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if data == "" || data == "[DONE]" {
			continue
		}
		var event ProxyAssistantMessageEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return err
		}
		if err := handle(event); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func processProxyEvent(proxyEvent ProxyAssistantMessageEvent, partial *llm.Message, partialToolJSON map[int]string) (llm.AssistantMessageEvent, bool, error) {
	switch proxyEvent.Type {
	case "start":
		return llm.AssistantMessageEvent{Type: "start", Partial: *partial}, true, nil
	case "text_start":
		setProxyContentPart(partial, proxyEvent.ContentIndex, llm.Text(""))
		return llm.AssistantMessageEvent{Type: "text_start", ContentIndex: proxyEvent.ContentIndex, Partial: *partial}, true, nil
	case "text_delta":
		part, err := proxyContentPart(partial, proxyEvent.ContentIndex, llm.ContentText, "text_delta")
		if err != nil {
			return llm.AssistantMessageEvent{}, false, err
		}
		part.Text += proxyEvent.Delta
		return llm.AssistantMessageEvent{Type: "text_delta", ContentIndex: proxyEvent.ContentIndex, Delta: proxyEvent.Delta, Partial: *partial}, true, nil
	case "text_end":
		part, err := proxyContentPart(partial, proxyEvent.ContentIndex, llm.ContentText, "text_end")
		if err != nil {
			return llm.AssistantMessageEvent{}, false, err
		}
		part.TextSignature = proxyEvent.ContentSignature
		return llm.AssistantMessageEvent{Type: "text_end", ContentIndex: proxyEvent.ContentIndex, Content: part.Text, Partial: *partial}, true, nil
	case "thinking_start":
		setProxyContentPart(partial, proxyEvent.ContentIndex, llm.Thinking(""))
		return llm.AssistantMessageEvent{Type: "thinking_start", ContentIndex: proxyEvent.ContentIndex, Partial: *partial}, true, nil
	case "thinking_delta":
		part, err := proxyContentPart(partial, proxyEvent.ContentIndex, llm.ContentThinking, "thinking_delta")
		if err != nil {
			return llm.AssistantMessageEvent{}, false, err
		}
		part.Thinking += proxyEvent.Delta
		return llm.AssistantMessageEvent{Type: "thinking_delta", ContentIndex: proxyEvent.ContentIndex, Delta: proxyEvent.Delta, Partial: *partial}, true, nil
	case "thinking_end":
		part, err := proxyContentPart(partial, proxyEvent.ContentIndex, llm.ContentThinking, "thinking_end")
		if err != nil {
			return llm.AssistantMessageEvent{}, false, err
		}
		part.ThinkingSignature = proxyEvent.ContentSignature
		return llm.AssistantMessageEvent{Type: "thinking_end", ContentIndex: proxyEvent.ContentIndex, Content: part.Thinking, Partial: *partial}, true, nil
	case "toolcall_start":
		setProxyContentPart(partial, proxyEvent.ContentIndex, llm.ToolCall(proxyEvent.ID, proxyEvent.ToolName, map[string]any{}))
		partialToolJSON[proxyEvent.ContentIndex] = ""
		return llm.AssistantMessageEvent{Type: "toolcall_start", ContentIndex: proxyEvent.ContentIndex, Partial: *partial}, true, nil
	case "toolcall_delta":
		part, err := proxyContentPart(partial, proxyEvent.ContentIndex, llm.ContentToolCall, "toolcall_delta")
		if err != nil {
			return llm.AssistantMessageEvent{}, false, err
		}
		partialToolJSON[proxyEvent.ContentIndex] += proxyEvent.Delta
		part.Arguments = parseProxyStreamingJSONObject(partialToolJSON[proxyEvent.ContentIndex])
		return llm.AssistantMessageEvent{Type: "toolcall_delta", ContentIndex: proxyEvent.ContentIndex, Delta: proxyEvent.Delta, Partial: *partial}, true, nil
	case "toolcall_end":
		part, err := proxyContentPart(partial, proxyEvent.ContentIndex, llm.ContentToolCall, "toolcall_end")
		if err != nil {
			return llm.AssistantMessageEvent{}, false, err
		}
		delete(partialToolJSON, proxyEvent.ContentIndex)
		return llm.AssistantMessageEvent{Type: "toolcall_end", ContentIndex: proxyEvent.ContentIndex, ToolCall: *part, Partial: *partial}, true, nil
	case "done":
		partial.StopReason = proxyEvent.Reason
		if partial.StopReason == "" {
			partial.StopReason = llm.StopReasonStop
		}
		partial.Usage = proxyEvent.Usage
		return llm.AssistantMessageEvent{Type: "done", Reason: partial.StopReason, Message: *partial}, true, nil
	case "error":
		partial.StopReason = proxyEvent.Reason
		if partial.StopReason == "" {
			partial.StopReason = llm.StopReasonError
		}
		partial.ErrorMessage = proxyEvent.ErrorMessage
		partial.Usage = proxyEvent.Usage
		return llm.AssistantMessageEvent{Type: "error", Reason: partial.StopReason, Error: *partial}, true, nil
	default:
		return llm.AssistantMessageEvent{}, false, nil
	}
}

func setProxyContentPart(message *llm.Message, index int, part llm.ContentPart) {
	if index < 0 {
		return
	}
	for len(message.Content) <= index {
		message.Content = append(message.Content, llm.ContentPart{})
	}
	message.Content[index] = part
}

func proxyContentPart(message *llm.Message, index int, expectedType, eventType string) (*llm.ContentPart, error) {
	if index < 0 || index >= len(message.Content) || message.Content[index].Type != expectedType {
		return nil, fmt.Errorf("received %s for non-%s content", eventType, expectedType)
	}
	return &message.Content[index], nil
}

func parseProxyStreamingJSONObject(data string) map[string]any {
	var result map[string]any
	if err := llm.UnmarshalJSONWithRepair([]byte(data), &result); err == nil && result != nil {
		return result
	}
	return map[string]any{}
}

func pushProxyHTTPError(stream *llm.AssistantMessageEventStream, partial llm.Message, resp *http.Response, ctx context.Context) {
	errorMessage := fmt.Sprintf("Proxy error: %s", resp.Status)
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil && strings.TrimSpace(payload.Error) != "" {
		errorMessage = "Proxy error: " + payload.Error
	}
	pushProxyStreamError(stream, partial, errorMessage, ctx)
}

func pushProxyStreamError(stream *llm.AssistantMessageEventStream, partial llm.Message, message string, ctx context.Context) {
	reason := llm.StopReasonError
	if ctx != nil && ctx.Err() != nil {
		reason = llm.StopReasonAborted
	}
	partial.StopReason = reason
	partial.ErrorMessage = message
	stream.Push(llm.AssistantMessageEvent{Type: "error", Reason: reason, Error: partial})
}
