package harness

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type CompactionSettings struct {
	Enabled          bool `json:"enabled"`
	ReserveTokens    int  `json:"reserveTokens"`
	KeepRecentTokens int  `json:"keepRecentTokens"`
}

var DefaultCompactionSettings = CompactionSettings{Enabled: true, ReserveTokens: 16_384, KeepRecentTokens: 20_000}

type ContextTokenEstimate struct {
	Tokens         int  `json:"tokens"`
	UsageTokens    int  `json:"usageTokens"`
	EstimateTokens int  `json:"estimateTokens"`
	LastUsageIndex *int `json:"lastUsageIndex"`
}

type CutPoint struct {
	FirstKeptEntryIndex int  `json:"firstKeptEntryIndex"`
	TurnStartIndex      int  `json:"turnStartIndex"`
	IsSplitTurn         bool `json:"isSplitTurn"`
}

type FileOps struct {
	Read    map[string]bool `json:"read"`
	Written map[string]bool `json:"written"`
	Edited  map[string]bool `json:"edited"`
}

type CompactionPreparation struct {
	FirstKeptEntryID    string             `json:"firstKeptEntryId"`
	MessagesToSummarize []llm.Message      `json:"messagesToSummarize"`
	TurnPrefixMessages  []llm.Message      `json:"turnPrefixMessages"`
	IsSplitTurn         bool               `json:"isSplitTurn"`
	TokensBefore        int                `json:"tokensBefore"`
	PreviousSummary     string             `json:"previousSummary,omitempty"`
	FileOps             FileOps            `json:"fileOps"`
	Settings            CompactionSettings `json:"settings"`
}

type CompactionResult struct {
	Summary          string         `json:"summary"`
	FirstKeptEntryID string         `json:"firstKeptEntryId"`
	TokensBefore     int            `json:"tokensBefore"`
	Usage            *llm.Usage     `json:"usage,omitempty"`
	Details          map[string]any `json:"details,omitempty"`
}

// SimpleCompletionRuntime is the minimal model-runtime contract required by
// generated summaries. Both llm.Models and coding-agent's ModelRuntime satisfy
// this interface, so the harness owns summary policy without owning provider
// registration or credentials.
type SimpleCompletionRuntime interface {
	CompleteSimple(context.Context, llm.Model, llm.Context, llm.ModelsStreamOptions) (llm.Message, error)
}

type CompactOptions struct {
	APIKey             string
	ThinkingLevel      string
	CustomInstructions string
	Runtime            SimpleCompletionRuntime
	RequestOptions     llm.ModelsStreamOptions
	Retry              llm.RetryPolicy
	RetryCallbacks     llm.RetryCallbacks
}

// SummaryWithUsage is the generated text and provider usage for one summary
// request.
type SummaryWithUsage struct {
	Text  string
	Usage llm.Usage
}

const summarizationSystemPrompt = `You are a context summarization assistant. Your task is to read a conversation between a user and an AI coding assistant, then produce a structured summary following the exact format specified.

Do NOT continue the conversation. Do NOT respond to any questions in the conversation. ONLY output the structured summary.`

const summarizationPrompt = `The messages above are a conversation to summarize. Create a structured context checkpoint summary that another LLM will use to continue the work.

Use this EXACT format:

## Goal
[What is the user trying to accomplish? Can be multiple items if the session covers different tasks.]

## Constraints & Preferences
- [Any constraints, preferences, or requirements mentioned by user]
- [Or "(none)" if none were mentioned]

## Progress
### Done
- [x] [Completed tasks/changes]

### In Progress
- [ ] [Current work]

### Blocked
- [Issues preventing progress, if any]

## Key Decisions
- **[Decision]**: [Brief rationale]

## Next Steps
1. [Ordered list of what should happen next]

## Critical Context
- [Any data, examples, or references needed to continue]
- [Or "(none)" if not applicable]

Keep each section concise. Preserve exact file paths, function names, and error messages.`

