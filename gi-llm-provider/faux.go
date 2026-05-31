package gillmprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

const (
	defaultFauxAPI      = "faux"
	defaultFauxProvider = "faux"
	defaultFauxModelID  = "faux-1"
)

type FauxModelDefinition struct {
	ID            string
	Name          string
	Reasoning     bool
	Input         []string
	Cost          ModelCost
	ContextWindow int
	MaxTokens     int
}

type FauxState struct {
	CallCount int
}

type FauxResponseFactory func(context Context, options StreamOptions, state FauxState, model Model) (Message, error)

type FauxResponseStep struct {
	Message Message
	Factory FauxResponseFactory
}

type FauxProviderRegistration struct {
	API      string
	Provider string
	Models   []Model
	State    FauxState

	minTokenSize    int
	maxTokenSize    int
	tokensPerSecond float64
	mu              sync.Mutex
	responses       []FauxResponseStep
	promptCache     map[string]string
}

type FauxOptions struct {
	API             string
	Provider        string
	Models          []FauxModelDefinition
	TokensPerSecond float64
	TokenSize       FauxTokenSize
}

type FauxOption func(*FauxOptions)

type FauxTokenSize struct {
	Min int
	Max int
}

func WithFauxAPI(api string) FauxOption {
	return func(options *FauxOptions) { options.API = api }
}

func WithFauxProvider(provider string) FauxOption {
	return func(options *FauxOptions) { options.Provider = provider }
}

func WithFauxModels(models ...FauxModelDefinition) FauxOption {
	return func(options *FauxOptions) { options.Models = models }
}

func WithFauxTokensPerSecond(tokensPerSecond float64) FauxOption {
	return func(options *FauxOptions) { options.TokensPerSecond = tokensPerSecond }
}

func WithFauxTokenSize(min, max int) FauxOption {
	return func(options *FauxOptions) { options.TokenSize = FauxTokenSize{Min: min, Max: max} }
}

func FauxText(text string) ContentPart {
	return Text(text)
}

func FauxThinking(thinking string) ContentPart {
	return Thinking(thinking)
}

func FauxToolCall(name string, arguments map[string]any, id string) ContentPart {
	if id == "" {
		id = "tool:" + UUIDLikeID()
	}
	return ToolCall(id, name, arguments)
}

func FauxAssistantMessage(content []ContentPart, stopReason string) Message {
	if stopReason == "" {
		stopReason = StopReasonStop
	}
	return Message{
		Role:       RoleAssistant,
		Content:    content,
		API:        defaultFauxAPI,
		Provider:   defaultFauxProvider,
		Model:      defaultFauxModelID,
		Usage:      EmptyUsage(),
		StopReason: stopReason,
		Timestamp:  NowMillis(),
	}
}

func FauxAssistantText(text string) Message {
	return FauxAssistantMessage([]ContentPart{Text(text)}, StopReasonStop)
}

func RegisterFauxProvider(options ...FauxOption) *FauxProviderRegistration {
	opts := FauxOptions{API: defaultFauxAPI, Provider: defaultFauxProvider}
	for _, option := range options {
		option(&opts)
	}
	if len(opts.Models) == 0 {
		opts.Models = []FauxModelDefinition{{ID: defaultFauxModelID, Name: "Faux Model", Input: []string{"text", "image"}, ContextWindow: 128000, MaxTokens: 16384}}
	}
	minTokenSize := opts.TokenSize.Min
	if minTokenSize <= 0 {
		minTokenSize = 1
	}
	maxTokenSize := opts.TokenSize.Max
	if maxTokenSize <= 0 {
		maxTokenSize = minTokenSize
	}
	if maxTokenSize < minTokenSize {
		maxTokenSize = minTokenSize
	}
	registration := &FauxProviderRegistration{
		API:             opts.API,
		Provider:        opts.Provider,
		minTokenSize:    minTokenSize,
		maxTokenSize:    maxTokenSize,
		tokensPerSecond: opts.TokensPerSecond,
		promptCache:     map[string]string{},
	}
	for _, definition := range opts.Models {
		name := definition.Name
		if name == "" {
			name = definition.ID
		}
		input := definition.Input
		if input == nil {
			input = []string{"text", "image"}
		}
		contextWindow := definition.ContextWindow
		if contextWindow == 0 {
			contextWindow = 128000
		}
		maxTokens := definition.MaxTokens
		if maxTokens == 0 {
			maxTokens = 16384
		}
		registration.Models = append(registration.Models, Model{
			ID:            definition.ID,
			Name:          name,
			API:           opts.API,
			Provider:      opts.Provider,
			BaseURL:       "http://localhost:0",
			Reasoning:     definition.Reasoning,
			Input:         input,
			Cost:          definition.Cost,
			ContextWindow: contextWindow,
			MaxTokens:     maxTokens,
		})
	}
	RegisterAPIProvider(opts.API, registration)
	return registration
}

