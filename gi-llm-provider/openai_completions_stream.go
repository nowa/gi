package gillmprovider

import (
	"encoding/json"
	"fmt"
)

const StopReasonToolUse = "toolUse"

type OpenAIChatCompletionChunk struct {
	ID      string                       `json:"id"`
	Model   string                       `json:"model,omitempty"`
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
	Reasoning        string                    `json:"reasoning,omitempty"`
	ReasoningText    string                    `json:"reasoning_text,omitempty"`
	ReasoningDetails []json.RawMessage         `json:"reasoning_details,omitempty"`
	ToolCalls        []OpenAIChatToolCallDelta `json:"tool_calls,omitempty"`
}

type OpenAIChatToolCallDelta struct {
	Index    *int                            `json:"index,omitempty"`
	ID       string                          `json:"id,omitempty"`
	Type     string                          `json:"type,omitempty"`
	Function OpenAIChatToolCallFunctionDelta `json:"function"`
	Custom   *OpenAIChatCustomToolCallDelta  `json:"custom,omitempty"`
}

type OpenAIChatToolCallFunctionDelta struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type OpenAIChatCustomToolCallDelta struct {
	Name  string `json:"name,omitempty"`
	Input string `json:"input,omitempty"`
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
	model                      Model
	output                     *Message
	grammarToolInputProperties map[string]string
	textIndex                  int
	thinkIndex                 int
	tools                      map[string]*openAIChatToolAccumulator
	toolsByID                  map[string]*openAIChatToolAccumulator
	pendingReasoningDetails    map[string]string
	textEnded                  bool
	thinkEnded                 bool
}

type openAIChatToolAccumulator struct {
	contentIndex  int
	id            string
	name          string
	argsJSON      string
	customInput   *openAIChatCustomInputAccumulator
	hasProviderID bool
	ended         bool
}

type openAIChatCustomInputAccumulator struct {
	property string
	buffer   GrammarToolInputJSONBuffer
}

type OpenAICompletionsStreamProcessorOptions struct {
	GrammarToolInputProperties map[string]string
}

func NewOpenAICompletionsStreamProcessor(model Model, output *Message) *OpenAICompletionsStreamProcessor {
	return NewOpenAICompletionsStreamProcessorWithOptions(
		model,
		output,
		OpenAICompletionsStreamProcessorOptions{},
	)
}

func NewOpenAICompletionsStreamProcessorWithOptions(
	model Model,
	output *Message,
	options OpenAICompletionsStreamProcessorOptions,
) *OpenAICompletionsStreamProcessor {
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
		model:                      model,
		output:                     output,
		grammarToolInputProperties: options.GrammarToolInputProperties,
		textIndex:                  -1,
		thinkIndex:                 -1,
		tools:                      map[string]*openAIChatToolAccumulator{},
		toolsByID:                  map[string]*openAIChatToolAccumulator{},
		pendingReasoningDetails:    map[string]string{},
	}
}