const updateSummarizationPrompt = `The messages above are NEW conversation messages to incorporate into the existing summary provided in <previous-summary> tags.

Update the existing structured summary with new information. RULES:
- PRESERVE all existing information from the previous summary
- ADD new progress, decisions, and context from the new messages
- UPDATE the Progress section: move items from "In Progress" to "Done" when completed
- UPDATE "Next Steps" based on what was accomplished
- PRESERVE exact file paths, function names, and error messages
- If something is no longer relevant, you may remove it

Use this EXACT format:

## Goal
[Preserve existing goals, add new ones if the task expanded]

## Constraints & Preferences
- [Preserve existing, add new ones discovered]

## Progress
### Done
- [x] [Include previously done items AND newly completed items]

### In Progress
- [ ] [Current work - update based on progress]

### Blocked
- [Current blockers - remove if resolved]

## Key Decisions
- **[Decision]**: [Brief rationale] (preserve all previous, add new)

## Next Steps
1. [Update based on current state]

## Critical Context
- [Preserve important context, add new if needed]

Keep each section concise. Preserve exact file paths, function names, and error messages.`

const turnPrefixSummarizationPrompt = `This is the PREFIX of a turn that was too large to keep. The SUFFIX (recent work) is retained.

Summarize the prefix to provide context for the retained suffix:

## Original Request
[What did the user ask for in this turn?]

## Early Progress
- [Key decisions and work done in the prefix]

## Context for Suffix
- [Information needed to understand the retained recent work]

Be concise. Focus on what's needed to understand the kept suffix.`

func CalculateContextTokens(usage llm.Usage) int {
	if usage.TotalTokens != 0 {
		return usage.TotalTokens
	}
	return usage.Input + usage.Output + usage.CacheRead + usage.CacheWrite
}

