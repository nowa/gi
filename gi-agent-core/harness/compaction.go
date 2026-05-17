package harness

import (
	"context"
	"fmt"
	"sort"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type CompactionSettings struct {
	Enabled          bool
	ReserveTokens    int
	KeepRecentTokens int
}

var DefaultCompactionSettings = CompactionSettings{Enabled: true, ReserveTokens: 10_000, KeepRecentTokens: 20_000}

type ContextTokenEstimate struct {
	Tokens         int
	UsageTokens    int
	EstimateTokens int
	LastUsageIndex *int
}

type CutPoint struct {
	FirstKeptEntryIndex int
	TurnStartIndex      int
	IsSplitTurn         bool
}

type FileOps struct {
	Read    map[string]bool
	Written map[string]bool
	Edited  map[string]bool
}

type CompactionPreparation struct {
	FirstKeptEntryID    string
	MessagesToSummarize []llm.Message
	TurnPrefixMessages  []llm.Message
	IsSplitTurn         bool
	TokensBefore        int
	PreviousSummary     string
	FileOps             FileOps
	Settings            CompactionSettings
}

type CompactionResult struct {
	Summary          string
	FirstKeptEntryID string
	TokensBefore     int
	Details          map[string]any
}

type CompactOptions struct {
	APIKey             string
	ThinkingLevel      string
	CustomInstructions string
}

const summarizationSystemPrompt = "You are a context summarization assistant. Read the conversation and produce a concise structured summary. Do not continue the conversation."

func CalculateContextTokens(usage llm.Usage) int {
	return usage.Input + usage.Output + usage.CacheRead + usage.CacheWrite
}

func ShouldCompact(contextTokens, contextWindow int, settings CompactionSettings) bool {
	if !settings.Enabled {
		return false
	}
	return contextTokens >= contextWindow-settings.ReserveTokens
}

func EstimateTokens(message llm.Message) int {
	switch message.Role {
	case llm.RoleUser, llm.RoleAssistant, llm.RoleToolResult, "custom", "branchSummary", "compactionSummary", "bashExecution":
	default:
		return 0
	}
	tokens := 4
	for _, part := range message.Content {
		switch part.Type {
		case llm.ContentText:
			tokens += len([]rune(part.Text))/4 + 1
		case llm.ContentThinking:
			tokens += len([]rune(part.Thinking))/4 + 1
		case llm.ContentToolCall:
			tokens += len(part.Name) + len(fmt.Sprint(part.Arguments))/4 + 8
		case llm.ContentImage:
			tokens += 1000 + len(part.Data)/1024
		}
	}
	if message.Role == llm.RoleAssistant && message.StopReason != llm.StopReasonError && message.StopReason != llm.StopReasonAborted {
		if usage := CalculateContextTokens(message.Usage); usage > tokens {
			return usage
		}
	}
	if message.Role == llm.RoleToolResult && len(message.Content) == 0 {
		tokens += len(fmt.Sprint(message.Details)) / 4
	}
	return tokens
}

func EstimateContextTokens(messages []llm.Message) ContextTokenEstimate {
	total := 0
	usageTokens := 0
	estimateTokens := 0
	var lastUsageIndex *int
	for i, message := range messages {
		estimated := EstimateTokens(message)
		estimateTokens += estimated
		total += estimated
		if message.Role == llm.RoleAssistant && message.StopReason != llm.StopReasonError && message.StopReason != llm.StopReasonAborted {
			usage := CalculateContextTokens(message.Usage)
			if usage > 0 {
				index := i
				lastUsageIndex = &index
				usageTokens = usage
				total = usage
			}
		}
	}
	return ContextTokenEstimate{Tokens: total, UsageTokens: usageTokens, EstimateTokens: estimateTokens, LastUsageIndex: lastUsageIndex}
}

func GetLastAssistantUsage(entries []Entry) *llm.Usage {
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if entry.Type == "message" && entry.Message.Role == llm.RoleAssistant && entry.Message.StopReason != llm.StopReasonError && entry.Message.StopReason != llm.StopReasonAborted {
			usage := entry.Message.Usage
			if CalculateContextTokens(usage) > 0 {
				return &usage
			}
		}
	}
	return nil
}

