package gillmprovider

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	fineGrainedToolStreamingBeta = "fine-grained-tool-streaming-2025-05-14"
	interleavedThinkingBeta      = "interleaved-thinking-2025-05-14"
)

var claudeCodeToolNames = []string{
	"Read",
	"Write",
	"Edit",
	"Bash",
	"Grep",
	"Glob",
	"AskUserQuestion",
	"EnterPlanMode",
	"ExitPlanMode",
	"KillShell",
	"NotebookEdit",
	"Skill",
	"Task",
	"TaskOutput",
	"TodoWrite",
	"WebFetch",
	"WebSearch",
}

var anthropicToolReferenceVersionPattern = regexp.MustCompile(
	`^claude-(?:opus|sonnet|fable)-(\d+)(?:-(\d+))?(?:-|$)`,
)

type AnthropicCompat struct {
	SupportsEagerToolInputStreaming bool
	SupportsCacheControlOnTools     bool
	SupportsLongCacheRetention      bool
	SupportsStrictTools             bool
	SupportsToolReferences          bool
	SupportsTemperature             bool
	SendSessionAffinityHeaders      bool
	AllowEmptySignature             bool
}

type AnthropicPayloadOptions struct {
	MaxTokens          int
	Temperature        *float64
	CacheRetention     string
	SessionID          string
	Reasoning          string
	ThinkingBudgets    map[string]int
	ThinkingDisplay    string
	Metadata           map[string]any
	Headers            map[string]string
	IsOAuthToken       bool
	InterleavedThink   *bool
	thinkingAllocation *ThinkingTokenAllocation
}

type AnthropicPayload struct {
	Model        string                  `json:"model"`
	Messages     []AnthropicMessage      `json:"messages"`
	System       []AnthropicContentBlock `json:"system,omitempty"`
	MaxTokens    int                     `json:"max_tokens"`
	Stream       bool                    `json:"stream"`
	Temperature  *float64                `json:"temperature,omitempty"`
	Tools        []AnthropicTool         `json:"tools,omitempty"`
	Thinking     map[string]any          `json:"thinking,omitempty"`
	OutputConfig map[string]any          `json:"output_config,omitempty"`
	Metadata     map[string]any          `json:"metadata,omitempty"`
}

type AnthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type AnthropicContentBlock struct {
	Type         string                    `json:"type"`
	Text         string                    `json:"text,omitempty"`
	Source       *AnthropicImageSource     `json:"source,omitempty"`
	ID           string                    `json:"id,omitempty"`
	Name         string                    `json:"name,omitempty"`
	Input        map[string]any            `json:"input,omitempty"`
	ToolUseID    string                    `json:"tool_use_id,omitempty"`
	ToolName     string                    `json:"tool_name,omitempty"`
	Content      any                       `json:"content,omitempty"`
	IsError      bool                      `json:"is_error,omitempty"`
	Thinking     string                    `json:"thinking,omitempty"`
	Signature    *string                   `json:"signature,omitempty"`
	Data         string                    `json:"data,omitempty"`
	CacheControl *OpenAICompatCacheControl `json:"cache_control,omitempty"`
}

type AnthropicImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type AnthropicTool struct {
	Name                string                    `json:"name"`
	Description         string                    `json:"description,omitempty"`
	EagerInputStreaming *bool                     `json:"eager_input_streaming,omitempty"`
	Strict              *bool                     `json:"strict,omitempty"`
	InputSchema         map[string]any            `json:"input_schema"`
	DeferLoading        bool                      `json:"defer_loading,omitempty"`
	CacheControl        *OpenAICompatCacheControl `json:"cache_control,omitempty"`
}

type AnthropicToolOptions struct {
	IsOAuthToken                    bool
	SupportsEagerToolInputStreaming bool
	SupportsStrictTools             bool
	CacheControl                    *OpenAICompatCacheControl
	DeferLoading                    bool
}

// AnthropicRequestState is resolved once per request so message references and
// the advertised tool list cannot disagree about deferred placement.
type AnthropicRequestState struct {
	Compat            AnthropicCompat
	ToolPlacement     DeferredToolSet
	DeferredToolNames map[string]struct{}
	NormalizeToolName ToolNameNormalizer
}

