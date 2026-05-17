package gillmprovider

import "strings"

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

type AnthropicCompat struct {
	SupportsEagerToolInputStreaming bool
	SupportsCacheControlOnTools     bool
	SupportsLongCacheRetention      bool
	SendSessionAffinityHeaders      bool
}

type AnthropicPayloadOptions struct {
	MaxTokens        int
	Temperature      *float64
	CacheRetention   string
	SessionID        string
	Reasoning        string
	ThinkingBudgets  map[string]int
	ThinkingDisplay  string
	Metadata         map[string]any
	IsOAuthToken     bool
	InterleavedThink *bool
}

type AnthropicPayload struct {
	Model        string
	Messages     []AnthropicMessage
	System       []AnthropicContentBlock
	MaxTokens    int
	Stream       bool
	Temperature  *float64
	Tools        []AnthropicTool
	Thinking     map[string]any
	OutputConfig map[string]any
	Metadata     map[string]any
}

type AnthropicMessage struct {
	Role    string
	Content any
}

type AnthropicContentBlock struct {
	Type         string                    `json:"type"`
	Text         string                    `json:"text,omitempty"`
	Source       *AnthropicImageSource     `json:"source,omitempty"`
	ID           string                    `json:"id,omitempty"`
	Name         string                    `json:"name,omitempty"`
	Input        map[string]any            `json:"input,omitempty"`
	ToolUseID    string                    `json:"tool_use_id,omitempty"`
	Content      any                       `json:"content,omitempty"`
	IsError      bool                      `json:"is_error,omitempty"`
	Thinking     string                    `json:"thinking,omitempty"`
	Signature    string                    `json:"signature,omitempty"`
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
	InputSchema         map[string]any            `json:"input_schema"`
	CacheControl        *OpenAICompatCacheControl `json:"cache_control,omitempty"`
}

func BuildAnthropicPayload(model Model, context Context, options AnthropicPayloadOptions) AnthropicPayload {
	compat := ResolveAnthropicCompat(model)
	cacheControl := anthropicCacheControl(options.CacheRetention, compat)
	maxTokens := options.MaxTokens
	if maxTokens == 0 {
		maxTokens = model.MaxTokens / 3
	}
	payload := AnthropicPayload{
		Model:     model.ID,
		Messages:  ConvertAnthropicMessages(model, context, options.IsOAuthToken, cacheControl),
		System:    buildAnthropicSystem(context.SystemPrompt, options.IsOAuthToken, cacheControl),
		MaxTokens: maxTokens,
		Stream:    true,
	}
	if len(context.Tools) > 0 {
		var toolCacheControl *OpenAICompatCacheControl
		if compat.SupportsCacheControlOnTools {
			toolCacheControl = cacheControl
		}
		payload.Tools = ConvertAnthropicTools(context.Tools, options.IsOAuthToken, compat.SupportsEagerToolInputStreaming, toolCacheControl)
	}
	if model.Reasoning {
		if options.Reasoning == "" {
			payload.Thinking = map[string]any{"type": "disabled"}
			payload.Temperature = options.Temperature
		} else if SupportsAnthropicAdaptiveThinking(model) {
			display := options.ThinkingDisplay
			if display == "" {
				display = "summarized"
			}
			payload.Thinking = map[string]any{"type": "adaptive", "display": display}
			payload.OutputConfig = map[string]any{"effort": MapAnthropicThinkingEffort(model, options.Reasoning)}
		} else {
			display := options.ThinkingDisplay
			if display == "" {
				display = "summarized"
			}
			budget := anthropicThinkingBudget(options.Reasoning, options.ThinkingBudgets)
			payload.Thinking = map[string]any{"type": "enabled", "budget_tokens": budget, "display": display}
		}
	} else {
		payload.Temperature = options.Temperature
	}
	if userID, ok := options.Metadata["user_id"].(string); ok {
		payload.Metadata = map[string]any{"user_id": userID}
	}
	return payload
}

