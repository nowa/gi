package gicodingagent

import (
	"context"
	"errors"
	"strings"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
)

type AgentSessionNavigateTreeOptions struct {
	Summarize          bool
	CustomInstructions string
}

type AgentSessionNavigateTreeResult struct {
	Cancelled    bool
	Aborted      bool
	EditorText   string
	SummaryEntry *FileEntry
}

type AgentSessionBranchSummarizer func(entries []FileEntry, customInstructions string, abort <-chan struct{}) (string, error)

var errAgentSessionBranchSummaryAborted = errors.New("branch summary aborted")

func (s *AgentSession) NavigateTree(targetID string, options AgentSessionNavigateTreeOptions) (AgentSessionNavigateTreeResult, error) {
	if s == nil || s.SessionManager == nil {
		return AgentSessionNavigateTreeResult{}, errors.New("session manager is required")
	}
	leaf := s.SessionManager.GetLeafID()
	if leaf != nil && *leaf == targetID {
		return AgentSessionNavigateTreeResult{}, nil
	}
	target := s.SessionManager.GetEntry(targetID)
	if target == nil {
		return AgentSessionNavigateTreeResult{}, errors.New("Entry " + targetID + " not found")
	}
	if s.ExtensionRuntime != nil {
		result, err := s.ExtensionRuntime.EmitSessionEvent(ProtocolSessionEvent{Type: ProtocolEventSessionBeforeTree, EntryID: targetID})
		if err != nil {
			return AgentSessionNavigateTreeResult{}, err
		}
		if result.Cancel {
			return AgentSessionNavigateTreeResult{Cancelled: true}, nil
		}
	}

	var summaryResult agentharness.BranchSummaryResult
	var entriesToSummarize []FileEntry
	if options.Summarize {
		entriesToSummarize = collectTreeNavigationSummaryEntries(
			s.SessionManager,
			leaf,
			targetID,
		)
		if len(entriesToSummarize) == 0 {
			options.Summarize = false
		}
	}
	if options.Summarize {
		ctx, cancel := context.WithCancel(context.Background())
		cleanupCancellation, started := s.lifecycle.tryStartExclusiveCancellableActivity(
			agentSessionActivityCompacting,
			agentSessionCancellationBranchSummary,
			cancel,
		)
		if !started {
			cancel()
			return AgentSessionNavigateTreeResult{},
				errors.New("session is busy")
		}
		defer func() {
			cleanupCancellation()
			cancel()
			s.lifecycle.setActivity(agentSessionActivityCompacting, false)
		}()
		var err error
		if s.BranchSummarizer != nil {
			summaryResult.Summary, err = s.BranchSummarizer(
				entriesToSummarize,
				options.CustomInstructions,
				ctx.Done(),
			)
		} else {
			if s.Agent == nil ||
				strings.TrimSpace(s.Agent.State.Model.ID) == "" {
				return AgentSessionNavigateTreeResult{},
					errors.New("No model selected")
			}
			if s.Preflight != nil {
				if err := s.Preflight(s.Agent.State.Model); err != nil {
					return AgentSessionNavigateTreeResult{}, err
				}
			}
			summaryResult, err = s.generateBranchSummary(
				ctx,
				entriesToSummarize,
				options.CustomInstructions,
			)
		}
		if err != nil {
			var summaryErr *agentharness.BranchSummaryError
			if errors.Is(err, errAgentSessionBranchSummaryAborted) ||
				errors.As(err, &summaryErr) &&
					summaryErr.Code == agentharness.BranchSummaryErrorAborted {
				return AgentSessionNavigateTreeResult{Cancelled: true, Aborted: true}, nil
			}
			return AgentSessionNavigateTreeResult{}, err
		}
	}

	newLeafID := &targetID
	editorText := ""
	if target.Type == "message" && sessionMessageRole(target.Message) == "user" {
		newLeafID = cloneStringPtr(target.ParentID)
		editorText = sessionMessageText(target.Message)
	}

	if strings.TrimSpace(summaryResult.Summary) != "" {
		summaryID, err := s.SessionManager.BranchWithSummaryOptions(
			newLeafID,
			summaryResult.Summary,
			SessionSummaryOptions{
				Details: map[string]any{
					"readFiles":     summaryResult.ReadFiles,
					"modifiedFiles": summaryResult.ModifiedFiles,
				},
				Usage: summaryResult.Usage,
			},
		)
		if err != nil {
			return AgentSessionNavigateTreeResult{}, err
		}
		return AgentSessionNavigateTreeResult{EditorText: editorText, SummaryEntry: s.SessionManager.GetEntry(summaryID)}, nil
	}
	if newLeafID == nil {
		s.SessionManager.ResetLeaf()
	} else if err := s.SessionManager.Branch(*newLeafID); err != nil {
		return AgentSessionNavigateTreeResult{}, err
	}
	return AgentSessionNavigateTreeResult{EditorText: editorText}, nil
}

func (s *AgentSession) AbortBranchSummary() {
	if s == nil {
		return
	}
	s.lifecycle.cancel(agentSessionCancellationBranchSummary)
}

func (s *AgentSession) IsBranchSummaryRunning() bool {
	return s != nil && s.lifecycle.hasCancellation(agentSessionCancellationBranchSummary)
}

func (s *AgentSession) IsCompacting() bool {
	if s == nil {
		return false
	}
	return s.lifecycle.isActive(agentSessionActivityCompacting)
}

func DefaultAgentSessionBranchSummarizer(entries []FileEntry, customInstructions string, abort <-chan struct{}) (string, error) {
	select {
	case <-abort:
		return "", errAgentSessionBranchSummaryAborted
	default:
	}
	parts := []string{"Branch summary"}
	if strings.TrimSpace(customInstructions) != "" {
		parts = append(parts, strings.TrimSpace(customInstructions))
	}
	for _, entry := range entries {
		if entry.Type == "message" {
			if text := strings.TrimSpace(sessionMessageText(entry.Message)); text != "" {
				parts = append(parts, text)
			}
			continue
		}
		if entry.Type == "branch_summary" && strings.TrimSpace(entry.Summary) != "" {
			parts = append(parts, strings.TrimSpace(entry.Summary))
		}
	}
	return strings.Join(parts, "\n"), nil
}

func collectTreeNavigationSummaryEntries(manager *SessionManager, oldLeafID *string, targetID string) []FileEntry {
	if manager == nil || oldLeafID == nil {
		return nil
	}
	oldBranch := manager.GetBranch(*oldLeafID)
	targetBranch := manager.GetBranch(targetID)
	targetIDs := map[string]bool{}
	for _, entry := range targetBranch {
		targetIDs[entry.ID] = true
	}
	commonIndex := -1
	for index, entry := range oldBranch {
		if targetIDs[entry.ID] {
			commonIndex = index
		}
	}
	if commonIndex < 0 || commonIndex+1 >= len(oldBranch) {
		return nil
	}
	return append([]FileEntry(nil), oldBranch[commonIndex+1:]...)
}
