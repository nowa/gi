package gillmprovider

import (
	"encoding/json"
	"fmt"
)

const StopReasonToolUse = "toolUse"

type OpenAIChatCompletionChunk struct {
	ID      string                       `json:"id"`
	Choices []OpenAIChatCompletionChoice `json:"choices"`
	Usage   *OpenAIChatUsage             `json:"usage,omitempty"`
}

type OpenAIChatCompletionChoice struct {
	Delta        OpenAIChatDelta  `json:"delta"`
	FinishReason *string          `json:"finish_reason"`
	Usage        *OpenAIChatUsage `json:"usage,omitempty"`
}

type OpenAIChatDelta struct {
	Content          string                    `json:"content,omitempty"`
	ReasoningContent string                    `json:"reasoning_content,omitempty"`
	ToolCalls        []OpenAIChatToolCallDelta `json:"tool_calls,omitempty"`
}

type OpenAIChatToolCallDelta struct {
	Index    *int                            `json:"index,omitempty"`
	ID       string                          `json:"id,omitempty"`
	Type     string                          `json:"type,omitempty"`
	Function OpenAIChatToolCallFunctionDelta `json:"function"`
}

type OpenAIChatToolCallFunctionDelta struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type OpenAIChatUsage struct {
	PromptTokens           int                              `json:"prompt_tokens"`
	CompletionTokens       int                              `json:"completion_tokens"`
	PromptCacheHitTokens   int                              `json:"prompt_cache_hit_tokens"`
	PromptTokensDetails    OpenAIChatPromptTokenDetails     `json:"prompt_tokens_details"`
	CompletionTokenDetails OpenAIChatCompletionTokenDetails `json:"completion_tokens_details"`
}

