package harness

import (
	"context"
	"fmt"
	"slices"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type CollectEntriesResult struct {
	Entries          []Entry
	CommonAncestorID *string
}

type BranchPreparation struct {
	Messages    []llm.Message
	FileOps     FileOps
	TotalTokens int
}

type BranchSummaryOptions struct {
	APIKey              string
	CustomInstructions  string
	ReplaceInstructions bool
	ReserveTokens       int
}

type BranchSummaryResult struct {
	Summary       string
	ReadFiles     []string
	ModifiedFiles []string
}

const branchSummaryPreamble = "The user explored a different conversation branch before returning here.\nSummary of that exploration:\n\n"

const branchSummaryPrompt = `Create a structured summary of this conversation branch for context when returning later.

Use this exact shape:

## Goal
[What was the user trying to accomplish in this branch?]

## Constraints & Preferences
- [Any constraints, preferences, or requirements mentioned]

## Progress
- [Completed or in-progress work]

## Key Decisions
- [Important decisions and rationale]

## Next Steps
1. [What should happen next]

Keep it concise. Preserve exact file paths, function names, and error messages.`

func CollectEntriesForBranchSummary(session *Session, oldLeafID *string, targetID string) (CollectEntriesResult, error) {
	if oldLeafID == nil {
		return CollectEntriesResult{}, nil
	}
	oldBranch, err := session.Branch(oldLeafID)
	if err != nil {
		return CollectEntriesResult{}, err
	}
	oldPath := map[string]bool{}
	for _, entry := range oldBranch {
		oldPath[entry.ID] = true
	}
	targetPath, err := session.Branch(&targetID)
	if err != nil {
		return CollectEntriesResult{}, err
	}
	var commonAncestorID *string
	for i := len(targetPath) - 1; i >= 0; i-- {
		if oldPath[targetPath[i].ID] {
			id := targetPath[i].ID
			commonAncestorID = &id
			break
		}
	}

	var entries []Entry
	current := *oldLeafID
	for commonAncestorID == nil || current != *commonAncestorID {
		entry, ok := session.Entry(current)
		if !ok {
			return CollectEntriesResult{}, newSessionError("invalid_session", "Entry %s not found", current)
		}
		entries = append(entries, entry)
		if entry.ParentID == nil {
			break
		}
		current = *entry.ParentID
	}
	slices.Reverse(entries)
	return CollectEntriesResult{Entries: entries, CommonAncestorID: commonAncestorID}, nil
}

func PrepareBranchEntries(entries []Entry, tokenBudget int) BranchPreparation {
	fileOps := FileOps{Read: map[string]bool{}, Written: map[string]bool{}, Edited: map[string]bool{}}
	for _, entry := range entries {
		if entry.Type == "branch_summary" && !entry.FromHook {
			if details, ok := entry.Details.(map[string]any); ok {
				addStringSlice(fileOps.Read, details["readFiles"])
				addStringSlice(fileOps.Edited, details["modifiedFiles"])
			}
		}
	}

	var messages []llm.Message
	totalTokens := 0
	for i := len(entries) - 1; i >= 0; i-- {
		message, ok := branchMessageFromEntry(entries[i])
		if !ok {
			continue
		}
		mergeFileOps(fileOps, collectFileOps(entries[i:i+1]))
		tokens := EstimateTokens(message)
		if tokenBudget > 0 && totalTokens+tokens > tokenBudget {
			if (entries[i].Type == "compaction" || entries[i].Type == "branch_summary") && totalTokens < tokenBudget*9/10 {
				messages = append([]llm.Message{message}, messages...)
				totalTokens += tokens
			}
			break
		}
		messages = append([]llm.Message{message}, messages...)
		totalTokens += tokens
	}
	return BranchPreparation{Messages: messages, FileOps: fileOps, TotalTokens: totalTokens}
}

func GenerateBranchSummary(ctx context.Context, entries []Entry, model llm.Model, options BranchSummaryOptions) (BranchSummaryResult, error) {
	reserveTokens := options.ReserveTokens
	if reserveTokens == 0 {
		reserveTokens = 16_384
	}
	tokenBudget := model.ContextWindow - reserveTokens
	preparation := PrepareBranchEntries(entries, tokenBudget)
	readFiles := keys(preparation.FileOps.Read)
	modifiedFiles := keys(preparation.FileOps.Edited)
	if len(preparation.Messages) == 0 {
		return BranchSummaryResult{Summary: "No content to summarize", ReadFiles: readFiles, ModifiedFiles: modifiedFiles}, nil
	}

	instructions := branchSummaryPrompt
	if options.ReplaceInstructions && options.CustomInstructions != "" {
		instructions = options.CustomInstructions
	} else if options.CustomInstructions != "" {
		instructions += "\n\nAdditional focus: " + options.CustomInstructions
	}
	prompt := "<conversation>\n" + SerializeConversation(preparation.Messages) + "\n</conversation>\n\n" + instructions
	stream, err := llm.StreamSimple(model, llm.Context{SystemPrompt: summarizationSystemPrompt, Messages: []llm.Message{llm.UserMessageText(prompt)}}, llm.SimpleStreamOptions{APIKey: options.APIKey, MaxTokens: min(2048, model.MaxTokens)})
	if err != nil {
		return BranchSummaryResult{}, err
	}
	message, err := stream.Result(ctx)
	if err != nil {
		return BranchSummaryResult{}, err
	}
	if message.StopReason == llm.StopReasonAborted {
		return BranchSummaryResult{}, fmt.Errorf("aborted: %s", message.ErrorMessage)
	}
	if message.StopReason == llm.StopReasonError {
		return BranchSummaryResult{}, fmt.Errorf("branch summary failed: %s", message.ErrorMessage)
	}
	var parts []string
	for _, part := range message.Content {
		if part.Type == llm.ContentText {
			parts = append(parts, part.Text)
		}
	}
	summary := branchSummaryPreamble + strings.Join(parts, "\n") + formatFileOperations(readFiles, modifiedFiles)
	return BranchSummaryResult{Summary: summary, ReadFiles: readFiles, ModifiedFiles: modifiedFiles}, nil
}

func branchMessageFromEntry(entry Entry) (llm.Message, bool) {
	if entry.Type == "message" {
		if entry.Message.Role == llm.RoleToolResult {
			return llm.Message{}, false
		}
		return entry.Message, true
	}
	message := entryMessage(entry)
	return message, message.Role != "unknown"
}

func mergeFileOps(target FileOps, source FileOps) {
	for key, value := range source.Read {
		if value {
			target.Read[key] = true
		}
	}
	for key, value := range source.Written {
		if value {
			target.Written[key] = true
			target.Edited[key] = true
		}
	}
	for key, value := range source.Edited {
		if value {
			target.Edited[key] = true
		}
	}
}

func formatFileOperations(readFiles, modifiedFiles []string) string {
	if len(readFiles) == 0 && len(modifiedFiles) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString("\n\n## File Operations")
	if len(readFiles) > 0 {
		out.WriteString("\nRead files:\n")
		for _, file := range readFiles {
			out.WriteString("- ")
			out.WriteString(file)
			out.WriteString("\n")
		}
	}
	if len(modifiedFiles) > 0 {
		out.WriteString("\nModified files:\n")
		for _, file := range modifiedFiles {
			out.WriteString("- ")
			out.WriteString(file)
			out.WriteString("\n")
		}
	}
	return out.String()
}