func FindTurnStartIndex(entries []Entry, index, minIndex int) int {
	for i := index; i >= minIndex; i-- {
		entry := entries[i]
		if entry.Type == "branch_summary" || entry.Type == "custom_message" {
			return i
		}
		if entry.Type != "message" {
			continue
		}
		if entry.Message.Role == llm.RoleUser || entry.Message.Role == "bashExecution" {
			return i
		}
	}
	return -1
}

func FindCutPoint(entries []Entry, start, end, keepRecentTokens int) CutPoint {
	if len(entries) == 0 || start >= end {
		return CutPoint{FirstKeptEntryIndex: start, TurnStartIndex: -1}
	}
	cutPoints := findValidCutPoints(entries, start, end)
	if len(cutPoints) == 0 {
		return CutPoint{FirstKeptEntryIndex: start, TurnStartIndex: -1}
	}
	accumulatedTokens := 0
	cutIndex := cutPoints[0]
	for i := end - 1; i >= start; i-- {
		if entries[i].Type != "message" {
			continue
		}
		accumulatedTokens += EstimateTokens(entries[i].Message)
		if accumulatedTokens >= keepRecentTokens {
			for _, candidate := range cutPoints {
				if candidate >= i {
					cutIndex = candidate
					break
				}
			}
			break
		}
	}
	for cutIndex > start {
		previous := entries[cutIndex-1]
		if previous.Type == "compaction" || previous.Type == "message" {
			break
		}
		cutIndex--
	}
	cutEntry := entries[cutIndex]
	isUserMessage := cutEntry.Type == "message" && cutEntry.Message.Role == llm.RoleUser
	turnStart := -1
	if !isUserMessage {
		turnStart = FindTurnStartIndex(entries, cutIndex, start)
	}
	return CutPoint{FirstKeptEntryIndex: cutIndex, TurnStartIndex: turnStart, IsSplitTurn: !isUserMessage && turnStart != -1}
}

func PrepareCompaction(pathEntries []Entry, settings CompactionSettings) (*CompactionPreparation, error) {
	if len(pathEntries) == 0 || !settings.Enabled || pathEntries[len(pathEntries)-1].Type == "compaction" {
		return nil, nil
	}
	sessionContext := BuildSessionContext(pathEntries)
	tokensBefore := EstimateContextTokens(sessionContext.Messages).Tokens

	prevCompactionIndex := -1
	for i := len(pathEntries) - 1; i >= 0; i-- {
		if pathEntries[i].Type == "compaction" {
			prevCompactionIndex = i
			break
		}
	}
	previousSummary := ""
	boundaryStart := 0
	if prevCompactionIndex >= 0 {
		previous := pathEntries[prevCompactionIndex]
		previousSummary = previous.Summary
		boundaryStart = prevCompactionIndex + 1
		for i, entry := range pathEntries {
			if entry.ID == previous.FirstKeptEntryID {
				boundaryStart = i
				break
			}
		}
	}

	cut := FindCutPoint(pathEntries, boundaryStart, len(pathEntries), settings.KeepRecentTokens)
	if cut.FirstKeptEntryIndex < boundaryStart || cut.FirstKeptEntryIndex >= len(pathEntries) {
		return nil, nil
	}
	if cut.FirstKeptEntryIndex == boundaryStart && !cut.IsSplitTurn {
		return nil, nil
	}
	firstKept := pathEntries[cut.FirstKeptEntryIndex]
	if firstKept.ID == "" {
		return nil, newSessionError("invalid_session", "first kept entry has no ID")
	}
	historyEnd := cut.FirstKeptEntryIndex
	if cut.IsSplitTurn {
		historyEnd = cut.TurnStartIndex
	}
	messagesToSummarize := entriesToMessages(pathEntries[boundaryStart:historyEnd])
	turnPrefix := []llm.Message{}
	if cut.IsSplitTurn && cut.TurnStartIndex >= 0 {
		turnPrefix = entriesToMessages(pathEntries[cut.TurnStartIndex:cut.FirstKeptEntryIndex])
	}
	if len(messagesToSummarize) == 0 && len(turnPrefix) == 0 {
		return nil, nil
	}
	fileOpsEnd := cut.FirstKeptEntryIndex
	prep := &CompactionPreparation{
		FirstKeptEntryID:    firstKept.ID,
		MessagesToSummarize: messagesToSummarize,
		TurnPrefixMessages:  turnPrefix,
		IsSplitTurn:         cut.IsSplitTurn,
		TokensBefore:        tokensBefore,
		PreviousSummary:     previousSummary,
		FileOps:             collectFileOps(pathEntries[boundaryStart:fileOpsEnd]),
		Settings:            settings,
	}
	return prep, nil
}

