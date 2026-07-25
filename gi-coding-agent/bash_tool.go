package gicodingagent

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"

	core "github.com/nowa/gi/gi-agent-core"
	harnessenv "github.com/nowa/gi/gi-agent-core/harness/env"
	harnesstools "github.com/nowa/gi/gi-agent-core/harness/tools"
	"github.com/nowa/gi/gi-coding-agent/internal/ansiutil"
	llm "github.com/nowa/gi/gi-llm-provider"
)

type BashTool struct {
	cwd           string
	commandPrefix string
	operations    BashOperations
}

type BashToolOptions struct {
	CommandPrefix string
	Operations    BashOperations
	ShellPath     string
}

type BashToolInput struct {
	Command string
	// Timeout is expressed in seconds to match the tool protocol. Fractional
	// values are preserved and converted to time.Duration at the harness edge.
	Timeout float64

	timeoutSet bool
}

func NewBashTool(cwd string, options ...BashToolOptions) BashTool {
	opts := BashToolOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	operations := opts.Operations
	if operations.Exec == nil {
		operations = CreateLocalBashOperations(BashLocalOperationsOptions{ShellPath: opts.ShellPath})
	}
	return BashTool{
		cwd:           cwd,
		commandPrefix: opts.CommandPrefix,
		operations:    operations,
	}
}

func (t BashTool) Execute(toolCallID string, input BashToolInput) (FileToolResult, error) {
	return t.ExecuteWithUpdates(toolCallID, input, nil)
}

func (t BashTool) ExecuteWithUpdates(toolCallID string, input BashToolInput, onUpdate func(FileToolResult)) (FileToolResult, error) {
	if strings.TrimSpace(input.Command) == "" {
		return FileToolResult{}, errors.New("command is required")
	}
	if stat, err := os.Stat(t.cwd); err != nil || !stat.IsDir() {
		return FileToolResult{}, errors.New("Working directory does not exist: " + t.cwd)
	}

	env := newCodingBashExecutionEnv(t.cwd, t.operations)
	tool := harnesstools.CreateBashTool(harnesstools.BashToolOptions{
		CommandPrefix: t.commandPrefix,
	})
	params := map[string]any{"command": input.Command}
	if input.timeoutSet || input.Timeout != 0 {
		params["timeout"] = input.Timeout
	}
	var updateCallback core.AgentToolUpdateCallback
	if onUpdate != nil {
		updateCallback = func(update core.AgentToolResult) {
			onUpdate(compatibilityBashResult(update))
		}
	}
	result, err := tool.Execute(
		context.Background(),
		toolCallID,
		params,
		updateCallback,
		harnesstools.NewExecutionToolContext(env),
	)
	if err != nil {
		return FileToolResult{}, err
	}
	return compatibilityBashResult(result), nil
}

func compatibilityBashResult(result core.AgentToolResult) FileToolResult {
	compatibilityResult := FileToolResult{
		Text:    textFromContentParts(result.Content),
		Content: append([]llm.ContentPart(nil), result.Content...),
	}
	if details, ok := result.Details.(*harnesstools.BashToolDetails); ok && details != nil {
		compatibilityResult.Details = &FileToolDetails{
			Truncation:     details.Truncation,
			FullOutputPath: details.FullOutputPath,
		}
	}
	return compatibilityResult
}

type codingBashExecutionEnv struct {
	*harnessenv.LocalExecutionEnv
	operations BashOperations
}

func newCodingBashExecutionEnv(cwd string, operations BashOperations) *codingBashExecutionEnv {
	return &codingBashExecutionEnv{
		LocalExecutionEnv: harnessenv.MustLocalExecutionEnv(cwd),
		operations:        operations,
	}
}

func (e *codingBashExecutionEnv) Exec(ctx context.Context, command string, options harnessenv.ExecOptions) (harnessenv.ExecResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx := ctx
	cancel := func() {}
	if options.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, options.Timeout)
	} else {
		runCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	var callbackMu sync.Mutex
	var callbackErr error
	var outputMu sync.Mutex
	var outputStripper ansiutil.StreamStripper
	setCallbackError := func(err error) {
		if err == nil {
			return
		}
		callbackMu.Lock()
		if callbackErr == nil {
			callbackErr = err
		}
		callbackMu.Unlock()
		cancel()
	}
	operationResult, operationErr := e.operations.Exec(command, options.CWD, BashExecOptions{
		Context: runCtx,
		Env:     options.Env,
		OnData: func(content []byte) {
			if options.OnStdout == nil {
				return
			}
			outputMu.Lock()
			filtered := outputStripper.Write(content)
			if len(filtered) > 0 {
				setCallbackError(options.OnStdout(string(filtered)))
			}
			outputMu.Unlock()
		},
	})
	callbackMu.Lock()
	capturedCallbackErr := callbackErr
	callbackMu.Unlock()
	if capturedCallbackErr != nil {
		return harnessenv.ExecResult{}, &harnessenv.ExecutionError{
			Code: harnessenv.ExecutionErrorCallbackError,
			Err:  capturedCallbackErr,
		}
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
		return harnessenv.ExecResult{}, &harnessenv.ExecutionError{
			Code: harnessenv.ExecutionErrorTimeout,
			Err:  runCtx.Err(),
		}
	}
	if errors.Is(runCtx.Err(), context.Canceled) || operationResult.Cancelled {
		return harnessenv.ExecResult{}, &harnessenv.ExecutionError{
			Code: harnessenv.ExecutionErrorAborted,
			Err:  errors.New("aborted"),
		}
	}
	if operationErr != nil {
		switch {
		case strings.HasPrefix(operationErr.Error(), "timeout:"):
			return harnessenv.ExecResult{}, &harnessenv.ExecutionError{
				Code: harnessenv.ExecutionErrorTimeout,
				Err:  operationErr,
			}
		case operationErr.Error() == "aborted":
			return harnessenv.ExecResult{}, &harnessenv.ExecutionError{
				Code: harnessenv.ExecutionErrorAborted,
				Err:  operationErr,
			}
		case operationResult.ExitCode == 0:
			return harnessenv.ExecResult{}, &harnessenv.ExecutionError{
				Code: harnessenv.ExecutionErrorUnknown,
				Err:  operationErr,
			}
		}
	}
	return harnessenv.ExecResult{ExitCode: operationResult.ExitCode}, nil
}

var _ harnessenv.ExecutionEnv = (*codingBashExecutionEnv)(nil)