func (r *FauxProviderRegistration) GetModel(modelID ...string) (Model, bool) {
	if len(modelID) == 0 || modelID[0] == "" {
		return r.Models[0], true
	}
	for _, model := range r.Models {
		if model.ID == modelID[0] {
			return model, true
		}
	}
	return Model{}, false
}

func (r *FauxProviderRegistration) MustModel(modelID ...string) Model {
	model, ok := r.GetModel(modelID...)
	if !ok {
		panic("faux model not found")
	}
	return model
}

func (r *FauxProviderRegistration) SetResponses(responses []FauxResponseStep) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.responses = append([]FauxResponseStep{}, responses...)
}

func (r *FauxProviderRegistration) AppendResponses(responses []FauxResponseStep) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.responses = append(r.responses, responses...)
}

func (r *FauxProviderRegistration) PendingResponseCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.responses)
}

func (r *FauxProviderRegistration) Unregister() {
	UnregisterAPIProvider(r.API)
}

func (r *FauxProviderRegistration) Stream(model Model, llmContext Context, options StreamOptions) (*AssistantMessageEventStream, error) {
	return r.StreamSimple(model, llmContext, options)
}

func (r *FauxProviderRegistration) StreamSimple(model Model, llmContext Context, options SimpleStreamOptions) (*AssistantMessageEventStream, error) {
	message, err := r.nextResponse(llmContext, options, model)
	if err != nil {
		message = AssistantErrorMessage(err.Error(), model, false)
	}
	message.API = r.API
	message.Provider = r.Provider
	message.Model = model.ID
	message.Usage = estimateFauxUsage(message, llmContext, options, r.promptCache)
	if message.StopReason == StopReasonError || message.StopReason == StopReasonAborted {
		return ErrorAssistantStream(message), nil
	}
	return streamFauxMessage(message, options, r.minTokenSize, r.maxTokenSize, r.tokensPerSecond), nil
}

func (r *FauxProviderRegistration) nextResponse(llmContext Context, options StreamOptions, model Model) (Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.State.CallCount++
	if len(r.responses) == 0 {
		return Message{}, fmt.Errorf("No more faux responses queued")
	}
	step := r.responses[0]
	r.responses = r.responses[1:]
	if step.Factory != nil {
		return step.Factory(llmContext, options, r.State, model)
	}
	return step.Message, nil
}

func streamFauxMessage(message Message, options StreamOptions, minTokenSize, maxTokenSize int, tokensPerSecond float64) *AssistantMessageEventStream {
	stream := NewAssistantMessageEventStream()
	go func() {
		ctx := options.Context
		if ctx == nil {
			ctx = context.Background()
		}
		partial := message
		partial.Content = nil
		if ctx.Err() != nil {
			pushFauxAbort(stream, partial, ctx.Err())
			return
		}
		stream.Push(AssistantMessageEvent{Type: "start", Partial: partial})
		for index, part := range message.Content {
			if ctx.Err() != nil {
				pushFauxAbort(stream, partial, ctx.Err())
				return
			}
			switch part.Type {
			case ContentThinking:
				partial.Content = append(partial.Content, Thinking(""))
				stream.Push(AssistantMessageEvent{Type: "thinking_start", ContentIndex: index, Partial: partial})
				for _, chunk := range splitFauxByTokenSize(part.Thinking, minTokenSize, maxTokenSize) {
					if !waitFauxChunk(ctx, chunk, tokensPerSecond) {
						pushFauxAbort(stream, partial, ctx.Err())
						return
					}
					partial.Content[index].Thinking += chunk
					stream.Push(AssistantMessageEvent{Type: "thinking_delta", ContentIndex: index, Delta: chunk, Partial: partial})
				}
				stream.Push(AssistantMessageEvent{Type: "thinking_end", ContentIndex: index, Content: part.Thinking, Partial: partial})
			case ContentText:
				partial.Content = append(partial.Content, Text(""))
				stream.Push(AssistantMessageEvent{Type: "text_start", ContentIndex: index, Partial: partial})
				for _, chunk := range splitFauxByTokenSize(part.Text, minTokenSize, maxTokenSize) {
					if !waitFauxChunk(ctx, chunk, tokensPerSecond) {
						pushFauxAbort(stream, partial, ctx.Err())
						return
					}
					partial.Content[index].Text += chunk
					stream.Push(AssistantMessageEvent{Type: "text_delta", ContentIndex: index, Delta: chunk, Partial: partial})
				}
				stream.Push(AssistantMessageEvent{Type: "text_end", ContentIndex: index, Content: part.Text, Partial: partial})
			case ContentToolCall:
				partial.Content = append(partial.Content, ToolCall(part.ID, part.Name, map[string]any{}))
				stream.Push(AssistantMessageEvent{Type: "toolcall_start", ContentIndex: index, Partial: partial})
				for _, chunk := range splitFauxByTokenSize(fauxJSONString(part.Arguments), minTokenSize, maxTokenSize) {
					if !waitFauxChunk(ctx, chunk, tokensPerSecond) {
						pushFauxAbort(stream, partial, ctx.Err())
						return
					}
					stream.Push(AssistantMessageEvent{Type: "toolcall_delta", ContentIndex: index, Delta: chunk, Partial: partial})
				}
				partial.Content[index].Arguments = part.Arguments
				stream.Push(AssistantMessageEvent{Type: "toolcall_end", ContentIndex: index, ToolCall: part, Partial: partial})
			}
		}
		if message.StopReason == StopReasonError || message.StopReason == StopReasonAborted {
			stream.Push(AssistantMessageEvent{Type: "error", Reason: message.StopReason, Error: message})
		} else {
			stream.Push(AssistantMessageEvent{Type: "done", Reason: message.StopReason, Message: message})
		}
	}()
	return stream
}

