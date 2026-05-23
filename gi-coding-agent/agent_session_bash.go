package gicodingagent

import (
	"context"
	"errors"
	"strconv"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type AgentSessionBashOptions struct {
	OnChunk            func(string)
	ExcludeFromContext bool
	Operations         BashOperations
	Context            context.Context
}

type AgentSessionBashRecordOptions struct {
	ExcludeFromContext bool
}

func (s *AgentSession) ExecuteBash(command string, options ...AgentSessionBashOptions) (BashResult, error) {
	if s == nil || s.SessionManager == nil {
		return BashResult{}, errors.New("session manager is required")
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return BashResult{}, errors.New("bash command is required")
	}
	opts := AgentSessionBashOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	s.bashAbort = cancel
	defer func() {
		if s.bashAbort != nil {
			s.bashAbort = nil
		}
	}()

	result, err := ExecuteBashWithOperations(command, s.SessionManager.GetCWD(), opts.Operations, BashExecutorOptions{
		Context: ctx,
		OnChunk: opts.OnChunk,
	})
	if ctx.Err() != nil {
		result.Cancelled = true
		result.ExitCode = 0
		err = nil
	}
	if err != nil && result.ExitCode == 0 && strings.TrimSpace(result.Output) == "" {
		return result, err
	}
	s.RecordBashResult(command, result, AgentSessionBashRecordOptions{ExcludeFromContext: opts.ExcludeFromContext})
	if result.ExitCode != 0 {
		return result, nil
	}
	return result, err
}

func (s *AgentSession) RecordBashResult(command string, result BashResult, options ...AgentSessionBashRecordOptions) {
	if s == nil || s.SessionManager == nil {
		return
	}
	opts := AgentSessionBashRecordOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	message := bashExecutionMessageValue(command, result, opts.ExcludeFromContext)
	if s.isStreaming {
		s.pendingBashMessages = append(s.pendingBashMessages, message)
		return
	}
	s.SessionManager.AppendMessage(message)
}

func (s *AgentSession) AbortBash() {
	if s == nil || s.bashAbort == nil {
		return
	}
	s.bashAbort()
}

func (s *AgentSession) IsBashRunning() bool {
	return s != nil && s.bashAbort != nil
}

func (s *AgentSession) HasPendingBashMessages() bool {
	return s != nil && len(s.pendingBashMessages) > 0
}

func (s *AgentSession) flushPendingBashMessages() {
	if s == nil || s.SessionManager == nil || len(s.pendingBashMessages) == 0 {
		return
	}
	for _, message := range s.pendingBashMessages {
		s.SessionManager.AppendMessage(message)
	}
	s.pendingBashMessages = nil
}

func bashExecutionMessageValue(command string, result BashResult, excludeFromContext bool) map[string]any {
	exitCode := any(result.ExitCode)
	if result.Cancelled {
		exitCode = nil
	}
	value := map[string]any{
		"role":      "bashExecution",
		"command":   command,
		"output":    result.Output,
		"exitCode":  exitCode,
		"cancelled": result.Cancelled,
		"truncated": result.Truncated,
		"content":   []any{map[string]any{"type": llm.ContentText, "text": bashExecutionText(command, result)}},
		"timestamp": llm.NowMillis(),
	}
	if result.FullOutputPath != "" {
		value["fullOutputPath"] = result.FullOutputPath
	}
	if result.TruncatedBy != "" {
		value["truncatedBy"] = result.TruncatedBy
	}
	if result.TotalLines != 0 {
		value["totalLines"] = result.TotalLines
	}
	if result.OutputLines != 0 {
		value["outputLines"] = result.OutputLines
	}
	if excludeFromContext {
		value["excludeFromContext"] = true
	}
	return value
}

func bashExecutionText(command string, result BashResult) string {
	var builder strings.Builder
	builder.WriteString("Ran `")
	builder.WriteString(command)
	builder.WriteString("`\n")
	if result.Output != "" {
		builder.WriteString("```\n")
		builder.WriteString(result.Output)
		if !strings.HasSuffix(result.Output, "\n") {
			builder.WriteByte('\n')
		}
		builder.WriteString("```")
	} else {
		builder.WriteString("(no output)")
	}
	if result.Cancelled {
		builder.WriteString("\n\n(command cancelled)")
	} else if result.ExitCode != 0 {
		builder.WriteString("\n\nCommand exited with code ")
		builder.WriteString(strconv.Itoa(result.ExitCode))
	}
	if result.Truncated && result.FullOutputPath != "" {
		builder.WriteString("\n\n[Output truncated. Full output: ")
		builder.WriteString(result.FullOutputPath)
		builder.WriteByte(']')
	}
	return builder.String()
}