type OpenAIChatPromptTokenDetails struct {
	CachedTokens     int `json:"cached_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
}

type OpenAIChatCompletionTokenDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

type OpenAICompletionsStreamProcessor struct {
	model      Model
	output     *Message
	textIndex  int
	thinkIndex int
	tools      map[string]*openAIChatToolAccumulator
}

type openAIChatToolAccumulator struct {
	contentIndex int
	id           string
	name         string
	argsJSON     string
}

func NewOpenAICompletionsStreamProcessor(model Model, output *Message) *OpenAICompletionsStreamProcessor {
	if output.API == "" {
		output.API = model.API
	}
	if output.Provider == "" {
		output.Provider = model.Provider
	}
	if output.Model == "" {
		output.Model = model.ID
	}
	if output.StopReason == "" {
		output.StopReason = StopReasonStop
	}
	return &OpenAICompletionsStreamProcessor{
		model:      model,
		output:     output,
		textIndex:  -1,
		thinkIndex: -1,
		tools:      map[string]*openAIChatToolAccumulator{},
	}
}

func (p *OpenAICompletionsStreamProcessor) Process(chunk *OpenAIChatCompletionChunk) []AssistantMessageEvent {
	if chunk == nil {
		return nil
	}
	if chunk.ID != "" {
		p.output.ResponseID = chunk.ID
	}
	if chunk.Usage != nil {
		p.output.Usage = ParseOpenAIChatUsage(*chunk.Usage, p.model)
	}
	var events []AssistantMessageEvent
	for _, choice := range chunk.Choices {
		if choice.Usage != nil {
			p.output.Usage = ParseOpenAIChatUsage(*choice.Usage, p.model)
		}
		if choice.Delta.Content != "" {
			events = append(events, p.appendText(choice.Delta.Content)...)
		}
		if choice.Delta.ReasoningContent != "" {
			events = append(events, p.appendThinking(choice.Delta.ReasoningContent)...)
		}
		for _, delta := range choice.Delta.ToolCalls {
			events = append(events, p.appendToolCall(delta)...)
		}
		if choice.FinishReason != nil {
			stopReason, errorMessage := MapOpenAIChatFinishReason(*choice.FinishReason)
			p.output.StopReason = stopReason
			p.output.ErrorMessage = errorMessage
			for _, tool := range p.tools {
				if tool.contentIndex >= 0 && tool.contentIndex < len(p.output.Content) {
					p.output.Content[tool.contentIndex].Arguments = parseStreamingJSONObject(tool.argsJSON)
				}
			}
		}
	}
	return events
}

func (p *OpenAICompletionsStreamProcessor) Result() Message {
	if p.output.StopReason == StopReasonStop && p.output.ErrorMessage == "" {
		// A stream that produced content but never received a finish reason is incomplete.
		p.output.StopReason = StopReasonError
		p.output.ErrorMessage = "Stream ended without finish_reason"
	}
	return *p.output
}

func ProcessOpenAICompletionsChunks(model Model, chunks []*OpenAIChatCompletionChunk) Message {
	output := AssistantMessage(nil, StopReasonStop, model)
	processor := NewOpenAICompletionsStreamProcessor(model, &output)
	hadFinishReason := false
	for _, chunk := range chunks {
		if chunk != nil {
			for _, choice := range chunk.Choices {
				if choice.FinishReason != nil {
					hadFinishReason = true
					break
				}
			}
		}
		processor.Process(chunk)
	}
	if hadFinishReason {
		return output
	}
	return processor.Result()
}

func ParseOpenAIChatUsage(raw OpenAIChatUsage, model Model) Usage {
	promptTokens := raw.PromptTokens
	cacheReadTokens := raw.PromptTokensDetails.CachedTokens
	if cacheReadTokens == 0 {
		cacheReadTokens = raw.PromptCacheHitTokens
	}
	cacheWriteTokens := raw.PromptTokensDetails.CacheWriteTokens
	input := promptTokens - cacheReadTokens - cacheWriteTokens
	if input < 0 {
		input = 0
	}
	usage := Usage{
		Input:       input,
		Output:      raw.CompletionTokens,
		CacheRead:   cacheReadTokens,
		CacheWrite:  cacheWriteTokens,
		TotalTokens: input + raw.CompletionTokens + cacheReadTokens + cacheWriteTokens,
	}
	usage.Cost = CalculateCost(model, usage)
	return usage
}

func MapOpenAIChatFinishReason(reason string) (string, string) {
	switch reason {
	case "", "stop", "end":
		return StopReasonStop, ""
	case "length":
		return StopReasonLength, ""
	case "function_call", "tool_calls":
		return StopReasonToolUse, ""
	case "content_filter", "network_error":
		return StopReasonError, "Provider finish_reason: " + reason
	default:
		return StopReasonError, "Provider finish_reason: " + reason
	}
}

func (p *OpenAICompletionsStreamProcessor) appendText(delta string) []AssistantMessageEvent {
	if p.textIndex < 0 {
		p.output.Content = append(p.output.Content, Text(""))
		p.textIndex = len(p.output.Content) - 1
	}
	p.output.Content[p.textIndex].Text += SanitizeSurrogates(delta)
	return []AssistantMessageEvent{{Type: "text_delta", Partial: *p.output}}
}

func (p *OpenAICompletionsStreamProcessor) appendThinking(delta string) []AssistantMessageEvent {
	if p.thinkIndex < 0 {
		part := Thinking("")
		part.ThinkingSignature = "reasoning_content"
		p.output.Content = append(p.output.Content, part)
		p.thinkIndex = len(p.output.Content) - 1
	}
	p.output.Content[p.thinkIndex].Thinking += SanitizeSurrogates(delta)
	return []AssistantMessageEvent{{Type: "thinking_delta", Partial: *p.output}}
}

func (p *OpenAICompletionsStreamProcessor) appendToolCall(delta OpenAIChatToolCallDelta) []AssistantMessageEvent {
	key := openAIToolCallDeltaKey(delta)
	acc := p.tools[key]
	if acc == nil {
		acc = &openAIChatToolAccumulator{id: delta.ID, name: delta.Function.Name}
		if acc.id == "" {
			acc.id = fmt.Sprintf("tool_%d", len(p.tools))
		}
		part := ToolCall(acc.id, acc.name, map[string]any{})
		p.output.Content = append(p.output.Content, part)
		acc.contentIndex = len(p.output.Content) - 1
		p.tools[key] = acc
	} else if p.output.Content[acc.contentIndex].ID == "" && delta.ID != "" {
		acc.id = delta.ID
		p.output.Content[acc.contentIndex].ID = delta.ID
	}
	if acc.name == "" && delta.Function.Name != "" {
		acc.name = delta.Function.Name
		p.output.Content[acc.contentIndex].Name = delta.Function.Name
	}
	acc.argsJSON += delta.Function.Arguments
	p.output.Content[acc.contentIndex].Arguments = parseStreamingJSONObject(acc.argsJSON)
	return []AssistantMessageEvent{{Type: "toolcall_delta", Partial: *p.output}}
}

func openAIToolCallDeltaKey(delta OpenAIChatToolCallDelta) string {
	if delta.Index != nil {
		return fmt.Sprintf("index:%d", *delta.Index)
	}
	if delta.ID != "" {
		return "id:" + delta.ID
	}
	nameArgs, _ := json.Marshal(delta.Function)
	return "anonymous:" + string(nameArgs)
}
