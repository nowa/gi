package gillmprovider

import "strings"

type BedrockPayloadOptions struct {
	Reasoning           string
	Region              string
	ThinkingBudgets     map[string]int
	ThinkingDisplay     string
	InterleavedThinking *bool
	CacheRetention      string
	ForcePromptCache    bool
}

type BedrockPayload struct {
	System                       []BedrockContentBlock
	Messages                     []BedrockMessage
	AdditionalModelRequestFields map[string]any
}

type BedrockMessage struct {
	Role    string
	Content []BedrockContentBlock
}

type BedrockContentBlock struct {
	Text             string
	Image            *BedrockImageBlock
	ToolUse          *BedrockToolUseBlock
	ToolResult       *BedrockToolResultBlock
	ReasoningContent *BedrockReasoningContent
	CachePoint       *BedrockCachePoint
}

type BedrockImageBlock struct {
	Format string
	Data   string
}

type BedrockToolUseBlock struct {
	ToolUseID string
	Name      string
	Input     map[string]any
}

type BedrockToolResultBlock struct {
	ToolUseID string
	Content   []BedrockContentBlock
	Status    string
}

type BedrockReasoningContent struct {
	Text      string
	Signature string
}

type BedrockCachePoint struct {
	Type string
	TTL  string
}

func BuildBedrockPayload(model Model, context Context, options BedrockPayloadOptions) BedrockPayload {
	cacheRetention := resolveCacheRetention(options.CacheRetention)
	return BedrockPayload{
		System:                       buildBedrockSystemPrompt(context.SystemPrompt, model, cacheRetention, options.ForcePromptCache),
		Messages:                     ConvertBedrockMessages(model, context, cacheRetention, options.ForcePromptCache),
		AdditionalModelRequestFields: BuildBedrockAdditionalModelRequestFields(model, options),
	}
}

func BuildBedrockAdditionalModelRequestFields(model Model, options BedrockPayloadOptions) map[string]any {
	if options.Reasoning == "" || !model.Reasoning || !IsAnthropicClaudeBedrockModel(model) {
		return nil
	}
	display := options.ThinkingDisplay
	if display == "" {
		display = "summarized"
	}
	if IsGovCloudBedrockTarget(model, options) {
		display = ""
	}
	result := map[string]any{}
	if SupportsBedrockAdaptiveThinking(model) {
		thinking := map[string]any{"type": "adaptive"}
		if display != "" {
			thinking["display"] = display
		}
		result["thinking"] = thinking
		result["output_config"] = map[string]any{"effort": MapBedrockThinkingEffort(model, options.Reasoning)}
		return result
	}

	level := options.Reasoning
	if level == "xhigh" {
		level = "high"
	}
	defaultBudgets := map[string]int{
		"minimal": 1024,
		"low":     2048,
		"medium":  8192,
		"high":    16384,
		"xhigh":   16384,
	}
	budget := defaultBudgets[options.Reasoning]
	if options.ThinkingBudgets != nil {
		if override, ok := options.ThinkingBudgets[level]; ok {
			budget = override
		}
	}
	thinking := map[string]any{"type": "enabled", "budget_tokens": budget}
	if display != "" {
		thinking["display"] = display
	}
	result["thinking"] = thinking
	if options.InterleavedThinking == nil || *options.InterleavedThinking {
		result["anthropic_beta"] = []string{"interleaved-thinking-2025-05-14"}
	}
	return result
}

func ConvertBedrockMessages(model Model, context Context, cacheRetention string, forcePromptCache bool) []BedrockMessage {
	transformed := TransformMessages(context.Messages, model, func(id string, _ Model, _ Message) string {
		return normalizeBedrockToolCallID(id)
	})
	result := make([]BedrockMessage, 0, len(transformed))
	for i := 0; i < len(transformed); i++ {
		message := transformed[i]
		switch message.Role {
		case RoleUser:
			if content := convertBedrockUserContent(message.Content); len(content) > 0 {
				result = append(result, BedrockMessage{Role: "user", Content: content})
			}
		case RoleAssistant:
			content := convertBedrockAssistantContent(model, message)
			if len(content) > 0 {
				result = append(result, BedrockMessage{Role: "assistant", Content: content})
			}
		case RoleToolResult:
			blocks := []BedrockContentBlock{convertBedrockToolResult(message)}
			j := i + 1
			for ; j < len(transformed) && transformed[j].Role == RoleToolResult; j++ {
				blocks = append(blocks, convertBedrockToolResult(transformed[j]))
			}
			i = j - 1
			result = append(result, BedrockMessage{Role: "user", Content: blocks})
		}
	}
	if cacheRetention != "none" && SupportsBedrockPromptCaching(model, forcePromptCache) && len(result) > 0 {
		last := len(result) - 1
		if result[last].Role == "user" {
			result[last].Content = append(result[last].Content, bedrockCachePoint(cacheRetention))
		}
	}
	return result
}

func IsAnthropicClaudeBedrockModel(model Model) bool {
	id := strings.ToLower(model.ID)
	name := strings.ToLower(model.Name)
	return strings.Contains(id, "anthropic.claude") ||
		strings.Contains(id, "anthropic/claude") ||
		strings.Contains(name, "anthropic.claude") ||
		strings.Contains(name, "anthropic/claude") ||
		strings.Contains(name, "claude")
}