func ShouldCompact(contextTokens, contextWindow int, settings CompactionSettings) bool {
	if !settings.Enabled {
		return false
	}
	return contextTokens > contextWindow-settings.ReserveTokens
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
	firstKept := pathEntries[cut.FirstKeptEntryIndex]
	if firstKept.ID == "" {
		return nil, newCompactionError(CompactionErrorInvalidSession, "first kept entry has no ID")
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
	entries := make([]string, 0, len(messages))
	for _, message := range messages {
		label := "[" + message.Role + "]:"
		switch message.Role {
		case llm.RoleUser:
			label = "[User]:"
		case llm.RoleAssistant:
			label = "[Assistant]:"
		case llm.RoleToolResult:
			label = "[Tool result]:"
		}
		var content strings.Builder
		for _, part := range message.Content {
			if part.Type == llm.ContentText {
				text := part.Text
				if message.Role == llm.RoleToolResult && len([]rune(text)) > 2000 {
					remaining := len([]rune(text)) - 2000
					text = string([]rune(text)[:2000]) + fmt.Sprintf("\n[... %d more characters truncated]", remaining)
				}
				content.WriteString(text)
			}
			if part.Type == llm.ContentToolCall {
				if content.Len() > 0 {
					content.WriteString("\n")
				}
				content.WriteString(fmt.Sprintf("[Tool call %s]: %v", part.Name, part.Arguments))
			}
		}
		text := strings.TrimSuffix(content.String(), "\n")
		if text == "" {
			entries = append(entries, label)
		} else {
			entries = append(entries, label+" "+text)
		}
	}
	return strings.Join(entries, "\n")
}

type directSimpleCompletionRuntime struct{}

func (directSimpleCompletionRuntime) CompleteSimple(
	ctx context.Context,
	model llm.Model,
	llmContext llm.Context,
	options llm.ModelsStreamOptions,
) (llm.Message, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	streamOptions := options.StreamOptions
	streamOptions.Context = ctx
	if options.APIKey != nil {
		streamOptions.APIKey = *options.APIKey
	}
	if options.TransformHeaders != nil {
		headers := make(map[string]string, len(streamOptions.Headers))
		for key, value := range streamOptions.Headers {
			headers[key] = value
		}
		transformed, err := options.TransformHeaders(ctx, headers)
		if err != nil {
			return llm.Message{}, err
		}
		streamOptions.Headers = transformed
	}
	stream, err := llm.StreamSimple(model, llmContext, streamOptions)
	if err != nil {
		return llm.Message{}, err
	}
	if stream == nil {
		return llm.Message{}, errors.New("simple completion returned nil stream")
	}
	message, err := stream.Result(ctx)
	if err != nil && ctx.Err() != nil {
		return llm.AssistantErrorMessage(ctx.Err().Error(), model, true), nil
	}
	return message, err
}

// CompleteSimpleWithRetries is the single request boundary for generated
// summaries. Every attempt reuses one isolated session ID and disables prompt
// cache retention because summary requests are never continued.
func CompleteSimpleWithRetries(
	ctx context.Context,
	runtime SimpleCompletionRuntime,
	model llm.Model,
	llmContext llm.Context,
	options llm.ModelsStreamOptions,
	retry llm.RetryPolicy,
	callbacks llm.RetryCallbacks,
) (llm.Message, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if runtime == nil {
		runtime = directSimpleCompletionRuntime{}
	}
	options.StreamOptions.Context = ctx
	options.StreamOptions.CacheRetention = "none"
	options.StreamOptions.SessionID = UUIDv7()
	return llm.RetryAssistantCall(ctx, retry, callbacks, func(callCtx context.Context) (llm.Message, error) {
		requestOptions := options
		requestOptions.StreamOptions.Context = callCtx
		return runtime.CompleteSimple(callCtx, model, llmContext, requestOptions)
	})
}

func GenerateSummary(ctx context.Context, messages []llm.Message, model llm.Model, maxTokens int, apiKey string, previousSummary, focus, thinkingLevel string) (string, error) {
	result, err := GenerateSummaryWithUsage(ctx, messages, model, maxTokens, previousSummary, focus, CompactOptions{
		APIKey:        apiKey,
		ThinkingLevel: thinkingLevel,
	})
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

// GenerateSummaryWithUsage generates or updates a conversation summary while
// preserving the provider usage from the successful terminal attempt.
func GenerateSummaryWithUsage(
	ctx context.Context,
	messages []llm.Message,
	model llm.Model,
	maxTokens int,
	previousSummary string,
	focus string,
	options CompactOptions,
) (SummaryWithUsage, error) {
	basePrompt := summarizationPrompt
	if previousSummary != "" {
		basePrompt = updateSummarizationPrompt
	}
	if focus != "" {
		basePrompt += "\n\nAdditional focus: " + focus
	}
	prompt := "<conversation>\n" + SerializeConversation(messages) + "\n</conversation>\n\n"
	if previousSummary != "" {
		prompt += "<previous-summary>\n" + previousSummary + "\n</previous-summary>\n\n"
	}
	prompt += basePrompt
	requestOptions := summaryRequestOptions(options)
	requestOptions.StreamOptions.MaxTokens = compactionSummaryMaxTokens(maxTokens, 8, 10, model.MaxTokens)
	if model.Reasoning && options.ThinkingLevel != "" && options.ThinkingLevel != "off" {
		requestOptions.StreamOptions.Reasoning = options.ThinkingLevel
	}
	message, err := CompleteSimpleWithRetries(
		ctx,
		options.Runtime,
		model,
		llm.Context{
			SystemPrompt: summarizationSystemPrompt,
			Messages:     []llm.Message{llm.UserMessageText(prompt)},
		},
		requestOptions,
		options.Retry,
		options.RetryCallbacks,
	)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return SummaryWithUsage{}, newCompactionError(CompactionErrorAborted, "summarization aborted: %v", err)
		}
		return SummaryWithUsage{}, newCompactionError(CompactionErrorSummarizationFailed, "summarization failed: %v", err)
	}
	if message.StopReason == llm.StopReasonAborted {
		return SummaryWithUsage{}, newCompactionError(CompactionErrorAborted, "summarization aborted: %s", message.ErrorMessage)
	}
	if message.StopReason == llm.StopReasonError {
		return SummaryWithUsage{}, newCompactionError(CompactionErrorSummarizationFailed, "summarization failed: %s", message.ErrorMessage)
	}
	var text strings.Builder
	for _, part := range message.Content {
		if part.Type == llm.ContentText {
			text.WriteString(part.Text)
		}
	}
	return SummaryWithUsage{Text: text.String(), Usage: message.Usage}, nil
}

