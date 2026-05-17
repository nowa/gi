package gillmprovider

import (
	"encoding/json"
	"fmt"
	"strings"
)

type OpenAIResponsesInputItem struct {
	Type      string `json:"type,omitempty"`
	Role      string `json:"role,omitempty"`
	Content   any    `json:"content,omitempty"`
	ID        string `json:"id,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Output    any    `json:"output,omitempty"`
	Status    string `json:"status,omitempty"`
	Phase     string `json:"phase,omitempty"`
}

type OpenAIResponsesContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Detail   string `json:"detail,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

type ConvertOpenAIResponsesOptions struct {
	AllowedToolCallProviders map[string]bool
	IncludeSystemPrompt      *bool
}

func ConvertOpenAIResponsesMessages(model Model, context Context, options ConvertOpenAIResponsesOptions) []OpenAIResponsesInputItem {
	includeSystemPrompt := true
	if options.IncludeSystemPrompt != nil {
		includeSystemPrompt = *options.IncludeSystemPrompt
	}
	allowedProviders := options.AllowedToolCallProviders
	if allowedProviders == nil {
		allowedProviders = map[string]bool{"openai": true, "openai-codex": true, "opencode": true}
	}
	normalize := func(id string, target Model, source Message) string {
		return NormalizeToolCallIDForOpenAIResponses(id, target, source, allowedProviders)
	}
	transformed := TransformMessages(context.Messages, model, normalize)

	var items []OpenAIResponsesInputItem
	if includeSystemPrompt && context.SystemPrompt != "" {
		role := "system"
		if model.Reasoning {
			role = "developer"
		}
		items = append(items, OpenAIResponsesInputItem{Role: role, Content: SanitizeSurrogates(context.SystemPrompt)})
	}

	messageIndex := 0
	for _, message := range transformed {
		switch message.Role {
		case RoleUser:
			content := convertOpenAIResponsesUserContent(message.Content)
			if len(content) == 0 {
				continue
			}
			items = append(items, OpenAIResponsesInputItem{Role: "user", Content: content})
		case RoleAssistant:
			isDifferentModel := message.Model != model.ID && message.Provider == model.Provider && message.API == model.API
			for _, part := range message.Content {
				switch part.Type {
				case ContentText:
					items = append(items, OpenAIResponsesInputItem{
						Type:    "message",
						Role:    "assistant",
						Content: []OpenAIResponsesContentPart{{Type: "output_text", Text: SanitizeSurrogates(part.Text)}},
						Status:  "completed",
						ID:      fmt.Sprintf("msg_%d", messageIndex),
					})
					messageIndex++
				case ContentThinking:
					if part.ThinkingSignature != "" {
						var raw map[string]any
						if json.Unmarshal([]byte(part.ThinkingSignature), &raw) == nil {
							items = append(items, mapToOpenAIResponsesInputItem(raw))
						}
					}
				case ContentToolCall:
					callID, itemID, _ := strings.Cut(part.ID, "|")
					if isDifferentModel && strings.HasPrefix(itemID, "fc_") {
						itemID = ""
					}
					arguments, _ := json.Marshal(part.Arguments)
					items = append(items, OpenAIResponsesInputItem{
						Type:      "function_call",
						ID:        itemID,
						CallID:    callID,
						Name:      part.Name,
						Arguments: string(arguments),
					})
				}
			}
		case RoleToolResult:
			callID, _, _ := strings.Cut(message.ToolCallID, "|")
			output := convertOpenAIResponsesToolOutput(model, message.Content)
			items = append(items, OpenAIResponsesInputItem{Type: "function_call_output", CallID: callID, Output: output})
		}
	}
	return items
}

func convertOpenAIResponsesUserContent(content []ContentPart) []OpenAIResponsesContentPart {
	parts := make([]OpenAIResponsesContentPart, 0, len(content))
	for _, part := range content {
		switch part.Type {
		case ContentText:
			if strings.TrimSpace(part.Text) == "" {
				continue
			}
			parts = append(parts, OpenAIResponsesContentPart{Type: "input_text", Text: SanitizeSurrogates(part.Text)})
		case ContentImage:
			parts = append(parts, OpenAIResponsesContentPart{Type: "input_image", Detail: "auto", ImageURL: "data:" + part.MIMEType + ";base64," + part.Data})
		}
	}
	return parts
}

func convertOpenAIResponsesToolOutput(model Model, content []ContentPart) any {
	var textParts []string
	var outputParts []OpenAIResponsesContentPart
	for _, part := range content {
		switch part.Type {
		case ContentText:
			textParts = append(textParts, part.Text)
		case ContentImage:
			if containsString(model.Input, "image") {
				outputParts = append(outputParts, OpenAIResponsesContentPart{Type: "input_image", Detail: "auto", ImageURL: "data:" + part.MIMEType + ";base64," + part.Data})
			}
		}
	}
	text := SanitizeSurrogates(strings.Join(textParts, "\n"))
	if len(outputParts) == 0 {
		if text == "" {
			return "(see attached image)"
		}
		return text
	}
	if text != "" {
		outputParts = append([]OpenAIResponsesContentPart{{Type: "input_text", Text: text}}, outputParts...)
	}
	return outputParts
}

func mapToOpenAIResponsesInputItem(value map[string]any) OpenAIResponsesInputItem {
	item := OpenAIResponsesInputItem{}
	if text, ok := value["type"].(string); ok {
		item.Type = text
	}
	if text, ok := value["id"].(string); ok {
		item.ID = text
	}
	return item
}