func SupportsBedrockAdaptiveThinking(model Model) bool {
	for _, candidate := range bedrockModelCandidates(model) {
		if strings.Contains(candidate, "opus-4-6") || strings.Contains(candidate, "opus-4-7") || strings.Contains(candidate, "sonnet-4-6") {
			return true
		}
	}
	return false
}

func SupportsBedrockPromptCaching(model Model, force bool) bool {
	candidates := bedrockModelCandidates(model)
	hasClaude := false
	for _, candidate := range candidates {
		if strings.Contains(candidate, "claude") {
			hasClaude = true
			break
		}
	}
	if !hasClaude {
		return force
	}
	for _, candidate := range candidates {
		if strings.Contains(candidate, "-4-") ||
			strings.Contains(candidate, "claude-3-7-sonnet") ||
			strings.Contains(candidate, "claude-3-5-haiku") {
			return true
		}
	}
	return false
}

func IsGovCloudBedrockTarget(model Model, options BedrockPayloadOptions) bool {
	region := strings.ToLower(options.Region)
	return strings.HasPrefix(region, "us-gov-") ||
		strings.HasPrefix(strings.ToLower(model.ID), "us-gov.") ||
		strings.HasPrefix(strings.ToLower(model.ID), "arn:aws-us-gov:")
}

func MapBedrockThinkingEffort(model Model, level string) string {
	if level == "xhigh" {
		for _, candidate := range bedrockModelCandidates(model) {
			if strings.Contains(candidate, "opus-4-7") {
				return "xhigh"
			}
		}
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

func buildBedrockSystemPrompt(systemPrompt string, model Model, cacheRetention string, forcePromptCache bool) []BedrockContentBlock {
	if systemPrompt == "" {
		return nil
	}
	blocks := []BedrockContentBlock{{Text: SanitizeSurrogates(systemPrompt)}}
	if cacheRetention != "none" && SupportsBedrockPromptCaching(model, forcePromptCache) {
		blocks = append(blocks, bedrockCachePoint(cacheRetention))
	}
	return blocks
}

func convertBedrockUserContent(content []ContentPart) []BedrockContentBlock {
	blocks := make([]BedrockContentBlock, 0, len(content))
	for _, part := range content {
		switch part.Type {
		case ContentText:
			blocks = append(blocks, BedrockContentBlock{Text: SanitizeSurrogates(part.Text)})
		case ContentImage:
			blocks = append(blocks, BedrockContentBlock{Image: &BedrockImageBlock{Format: bedrockImageFormat(part.MIMEType), Data: part.Data}})
		}
	}
	return blocks
}

func convertBedrockAssistantContent(model Model, message Message) []BedrockContentBlock {
	blocks := make([]BedrockContentBlock, 0, len(message.Content))
	for _, part := range message.Content {
		switch part.Type {
		case ContentText:
			if strings.TrimSpace(part.Text) != "" {
				blocks = append(blocks, BedrockContentBlock{Text: SanitizeSurrogates(part.Text)})
			}
		case ContentToolCall:
			blocks = append(blocks, BedrockContentBlock{ToolUse: &BedrockToolUseBlock{ToolUseID: part.ID, Name: part.Name, Input: part.Arguments}})
		case ContentThinking:
			if strings.TrimSpace(part.Thinking) == "" {
				continue
			}
			if IsAnthropicClaudeBedrockModel(model) && strings.TrimSpace(part.ThinkingSignature) == "" {
				blocks = append(blocks, BedrockContentBlock{Text: SanitizeSurrogates(part.Thinking)})
			} else {
				blocks = append(blocks, BedrockContentBlock{ReasoningContent: &BedrockReasoningContent{Text: SanitizeSurrogates(part.Thinking), Signature: part.ThinkingSignature}})
			}
		}
	}
	return blocks
}

func convertBedrockToolResult(message Message) BedrockContentBlock {
	content := make([]BedrockContentBlock, 0, len(message.Content))
	for _, part := range message.Content {
		switch part.Type {
		case ContentText:
			content = append(content, BedrockContentBlock{Text: SanitizeSurrogates(part.Text)})
		case ContentImage:
			content = append(content, BedrockContentBlock{Image: &BedrockImageBlock{Format: bedrockImageFormat(part.MIMEType), Data: part.Data}})
		}
	}
	status := "success"
	if message.IsError {
		status = "error"
	}
	return BedrockContentBlock{ToolResult: &BedrockToolResultBlock{ToolUseID: message.ToolCallID, Content: content, Status: status}}
}

func bedrockModelCandidates(model Model) []string {
	values := []string{model.ID}
	if model.Name != "" {
		values = append(values, model.Name)
	}
	result := make([]string, 0, len(values)*2)
	for _, value := range values {
		lower := strings.ToLower(value)
		result = append(result, lower, strings.NewReplacer(" ", "-", "_", "-", ".", "-", ":", "-").Replace(lower))
	}
	return result
}

func normalizeBedrockToolCallID(id string) string {
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

func bedrockCachePoint(cacheRetention string) BedrockContentBlock {
	point := &BedrockCachePoint{Type: "default"}
	if cacheRetention == "long" {
		point.TTL = "1h"
	}
	return BedrockContentBlock{CachePoint: point}
}

func bedrockImageFormat(mimeType string) string {
	switch strings.ToLower(mimeType) {
	case "image/jpeg", "image/jpg":
		return "jpeg"
	case "image/png":
		return "png"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	default:
		return "png"
	}
}
