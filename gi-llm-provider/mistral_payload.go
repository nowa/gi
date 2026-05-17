package gillmprovider

import (
	"encoding/json"
	"strings"
)

type MistralPayloadOptions struct {
	MaxTokens   int
	Temperature *float64
	Reasoning   string
	ToolChoice  any
}

type MistralPayload struct {
	Model           string           `json:"model"`
	Messages        []MistralMessage `json:"messages"`
	Tools           []MistralTool    `json:"tools,omitempty"`
	Stream          bool             `json:"stream"`
	MaxTokens       int              `json:"max_tokens,omitempty"`
	Temperature     *float64         `json:"temperature,omitempty"`
	ToolChoice      any              `json:"tool_choice,omitempty"`
	PromptMode      string           `json:"prompt_mode,omitempty"`
	ReasoningEffort string           `json:"reasoning_effort,omitempty"`
}

type MistralMessage struct {
	Role       string            `json:"role"`
	Content    any               `json:"content,omitempty"`
	ToolCalls  []MistralToolCall `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
	Name       string            `json:"name,omitempty"`
}

type MistralContentPart struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL string        `json:"image_url,omitempty"`
	Thinking []MistralText `json:"thinking,omitempty"`
}

type MistralText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type MistralToolCall struct {
	ID       string                  `json:"id"`
	Type     string                  `json:"type"`
	Function MistralToolCallFunction `json:"function"`
}

type MistralToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type MistralTool struct {
	Type     string              `json:"type"`
	Function MistralToolFunction `json:"function"`
}

type MistralToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
	Strict      bool   `json:"strict"`
}

func BuildMistralPayload(model Model, context Context, options MistralPayloadOptions) MistralPayload {
	level := ""
	if options.Reasoning != "" {
		level = ClampThinkingLevel(model, options.Reasoning)
		if level == "off" {
			level = ""
		}
	}
	payload := MistralPayload{
		Model:    model.ID,
		Messages: ConvertMistralMessages(model, context),
		Tools:    ConvertMistralTools(context.Tools),
		Stream:   true,
	}
	if options.MaxTokens > 0 {
		payload.MaxTokens = options.MaxTokens
	}
	if options.Temperature != nil {
		payload.Temperature = options.Temperature
	}
	if options.ToolChoice != nil {
		payload.ToolChoice = options.ToolChoice
	}
	if model.Reasoning && level != "" {
		if UsesMistralReasoningEffort(model) {
			payload.ReasoningEffort = MapMistralReasoningEffort(model, level)
		} else {
			payload.PromptMode = "reasoning"
		}
	}
	return payload
}

func ConvertMistralTools(tools []Tool) []MistralTool {
	if len(tools) == 0 {
		return nil
	}
	result := make([]MistralTool, 0, len(tools))
	for _, tool := range tools {
		result = append(result, MistralTool{
			Type: "function",
			Function: MistralToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  SchemaToMap(tool.Parameters),
				Strict:      false,
			},
		})
	}
	return result
}

func ConvertMistralMessages(model Model, context Context) []MistralMessage {
	transformed := TransformMessages(context.Messages, model, func(id string, _ Model, _ Message) string {
		return NormalizeToolCallIDForOpenAICompletions(id, model)
	})
	supportsImages := containsString(model.Input, "image")
	result := make([]MistralMessage, 0, len(transformed)+1)
	if context.SystemPrompt != "" {
		result = append(result, MistralMessage{Role: "system", Content: SanitizeSurrogates(context.SystemPrompt)})
	}
	for _, message := range transformed {
		switch message.Role {
		case RoleUser:
			parts := convertMistralUserContent(message.Content, supportsImages)
			if len(parts) > 0 {
				result = append(result, MistralMessage{Role: "user", Content: parts})
			}
		case RoleAssistant:
			assistant := convertMistralAssistantMessage(message)
			if assistant != nil {
				result = append(result, *assistant)
			}
		case RoleToolResult:
			result = append(result, MistralMessage{
				Role:       "tool",
				ToolCallID: message.ToolCallID,
				Name:       message.ToolName,
				Content:    []MistralContentPart{{Type: "text", Text: buildMistralToolResultText(message, supportsImages)}},
			})
		}
	}
	return result
}

func UsesMistralReasoningEffort(model Model) bool {
	return model.ID == "mistral-small-2603" || model.ID == "mistral-small-latest" || model.ID == "mistral-medium-3.5"
}

func MapMistralReasoningEffort(model Model, level string) string {
	if mapped, ok := model.ThinkingLevelMap[level]; ok && mapped != nil {
		return *mapped
	}
	return "high"
}

func convertMistralUserContent(content []ContentPart, supportsImages bool) []MistralContentPart {
	parts := make([]MistralContentPart, 0, len(content))
	hadImages := false
	for _, part := range content {
		switch part.Type {
		case ContentText:
			parts = append(parts, MistralContentPart{Type: "text", Text: SanitizeSurrogates(part.Text)})
		case ContentImage:
			hadImages = true
			if supportsImages {
				parts = append(parts, MistralContentPart{Type: "image_url", ImageURL: "data:" + part.MIMEType + ";base64," + part.Data})
			}
		}
	}
	if len(parts) == 0 && hadImages {
		parts = append(parts, MistralContentPart{Type: "text", Text: "(image omitted: model does not support images)"})
	}
	return parts
}

func convertMistralAssistantMessage(message Message) *MistralMessage {
	assistant := MistralMessage{Role: RoleAssistant}
	var content []MistralContentPart
	for _, part := range message.Content {
		switch part.Type {
		case ContentText:
			if strings.TrimSpace(part.Text) != "" {
				content = append(content, MistralContentPart{Type: "text", Text: SanitizeSurrogates(part.Text)})
			}
		case ContentThinking:
			if strings.TrimSpace(part.Thinking) != "" {
				content = append(content, MistralContentPart{Type: "thinking", Thinking: []MistralText{{Type: "text", Text: SanitizeSurrogates(part.Thinking)}}})
			}
		case ContentToolCall:
			args, _ := json.Marshal(part.Arguments)
			assistant.ToolCalls = append(assistant.ToolCalls, MistralToolCall{
				ID:       part.ID,
				Type:     "function",
				Function: MistralToolCallFunction{Name: part.Name, Arguments: string(args)},
			})
		}
	}
	if len(content) > 0 {
		assistant.Content = content
	}
	if len(content) == 0 && len(assistant.ToolCalls) == 0 {
		return nil
	}
	return &assistant
}

func buildMistralToolResultText(message Message, supportsImages bool) string {
	text := strings.TrimSpace(SanitizeSurrogates(joinTextContent(message.Content)))
	hasImages := false
	for _, part := range message.Content {
		if part.Type == ContentImage {
			hasImages = true
			break
		}
	}
	prefix := ""
	if message.IsError {
		prefix = "[tool error] "
	}
	if text != "" {
		if hasImages && !supportsImages {
			return prefix + text + "\n[tool image omitted: model does not support images]"
		}
		return prefix + text
	}
	if hasImages {
		if supportsImages {
			return prefix + "(see attached image)"
		}
		return prefix + "(image omitted: model does not support images)"
	}
	return prefix + "(no tool output)"
}
