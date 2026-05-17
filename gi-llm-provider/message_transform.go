package gillmprovider

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	NonVisionUserImagePlaceholder = "(image omitted: model does not support images)"
	NonVisionToolImagePlaceholder = "(tool image omitted: model does not support images)"
)

type NormalizeToolCallIDFunc func(id string, model Model, source Message) string

func SanitizeSurrogates(text string) string {
	if utf8.ValidString(text) {
		return text
	}
	var builder strings.Builder
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)
		if r != utf8.RuneError || size != 1 {
			builder.WriteRune(r)
		}
		text = text[size:]
	}
	return builder.String()
}

func TransformMessages(messages []Message, model Model, normalizeToolCallID NormalizeToolCallIDFunc) []Message {
	toolCallIDMap := map[string]string{}
	imageAware := downgradeUnsupportedImages(messages, model)
	transformed := make([]Message, 0, len(imageAware))

	for _, message := range imageAware {
		switch message.Role {
		case RoleUser, RoleToolResult:
			if message.Role == RoleToolResult {
				if normalizedID, ok := toolCallIDMap[message.ToolCallID]; ok && normalizedID != message.ToolCallID {
					message.ToolCallID = normalizedID
				}
			}
			transformed = append(transformed, message)
		case RoleAssistant:
			isSameModel := message.Provider == model.Provider && message.API == model.API && message.Model == model.ID
			next := message
			next.Content = make([]ContentPart, 0, len(message.Content))
			for _, part := range message.Content {
				switch part.Type {
				case ContentThinking:
					if part.Redacted {
						if isSameModel {
							next.Content = append(next.Content, part)
						}
						continue
					}
					if isSameModel && part.ThinkingSignature != "" {
						next.Content = append(next.Content, part)
						continue
					}
					if strings.TrimSpace(part.Thinking) == "" {
						continue
					}
					if isSameModel {
						next.Content = append(next.Content, part)
					} else {
						next.Content = append(next.Content, Text(part.Thinking))
					}
				case ContentText:
					next.Content = append(next.Content, part)
				case ContentToolCall:
					normalized := part
					if !isSameModel {
						normalized.ThoughtSignature = ""
						if normalizeToolCallID != nil {
							normalizedID := normalizeToolCallID(part.ID, model, message)
							if normalizedID != part.ID {
								toolCallIDMap[part.ID] = normalizedID
								normalized.ID = normalizedID
							}
						}
					}
					next.Content = append(next.Content, normalized)
				default:
					next.Content = append(next.Content, part)
				}
			}
			transformed = append(transformed, next)
		default:
			transformed = append(transformed, message)
		}
	}

	return insertSyntheticToolResults(transformed)
}

func NormalizeToolCallIDForAnthropic(id string) string {
	return truncateString(sanitizeIDPart(id, "_", true), 64)
}

func NormalizeToolCallIDForOpenAICompletions(id string, model Model) string {
	if strings.Contains(id, "|") {
		callID, _, _ := strings.Cut(id, "|")
		return truncateString(sanitizeIDPart(callID, "_", true), 40)
	}
	if model.Provider == "openai" && len(id) > 40 {
		return id[:40]
	}
	return id
}

func NormalizeToolCallIDForOpenAIResponses(id string, target Model, source Message, allowedToolCallProviders map[string]bool) string {
	normalizeIDPart := func(part string) string {
		return strings.TrimRight(truncateString(sanitizeIDPart(part, "_", true), 64), "_")
	}
	if !allowedToolCallProviders[target.Provider] {
		return normalizeIDPart(id)
	}
	if !strings.Contains(id, "|") {
		return normalizeIDPart(id)
	}
	callID, itemID, _ := strings.Cut(id, "|")
	normalizedCallID := normalizeIDPart(callID)
	isForeign := source.Provider != target.Provider || source.API != target.API
	normalizedItemID := normalizeIDPart(itemID)
	if isForeign {
		normalizedItemID = "fc_" + shortHash(itemID)
		if len(normalizedItemID) > 64 {
			normalizedItemID = normalizedItemID[:64]
		}
	} else if !strings.HasPrefix(normalizedItemID, "fc_") {
		normalizedItemID = normalizeIDPart("fc_" + normalizedItemID)
	}
	return normalizedCallID + "|" + normalizedItemID
}

