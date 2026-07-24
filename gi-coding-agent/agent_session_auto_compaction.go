package gicodingagent

import (
	"context"
	"errors"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	llm "github.com/nowa/gi/gi-llm-provider"
)

type AgentSessionAutoCompactionRunner func(reason string, willRetry bool) error

func (s *AgentSession) RunAutoCompaction(reason string, willRetry bool) error {
	if s == nil {
		return errors.New("session is required")
	}
	s.SyncRuntimeSettings()
	if _, err := s.runAutoCompaction(
		context.Background(),
		reason,
		willRetry,
	); err != nil {
		return err
	}
	if s.HasAgentQueuedMessages() && s.AgentContinue != nil {
		if err := s.AgentContinue(); err != nil {
			return err
		}
	}
	return nil
}

func (s *AgentSession) CheckCompaction(assistantMessage llm.Message, skipAbortedCheck ...bool) error {
	if s == nil {
		return errors.New("session is required")
	}
	s.SyncRuntimeSettings()
	skipAborted := true
	if len(skipAbortedCheck) > 0 {
		skipAborted = skipAbortedCheck[0]
	}
	_, err := s.checkCompaction(
		context.Background(),
		assistantMessage,
		"",
		skipAborted,
	)
	return err
}

func (s *AgentSession) checkCompactionAfterAgentRun(
	ctx context.Context,
	assistantMessage llm.Message,
	entryID string,
) (bool, error) {
	continued, err := s.checkCompaction(
		ctx,
		assistantMessage,
		entryID,
		true,
	)
	if err != nil {
		// Automatic compaction is recovery/maintenance work. Its terminal event
		// carries the actionable error; it must not turn an otherwise completed
		// agent run into a Prompt error.
		return false, nil
	}
	return continued, nil
}

func (s *AgentSession) checkCompaction(
	ctx context.Context,
	assistantMessage llm.Message,
	entryID string,
	skipAborted bool,
) (bool, error) {
	if !s.CompactionSettings.Enabled {
		return false, nil
	}
	if skipAborted &&
		assistantMessage.StopReason == llm.StopReasonAborted {
		return false, nil
	}
	if s.isStaleBeforeLastCompaction(assistantMessage) {
		return false, nil
	}

	contextWindow := s.contextWindow()
	sameModel := s.sameModelAsCurrent(assistantMessage)
	if sameModel &&
		llm.IsContextOverflow(assistantMessage, contextWindow) {
		willRetry := assistantMessage.StopReason != llm.StopReasonStop
		if !willRetry {
			return s.runAutoCompaction(ctx, "overflow", false)
		}
		if s.overflowRecovered {
			s.emit(AgentSessionEvent{
				Type:         "compaction_end",
				Reason:       "overflow",
				Aborted:      false,
				WillRetry:    false,
				ErrorMessage: "Context overflow recovery failed after one compact-and-retry attempt. Try reducing context or switching to a larger-context model.",
			})
			return false, nil
		}
		s.overflowRecovered = true
		s.providerContext.exclude(entryID)
		return s.runAutoCompaction(ctx, "overflow", true)
	}
	if s.shouldThresholdCompact(assistantMessage) {
		return s.runAutoCompaction(ctx, "threshold", false)
	}
	return false, nil
}

func (s *AgentSession) sameModelAsCurrent(message llm.Message) bool {
	if s == nil || s.Agent == nil {
		return false
	}
	current := s.Agent.State.Model
	providerMatches := message.Provider == "" ||
		message.Provider == current.Provider
	modelMatches := message.Model == "" ||
		message.Model == current.ID
	return providerMatches && modelMatches
}