func SerializeConversation(messages []llm.Message) string {
	var out strings.Builder
	for _, message := range messages {
		switch message.Role {
		case llm.RoleUser:
			out.WriteString("[User]:\n")
		case llm.RoleAssistant:
			out.WriteString("[Assistant]:\n")
		case llm.RoleToolResult:
			out.WriteString("[Tool result]:\n")
		default:
			out.WriteString("[" + message.Role + "]:\n")
		}
		for _, part := range message.Content {
			if part.Type == llm.ContentText {
				text := part.Text
				if message.Role == llm.RoleToolResult && len([]rune(text)) > 2000 {
					remaining := len([]rune(text)) - 2000
					text = string([]rune(text)[:2000]) + fmt.Sprintf("\n[... %d more characters truncated]", remaining)
				}
				out.WriteString(text)
				out.WriteString("\n")
			}
			if part.Type == llm.ContentToolCall {
				out.WriteString(fmt.Sprintf("[Tool call %s]: %v\n", part.Name, part.Arguments))
			}
		}
	}
	return out.String()
}

func GenerateSummary(ctx context.Context, messages []llm.Message, model llm.Model, maxTokens int, apiKey string, previousSummary, focus, thinkingLevel string) (string, error) {
	prompt := "Summarize the following conversation.\n"
	if previousSummary != "" {
		prompt += "<previous-summary>\n" + previousSummary + "\n</previous-summary>\n"
	}
	if focus != "" {
		prompt += "Additional focus: " + focus + "\n"
	}
	prompt += SerializeConversation(messages)
	options := llm.SimpleStreamOptions{APIKey: apiKey, MaxTokens: min(maxTokens, model.MaxTokens)}
	if model.Reasoning && thinkingLevel != "" && thinkingLevel != "off" {
		options.Reasoning = thinkingLevel
	}
	stream, err := llm.StreamSimple(model, llm.Context{SystemPrompt: summarizationSystemPrompt, Messages: []llm.Message{llm.UserMessageText(prompt)}}, options)
	if err != nil {
		return "", err
	}
	message, err := stream.Result(ctx)
	if err != nil {
		return "", err
	}
	if message.StopReason == llm.StopReasonAborted {
		return "", fmt.Errorf("aborted: %s", message.ErrorMessage)
	}
	if message.StopReason == llm.StopReasonError {
		return "", fmt.Errorf("summarization failed: %s", message.ErrorMessage)
	}
	for _, part := range message.Content {
		if part.Type == llm.ContentText {
			return part.Text, nil
		}
	}
	return "", nil
}

func Compact(ctx context.Context, prep CompactionPreparation, model llm.Model, apiKey string, thinkingLevel string) (CompactionResult, error) {
	return CompactWithOptions(ctx, prep, model, CompactOptions{APIKey: apiKey, ThinkingLevel: thinkingLevel})
}

