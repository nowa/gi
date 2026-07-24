package gillmprovider

import (
	"encoding/json"
	"math"
)

const (
	estimatedCharactersPerToken = 4
	estimatedImageCharacters    = 4800
)

// ContextUsageEstimate separates a provider-reported prefix checkpoint from
// locally estimated content appended after that checkpoint.
type ContextUsageEstimate struct {
	Tokens         int  `json:"tokens"`
	UsageTokens    int  `json:"usageTokens"`
	TrailingTokens int  `json:"trailingTokens"`
	LastUsageIndex *int `json:"lastUsageIndex"`
}

// CalculateContextTokens returns the provider total when present and otherwise
// reconstructs it from the four input/output/cache buckets.
func CalculateContextTokens(usage Usage) int {
	if usage.TotalTokens != 0 {
		return usage.TotalTokens
	}
	return usage.Input + usage.Output + usage.CacheRead + usage.CacheWrite
}

// EstimateTextTokens applies Pi's deliberately cheap four UTF-16-code-units
// per token heuristic.
func EstimateTextTokens(text string) int {
	return divideCharactersIntoTokens(utf16CodeUnits(text))
}

// EstimateTextAndImageContentTokens estimates text blocks and assigns the same
// fixed 1,200-token budget Pi uses for each image.
func EstimateTextAndImageContentTokens(content []ContentPart) int {
	characters := 0
	for _, part := range content {
		switch part.Type {
		case ContentText:
			characters += utf16CodeUnits(part.Text)
		case ContentImage:
			characters += estimatedImageCharacters
		}
	}
	return divideCharactersIntoTokens(characters)
}

// EstimateMessageTokens estimates one canonical LLM message. Unknown roles are
// intentionally ignored because they belong to higher-level transcript types.
func EstimateMessageTokens(message Message) int {
	switch message.Role {
	case RoleUser, RoleToolResult:
		return EstimateTextAndImageContentTokens(message.Content)
	case RoleAssistant:
		characters := 0
		for _, part := range message.Content {
			switch part.Type {
			case ContentText:
				characters += utf16CodeUnits(part.Text)
			case ContentThinking:
				characters += utf16CodeUnits(part.Thinking)
			case ContentToolCall:
				characters += utf16CodeUnits(part.Name)
				characters += utf16CodeUnits(safeJSONString(part.Arguments))
			}
		}
		return divideCharactersIntoTokens(characters)
	default:
		return 0
	}
}

// EstimateMessagesTokens estimates a message-only context.
func EstimateMessagesTokens(messages []Message) ContextUsageEstimate {
	usage, usageIndex, ok := lastApplicableAssistantUsage(messages)
	if ok {
		usageTokens := CalculateContextTokens(usage)
		trailingTokens := 0
		for _, message := range messages[usageIndex+1:] {
			trailingTokens += EstimateMessageTokens(message)
		}
		index := usageIndex
		return ContextUsageEstimate{
			Tokens:         usageTokens + trailingTokens,
			UsageTokens:    usageTokens,
			TrailingTokens: trailingTokens,
			LastUsageIndex: &index,
		}
	}

	tokens := 0
	for _, message := range messages {
		tokens += EstimateMessageTokens(message)
	}
	return ContextUsageEstimate{Tokens: tokens, TrailingTokens: tokens}
}

// EstimateContextTokens estimates the full provider context. A valid assistant
// usage checkpoint already includes the prior system prompt and tool schemas,
// so only transcript-loaded tools introduced after that checkpoint are added.
func EstimateContextTokens(context Context) ContextUsageEstimate {
	estimate := EstimateMessagesTokens(context.Messages)
	if estimate.LastUsageIndex != nil {
		addedNames := make(map[string]struct{})
		for _, message := range context.Messages[*estimate.LastUsageIndex+1:] {
			if message.Role != RoleToolResult {
				continue
			}
			for _, name := range message.AddedToolNames {
				addedNames[name] = struct{}{}
			}
		}
		if len(addedNames) > 0 {
			addedTools := make([]Tool, 0, len(addedNames))
			for _, tool := range context.Tools {
				if _, ok := addedNames[tool.Name]; ok {
					addedTools = append(addedTools, tool)
				}
			}
			addedTokens := estimateToolsTokens(addedTools)
			estimate.Tokens += addedTokens
			estimate.TrailingTokens += addedTokens
		}
		return estimate
	}

	prefixTokens := EstimateTextTokens(context.SystemPrompt) + estimateToolsTokens(context.Tools)
	estimate.Tokens += prefixTokens
	estimate.TrailingTokens += prefixTokens
	return estimate
}

func lastApplicableAssistantUsage(messages []Message) (Usage, int, bool) {
	latestPrefixTimestamp := int64(math.MinInt64)
	var usage Usage
	usageIndex := -1
	for index, message := range messages {
		if message.Role == RoleAssistant &&
			message.Timestamp >= latestPrefixTimestamp &&
			message.StopReason != StopReasonAborted &&
			message.StopReason != StopReasonError &&
			CalculateContextTokens(message.Usage) > 0 {
			usage = message.Usage
			usageIndex = index
		}
		if message.Timestamp > latestPrefixTimestamp {
			latestPrefixTimestamp = message.Timestamp
		}
	}
	return usage, usageIndex, usageIndex >= 0
}

func estimateToolsTokens(tools []Tool) int {
	if len(tools) == 0 {
		return 0
	}
	return EstimateTextTokens(safeJSONString(tools))
}

func safeJSONString(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "[unserializable]"
	}
	return string(encoded)
}

func divideCharactersIntoTokens(characters int) int {
	return (characters + estimatedCharactersPerToken - 1) / estimatedCharactersPerToken
}

func utf16CodeUnits(value string) int {
	units := 0
	for _, codePoint := range value {
		if codePoint > 0xffff {
			units += 2
		} else {
			units++
		}
	}
	return units
}
