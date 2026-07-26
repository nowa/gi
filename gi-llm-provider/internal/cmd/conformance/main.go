package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	CacheRetention  string                    `json:"cacheRetention,omitempty"`
	SessionID       string                    `json:"sessionId,omitempty"`
	Reasoning       string                    `json:"reasoning,omitempty"`
	ToolChoice      any                       `json:"toolChoice,omitempty"`
	ThinkingBudgets map[string]int            `json:"thinkingBudgets,omitempty"`
	Headers         map[string]string         `json:"headers,omitempty"`
	Env             gillmprovider.ProviderEnv `json:"env,omitempty"`
	Metadata        map[string]any            `json:"metadata,omitempty"`
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
