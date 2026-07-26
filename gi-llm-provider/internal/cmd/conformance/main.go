package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	gillmprovider "github.com/nowa/gi/gi-llm-provider"
)

const schemaVersion = 1

var errPayloadCaptured = errors.New("pi parity payload captured")

type envelope struct {
	SchemaVersion int             `json:"schemaVersion"`
	ID            string          `json:"id"`
	Kind          string          `json:"kind"`
	Input         json.RawMessage `json:"input"`
}

type resultEnvelope struct {
	SchemaVersion int          `json:"schemaVersion"`
	ID            string       `json:"id"`
	Kind          string       `json:"kind"`
	Output        any          `json:"output,omitempty"`
	Error         *resultError `json:"error,omitempty"`
}

type resultError struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

type costInput struct {
	Model gillmprovider.Model `json:"model"`
	Usage gillmprovider.Usage `json:"usage"`
}

type payloadInput struct {
	API     string                   `json:"api"`
	Model   gillmprovider.Model      `json:"model"`
	Context gillmprovider.Context    `json:"context"`
	Options conformanceStreamOptions `json:"options"`
}

type conformanceStreamOptions struct {
	Temperature     *float64                  `json:"temperature,omitempty"`
	MaxTokens       int                       `json:"maxTokens,omitempty"`
	Transport       string                    `json:"transport,omitempty"`
	CacheRetention  string                    `json:"cacheRetention,omitempty"`
	SessionID       string                    `json:"sessionId,omitempty"`
	Reasoning       string                    `json:"reasoning,omitempty"`
	ToolChoice      any                       `json:"toolChoice,omitempty"`
	ThinkingBudgets map[string]int            `json:"thinkingBudgets,omitempty"`
	Headers         map[string]string         `json:"headers,omitempty"`
	Env             gillmprovider.ProviderEnv `json:"env,omitempty"`
	Metadata        map[string]any            `json:"metadata,omitempty"`
}

type streamInput struct {
	API          string                   `json:"api"`
	Model        gillmprovider.Model      `json:"model"`
	Context      gillmprovider.Context    `json:"context"`
	Options      conformanceStreamOptions `json:"options"`
	Body         string                   `json:"body"`
	ChunkPattern []int                    `json:"chunkPattern,omitempty"`
	Events       []bedrockFixtureEvent    `json:"events,omitempty"`
}