func ResolveAnthropicRequestState(model Model, context Context, isOAuthToken bool) AnthropicRequestState {
	compat := ResolveAnthropicCompat(model)
	normalizeToolName := ToolNameNormalizer(func(name string) string { return name })
	if isOAuthToken {
		normalizeToolName = ToClaudeCodeToolName
	}
	toolPlacement := SplitDeferredTools(
		context,
		compat.SupportsToolReferences,
		normalizeToolName,
	)
	if len(toolPlacement.Immediate) == 0 && len(toolPlacement.DeferredTools) > 0 {
		toolPlacement.Immediate = append([]Tool(nil), toolPlacement.DeferredTools...)
		toolPlacement.Deferred = map[string]Tool{}
		toolPlacement.DeferredTools = nil
	}
	deferredToolNames := make(map[string]struct{}, len(toolPlacement.Deferred))
	for name := range toolPlacement.Deferred {
		deferredToolNames[name] = struct{}{}
	}
	return AnthropicRequestState{
		Compat:            compat,
		ToolPlacement:     toolPlacement,
		DeferredToolNames: deferredToolNames,
		NormalizeToolName: normalizeToolName,
	}
}

func BuildAnthropicPayload(model Model, context Context, options AnthropicPayloadOptions) AnthropicPayload {
	payload, _ := buildAnthropicPayload(model, context, options)
	return payload
}

func BuildAnthropicPayloadChecked(model Model, context Context, options AnthropicPayloadOptions) (AnthropicPayload, error) {
	return buildAnthropicPayload(model, context, options)
}

func buildAnthropicPayload(model Model, context Context, options AnthropicPayloadOptions) (AnthropicPayload, error) {
	transformed := transformAnthropicMessages(model, context.Messages)
	transformedContext := context
	transformedContext.Messages = transformed
	state := ResolveAnthropicRequestState(model, transformedContext, options.IsOAuthToken)
	compat := state.Compat
	cacheControl := anthropicCacheControl(options.CacheRetention, compat)
	reasoning := options.Reasoning
	if reasoning == "off" {
		reasoning = ""
	}
	maxTokens := options.MaxTokens
	if maxTokens == 0 && model.MaxTokens > 0 {
		maxTokens = model.MaxTokens
	}
	payload := AnthropicPayload{
		Model: model.ID,
		Messages: convertAnthropicMessages(
			transformed,
			options.IsOAuthToken,
			cacheControl,
			compat.AllowEmptySignature,
			state.DeferredToolNames,
			state.NormalizeToolName,
		),
		System:    buildAnthropicSystem(context.SystemPrompt, options.IsOAuthToken, cacheControl),
		MaxTokens: maxTokens,
		Stream:    true,
	}
	if len(state.ToolPlacement.Immediate) > 0 || len(state.ToolPlacement.DeferredTools) > 0 {
		var toolCacheControl *OpenAICompatCacheControl
		if compat.SupportsCacheControlOnTools {
			toolCacheControl = cacheControl
		}
		immediateTools, err := ConvertAnthropicToolsChecked(
			state.ToolPlacement.Immediate,
			AnthropicToolOptions{
				IsOAuthToken:                    options.IsOAuthToken,
				SupportsEagerToolInputStreaming: compat.SupportsEagerToolInputStreaming,
				SupportsStrictTools:             compat.SupportsStrictTools,
				CacheControl:                    toolCacheControl,
			},
		)
		if err != nil {
			return AnthropicPayload{}, err
		}
		deferredTools, err := ConvertAnthropicToolsChecked(
			state.ToolPlacement.DeferredTools,
			AnthropicToolOptions{
				IsOAuthToken:                    options.IsOAuthToken,
				SupportsEagerToolInputStreaming: compat.SupportsEagerToolInputStreaming,
				SupportsStrictTools:             compat.SupportsStrictTools,
				DeferLoading:                    true,
			},
		)
		if err != nil {
			return AnthropicPayload{}, err
		}
		payload.Tools = append(immediateTools, deferredTools...)
	}
	if model.Reasoning {
		if reasoning == "" {
			payload.Thinking = map[string]any{"type": "disabled"}
			if compat.SupportsTemperature {
				payload.Temperature = options.Temperature
			}
		} else if SupportsAnthropicAdaptiveThinking(model) {
			display := options.ThinkingDisplay
			if display == "" {
				display = "summarized"
			}
			payload.Thinking = map[string]any{"type": "adaptive", "display": display}
			payload.OutputConfig = map[string]any{"effort": MapAnthropicThinkingEffort(model, reasoning)}
		} else {
			display := options.ThinkingDisplay
			if display == "" {
				display = "summarized"
			}
			allocation := AdjustMaxTokensForThinking(maxTokens, model.MaxTokens, reasoning, options.ThinkingBudgets)
			if options.thinkingAllocation != nil {
				allocation = *options.thinkingAllocation
			}
			payload.MaxTokens = allocation.MaxTokens
			payload.Thinking = map[string]any{
				"type":          "enabled",
				"budget_tokens": allocation.ThinkingBudget,
				"display":       display,
			}
		}
	} else {
		if compat.SupportsTemperature {
			payload.Temperature = options.Temperature
		}
	}
	if userID, ok := options.Metadata["user_id"].(string); ok {
		payload.Metadata = map[string]any{"user_id": userID}
	}
	return payload, nil
}

