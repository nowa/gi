package harness

import (
	"fmt"
	"strings"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

const CompactionSummaryPrefix = `The conversation history before this point was compacted into the following summary:

<summary>
`

const CompactionSummarySuffix = `
</summary>`

const BranchSummaryPrefix = `The following is a summary of a branch that this conversation came back from:

<summary>
`

const BranchSummarySuffix = `</summary>`

type BashExecutionTextOptions struct {
	Output         string
	ExitCode       *int
	Cancelled      bool
	Truncated      bool
	FullOutputPath string
}

func BashExecutionText(command string, options BashExecutionTextOptions) string {
	var builder strings.Builder
	builder.WriteString("Ran `")
	builder.WriteString(command)
	builder.WriteString("`\n")
	if options.Output != "" {
		builder.WriteString("```\n")
		builder.WriteString(options.Output)
		if !strings.HasSuffix(options.Output, "\n") {
			builder.WriteByte('\n')
		}
		builder.WriteString("```")
	} else {
		builder.WriteString("(no output)")
	}
	if options.Cancelled {
		builder.WriteString("\n\n(command cancelled)")
	} else if options.ExitCode != nil && *options.ExitCode != 0 {
		builder.WriteString(fmt.Sprintf("\n\nCommand exited with code %d", *options.ExitCode))
	}
	if options.Truncated && options.FullOutputPath != "" {
		builder.WriteString("\n\n[Output truncated. Full output: ")
		builder.WriteString(options.FullOutputPath)
		builder.WriteByte(']')
	}
	return builder.String()
}

func BranchSummaryText(summary string) string {
	return BranchSummaryPrefix + summary + BranchSummarySuffix
}

func CompactionSummaryText(summary string) string {
	return CompactionSummaryPrefix + summary + CompactionSummarySuffix
}

func ConvertToLLM(messages []llm.Message) []llm.Message {
	result := make([]llm.Message, 0, len(messages))
	for _, message := range messages {
		switch message.Role {
		case "bashExecution":
			if MessageExcludedFromContext(message) {
				continue
			}
			message.Role = llm.RoleUser
			clearSyntheticMessageFields(&message)
		case "custom":
			message.Role = llm.RoleUser
			clearSyntheticMessageFields(&message)
		case "branchSummary":
			message.Role = llm.RoleUser
			message.Content = []llm.ContentPart{llm.Text(BranchSummaryText(messageTextContent(message)))}
			clearSyntheticMessageFields(&message)
		case "compactionSummary":
			message.Role = llm.RoleUser
			message.Content = []llm.ContentPart{llm.Text(CompactionSummaryText(messageTextContent(message)))}
			clearSyntheticMessageFields(&message)
		case llm.RoleUser, llm.RoleAssistant, llm.RoleToolResult:
		default:
			continue
		}
		result = append(result, message)
	}
	return result
}

func MessageExcludedFromContext(message llm.Message) bool {
	switch details := message.Details.(type) {
	case map[string]any:
		excluded, _ := details["excludeFromContext"].(bool)
		return excluded
	case map[string]bool:
		return details["excludeFromContext"]
	default:
		return false
	}
}

func customMessageFromEntry(entry Entry) llm.Message {
	display := entry.Display
	return llm.Message{
		Role:       "custom",
		Content:    contentPartsFromValue(entry.Content),
		Timestamp:  entryTimestampMillis(entry.Timestamp),
		CustomType: entry.CustomType,
		Display:    &display,
		Details:    entry.Details,
	}
}

func branchSummaryMessageFromEntry(entry Entry) llm.Message {
	details := map[string]any{}
	if entry.FromID != "" {
		details["fromId"] = entry.FromID
	}
	return syntheticSummaryMessage("branchSummary", entry.Summary, entry.Timestamp, details)
}

func compactionSummaryMessageFromEntry(entry Entry) llm.Message {
	details := map[string]any{}
	if entry.TokensBefore != 0 {
		details["tokensBefore"] = entry.TokensBefore
	}
	return syntheticSummaryMessage("compactionSummary", entry.Summary, entry.Timestamp, details)
}

func syntheticSummaryMessage(role, summary, timestamp string, details map[string]any) llm.Message {
	var messageDetails any
	if len(details) > 0 {
		messageDetails = details
	}
	return llm.Message{
		Role:      role,
		Content:   []llm.ContentPart{llm.Text(summary)},
		Timestamp: entryTimestampMillis(timestamp),
		Details:   messageDetails,
	}
}

func entryMessage(entry Entry) llm.Message {
	switch entry.Type {
	case "message":
		return entry.Message
	case "custom_message":
		return customMessageFromEntry(entry)
	case "branch_summary":
		return branchSummaryMessageFromEntry(entry)
	default:
		return llm.Message{Role: "unknown"}
	}
}

func contentPartsFromValue(value any) []llm.ContentPart {
	switch typed := value.(type) {
	case []llm.ContentPart:
		return append([]llm.ContentPart(nil), typed...)
	case string:
		return []llm.ContentPart{llm.Text(typed)}
	case []byte:
		return []llm.ContentPart{llm.Text(string(typed))}
	case nil:
		return []llm.ContentPart{llm.Text("")}
	case []any:
		parts := make([]llm.ContentPart, 0, len(typed))
		for _, item := range typed {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			part, ok := contentPartFromMap(block)
			if ok {
				parts = append(parts, part)
			}
		}
		if len(parts) > 0 {
			return parts
		}
	}
	return []llm.ContentPart{llm.Text(fmt.Sprint(value))}
}

func contentPartFromMap(block map[string]any) (llm.ContentPart, bool) {
	blockType, _ := block["type"].(string)
	switch blockType {
	case llm.ContentText:
		part := llm.Text(stringValue(block["text"]))
		part.TextSignature = stringValue(block["textSignature"])
		return part, true
	case llm.ContentThinking:
		part := llm.Thinking(stringValue(block["thinking"]))
		part.ThinkingSignature = stringValue(block["thinkingSignature"])
		if redacted, ok := block["redacted"].(bool); ok {
			part.Redacted = redacted
		}
		return part, true
	case llm.ContentImage:
		return llm.Image(stringValue(block["data"]), stringValue(block["mimeType"])), true
	case llm.ContentToolCall:
		part := llm.ToolCall(stringValue(block["id"]), stringValue(block["name"]), mapValue(block["arguments"]))
		part.ThoughtSignature = stringValue(block["thoughtSignature"])
		return part, true
	default:
		return llm.ContentPart{}, false
	}
}

func messageTextContent(message llm.Message) string {
	parts := make([]string, 0, len(message.Content))
	for _, part := range message.Content {
		if part.Type == llm.ContentText {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "")
}

func clearSyntheticMessageFields(message *llm.Message) {
	message.CustomType = ""
	message.Display = nil
	message.Details = nil
}

func entryTimestampMillis(timestamp string) int64 {
	if strings.TrimSpace(timestamp) == "" {
		return llm.NowMillis()
	}
	parsed, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return llm.NowMillis()
	}
	return parsed.UnixMilli()
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func mapValue(value any) map[string]any {
	if value == nil {
		return nil
	}
	result, _ := value.(map[string]any)
	return result
}