func GenerateTurnPrefixSummary(ctx context.Context, messages []llm.Message, model llm.Model, reserveTokens int, apiKey string, thinkingLevel string) (string, error) {
	result, err := generateTurnPrefixSummaryWithUsage(ctx, messages, model, reserveTokens, CompactOptions{
		APIKey:        apiKey,
		ThinkingLevel: thinkingLevel,
	})
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

func generateTurnPrefixSummaryWithUsage(
	ctx context.Context,
	messages []llm.Message,
	model llm.Model,
	reserveTokens int,
	options CompactOptions,
) (SummaryWithUsage, error) {
	prompt := "<conversation>\n" + SerializeConversation(messages) + "\n</conversation>\n\n" + turnPrefixSummarizationPrompt
	requestOptions := summaryRequestOptions(options)
	requestOptions.StreamOptions.MaxTokens = compactionSummaryMaxTokens(reserveTokens, 5, 10, model.MaxTokens)
	if model.Reasoning && options.ThinkingLevel != "" && options.ThinkingLevel != "off" {
		requestOptions.StreamOptions.Reasoning = options.ThinkingLevel
	}
	message, err := CompleteSimpleWithRetries(
		ctx,
		options.Runtime,
		model,
		llm.Context{
			SystemPrompt: summarizationSystemPrompt,
			Messages:     []llm.Message{llm.UserMessageText(prompt)},
		},
		requestOptions,
		options.Retry,
		options.RetryCallbacks,
	)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return SummaryWithUsage{}, newCompactionError(CompactionErrorAborted, "turn prefix summarization aborted: %v", err)
		}
		return SummaryWithUsage{}, newCompactionError(CompactionErrorSummarizationFailed, "turn prefix summarization failed: %v", err)
	}
	if message.StopReason == llm.StopReasonAborted {
		return SummaryWithUsage{}, newCompactionError(CompactionErrorAborted, "turn prefix summarization aborted: %s", message.ErrorMessage)
	}
	if message.StopReason == llm.StopReasonError {
		return SummaryWithUsage{}, newCompactionError(CompactionErrorSummarizationFailed, "turn prefix summarization failed: %s", message.ErrorMessage)
	}
	var text strings.Builder
	for _, part := range message.Content {
		if part.Type == llm.ContentText {
			text.WriteString(part.Text)
		}
	}
	return SummaryWithUsage{Text: text.String(), Usage: message.Usage}, nil
}

func summaryRequestOptions(options CompactOptions) llm.ModelsStreamOptions {
	requestOptions := options.RequestOptions
	if requestOptions.APIKey == nil && (options.Runtime == nil || options.APIKey != "") {
		apiKey := options.APIKey
		requestOptions.APIKey = &apiKey
	}
	return requestOptions
}

func compactionSummaryMaxTokens(reserveTokens, numerator, denominator, modelMaxTokens int) int {
	maxTokens := reserveTokens
	if denominator > 0 {
		maxTokens = reserveTokens * numerator / denominator
	}
	if modelMaxTokens > 0 && maxTokens > modelMaxTokens {
		return modelMaxTokens
	}
	return maxTokens
}