func BuildAnthropicHeaders(model Model, context Context, options AnthropicPayloadOptions) map[string]string {
	headers := map[string]string{}
	features := []string{}
	compat := ResolveAnthropicCompat(model)
	reasoning := options.Reasoning
	if reasoning == "off" {
		reasoning = ""
	}
	if len(context.Tools) > 0 && !compat.SupportsEagerToolInputStreaming {
		features = append(features, fineGrainedToolStreamingBeta)
	}
	interleaved := true
	if options.InterleavedThink != nil {
		interleaved = *options.InterleavedThink
	}
	if interleaved && reasoning != "" && !SupportsAnthropicAdaptiveThinking(model) {
		features = append(features, interleavedThinkingBeta)
	}
	if len(features) > 0 {
		headers["anthropic-beta"] = strings.Join(features, ",")
	}
	if options.SessionID != "" && resolveCacheRetention(options.CacheRetention) != "none" && compat.SendSessionAffinityHeaders {
		headers["x-session-affinity"] = options.SessionID
	}
	return headers
}

func ConvertAnthropicMessages(model Model, context Context, isOAuthToken bool, cacheControl *OpenAICompatCacheControl) []AnthropicMessage {
	transformed := transformAnthropicMessages(model, context.Messages)
	transformedContext := context
	transformedContext.Messages = transformed
	state := ResolveAnthropicRequestState(model, transformedContext, isOAuthToken)
	return convertAnthropicMessages(
		transformed,
		isOAuthToken,
		cacheControl,
		state.Compat.AllowEmptySignature,
		state.DeferredToolNames,
		state.NormalizeToolName,
	)
}

func transformAnthropicMessages(model Model, messages []Message) []Message {
	return TransformMessages(messages, model, func(id string, _ Model, _ Message) string {
		return normalizeAnthropicToolCallID(id)
	})
}

