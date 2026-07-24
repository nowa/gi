package gicodingagent

import (
	"context"
	"errors"
	"time"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	llm "github.com/nowa/gi/gi-llm-provider"
)

type agentSessionSummarySource struct {
	Source string
	Reason string
}

type agentSessionResponderCompletionRuntime struct {
	responder       AgentSessionResponder
	streamResponder AgentSessionStreamResponder
}

func (r agentSessionResponderCompletionRuntime) CompleteSimple(
	ctx context.Context,
	model llm.Model,
	llmContext llm.Context,
	_ llm.ModelsStreamOptions,
) (llm.Message, error) {
	if r.streamResponder != nil {
		stream, err := r.streamResponder(
			"",
			llmContext.Messages,
			model,
		)
		if err != nil {
			return llm.Message{}, err
		}
		if stream == nil {
			return llm.Message{},
				errors.New("summary stream responder returned nil stream")
		}
		return stream.Result(ctx)
	}
	if r.responder == nil {
		return llm.Message{},
			errors.New("summary responder is required")
	}
	return r.responder("", llmContext.Messages, model)
}

func (s *AgentSession) summarizationRetryPolicy() llm.RetryPolicy {
	if s == nil {
		return llm.RetryPolicy{}
	}
	return llm.RetryPolicy{
		Enabled:    s.RetrySettings.Enabled,
		MaxRetries: s.RetrySettings.MaxRetries,
		BaseDelay:  time.Duration(s.RetrySettings.BaseDelayMs) * time.Millisecond,
	}
}

func (s *AgentSession) summarizationRetryCallbacks(
	source agentSessionSummarySource,
) llm.RetryCallbacks {
	return llm.RetryCallbacks{
		OnRetryScheduled: func(attempt llm.RetryAttempt) {
			s.emit(AgentSessionEvent{
				Type:         "summarization_retry_scheduled",
				Attempt:      attempt.Attempt,
				MaxAttempts:  attempt.MaxAttempts,
				DelayMs:      int(attempt.Delay.Milliseconds()),
				ErrorMessage: attempt.ErrorMessage,
			})
		},
		OnRetryAttemptStart: func(attempt int) {
			s.emit(AgentSessionEvent{
				Type:    "summarization_retry_attempt_start",
				Source:  source.Source,
				Reason:  source.Reason,
				Attempt: attempt,
			})
		},
		OnRetryFinished: func(llm.RetryResult) {
			s.emit(AgentSessionEvent{Type: "summarization_retry_finished"})
		},
	}
}

func (s *AgentSession) generateCompactionSummary(
	ctx context.Context,
	preparation agentharness.CompactionPreparation,
	customInstructions string,
	reason string,
) (agentharness.CompactionResult, error) {
	if s == nil {
		return DefaultAgentSessionCompactionSummarizer(
			preparation,
			customInstructions,
		)
	}
	runtime := s.SummaryRuntime
	if runtime == nil && s.ModelRuntime != nil {
		runtime = s.ModelRuntime
	}
	if runtime == nil {
		return DefaultAgentSessionCompactionSummarizer(
			preparation,
			customInstructions,
		)
	}
	return agentharness.CompactWithOptions(
		ctx,
		preparation,
		s.Agent.State.Model,
		agentharness.CompactOptions{
			ThinkingLevel:      s.Agent.State.ThinkingLevel,
			CustomInstructions: customInstructions,
			Runtime:            runtime,
			Retry:              s.summarizationRetryPolicy(),
			RetryCallbacks: s.summarizationRetryCallbacks(
				agentSessionSummarySource{
					Source: "compaction",
					Reason: reason,
				},
			),
		},
	)
}

func (s *AgentSession) generateBranchSummary(
	ctx context.Context,
	entries []FileEntry,
	customInstructions string,
) (agentharness.BranchSummaryResult, error) {
	if s == nil {
		summary, err := DefaultAgentSessionBranchSummarizer(
			entries,
			customInstructions,
			ctx.Done(),
		)
		return agentharness.BranchSummaryResult{Summary: summary}, err
	}
	runtime := s.SummaryRuntime
	if runtime == nil && s.ModelRuntime != nil {
		runtime = s.ModelRuntime
	}
	if runtime == nil {
		summary, err := DefaultAgentSessionBranchSummarizer(
			entries,
			customInstructions,
			ctx.Done(),
		)
		return agentharness.BranchSummaryResult{Summary: summary}, err
	}
	return agentharness.GenerateBranchSummary(
		ctx,
		fileEntriesToHarnessEntries(entries),
		s.Agent.State.Model,
		agentharness.BranchSummaryOptions{
			CustomInstructions: customInstructions,
			Runtime:            runtime,
			Retry:              s.summarizationRetryPolicy(),
			RetryCallbacks: s.summarizationRetryCallbacks(
				agentSessionSummarySource{Source: "branchSummary"},
			),
		},
	)
}