func BuildAnthropicHeaders(model Model, context Context, options AnthropicPayloadOptions) map[string]string {
	headers := map[string]string{}
	features := []string{}
	compat := ResolveAnthropicCompat(model)
	if len(context.Tools) > 0 && !compat.SupportsEagerToolInputStreaming {
		features = append(features, fineGrainedToolStreamingBeta)
	}
	interleaved := true
	if options.InterleavedThink != nil {
		interleaved = *options.InterleavedThink
	}
	if interleaved && options.Reasoning != "" && !SupportsAnthropicAdaptiveThinking(model) {
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
	transformed := TransformMessages(context.Messages, model, func(id string, _ Model, _ Message) string {
		return normalizeAnthropicToolCallID(id)
	})
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
			blocks := convertAnthropicAssistantContent(message, isOAuthToken)
			if len(blocks) > 0 {
				result = append(result, AnthropicMessage{Role: "assistant", Content: blocks})
			}
		case RoleToolResult:
			blocks := []AnthropicContentBlock{convertAnthropicToolResult(message)}
			j := i + 1
			for ; j < len(transformed) && transformed[j].Role == RoleToolResult; j++ {
				blocks = append(blocks, convertAnthropicToolResult(transformed[j]))
			}
			i = j - 1
			result = append(result, AnthropicMessage{Role: "user", Content: blocks})
		}
	}
	if cacheControl != nil && len(result) > 0 && result[len(result)-1].Role == "user" {
		applyAnthropicLastUserCacheControl(&result[len(result)-1], cacheControl)
	}
	return result
}

func ConvertAnthropicTools(tools []Tool, isOAuthToken bool, supportsEagerToolInputStreaming bool, cacheControl *OpenAICompatCacheControl) []AnthropicTool {
	result := make([]AnthropicTool, 0, len(tools))
	for i, tool := range tools {
		name := tool.Name
		if isOAuthToken {
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
		converted := AnthropicTool{
			Name:        name,
			Description: tool.Description,
			InputSchema: inputSchema,
		}
		if supportsEagerToolInputStreaming {
			converted.EagerInputStreaming = ptrBool(true)
		}
		if cacheControl != nil && i == len(tools)-1 {
			converted.CacheControl = cacheControl
		}
		result = append(result, converted)
	}
	return result
}

func ResolveAnthropicCompat(model Model) AnthropicCompat {
	compat := AnthropicCompat{
		SupportsEagerToolInputStreaming: true,
		SupportsCacheControlOnTools:     true,
		SupportsLongCacheRetention:      true,
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
	if model.Compat.SendSessionAffinityHeaders != nil {
		compat.SendSessionAffinityHeaders = *model.Compat.SendSessionAffinityHeaders
	}
	return compat
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
	id := strings.ToLower(model.ID)
	return strings.Contains(id, "opus-4-6") ||
		strings.Contains(id, "opus-4.6") ||
		strings.Contains(id, "opus-4-7") ||
		strings.Contains(id, "opus-4.7") ||
		strings.Contains(id, "sonnet-4-6") ||
		strings.Contains(id, "sonnet-4.6")
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

func convertAnthropicAssistantContent(message Message, isOAuthToken bool) []AnthropicContentBlock {
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
				blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: SanitizeSurrogates(part.Thinking)})
			} else {
				blocks = append(blocks, AnthropicContentBlock{Type: "thinking", Thinking: SanitizeSurrogates(part.Thinking), Signature: part.ThinkingSignature})
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

func convertAnthropicToolResult(message Message) AnthropicContentBlock {
	return AnthropicContentBlock{
		Type:      "tool_result",
		ToolUseID: message.ToolCallID,
		Content:   convertAnthropicToolResultContent(message.Content),
		IsError:   message.IsError,
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

func anthropicThinkingBudget(level string, overrides map[string]int) int {
	defaults := map[string]int{
		"minimal": 1024,
		"low":     2048,
		"medium":  8192,
		"high":    16384,
		"xhigh":   16384,
	}
	if level == "xhigh" {
		if overrides != nil {
			if value, ok := overrides["high"]; ok {
				return value
			}
		}
	}
	if overrides != nil {
		if value, ok := overrides[level]; ok {
			return value
		}
	}
	if value, ok := defaults[level]; ok {
		return value
	}
	return 1024
}