func convertAnthropicMessages(
	transformed []Message,
	isOAuthToken bool,
	cacheControl *OpenAICompatCacheControl,
	allowEmptySignature bool,
	deferredToolNames map[string]struct{},
	normalizeToolName ToolNameNormalizer,
) []AnthropicMessage {
	if normalizeToolName == nil {
		normalizeToolName = func(name string) string { return name }
	}
	loadedToolNames := make(map[string]struct{})
	result := make([]AnthropicMessage, 0, len(transformed))
	for i := 0; i < len(transformed); i++ {
		message := transformed[i]
		switch message.Role {
		case RoleUser:
			blocks := convertAnthropicUserContent(message.Content)
			if len(blocks) > 0 {
				result = append(result, AnthropicMessage{Role: "user", Content: blocks})
			}
		case RoleAssistant:
			blocks := convertAnthropicAssistantContent(message, isOAuthToken, allowEmptySignature)
			if len(blocks) > 0 {
				result = append(result, AnthropicMessage{Role: "assistant", Content: blocks})
			}
		case RoleToolResult:
			toolResults := make([]AnthropicContentBlock, 0)
			siblingContent := make([]AnthropicContentBlock, 0)
			j := i
			for ; j < len(transformed) && transformed[j].Role == RoleToolResult; j++ {
				toolResult, siblings := convertAnthropicToolResult(
					transformed[j],
					deferredToolNames,
					loadedToolNames,
					normalizeToolName,
				)
				toolResults = append(toolResults, toolResult)
				siblingContent = append(siblingContent, siblings...)
			}
			i = j - 1
			blocks := append(toolResults, siblingContent...)
			result = append(result, AnthropicMessage{Role: "user", Content: blocks})
		}
	}
	if cacheControl != nil && len(result) > 0 && result[len(result)-1].Role == "user" {
		applyAnthropicLastUserCacheControl(&result[len(result)-1], cacheControl)
	}
	return result
}

func ConvertAnthropicTools(tools []Tool, isOAuthToken bool, supportsEagerToolInputStreaming bool, cacheControl *OpenAICompatCacheControl) []AnthropicTool {
	converted, _ := ConvertAnthropicToolsChecked(tools, AnthropicToolOptions{
		IsOAuthToken:                    isOAuthToken,
		SupportsEagerToolInputStreaming: supportsEagerToolInputStreaming,
		CacheControl:                    cacheControl,
	})
	return converted
}

func ConvertAnthropicToolsChecked(tools []Tool, options AnthropicToolOptions) ([]AnthropicTool, error) {
	result := make([]AnthropicTool, 0, len(tools))
	for i, tool := range tools {
		name := tool.Name
		if options.IsOAuthToken {
			name = ToClaudeCodeToolName(name)
		}
		schema := SchemaToMap(tool.Parameters)
		properties, _ := schema["properties"].(map[string]any)
		required, _ := schema["required"].([]any)
		inputSchema := map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []any{},
		}
		if properties != nil {
			inputSchema["properties"] = properties
		}
		if required != nil {
			inputSchema["required"] = required
		}
		strict, err := ResolveJSONSchemaStrictSampling(tool, options.SupportsStrictTools)
		if err != nil {
			return nil, err
		}
		if strict != nil && *strict {
			inputSchema = schema
			inputSchema["type"] = "object"
			inputSchema["properties"] = map[string]any{}
			inputSchema["required"] = []any{}
			if properties != nil {
				inputSchema["properties"] = properties
			}
			if required != nil {
				inputSchema["required"] = required
			}
		}
		converted := AnthropicTool{
			Name:         name,
			Description:  tool.Description,
			InputSchema:  inputSchema,
			DeferLoading: options.DeferLoading,
		}
		if strict != nil && *strict {
			converted.Strict = ptrBool(true)
		}
		if options.SupportsEagerToolInputStreaming {
			converted.EagerInputStreaming = ptrBool(true)
		}
		if options.CacheControl != nil && i == len(tools)-1 {
			converted.CacheControl = options.CacheControl
		}
		result = append(result, converted)
	}
	return result, nil
}

func ResolveAnthropicCompat(model Model) AnthropicCompat {
	compat := AnthropicCompat{
		SupportsEagerToolInputStreaming: true,
		SupportsCacheControlOnTools:     true,
		SupportsLongCacheRetention:      true,
		SupportsTemperature:             true,
		SupportsToolReferences:          defaultSupportsToolReferences(model),
	}
	if model.Compat.SupportsEagerToolInputStreaming != nil {
		compat.SupportsEagerToolInputStreaming = *model.Compat.SupportsEagerToolInputStreaming
	}
	if model.Compat.SupportsCacheControlOnTools != nil {
		compat.SupportsCacheControlOnTools = *model.Compat.SupportsCacheControlOnTools
	}
	if model.Compat.SupportsLongCacheRetention != nil {
		compat.SupportsLongCacheRetention = *model.Compat.SupportsLongCacheRetention
	}
	if model.Compat.SupportsStrictTools != nil {
		compat.SupportsStrictTools = *model.Compat.SupportsStrictTools
	}
	if model.Compat.SupportsToolReferences != nil {
		compat.SupportsToolReferences = *model.Compat.SupportsToolReferences
	}
	if model.Compat.SupportsTemperature != nil {
		compat.SupportsTemperature = *model.Compat.SupportsTemperature
	}
	if model.Compat.SendSessionAffinityHeaders != nil {
		compat.SendSessionAffinityHeaders = *model.Compat.SendSessionAffinityHeaders
	}
	if model.Compat.AllowEmptySignature != nil {
		compat.AllowEmptySignature = *model.Compat.AllowEmptySignature
	}
	return compat
}