func (p *OpenAICompletionsStreamProcessor) Process(chunk *OpenAIChatCompletionChunk) []AssistantMessageEvent {
	if chunk == nil {
		return nil
	}
	if chunk.ID != "" {
		p.output.ResponseID = chunk.ID
	}
	if chunk.Model != "" && p.output.ResponseModel == "" {
		p.output.ResponseModel = chunk.Model
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
		if field, delta := openAIChatReasoningDelta(choice.Delta); delta != "" {
			events = append(events, p.appendThinking(field, delta)...)
		}
		for _, delta := range choice.Delta.ToolCalls {
			events = append(events, p.appendToolCall(delta)...)
		}
		p.applyReasoningDetails(choice.Delta.ReasoningDetails)
		if choice.FinishReason != nil {
			stopReason, errorMessage := MapOpenAIChatFinishReason(*choice.FinishReason)
			p.output.StopReason = stopReason
			p.output.ErrorMessage = errorMessage
			events = append(events, p.finishOpenContentBlocks()...)
		}
	}
	// Pi emits all events derived from one provider chunk synchronously. Each
	// event therefore observes the state after that whole chunk, but must not be
	// mutated by a later chunk.
	for index := range events {
		if events[index].Partial.Role != "" {
			events[index].Partial = cloneMessageState(*p.output)
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
	return ProcessOpenAICompletionsChunksWithOptions(
		model,
		chunks,
		OpenAICompletionsStreamProcessorOptions{},
	)
}

func ProcessOpenAICompletionsChunksWithOptions(
	model Model,
	chunks []*OpenAIChatCompletionChunk,
	options OpenAICompletionsStreamProcessorOptions,
) Message {
	output := AssistantMessage(nil, StopReasonStop, model)
	processor := NewOpenAICompletionsStreamProcessorWithOptions(model, &output, options)
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
		Reasoning:   ptrInt(raw.CompletionTokenDetails.ReasoningTokens),
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
	var events []AssistantMessageEvent
	if p.textIndex < 0 {
		p.output.Content = append(p.output.Content, Text(""))
		p.textIndex = len(p.output.Content) - 1
		p.textEnded = false
		events = append(events, AssistantMessageEvent{Type: "text_start", ContentIndex: p.textIndex, Partial: cloneMessageState(*p.output)})
	}
	p.output.Content[p.textIndex].Text += SanitizeSurrogates(delta)
	events = append(events, AssistantMessageEvent{Type: "text_delta", ContentIndex: p.textIndex, Delta: delta, Partial: cloneMessageState(*p.output)})
	return events
}

func openAIChatReasoningDelta(delta OpenAIChatDelta) (string, string) {
	switch {
	case delta.ReasoningContent != "":
		return "reasoning_content", delta.ReasoningContent
	case delta.Reasoning != "":
		return "reasoning", delta.Reasoning
	case delta.ReasoningText != "":
		return "reasoning_text", delta.ReasoningText
	default:
		return "", ""
	}
}

func (p *OpenAICompletionsStreamProcessor) appendThinking(signature, delta string) []AssistantMessageEvent {
	var events []AssistantMessageEvent
	if p.thinkIndex < 0 {
		part := Thinking("")
		part.ThinkingSignature = signature
		p.output.Content = append(p.output.Content, part)
		p.thinkIndex = len(p.output.Content) - 1
		p.thinkEnded = false
		events = append(events, AssistantMessageEvent{Type: "thinking_start", ContentIndex: p.thinkIndex, Partial: cloneMessageState(*p.output)})
	}
	p.output.Content[p.thinkIndex].Thinking += SanitizeSurrogates(delta)
	events = append(events, AssistantMessageEvent{Type: "thinking_delta", ContentIndex: p.thinkIndex, Delta: delta, Partial: cloneMessageState(*p.output)})
	return events
}

func (p *OpenAICompletionsStreamProcessor) appendToolCall(delta OpenAIChatToolCallDelta) []AssistantMessageEvent {
	key := openAIToolCallDeltaKey(delta)
	acc := p.tools[key]
	var events []AssistantMessageEvent
	name := delta.Function.Name
	if name == "" && delta.Custom != nil {
		name = delta.Custom.Name
	}
	if acc == nil {
		acc = &openAIChatToolAccumulator{
			id:            delta.ID,
			name:          name,
			hasProviderID: delta.ID != "",
		}
		if acc.id == "" {
			acc.id = fmt.Sprintf("tool_%d", len(p.tools))
		}
		part := ToolCall(acc.id, acc.name, map[string]any{})
		p.output.Content = append(p.output.Content, part)
		acc.contentIndex = len(p.output.Content) - 1
		p.tools[key] = acc
		if delta.Custom != nil {
			p.configureOpenAIChatCustomInput(acc)
		}
		p.registerOpenAIChatToolCallID(acc, delta.ID)
		events = append(events, AssistantMessageEvent{Type: "toolcall_start", ContentIndex: acc.contentIndex, Partial: cloneMessageState(*p.output)})
	} else if !acc.hasProviderID && delta.ID != "" {
		acc.id = delta.ID
		acc.hasProviderID = true
		p.output.Content[acc.contentIndex].ID = delta.ID
		p.registerOpenAIChatToolCallID(acc, delta.ID)
	}
	if acc.name == "" && name != "" {
		acc.name = name
		p.output.Content[acc.contentIndex].Name = name
	}
	if delta.Custom != nil && acc.customInput == nil {
		p.configureOpenAIChatCustomInput(acc)
	}

	emittedDelta := ""
	switch {
	case delta.Function.Arguments != "":
		acc.argsJSON += delta.Function.Arguments
		p.output.Content[acc.contentIndex].Arguments = parseStreamingJSONObject(acc.argsJSON)
		emittedDelta = delta.Function.Arguments
	case delta.Custom != nil && delta.Custom.Input != "":
		nextInput := p.openAIChatCustomInput(acc) + delta.Custom.Input
		var ok bool
		var err error
		emittedDelta, ok, err = p.appendOpenAIChatCustomInput(acc, nextInput, false)
		if err != nil {
			return p.openAIChatCustomInputError(err)
		}
		if !ok {
			emittedDelta = ""
		}
	}
	events = append(events, AssistantMessageEvent{
		Type:         "toolcall_delta",
		ContentIndex: acc.contentIndex,
		Delta:        emittedDelta,
		Partial:      cloneMessageState(*p.output),
	})
	return events
}

func (p *OpenAICompletionsStreamProcessor) registerOpenAIChatToolCallID(
	acc *openAIChatToolAccumulator,
	id string,
) {
	if acc == nil || id == "" {
		return
	}
	p.toolsByID[id] = acc
	if signature, ok := p.pendingReasoningDetails[id]; ok {
		p.output.Content[acc.contentIndex].ThoughtSignature = signature
		delete(p.pendingReasoningDetails, id)
	}
}

func (p *OpenAICompletionsStreamProcessor) applyReasoningDetails(
	details []json.RawMessage,
) {
	for _, raw := range details {
		id, signature, ok := parseOpenAIEncryptedReasoningDetail(raw)
		if !ok {
			continue
		}
		if acc := p.toolsByID[id]; acc != nil {
			p.output.Content[acc.contentIndex].ThoughtSignature = signature
			continue
		}
		p.pendingReasoningDetails[id] = signature
	}
}

func parseOpenAIEncryptedReasoningDetail(
	raw json.RawMessage,
) (id string, signature string, ok bool) {
	var detail struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Data string `json:"data"`
	}
	if json.Unmarshal(raw, &detail) != nil ||
		detail.Type != "reasoning.encrypted" ||
		detail.ID == "" ||
		detail.Data == "" {
		return "", "", false
	}
	compacted, err := json.Marshal(json.RawMessage(raw))
	if err != nil {
		return "", "", false
	}
	return detail.ID, string(compacted), true
}

func (p *OpenAICompletionsStreamProcessor) configureOpenAIChatCustomInput(
	acc *openAIChatToolAccumulator,
) {
	if acc == nil || acc.contentIndex < 0 || acc.contentIndex >= len(p.output.Content) {
		return
	}
	property := p.grammarToolInputProperties[acc.name]
	if property == "" {
		property = "input"
	}
	acc.argsJSON = ""
	acc.customInput = &openAIChatCustomInputAccumulator{property: property}
	p.output.Content[acc.contentIndex].Arguments = map[string]any{property: ""}
}

func (p *OpenAICompletionsStreamProcessor) openAIChatCustomInput(
	acc *openAIChatToolAccumulator,
) string {
	if acc == nil ||
		acc.customInput == nil ||
		acc.contentIndex < 0 ||
		acc.contentIndex >= len(p.output.Content) {
		return ""
	}
	input, _ := p.output.Content[acc.contentIndex].Arguments[acc.customInput.property].(string)
	return input
}

func (p *OpenAICompletionsStreamProcessor) appendOpenAIChatCustomInput(
	acc *openAIChatToolAccumulator,
	nextInput string,
	closeInput bool,
) (string, bool, error) {
	if acc == nil ||
		acc.customInput == nil ||
		acc.contentIndex < 0 ||
		acc.contentIndex >= len(p.output.Content) {
		return "", false, nil
	}
	custom := acc.customInput
	delta, ok, err := AppendGrammarToolInputJSONDelta(
		&custom.buffer,
		custom.property,
		nextInput,
		closeInput,
	)
	if err != nil {
		return "", false, err
	}
	p.output.Content[acc.contentIndex].Arguments = map[string]any{custom.property: nextInput}
	return delta, ok, nil
}

func (p *OpenAICompletionsStreamProcessor) openAIChatCustomInputError(
	err error,
) []AssistantMessageEvent {
	p.output.StopReason = StopReasonError
	p.output.ErrorMessage = err.Error()
	return []AssistantMessageEvent{{
		Type:   "error",
		Reason: StopReasonError,
		Error:  *p.output,
	}}
}

func (p *OpenAICompletionsStreamProcessor) finishOpenContentBlocks() []AssistantMessageEvent {
	var events []AssistantMessageEvent
	for index, part := range p.output.Content {
		switch part.Type {
		case ContentText:
			if index == p.textIndex && !p.textEnded {
				p.textEnded = true
				events = append(events, AssistantMessageEvent{
					Type:         "text_end",
					ContentIndex: index,
					Content:      p.output.Content[index].Text,
					Partial:      cloneMessageState(*p.output),
				})
			}
		case ContentThinking:
			if index == p.thinkIndex && !p.thinkEnded {
				p.thinkEnded = true
				events = append(events, AssistantMessageEvent{
					Type:         "thinking_end",
					ContentIndex: index,
					Content:      p.output.Content[index].Thinking,
					Partial:      cloneMessageState(*p.output),
				})
			}
		case ContentToolCall:
			tool := p.toolAccumulatorAt(index)
			if tool == nil || tool.ended {
				continue
			}
			tool.ended = true
			if tool.customInput != nil {
				delta, ok, err := p.appendOpenAIChatCustomInput(
					tool,
					p.openAIChatCustomInput(tool),
					true,
				)
				if err != nil {
					events = append(events, p.openAIChatCustomInputError(err)...)
					continue
				}
				if ok {
					events = append(events, AssistantMessageEvent{
						Type:         "toolcall_delta",
						ContentIndex: index,
						Delta:        delta,
						Partial:      cloneMessageState(*p.output),
					})
				}
			} else {
				p.output.Content[index].Arguments = parseStreamingJSONObject(tool.argsJSON)
			}
			events = append(events, AssistantMessageEvent{
				Type:         "toolcall_end",
				ContentIndex: index,
				ToolCall:     p.output.Content[index],
				Partial:      cloneMessageState(*p.output),
			})
		}
	}
	return events
}

func (p *OpenAICompletionsStreamProcessor) toolAccumulatorAt(contentIndex int) *openAIChatToolAccumulator {
	for _, tool := range p.tools {
		if tool.contentIndex == contentIndex {
			return tool
		}
	}
	return nil
}

func openAIToolCallDeltaKey(delta OpenAIChatToolCallDelta) string {
	if delta.Index != nil {
		return fmt.Sprintf("index:%d", *delta.Index)
	}
	if delta.ID != "" {
		return "id:" + delta.ID
	}
	nameArgs, _ := json.Marshal(struct {
		Function OpenAIChatToolCallFunctionDelta `json:"function"`
		Custom   *OpenAIChatCustomToolCallDelta  `json:"custom,omitempty"`
	}{
		Function: delta.Function,
		Custom:   delta.Custom,
	})
	return "anonymous:" + string(nameArgs)
}