func (s *AgentSession) runAutoCompaction(
	parent context.Context,
	reason string,
	willRetry bool,
) (bool, error) {
	if s == nil || s.Agent == nil ||
		s.Agent.State.Model.ID == "" {
		return false, errors.New("No model selected")
	}
	if s.Preflight != nil {
		if err := s.Preflight(s.Agent.State.Model); err != nil {
			return false, err
		}
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	cleanup := s.lifecycle.startNestedCancellableActivity(
		agentSessionActivityCompacting,
		agentSessionCancellationCompaction,
		cancel,
	)
	defer func() {
		cleanup()
		cancel()
		s.lifecycle.setActivity(agentSessionActivityCompacting, false)
	}()

	s.emit(AgentSessionEvent{
		Type:   "compaction_start",
		Reason: reason,
	})
	var result *agentharness.CompactionResult
	var err error
	if s.AutoCompactionRunner != nil {
		err = s.AutoCompactionRunner(reason, willRetry)
	} else {
		value, compactErr := s.compactSession(
			ctx,
			"",
			reason,
			willRetry,
		)
		err = compactErr
		if err == nil {
			result = &value
		}
	}
	if err != nil {
		aborted := isCompactionCancelledError(err)
		errorMessage := autoCompactionErrorMessage(reason, err)
		if aborted {
			errorMessage = ""
		}
		s.emit(AgentSessionEvent{
			Type:         "compaction_end",
			Reason:       reason,
			Aborted:      aborted,
			WillRetry:    false,
			ErrorMessage: errorMessage,
		})
		return false, err
	}
	event := AgentSessionEvent{
		Type:      "compaction_end",
		Reason:    reason,
		Aborted:   false,
		WillRetry: willRetry,
		Result:    result,
	}
	s.emit(event)
	return willRetry || s.hasPostRunQueue(), nil
}

func autoCompactionErrorMessage(reason string, err error) string {
	if err == nil {
		return ""
	}
	if reason == "overflow" {
		return "Context overflow recovery failed: " + err.Error()
	}
	return "Auto-compaction failed: " + err.Error()
}

func (s *AgentSession) hasPostRunQueue() bool {
	return s.hasQueuedPrompt("steering") ||
		s.hasQueuedPrompt("follow-up") ||
		s.HasAgentQueuedMessages()
}

func (s *AgentSession) isStaleBeforeLastCompaction(message llm.Message) bool {
	branch := s.sessionBranch()
	compactionIndex := lastSessionCompactionIndex(branch)
	if compactionIndex < 0 {
		return false
	}
	return message.Timestamp <= sessionEntryTimestampMillis(
		branch[compactionIndex].Timestamp,
	)
}

func (s *AgentSession) QueueAgentMessage(message llm.Message) {
	if s == nil {
		return
	}
	s.queues.enqueueAgentMessage(message)
}

func (s *AgentSession) HasAgentQueuedMessages() bool {
	return s != nil && s.queues.hasAgentMessages()
}

func (s *AgentSession) shouldThresholdCompact(assistantMessage llm.Message) bool {
	if !s.CompactionSettings.Enabled {
		return false
	}
	contextWindow := s.contextWindow()
	if contextWindow == 0 {
		return false
	}
	tokens := usageTokenTotal(assistantMessage.Usage)
	if assistantMessage.StopReason == llm.StopReasonError || tokens == 0 {
		var ok bool
		tokens, ok = s.estimatedContextTokensAfterCompaction()
		if !ok {
			return false
		}
	}
	return tokens > contextWindow-s.CompactionSettings.ReserveTokens
}

func (s *AgentSession) estimatedContextTokensAfterCompaction() (int, bool) {
	messages := s.baseProviderContextMessages()
	compactionTimestamp := int64(0)
	branch := s.sessionBranch()
	if index := lastSessionCompactionIndex(branch); index >= 0 {
		compactionTimestamp = sessionEntryTimestampMillis(
			branch[index].Timestamp,
		)
	}

	lastUsageIndex := -1
	tokens := 0
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message.Role != llm.RoleAssistant ||
			message.StopReason == llm.StopReasonError ||
			message.StopReason == llm.StopReasonAborted ||
			usageTokenTotal(message.Usage) == 0 {
			continue
		}
		if compactionTimestamp != 0 &&
			message.Timestamp <= compactionTimestamp {
			return 0, false
		}
		lastUsageIndex = index
		tokens = usageTokenTotal(message.Usage)
		break
	}
	if lastUsageIndex < 0 {
		return 0, false
	}
	for index := lastUsageIndex + 1; index < len(messages); index++ {
		tokens += agentharness.EstimateTokens(messages[index])
	}
	return tokens, true
}