func defaultSupportsToolReferences(model Model) bool {
	id := strings.ToLower(model.ID)
	if model.Provider != "anthropic" || strings.Contains(id, "haiku") {
		return false
	}
	version := anthropicToolReferenceVersionPattern.FindStringSubmatch(id)
	if len(version) == 0 {
		return false
	}
	major, err := strconv.Atoi(version[1])
	if err != nil {
		return false
	}
	minor := 0
	if len(version) > 2 && version[2] != "" && len(version[2]) < 8 {
		minor, _ = strconv.Atoi(version[2])
	}
	return major > 4 || major == 4 && minor >= 5
}

func ToClaudeCodeToolName(name string) string {
	lower := strings.ToLower(name)
	for _, candidate := range claudeCodeToolNames {
		if strings.ToLower(candidate) == lower {
			return candidate
		}
	}
	return name
}

func FromClaudeCodeToolName(name string, tools []Tool) string {
	lower := strings.ToLower(name)
	for _, tool := range tools {
		if strings.ToLower(tool.Name) == lower {
			return tool.Name
		}
	}
	return name
}

func SupportsAnthropicAdaptiveThinking(model Model) bool {
	return model.Compat.ForceAdaptiveThinking != nil &&
		*model.Compat.ForceAdaptiveThinking
}

func MapAnthropicThinkingEffort(model Model, level string) string {
	if level != "" {
		level = ClampThinkingLevel(model, level)
	}
	if mapped, ok := model.ThinkingLevelMap[level]; ok && mapped != nil {
		return *mapped
	}
	switch level {
	case "minimal", "low":
		return "low"
	case "medium":
		return "medium"
	default:
		return "high"
	}
}

func buildAnthropicSystem(systemPrompt string, isOAuthToken bool, cacheControl *OpenAICompatCacheControl) []AnthropicContentBlock {
	var blocks []AnthropicContentBlock
	if isOAuthToken {
		blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: "You are Claude Code, Anthropic's official CLI for Claude.", CacheControl: cacheControl})
	}
	if systemPrompt != "" {
		blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: SanitizeSurrogates(systemPrompt), CacheControl: cacheControl})
	}
	return blocks
}

func anthropicCacheControl(cacheRetention string, compat AnthropicCompat) *OpenAICompatCacheControl {
	cacheRetention = resolveCacheRetention(cacheRetention)
	if cacheRetention == "none" {
		return nil
	}
	result := &OpenAICompatCacheControl{Type: "ephemeral"}
	if cacheRetention == "long" && compat.SupportsLongCacheRetention {
		result.TTL = "1h"
	}
	return result
}

func convertAnthropicUserContent(content []ContentPart) []AnthropicContentBlock {
	blocks := make([]AnthropicContentBlock, 0, len(content))
	for _, part := range content {
		switch part.Type {
		case ContentText:
			if strings.TrimSpace(part.Text) != "" {
				blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: SanitizeSurrogates(part.Text)})
			}
		case ContentImage:
			blocks = append(blocks, AnthropicContentBlock{
				Type:   "image",
				Source: &AnthropicImageSource{Type: "base64", MediaType: part.MIMEType, Data: part.Data},
			})
		}
	}
	return blocks
}

