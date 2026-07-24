package gicodingagent

import (
	"context"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type agentPromptRunState struct {
	prompt       string
	retryAttempt int
}

type agentPromptRunResult struct {
	assistant llm.Message
	entryID   string
}

type agentPromptContinuation uint8

const (
	agentPromptContinuationNone agentPromptContinuation = iota
	agentPromptContinuationRetry
	agentPromptContinuationCompaction
	agentPromptContinuationSteering
	agentPromptContinuationFollowUp
	agentPromptContinuationAgentQueue
)

// runAgentPrompt executes one agent run. Tool calls and messages already queued
// before agent_end remain part of the same run; retry, compaction recovery, and
// messages queued by agent_end handlers are decided by handlePostAgentRun.
func (s *AgentSession) runAgentPrompt(
	state *agentPromptRunState,
) (agentPromptRunResult, error) {
	responder := s.Responder
	if responder == nil {
		responder = DefaultAgentSessionResponder
	}
	for {
		messages, err := s.providerContextMessages()
		if err != nil {
			return agentPromptRunResult{}, err
		}
		assistant, err := s.respondToPrompt(
			state.prompt,
			messages,
			s.Agent.State.Model,
			responder,
		)
		streamed := s.StreamResponder != nil && err == nil
		if err != nil {
			assistant = llm.Message{
				Role:         llm.RoleAssistant,
				StopReason:   llm.StopReasonError,
				ErrorMessage: err.Error(),
			}
		}
		assistant = s.normalizeAssistantMessage(assistant)
		if !streamed {
			if err := s.emitMessageStart(assistant); err != nil {
				return agentPromptRunResult{}, err
			}
			assistant, err = s.emitAssistantMessageUpdates(assistant)
			if err != nil {
				return agentPromptRunResult{}, err
			}
		}
		assistant, err = s.emitExtensionMessageEnd(assistant)
		if err != nil {
			return agentPromptRunResult{}, err
		}
		entryID := s.SessionManager.AppendMessage(
			sessionMessageValue(assistant),
		)
		s.emit(AgentSessionEvent{
			Type:    "message_end",
			Message: &assistant,
		})

		if assistant.StopReason != llm.StopReasonError {
			s.overflowRecovered = false
			if state.retryAttempt > 0 {
				s.emit(AgentSessionEvent{
					Type:    "auto_retry_end",
					Success: true,
					Attempt: state.retryAttempt,
				})
				state.retryAttempt = 0
			}
		}

		result := agentPromptRunResult{
			assistant: assistant,
			entryID:   entryID,
		}
		if assistant.StopReason == llm.StopReasonError ||
			assistant.StopReason == llm.StopReasonAborted {
			if err := s.emitAgentLifecycleEvent(
				AgentSessionEvent{Type: "turn_end"},
			); err != nil {
				return agentPromptRunResult{}, err
			}
			return result, nil
		}

		if assistant.StopReason == llm.StopReasonToolUse {
			if err := s.executeAssistantToolCalls(assistant); err != nil {
				return agentPromptRunResult{}, err
			}
			if err := s.emitAgentLifecycleEvent(
				AgentSessionEvent{Type: "turn_end"},
			); err != nil {
				return agentPromptRunResult{}, err
			}
			continued, err := s.continueQueuedTurn(state, "steering")
			if err != nil {
				return agentPromptRunResult{}, err
			}
			if continued {
				continue
			}
			if err := s.emitAgentLifecycleEvent(
				AgentSessionEvent{Type: "turn_start"},
			); err != nil {
				return agentPromptRunResult{}, err
			}
			state.prompt = ""
			continue
		}

		if err := s.emitAgentLifecycleEvent(
			AgentSessionEvent{Type: "turn_end"},
		); err != nil {
			return agentPromptRunResult{}, err
		}
		continued, err := s.continueQueuedTurn(state, "steering")
		if err != nil {
			return agentPromptRunResult{}, err
		}
		if continued {
			continue
		}
		continued, err = s.continueQueuedTurn(state, "follow-up")
		if err != nil {
			return agentPromptRunResult{}, err
		}
		if continued {
			continue
		}
		return result, nil
	}
}

func (s *AgentSession) continueQueuedTurn(
	state *agentPromptRunState,
	kind string,
) (bool, error) {
	if !s.hasQueuedPrompt(kind) {
		return false, nil
	}
	if err := s.emitAgentLifecycleEvent(
		AgentSessionEvent{Type: "turn_start"},
	); err != nil {
		return false, err
	}
	prompt, ok, err := s.dequeueQueuedPrompt(kind)
	if err != nil || !ok {
		return false, err
	}
	state.prompt = prompt
	return true, nil
}

func (s *AgentSession) hasQueuedPrompt(kind string) bool {
	if s == nil {
		return false
	}
	switch kind {
	case "steering":
		return s.queues.hasPrompt(agentSessionSteeringQueue)
	case "follow-up":
		return s.queues.hasPrompt(agentSessionFollowUpQueue)
	default:
		return false
	}
}

func (s *AgentSession) willRetryAfterAgentRun(
	state agentPromptRunState,
	result agentPromptRunResult,
) bool {
	return s.isRetryableAgentError(result.assistant) &&
		s.RetrySettings.Enabled &&
		state.retryAttempt < s.RetrySettings.MaxRetries
}

func (s *AgentSession) isRetryableAgentError(message llm.Message) bool {
	if s == nil {
		return false
	}
	if llm.IsContextOverflow(message, s.contextWindow()) {
		return false
	}
	return llm.IsRetryableAssistantError(message)
}

func (s *AgentSession) handlePostAgentRun(
	state *agentPromptRunState,
	result agentPromptRunResult,
) (agentPromptContinuation, error) {
	if s.isRetryableAgentError(result.assistant) {
		if s.prepareAgentRetry(state, result) {
			return s.nextAgentContinuation(
				agentPromptContinuationRetry,
			), nil
		}
	}

	if result.assistant.StopReason == llm.StopReasonError &&
		state.retryAttempt > 0 {
		s.emit(AgentSessionEvent{
			Type:       "auto_retry_end",
			Success:    false,
			Attempt:    state.retryAttempt,
			FinalError: result.assistant.ErrorMessage,
		})
		state.retryAttempt = 0
	}

	continued, err := s.checkCompactionAfterAgentRun(
		context.Background(),
		result.assistant,
		result.entryID,
	)
	if err != nil {
		return agentPromptContinuationNone, err
	}
	if continued {
		return s.nextAgentContinuation(
			agentPromptContinuationCompaction,
		), nil
	}
	return s.nextAgentContinuation(agentPromptContinuationNone), nil
}

// nextAgentContinuation mirrors Agent.continue(): messages queued by an
// agent_end handler are consumed by the already-scheduled continuation instead
// of causing an empty provider call followed by a second continuation.
func (s *AgentSession) nextAgentContinuation(
	fallback agentPromptContinuation,
) agentPromptContinuation {
	if s.hasQueuedPrompt("steering") {
		return agentPromptContinuationSteering
	}
	if s.hasQueuedPrompt("follow-up") {
		return agentPromptContinuationFollowUp
	}
	if s.HasAgentQueuedMessages() {
		return agentPromptContinuationAgentQueue
	}
	return fallback
}

func (s *AgentSession) prepareAgentRetry(
	state *agentPromptRunState,
	result agentPromptRunResult,
) bool {
	if !s.RetrySettings.Enabled ||
		state.retryAttempt >= s.RetrySettings.MaxRetries {
		return false
	}
	state.retryAttempt++
	s.providerContext.exclude(result.entryID)
	delayMs := retryDelayMS(
		s.RetrySettings.BaseDelayMs,
		state.retryAttempt,
	)
	cancelled, cleanup := s.prepareRetryDelay()
	s.emit(AgentSessionEvent{
		Type:         "auto_retry_start",
		Attempt:      state.retryAttempt,
		MaxAttempts:  s.RetrySettings.MaxRetries,
		DelayMs:      delayMs,
		ErrorMessage: result.assistant.ErrorMessage,
	})
	completed := waitForRetryDelay(delayMs, cancelled)
	cleanup()
	s.lifecycle.setActivity(agentSessionActivityRetrying, false)
	if completed {
		return true
	}
	attempt := state.retryAttempt
	state.retryAttempt = 0
	s.emit(AgentSessionEvent{
		Type:       "auto_retry_end",
		Success:    false,
		Attempt:    attempt,
		FinalError: "Retry cancelled",
	})
	return false
}

func (s *AgentSession) beginAgentContinuation(
	state *agentPromptRunState,
	continuation agentPromptContinuation,
) error {
	if err := s.emitAgentLifecycleEvent(
		AgentSessionEvent{Type: "agent_start"},
	); err != nil {
		return err
	}
	if err := s.emitAgentLifecycleEvent(
		AgentSessionEvent{Type: "turn_start"},
	); err != nil {
		return err
	}
	state.prompt = ""
	switch continuation {
	case agentPromptContinuationRetry,
		agentPromptContinuationCompaction:
	case agentPromptContinuationSteering:
		prompt, ok, err := s.dequeueQueuedPrompt("steering")
		if err != nil {
			return err
		}
		if ok {
			state.prompt = prompt
		}
	case agentPromptContinuationFollowUp:
		prompt, ok, err := s.dequeueQueuedPrompt("follow-up")
		if err != nil {
			return err
		}
		if ok {
			state.prompt = prompt
		}
	case agentPromptContinuationAgentQueue:
		return s.appendAgentQueuedMessages(state)
	}
	return nil
}

func (s *AgentSession) appendAgentQueuedMessages(
	state *agentPromptRunState,
) error {
	messages := s.queues.takeAgentMessages()
	state.prompt = ""
	for _, message := range messages {
		if message.Timestamp == 0 {
			message.Timestamp = llm.NowMillis()
		}
		if err := s.emitMessageStart(message); err != nil {
			return err
		}
		var err error
		message, err = s.emitExtensionMessageEnd(message)
		if err != nil {
			return err
		}
		s.SessionManager.AppendMessage(sessionMessageValue(message))
		s.emit(AgentSessionEvent{
			Type:    "message_end",
			Message: &message,
		})
		if text := sessionMessageText(message); text != "" {
			state.prompt = text
		}
	}
	return nil
}

func (s *AgentSession) emitAgentLifecycleEvent(
	event AgentSessionEvent,
) error {
	if event.Type == "agent_start" {
		s.activeRunMessages = nil
		s.runMessageCapture = true
	}
	if event.Type == "agent_end" {
		event.Messages = append(
			[]llm.Message(nil),
			s.activeRunMessages...,
		)
		defer func() {
			s.activeRunMessages = nil
			s.runMessageCapture = false
		}()
	}
	if err := s.emitExtensionEvent(ProtocolSessionEvent{
		Type:      event.Type,
		Reason:    event.Reason,
		WillRetry: event.WillRetry,
		Messages:  append([]llm.Message(nil), event.Messages...),
	}); err != nil {
		return err
	}
	s.emit(event)
	return nil
}