type bedrockFixtureEvent struct {
	MessageStart      *gillmprovider.BedrockMessageStartEvent `json:"messageStart,omitempty"`
	ContentBlockStart *struct {
		ContentBlockIndex int `json:"contentBlockIndex"`
		Start             *struct {
			ToolUse *gillmprovider.BedrockToolUseBlock `json:"toolUse,omitempty"`
		} `json:"start,omitempty"`
	} `json:"contentBlockStart,omitempty"`
	ContentBlockDelta *struct {
		ContentBlockIndex int `json:"contentBlockIndex"`
		Delta             *struct {
			Text    string `json:"text,omitempty"`
			ToolUse *struct {
				Input string `json:"input,omitempty"`
			} `json:"toolUse,omitempty"`
			ReasoningContent *gillmprovider.BedrockReasoningContent `json:"reasoningContent,omitempty"`
		} `json:"delta,omitempty"`
	} `json:"contentBlockDelta,omitempty"`
	ContentBlockStop *gillmprovider.BedrockContentBlockStopEvent `json:"contentBlockStop,omitempty"`
	MessageStop      *gillmprovider.BedrockMessageStopEvent      `json:"messageStop,omitempty"`
	Metadata         *struct {
		Usage *struct {
			InputTokens           int `json:"inputTokens"`
			OutputTokens          int `json:"outputTokens"`
			CacheReadInputTokens  int `json:"cacheReadInputTokens"`
			CacheWriteInputTokens int `json:"cacheWriteInputTokens"`
			TotalTokens           int `json:"totalTokens"`
		} `json:"usage,omitempty"`
	} `json:"metadata,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var request envelope
		if err := json.Unmarshal([]byte(line), &request); err != nil {
			return fmt.Errorf("decode conformance case: %w", err)
		}
		result := execute(request)
		if err := encoder.Encode(result); err != nil {
			return fmt.Errorf("encode conformance result: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read conformance cases: %w", err)
	}
	return nil
}

func execute(request envelope) (result resultEnvelope) {
	result = resultEnvelope{
		SchemaVersion: schemaVersion,
		ID:            request.ID,
		Kind:          request.Kind,
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			result.Output = nil
			result.Error = &resultError{
				Name:    "panic",
				Message: fmt.Sprint(recovered),
			}
		}
	}()
	if request.SchemaVersion != schemaVersion {
		result.Error = &resultError{
			Name:    "SchemaVersionError",
			Message: fmt.Sprintf("unsupported schemaVersion %d", request.SchemaVersion),
		}
		return result
	}

	var err error
	switch request.Kind {
	case "cost":
		result.Output, err = executeCost(request.Input)
	case "payload":
		result.Output, err = executePayload(request.Input)
	case "stream":
		result.Output, err = executeStream(request.Input)
	default:
		err = fmt.Errorf("unsupported conformance kind %q", request.Kind)
	}
	if err != nil {
		result.Output = nil
		result.Error = &resultError{
			Name:    errorName(err),
			Message: err.Error(),
		}
	}
	return result
}

func executeCost(raw json.RawMessage) (any, error) {
	var input costInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, fmt.Errorf("decode cost input: %w", err)
	}
	return gillmprovider.CalculateCost(input.Model, input.Usage), nil
}

func executePayload(raw json.RawMessage) (any, error) {
	var input payloadInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, fmt.Errorf("decode payload input: %w", err)
	}
	if input.API == "" {
		input.API = input.Model.API
	}
	if input.Model.API == "" {
		input.Model.API = input.API
	}
	implementation := gillmprovider.GetAPIProvider(input.API)
	if implementation == nil {
		return nil, fmt.Errorf("no Gi provider registered for API %q", input.API)
	}

	captured := make(chan any, 1)
	options := gillmprovider.SimpleStreamOptions{
		Context:         context.Background(),
		Temperature:     input.Options.Temperature,
		MaxTokens:       input.Options.MaxTokens,
		APIKey:          conformanceAPIKey(input.API),
		Transport:       input.Options.Transport,
		CacheRetention:  input.Options.CacheRetention,
		SessionID:       input.Options.SessionID,
		Reasoning:       input.Options.Reasoning,
		ToolChoice:      input.Options.ToolChoice,
		ThinkingBudgets: input.Options.ThinkingBudgets,
		Headers:         input.Options.Headers,
		Env:             input.Options.Env,
		Metadata:        input.Options.Metadata,
		OnPayload: func(payload any, _ gillmprovider.Model) (any, bool, error) {
			captured <- payload
			return nil, false, errPayloadCaptured
		},
	}
	stream, streamErr := implementation.StreamSimple(input.Model, input.Context, options)
	if streamErr != nil {
		return nil, streamErr
	}

	select {
	case payload := <-captured:
		return canonicalPayload(input.API, payload)
	case <-time.After(5 * time.Second):
		if stream != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			if message, resultErr := stream.Result(ctx); resultErr == nil && message.ErrorMessage != "" {
				return nil, fmt.Errorf("payload was not captured: %s", message.ErrorMessage)
			}
		}
		return nil, errors.New("timed out waiting for payload capture")
	}
}

func executeStream(raw json.RawMessage) (any, error) {
	var input streamInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, fmt.Errorf("decode stream input: %w", err)
	}
	if input.API == "" {
		input.API = input.Model.API
	}
	if input.Model.API == "" {
		input.Model.API = input.API
	}
	implementation := streamImplementation(input)
	if implementation == nil {
		return nil, fmt.Errorf("no Gi provider registered for API %q", input.API)
	}
	for _, size := range input.ChunkPattern {
		if size <= 0 {
			return nil, fmt.Errorf("chunkPattern values must be positive, got %d", size)
		}
	}

	env := input.Options.Env
	if input.API == "bedrock-converse-stream" {
		env = cloneProviderEnv(env)
		env["AWS_BEDROCK_SKIP_AUTH"] = "1"
	}
	options := gillmprovider.SimpleStreamOptions{
		Context:         context.Background(),
		Temperature:     input.Options.Temperature,
		MaxTokens:       input.Options.MaxTokens,
		APIKey:          conformanceAPIKey(input.API),
		Transport:       input.Options.Transport,
		CacheRetention:  input.Options.CacheRetention,
		SessionID:       input.Options.SessionID,
		Reasoning:       input.Options.Reasoning,
		ToolChoice:      input.Options.ToolChoice,
		ThinkingBudgets: input.Options.ThinkingBudgets,
		Headers:         input.Options.Headers,
		Env:             env,
		Metadata:        input.Options.Metadata,
		HTTPClient: &fixtureHTTPClient{
			body:         []byte(input.Body),
			chunkPattern: append([]int(nil), input.ChunkPattern...),
		},
	}
	stream, streamErr := implementation.StreamSimple(input.Model, input.Context, options)
	if streamErr != nil {
		return nil, streamErr
	}
	if stream == nil {
		return nil, errors.New("Gi provider returned a nil stream")
	}

	type collection struct {
		events []any
		err    error
	}
	eventsCh := make(chan collection, 1)
	go func() {
		var events []any
		for event := range stream.Events() {
			value, err := toJSONValue(event)
			if err != nil {
				eventsCh <- collection{err: err}
				return
			}
			events = append(events, removeVolatileFields(value))
		}
		eventsCh <- collection{events: events}
	}()

	var events []any
	select {
	case collected := <-eventsCh:
		if collected.err != nil {
			return nil, collected.err
		}
		events = collected.events
	case <-time.After(5 * time.Second):
		return nil, errors.New("timed out collecting Gi stream events")
	}
	resultContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	message, err := stream.Result(resultContext)
	if err != nil {
		return nil, err
	}
	output, err := toJSONValue(map[string]any{
		"events": events,
		"result": message,
	})
	if err != nil {
		return nil, err
	}
	return removeVolatileFields(output), nil
}

func streamImplementation(input streamInput) gillmprovider.APIProvider {
	if input.API != "bedrock-converse-stream" {
		return gillmprovider.GetAPIProvider(input.API)
	}
	events := append([]bedrockFixtureEvent(nil), input.Events...)
	return gillmprovider.NewBedrockConverseStreamProvider(
		func(
			context.Context,
			gillmprovider.BedrockConverseStreamRequest,
		) (<-chan gillmprovider.BedrockConverseStreamEvent, error) {
			stream := make(
				chan gillmprovider.BedrockConverseStreamEvent,
				len(events),
			)
			for _, event := range events {
				stream <- event.providerEvent()
			}
			close(stream)
			return stream, nil
		},
	)
}

func (event bedrockFixtureEvent) providerEvent() gillmprovider.BedrockConverseStreamEvent {
	result := gillmprovider.BedrockConverseStreamEvent{
		MessageStart:     event.MessageStart,
		ContentBlockStop: event.ContentBlockStop,
		MessageStop:      event.MessageStop,
	}
	if event.ContentBlockStart != nil {
		result.ContentBlockStart = &gillmprovider.BedrockContentBlockStartEvent{
			ContentBlockIndex: event.ContentBlockStart.ContentBlockIndex,
		}
		if event.ContentBlockStart.Start != nil {
			result.ContentBlockStart.ToolUse =
				event.ContentBlockStart.Start.ToolUse
		}
	}
	if event.ContentBlockDelta != nil {
		result.ContentBlockDelta = &gillmprovider.BedrockContentBlockDeltaEvent{
			ContentBlockIndex: event.ContentBlockDelta.ContentBlockIndex,
		}
		if event.ContentBlockDelta.Delta != nil {
			result.ContentBlockDelta.Text =
				event.ContentBlockDelta.Delta.Text
			result.ContentBlockDelta.ReasoningContent =
				event.ContentBlockDelta.Delta.ReasoningContent
			if event.ContentBlockDelta.Delta.ToolUse != nil {
				result.ContentBlockDelta.ToolUseInput =
					event.ContentBlockDelta.Delta.ToolUse.Input
			}
		}
	}
	if event.Metadata != nil && event.Metadata.Usage != nil {
		usage := event.Metadata.Usage
		result.Metadata = &gillmprovider.BedrockMetadataEvent{
			Usage: gillmprovider.BedrockUsage{
				InputTokens:          usage.InputTokens,
				OutputTokens:         usage.OutputTokens,
				CacheReadInputTokens: usage.CacheReadInputTokens,
				CacheWriteTokens:     usage.CacheWriteInputTokens,
				TotalTokens:          usage.TotalTokens,
			},
		}
	}
	return result
}

func cloneProviderEnv(env gillmprovider.ProviderEnv) gillmprovider.ProviderEnv {
	cloned := make(gillmprovider.ProviderEnv, len(env)+1)
	for key, value := range env {
		cloned[key] = value
	}
	return cloned
}

type fixtureHTTPClient struct {
	body         []byte
	chunkPattern []int
}

func (c *fixtureHTTPClient) Do(request *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode:    http.StatusOK,
		Status:        "200 OK",
		Header:        http.Header{"content-type": []string{"text/event-stream"}},
		Body:          &patternReadCloser{remaining: append([]byte(nil), c.body...), pattern: c.chunkPattern},
		ContentLength: -1,
		Request:       request,
	}, nil
}

type patternReadCloser struct {
	remaining []byte
	pattern   []int
	next      int
	closed    bool
}

func (r *patternReadCloser) Read(target []byte) (int, error) {
	if r.closed || len(r.remaining) == 0 {
		return 0, io.EOF
	}
	size := len(r.remaining)
	if len(r.pattern) > 0 {
		size = min(size, r.pattern[r.next%len(r.pattern)])
		r.next++
	}
	size = min(size, len(target))
	copy(target, r.remaining[:size])
	r.remaining = r.remaining[size:]
	return size, nil
}

func (r *patternReadCloser) Close() error {
	r.closed = true
	return nil
}

func removeVolatileFields(value any) any {
	switch typed := value.(type) {
	case []any:
		for index, child := range typed {
			typed[index] = removeVolatileFields(child)
		}
	case map[string]any:
		delete(typed, "timestamp")
		if typed["cacheWrite1h"] == float64(0) {
			delete(typed, "cacheWrite1h")
		}
		for key, child := range typed {
			typed[key] = removeVolatileFields(child)
		}
	}
	return value
}

func conformanceAPIKey(api string) string {
	if api == "openai-codex-responses" {
		return "e30.eyJodHRwczovL2FwaS5vcGVuYWkuY29tL2F1dGgiOnsiY2hhdGdwdF9hY2NvdW50X2lkIjoiYWNjdC1waS1wYXJpdHkifX0.eA"
	}
	return "pi-parity-key"
}

func canonicalPayload(api string, payload any) (any, error) {
	switch api {
	case "bedrock-converse-stream":
		typed, ok := payload.(gillmprovider.BedrockPayload)
		if !ok {
			return nil, fmt.Errorf("unexpected Bedrock payload type %T", payload)
		}
		return canonicalBedrockPayload(typed), nil
	case "google-generative-ai", "google-vertex":
		typed, ok := payload.(gillmprovider.GooglePayload)
		if !ok {
			return nil, fmt.Errorf("unexpected Google payload type %T", payload)
		}
		return canonicalGooglePayload(typed), nil
	default:
		return toJSONValue(payload)
	}
}

func canonicalGooglePayload(payload gillmprovider.GooglePayload) map[string]any {
	result := map[string]any{
		"contents": payload.Contents,
		"config":   payload.Config,
		"model":    payload.Model,
	}
	if payload.SystemInstruction != nil {
		result["systemInstruction"] = payload.SystemInstruction
	}
	if len(payload.Tools) > 0 {
		result["tools"] = payload.Tools
	}
	if payload.ToolConfig != nil {
		result["toolConfig"] = payload.ToolConfig
	}
	return mustJSONValue(result).(map[string]any)
}

func canonicalBedrockPayload(payload gillmprovider.BedrockPayload) map[string]any {
	result := map[string]any{}
	if len(payload.System) > 0 {
		result["system"] = canonicalBedrockContent(payload.System)
	}
	if len(payload.Messages) > 0 {
		messages := make([]any, 0, len(payload.Messages))
		for _, message := range payload.Messages {
			messages = append(messages, map[string]any{
				"role":    message.Role,
				"content": canonicalBedrockContent(message.Content),
			})
		}
		result["messages"] = messages
	}
	if payload.ToolConfig != nil {
		tools := make([]any, 0, len(payload.ToolConfig.Tools))
		for _, tool := range payload.ToolConfig.Tools {
			spec := map[string]any{
				"name": tool.ToolSpec.Name,
				"inputSchema": map[string]any{
					"json": tool.ToolSpec.InputSchema.JSON,
				},
			}
			if tool.ToolSpec.Description != "" {
				spec["description"] = tool.ToolSpec.Description
			}
			if tool.ToolSpec.Strict != nil {
				spec["strict"] = *tool.ToolSpec.Strict
			}
			tools = append(tools, map[string]any{"toolSpec": spec})
		}
		toolConfig := map[string]any{"tools": tools}
		if payload.ToolConfig.ToolChoice != nil {
			toolConfig["toolChoice"] = payload.ToolConfig.ToolChoice
		}
		result["toolConfig"] = toolConfig
	}
	if len(payload.AdditionalModelRequestFields) > 0 {
		result["additionalModelRequestFields"] = payload.AdditionalModelRequestFields
	}
	return mustJSONValue(result).(map[string]any)
}

func canonicalBedrockContent(blocks []gillmprovider.BedrockContentBlock) []any {
	result := make([]any, 0, len(blocks))
	for _, block := range blocks {
		switch {
		case block.Text != "":
			result = append(result, map[string]any{"text": block.Text})
		case block.Image != nil:
			result = append(result, map[string]any{
				"image": map[string]any{
					"format": block.Image.Format,
					"source": map[string]any{"bytes": block.Image.Data},
				},
			})
		case block.ToolUse != nil:
			result = append(result, map[string]any{
				"toolUse": map[string]any{
					"toolUseId": block.ToolUse.ToolUseID,
					"name":      block.ToolUse.Name,
					"input":     block.ToolUse.Input,
				},
			})
		case block.ToolResult != nil:
			toolResult := map[string]any{
				"toolUseId": block.ToolResult.ToolUseID,
				"content":   canonicalBedrockContent(block.ToolResult.Content),
			}
			if block.ToolResult.Status != "" {
				toolResult["status"] = block.ToolResult.Status
			}
			result = append(result, map[string]any{"toolResult": toolResult})
		case block.ReasoningContent != nil:
			result = append(result, map[string]any{
				"reasoningContent": map[string]any{
					"reasoningText": map[string]any{
						"text":      block.ReasoningContent.Text,
						"signature": block.ReasoningContent.Signature,
					},
				},
			})
		case block.CachePoint != nil:
			cachePoint := map[string]any{"type": block.CachePoint.Type}
			if block.CachePoint.TTL != "" {
				cachePoint["ttl"] = block.CachePoint.TTL
			}
			result = append(result, map[string]any{"cachePoint": cachePoint})
		}
	}
	return result
}

func toJSONValue(value any) (any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func mustJSONValue(value any) any {
	converted, err := toJSONValue(value)
	if err != nil {
		panic(err)
	}
	return converted
}

func errorName(err error) string {
	if err == nil {
		return ""
	}
	name := fmt.Sprintf("%T", err)
	name = strings.TrimPrefix(name, "*")
	if last := strings.LastIndexByte(name, '.'); last >= 0 {
		name = name[last+1:]
	}
	return name
}