func Compact(ctx context.Context, prep CompactionPreparation, model llm.Model, apiKey string, thinkingLevel string) (CompactionResult, error) {
	return CompactWithOptions(ctx, prep, model, CompactOptions{APIKey: apiKey, ThinkingLevel: thinkingLevel})
}

func CompactWithOptions(ctx context.Context, prep CompactionPreparation, model llm.Model, options CompactOptions) (CompactionResult, error) {
	if prep.FirstKeptEntryID == "" {
		return CompactionResult{}, newCompactionError(CompactionErrorInvalidSession, "invalid compaction preparation")
	}
	var summary string
	var summaryUsage *llm.Usage
	var err error
	if len(prep.MessagesToSummarize) > 0 || !prep.IsSplitTurn {
		var generated SummaryWithUsage
		generated, err = GenerateSummaryWithUsage(
			ctx,
			prep.MessagesToSummarize,
			model,
			prep.Settings.ReserveTokens,
			prep.PreviousSummary,
			options.CustomInstructions,
			options,
		)
		if err != nil {
			return CompactionResult{}, err
		}
		summary = generated.Text
		summaryUsage = cloneUsage(generated.Usage)
	} else if prep.IsSplitTurn {
		summary = "No prior history."
	}
	if len(prep.TurnPrefixMessages) > 0 {
		prefix, err := generateTurnPrefixSummaryWithUsage(
			ctx,
			prep.TurnPrefixMessages,
			model,
			prep.Settings.ReserveTokens,
			options,
		)
		if err != nil {
			return CompactionResult{}, err
		}
		if summary != "" {
			summary += "\n\n---\n\n"
		}
		summary += "**Turn Context (split turn):**\n\n" + prefix.Text
		if summaryUsage == nil {
			summaryUsage = cloneUsage(prefix.Usage)
		} else {
			combined := combineUsage(*summaryUsage, prefix.Usage)
			summaryUsage = &combined
		}
	}
	readFiles, modifiedFiles := fileListsFromOps(prep.FileOps)
	summary += formatFileOperations(readFiles, modifiedFiles)
	return CompactionResult{
		Summary:          summary,
		FirstKeptEntryID: prep.FirstKeptEntryID,
		TokensBefore:     prep.TokensBefore,
		Usage:            summaryUsage,
		Details: map[string]any{
			"readFiles":     readFiles,
			"modifiedFiles": modifiedFiles,
		},
	}, nil
}

func combineUsage(first, second llm.Usage) llm.Usage {
	return llm.Usage{
		Input:        first.Input + second.Input,
		Output:       first.Output + second.Output,
		CacheRead:    first.CacheRead + second.CacheRead,
		CacheWrite:   first.CacheWrite + second.CacheWrite,
		CacheWrite1h: first.CacheWrite1h + second.CacheWrite1h,
		TotalTokens:  first.TotalTokens + second.TotalTokens,
		Cost: llm.UsageCost{
			Input:      first.Cost.Input + second.Cost.Input,
			Output:     first.Cost.Output + second.Cost.Output,
			CacheRead:  first.Cost.CacheRead + second.Cost.CacheRead,
			CacheWrite: first.Cost.CacheWrite + second.Cost.CacheWrite,
			Total:      first.Cost.Total + second.Cost.Total,
		},
	}
}

func cloneUsage(usage llm.Usage) *llm.Usage {
	cloned := usage
	return &cloned
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
		if entry.Type == "compaction" && !entry.FromHook {
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

func fileListsFromOps(ops FileOps) ([]string, []string) {
	modified := map[string]bool{}
	for path, ok := range ops.Edited {
		if ok && path != "" {
			modified[path] = true
		}
	}
	for path, ok := range ops.Written {
		if ok && path != "" {
			modified[path] = true
		}
	}
	readOnly := map[string]bool{}
	for path, ok := range ops.Read {
		if ok && path != "" && !modified[path] {
			readOnly[path] = true
		}
	}
	return keys(readOnly), keys(modified)
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