func splitFauxByTokenSize(text string, minTokenSize, maxTokenSize int) []string {
	if minTokenSize <= 0 {
		minTokenSize = 1
	}
	if maxTokenSize < minTokenSize {
		maxTokenSize = minTokenSize
	}
	charSize := max(1, minTokenSize*4)
	var chunks []string
	for len(text) > 0 {
		if len(text) <= charSize {
			chunks = append(chunks, text)
			break
		}
		chunks = append(chunks, text[:charSize])
		text = text[charSize:]
	}
	if len(chunks) == 0 {
		return []string{""}
	}
	return chunks
}

func waitFauxChunk(ctx context.Context, chunk string, tokensPerSecond float64) bool {
	if tokensPerSecond <= 0 {
		select {
		case <-ctx.Done():
			return false
		default:
			return true
		}
	}
	delay := time.Duration(float64(estimateFauxTokens(chunk)) / tokensPerSecond * float64(time.Second))
	if delay <= 0 {
		delay = time.Nanosecond
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return ctx.Err() == nil
	}
}

func pushFauxAbort(stream *AssistantMessageEventStream, partial Message, err error) {
	if err == nil {
		err = context.Canceled
	}
	aborted := partial
	aborted.StopReason = StopReasonAborted
	aborted.ErrorMessage = err.Error()
	aborted.Timestamp = NowMillis()
	stream.Push(AssistantMessageEvent{Type: "error", Reason: StopReasonAborted, Error: aborted})
}

func fauxJSONString(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func estimateFauxUsage(message Message, llmContext Context, options StreamOptions, promptCache map[string]string) Usage {
	promptText := serializeFauxContext(llmContext)
	promptTokens := estimateFauxTokens(promptText)
	outputTokens := estimateFauxTokens(assistantContentToText(message.Content))
	input := promptTokens
	cacheRead := 0
	cacheWrite := 0
	if options.SessionID != "" && options.CacheRetention != "none" {
		if previous, ok := promptCache[options.SessionID]; ok {
			prefix := commonPrefixLength(previous, promptText)
			cacheRead = estimateFauxTokens(previous[:prefix])
			cacheWrite = estimateFauxTokens(promptText[prefix:])
			input = promptTokens - cacheRead
		} else {
			cacheWrite = promptTokens
		}
		promptCache[options.SessionID] = promptText
	}
	return Usage{
		Input:       input,
		Output:      outputTokens,
		CacheRead:   cacheRead,
		CacheWrite:  cacheWrite,
		TotalTokens: input + outputTokens + cacheRead + cacheWrite,
	}
}

func serializeFauxContext(llmContext Context) string {
	var parts []string
	if llmContext.SystemPrompt != "" {
		parts = append(parts, "system:"+llmContext.SystemPrompt)
	}
	for _, message := range llmContext.Messages {
		parts = append(parts, message.Role+":"+messageToText(message))
	}
	if len(llmContext.Tools) > 0 {
		parts = append(parts, "tools:"+fauxJSONString(llmContext.Tools))
	}
	return strings.Join(parts, "\n\n")
}

func messageToText(message Message) string {
	if message.Role == RoleAssistant {
		return assistantContentToText(message.Content)
	}
	var parts []string
	for _, part := range message.Content {
		switch part.Type {
		case ContentText:
			parts = append(parts, part.Text)
		case ContentImage:
			parts = append(parts, fmt.Sprintf("[image:%s:%d]", part.MIMEType, len(part.Data)))
		}
	}
	if message.Role == RoleToolResult {
		return message.ToolName + "\n" + strings.Join(parts, "\n")
	}
	return strings.Join(parts, "\n")
}

func assistantContentToText(content []ContentPart) string {
	var parts []string
	for _, part := range content {
		switch part.Type {
		case ContentText:
			parts = append(parts, part.Text)
		case ContentThinking:
			parts = append(parts, part.Thinking)
		case ContentToolCall:
			parts = append(parts, part.Name+":"+fauxJSONString(part.Arguments))
		}
	}
	return strings.Join(parts, "\n")
}

func estimateFauxTokens(text string) int {
	return int(math.Ceil(float64(len(text)) / 4.0))
}

func commonPrefixLength(a, b string) int {
	max := min(len(a), len(b))
	for i := 0; i < max; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return max
}

func UUIDLikeID() string {
	id := fmt.Sprintf("%d", NowMillis())
	if len(id) > 8 {
		return id[len(id)-8:]
	}
	return id
}