func convertAnthropicAssistantContent(message Message, isOAuthToken, allowEmptySignature bool) []AnthropicContentBlock {
	blocks := make([]AnthropicContentBlock, 0, len(message.Content))
	for _, part := range message.Content {
		switch part.Type {
		case ContentText:
			if strings.TrimSpace(part.Text) != "" {
				blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: SanitizeSurrogates(part.Text)})
			}
		case ContentThinking:
			if part.Redacted {
				blocks = append(blocks, AnthropicContentBlock{Type: "redacted_thinking", Data: part.ThinkingSignature})
				continue
			}
			if strings.TrimSpace(part.Thinking) == "" {
				continue
			}
			if strings.TrimSpace(part.ThinkingSignature) == "" {
				if allowEmptySignature {
					blocks = append(blocks, AnthropicContentBlock{
						Type:      "thinking",
						Thinking:  SanitizeSurrogates(part.Thinking),
						Signature: ptrString(""),
					})
				} else {
					blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: SanitizeSurrogates(part.Thinking)})
				}
			} else {
				blocks = append(blocks, AnthropicContentBlock{
					Type:      "thinking",
					Thinking:  SanitizeSurrogates(part.Thinking),
					Signature: ptrString(part.ThinkingSignature),
				})
			}
		case ContentToolCall:
			name := part.Name
			if isOAuthToken {
				name = ToClaudeCodeToolName(name)
			}
			blocks = append(blocks, AnthropicContentBlock{Type: "tool_use", ID: part.ID, Name: name, Input: part.Arguments})
		}
	}
	return blocks
}

func convertAnthropicToolResult(
	message Message,
	deferredToolNames map[string]struct{},
	loadedToolNames map[string]struct{},
	normalizeToolName ToolNameNormalizer,
) (AnthropicContentBlock, []AnthropicContentBlock) {
	references := make([]AnthropicContentBlock, 0, len(message.AddedToolNames))
	for _, name := range message.AddedToolNames {
		normalizedName := normalizeToolName(name)
		if _, deferred := deferredToolNames[normalizedName]; !deferred {
			continue
		}
		if _, loaded := loadedToolNames[normalizedName]; loaded {
			continue
		}
		loadedToolNames[normalizedName] = struct{}{}
		references = append(references, AnthropicContentBlock{
			Type:     "tool_reference",
			ToolName: normalizedName,
		})
	}

	convertedContent := convertAnthropicToolResultContent(message.Content)
	toolResult := AnthropicContentBlock{
		Type:      "tool_result",
		ToolUseID: message.ToolCallID,
		Content:   convertedContent,
		IsError:   message.IsError,
	}
	if len(references) == 0 {
		return toolResult, nil
	}
	toolResult.Content = references
	switch content := convertedContent.(type) {
	case string:
		return toolResult, []AnthropicContentBlock{{Type: "text", Text: content}}
	case []AnthropicContentBlock:
		return toolResult, content
	default:
		return toolResult, nil
	}
}

func convertAnthropicToolResultContent(content []ContentPart) any {
	blocks := make([]AnthropicContentBlock, 0, len(content))
	for _, part := range content {
		switch part.Type {
		case ContentText:
			blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: SanitizeSurrogates(part.Text)})
		case ContentImage:
			blocks = append(blocks, AnthropicContentBlock{Type: "image", Source: &AnthropicImageSource{Type: "base64", MediaType: part.MIMEType, Data: part.Data}})
		}
	}
	if len(blocks) == 1 && blocks[0].Type == "text" {
		return blocks[0].Text
	}
	return blocks
}

func applyAnthropicLastUserCacheControl(message *AnthropicMessage, cacheControl *OpenAICompatCacheControl) {
	switch content := message.Content.(type) {
	case string:
		if content != "" {
			message.Content = []AnthropicContentBlock{{Type: "text", Text: content, CacheControl: cacheControl}}
		}
	case []AnthropicContentBlock:
		if len(content) > 0 {
			content[len(content)-1].CacheControl = cacheControl
			message.Content = content
		}
	}
}

func normalizeAnthropicToolCallID(id string) string {
	var b strings.Builder
	for _, r := range id {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
		if b.Len() >= 64 {
			break
		}
	}
	return b.String()
}