func CompactWithOptions(ctx context.Context, prep CompactionPreparation, model llm.Model, options CompactOptions) (CompactionResult, error) {
	if prep.FirstKeptEntryID == "" || (len(prep.MessagesToSummarize) == 0 && len(prep.TurnPrefixMessages) == 0) {
		return CompactionResult{}, newSessionError("invalid_session", "invalid compaction preparation")
	}
	var summary string
	var err error
	if len(prep.MessagesToSummarize) > 0 {
		summary, err = GenerateSummary(ctx, prep.MessagesToSummarize, model, prep.Settings.ReserveTokens, options.APIKey, prep.PreviousSummary, options.CustomInstructions, options.ThinkingLevel)
		if err != nil {
			return CompactionResult{}, err
		}
	} else if prep.IsSplitTurn {
		summary = "No prior history."
	}
	if len(prep.TurnPrefixMessages) > 0 {
		prefix, err := GenerateSummary(ctx, prep.TurnPrefixMessages, model, prep.Settings.ReserveTokens, options.APIKey, "", "Summarize this prefix of a turn; the suffix remains in context.", options.ThinkingLevel)
		if err != nil {
			return CompactionResult{}, err
		}
		if summary != "" {
			summary += "\n\n---\n\n"
		}
		summary += "**Turn Context (split turn):**\n\n" + prefix
	}
	return CompactionResult{
		Summary:          summary,
		FirstKeptEntryID: prep.FirstKeptEntryID,
		TokensBefore:     prep.TokensBefore,
		Details: map[string]any{
			"readFiles":     keys(prep.FileOps.Read),
			"writtenFiles":  keys(prep.FileOps.Written),
			"modifiedFiles": keys(prep.FileOps.Edited),
		},
	}, nil
}

func entryMessage(entry Entry) llm.Message {
	switch entry.Type {
	case "message":
		return entry.Message
	case "custom_message":
		return llm.Message{Role: entry.CustomType, Content: []llm.ContentPart{llm.Text(fmt.Sprint(entry.Content))}}
	case "branch_summary":
		return llm.Message{Role: "branchSummary", Content: []llm.ContentPart{llm.Text(entry.Summary)}}
	case "compaction":
		return llm.Message{Role: "compactionSummary", Content: []llm.ContentPart{llm.Text(entry.Summary)}}
	default:
		return llm.Message{Role: "unknown"}
	}
}

func findValidCutPoints(entries []Entry, start, end int) []int {
	var cutPoints []int
	for i := start; i < end; i++ {
		entry := entries[i]
		if entry.Type == "message" {
			switch entry.Message.Role {
			case "bashExecution", "custom", "branchSummary", "compactionSummary", llm.RoleUser, llm.RoleAssistant:
				cutPoints = append(cutPoints, i)
			}
		}
		if entry.Type == "branch_summary" || entry.Type == "custom_message" {
			cutPoints = append(cutPoints, i)
		}
	}
	return cutPoints
}

func entriesToMessages(entries []Entry) []llm.Message {
	var messages []llm.Message
	for _, entry := range entries {
		message := entryMessage(entry)
		if message.Role != "unknown" {
			messages = append(messages, message)
		}
	}
	return messages
}

func collectFileOps(entries []Entry) FileOps {
	ops := FileOps{Read: map[string]bool{}, Written: map[string]bool{}, Edited: map[string]bool{}}
	for _, entry := range entries {
		if entry.Type == "compaction" {
			if details, ok := entry.Details.(map[string]any); ok {
				addStringSlice(ops.Read, details["readFiles"])
				addStringSlice(ops.Written, details["writtenFiles"])
				addStringSlice(ops.Edited, details["modifiedFiles"])
			}
		}
		if entry.Type == "message" && entry.Message.Role == llm.RoleAssistant {
			for _, part := range entry.Message.Content {
				if part.Type == llm.ContentToolCall {
					path, _ := part.Arguments["path"].(string)
					switch part.Name {
					case "read":
						ops.Read[path] = path != ""
					case "write":
						ops.Written[path] = path != ""
					case "edit":
						ops.Edited[path] = path != ""
					}
				}
			}
		}
	}
	return ops
}

func addStringSlice(target map[string]bool, value any) {
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			target[item] = true
		}
	case []any:
		for _, item := range typed {
			if s, ok := item.(string); ok {
				target[s] = true
			}
		}
	}
}

func keys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for key, ok := range values {
		if ok && key != "" {
			result = append(result, key)
		}
	}
	sort.Strings(result)
	return result
}