func downgradeUnsupportedImages(messages []Message, model Model) []Message {
	if containsString(model.Input, "image") {
		return cloneMessageSlice(messages)
	}
	result := cloneMessageSlice(messages)
	for i := range result {
		switch result[i].Role {
		case RoleUser:
			result[i].Content = replaceImagesWithPlaceholder(result[i].Content, NonVisionUserImagePlaceholder)
		case RoleToolResult:
			result[i].Content = replaceImagesWithPlaceholder(result[i].Content, NonVisionToolImagePlaceholder)
		}
	}
	return result
}

func replaceImagesWithPlaceholder(content []ContentPart, placeholder string) []ContentPart {
	result := make([]ContentPart, 0, len(content))
	previousWasPlaceholder := false
	for _, part := range content {
		if part.Type == ContentImage {
			if !previousWasPlaceholder {
				result = append(result, Text(placeholder))
			}
			previousWasPlaceholder = true
			continue
		}
		result = append(result, part)
		previousWasPlaceholder = part.Type == ContentText && part.Text == placeholder
	}
	return result
}

func insertSyntheticToolResults(messages []Message) []Message {
	var result []Message
	var pendingToolCalls []ContentPart
	existingToolResultIDs := map[string]bool{}

	insertSynthetic := func() {
		for _, toolCall := range pendingToolCalls {
			if existingToolResultIDs[toolCall.ID] {
				continue
			}
			result = append(result, Message{
				Role:       RoleToolResult,
				ToolCallID: toolCall.ID,
				ToolName:   toolCall.Name,
				Content:    []ContentPart{Text("No result provided")},
				IsError:    true,
				Timestamp:  NowMillis(),
			})
		}
		pendingToolCalls = nil
		existingToolResultIDs = map[string]bool{}
	}

	for _, message := range messages {
		switch message.Role {
		case RoleAssistant:
			insertSynthetic()
			if message.StopReason == StopReasonError || message.StopReason == StopReasonAborted {
				continue
			}
			for _, part := range message.Content {
				if part.Type == ContentToolCall {
					pendingToolCalls = append(pendingToolCalls, part)
				}
			}
			result = append(result, message)
		case RoleToolResult:
			existingToolResultIDs[message.ToolCallID] = true
			result = append(result, message)
		case RoleUser:
			insertSynthetic()
			result = append(result, message)
		default:
			result = append(result, message)
		}
	}
	insertSynthetic()
	return result
}

var nonToolIDChar = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func sanitizeIDPart(value, replacement string, allowUnderscore bool) string {
	if allowUnderscore {
		return nonToolIDChar.ReplaceAllString(value, replacement)
	}
	var builder strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func truncateString(value string, maxLen int) string {
	if maxLen <= 0 || len(value) <= maxLen {
		return value
	}
	return value[:maxLen]
}

func shortHash(value string) string {
	var h1 uint32 = 0xdeadbeef
	var h2 uint32 = 0x41c6ce57
	for _, ch := range utf16.Encode([]rune(value)) {
		h1 = imul32(h1^uint32(ch), 2654435761)
		h2 = imul32(h2^uint32(ch), 1597334677)
	}
	h1 = imul32(h1^(h1>>16), 2246822507) ^ imul32(h2^(h2>>13), 3266489909)
	h2 = imul32(h2^(h2>>16), 2246822507) ^ imul32(h1^(h1>>13), 3266489909)
	return strconv.FormatUint(uint64(h2), 36) + strconv.FormatUint(uint64(h1), 36)
}

func imul32(a, b uint32) uint32 {
	return uint32(uint64(a) * uint64(b))
}

func cloneMessageSlice(messages []Message) []Message {
	if messages == nil {
		return nil
	}
	result := make([]Message, len(messages))
	for i, message := range messages {
		result[i] = message
		result[i].Content = append([]ContentPart{}, message.Content...)
	}
	return result
}
