package gicodingagent

import (
	"errors"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type AgentSessionAutoCompactionRunner func(reason string, willRetry bool) error

func (s *AgentSession) RunAutoCompaction(reason string, willRetry bool) error {
	if s == nil {
		return errors.New("session is required")
	}
	if s.AutoCompactionRunner != nil {
		if err := s.AutoCompactionRunner(reason, willRetry); err != nil {
			return err
		}
	} else if _, err := s.Compact(); err != nil {
		return err
	}
	if len(s.agentQueuedMessages) > 0 && s.AgentContinue != nil {
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
	if assistantMessage.StopReason == llm.StopReasonAborted && len(skipAbortedCheck) == 0 {
		return nil
	}
	if isContextOverflowError(assistantMessage) {
		if s.overflowRecovered {
			s.emit(AgentSessionEvent{
				Type:         "compaction_end",
				Reason:       "overflow",
				ErrorMessage: "Context overflow recovery failed after one compact-and-retry attempt. Try reducing context or switching to a larger-context model.",
			})
			return nil
		}
		s.overflowRecovered = true
		return s.RunAutoCompaction("overflow", true)
	}
	if assistantMessage.StopReason != llm.StopReasonError && s.isStaleBeforeLastCompaction(assistantMessage) {
		return nil
	}
	if s.shouldThresholdCompact(assistantMessage) {
		return s.RunAutoCompaction("threshold", false)
	}
	return nil
}

func (s *AgentSession) isStaleBeforeLastCompaction(message llm.Message) bool {
	if lastSessionCompactionIndex(s.sessionBranch()) < 0 {
		return false
	}
	branch := s.sessionBranch()
	compactionIndex := lastSessionCompactionIndex(branch)
	for index := compactionIndex + 1; index < len(branch); index++ {
		entryMessage, ok := sessionMessageToLLM(branch[index].Message)
		if !ok || entryMessage.Role != llm.RoleAssistant {
			continue
		}
		if entryMessage.Timestamp == message.Timestamp {
			return false
		}
	}
	return true
}

func (s *AgentSession) QueueAgentMessage(message llm.Message) {
	if s == nil {
		return
	}
	s.agentQueuedMessages = append(s.agentQueuedMessages, message)
}

func (s *AgentSession) HasAgentQueuedMessages() bool {
	return s != nil && len(s.agentQueuedMessages) > 0
}

func (s *AgentSession) shouldThresholdCompact(assistantMessage llm.Message) bool {
	if !s.CompactionSettings.Enabled {
		return false
	}
	contextWindow := s.contextWindow()
	if contextWindow == 0 {
		return false
	}
	usage := llm.Usage{}
	if assistantMessage.StopReason == llm.StopReasonError {
		var ok bool
		usage, ok = s.lastSuccessfulUsageAfterCompaction()
		if !ok {
			return false
		}
	} else {
		usage = assistantMessage.Usage
	}
	tokens := usageTokenTotal(usage)
	if tokens == 0 {
		return false
	}
	return tokens > contextWindow-s.CompactionSettings.ReserveTokens
}

func (s *AgentSession) lastSuccessfulUsageAfterCompaction() (llm.Usage, bool) {
	branch := s.sessionBranch()
	start := 0
	if compactionIndex := lastSessionCompactionIndex(branch); compactionIndex >= 0 {
		start = compactionIndex + 1
	}
	for index := len(branch) - 1; index >= start; index-- {
		usage, ok := assistantEntryUsage(branch[index])
		if ok {
			return usage, true
		}
	}
	return llm.Usage{}, false
}

func isContextOverflowError(message llm.Message) bool {
	if message.StopReason != llm.StopReasonError {
		return false
	}
	text := strings.ToLower(message.ErrorMessage)
	return strings.Contains(text, "prompt is too long") || strings.Contains(text, "context") && strings.Contains(text, "overflow")
}
