package tools

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	core "github.com/nowa/gi/gi-agent-core"
	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	harnessutils "github.com/nowa/gi/gi-agent-core/harness/utils"
	llm "github.com/nowa/gi/gi-llm-provider"
)

const (
	maxBashTimeoutSeconds = float64(2_147_483_647) / 1000
	bashUpdateThrottle    = 100 * time.Millisecond
)

type BashToolDetails struct {
	Truncation     *agentharness.TruncationResult `json:"truncation,omitempty"`
	FullOutputPath string                         `json:"fullOutputPath,omitempty"`
}

type BashExecution struct {
	Command    string
	CWD        string
	Env        map[string]string
	InheritEnv bool
}

type BashPrepare func(ctx context.Context, execution *BashExecution, toolContext any) error

type BashToolOptions struct {
	CommandPrefix string
	Prepare       BashPrepare
}

func CreateBashTool(options ...BashToolOptions) agentharness.AgentHarnessTool {
	opts := BashToolOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	return agentharness.AgentHarnessTool{
		Name:        "bash",
		Label:       "bash",
		Description: fmt.Sprintf("Execute a bash command in the current working directory. Returns stdout and stderr. Output is truncated to last %d lines or %dKB (whichever is hit first). If truncated, full output is saved to a temp file. Optionally provide a timeout in seconds.", agentharness.DefaultMaxLines, agentharness.DefaultMaxBytes/1024),
		Parameters: llm.Object(map[string]llm.Schema{
			"command": {
				Type:        "string",
				Description: "Bash command to execute",
			},
			"timeout": {
				Type:        "number",
				Description: "Timeout in seconds (optional, no default timeout)",
			},
		}, "command"),
		Execute: func(ctx context.Context, _ string, params map[string]any, onUpdate core.AgentToolUpdateCallback, contextValue any) (core.AgentToolResult, error) {
			provider, _, err := executionContext(contextValue)
			if err != nil {
				return core.AgentToolResult{}, err
			}
			command, err := requiredString(params, "command")
			if err != nil {
				return core.AgentToolResult{}, err
			}
			timeoutSeconds, hasTimeout, err := optionalNumber(params, "timeout")
			if err != nil {
				return core.AgentToolResult{}, err
			}
			timeout, err := resolveBashTimeout(timeoutSeconds, hasTimeout)
			if err != nil {
				return core.AgentToolResult{}, err
			}
			if opts.CommandPrefix != "" {
				command = opts.CommandPrefix + "\n" + command
			}
			env := provider.ExecutionEnvironment()
			execution := BashExecution{
				Command:    command,
				CWD:        env.CWD(),
				Env:        map[string]string{},
				InheritEnv: true,
			}
			if opts.Prepare != nil {
				if err := opts.Prepare(ctx, &execution, contextValue); err != nil {
					return core.AgentToolResult{}, err
				}
			}
			if onUpdate != nil {
				onUpdate(core.AgentToolResult{})
			}

			var updateMu sync.Mutex
			lastUpdate := time.Time{}
			settled := false
			emitProgress := func(progress func() harnessutils.ShellCaptureProgress, force bool) {
				if onUpdate == nil {
					return
				}
				updateMu.Lock()
				defer updateMu.Unlock()
				if settled && !force {
					return
				}
				now := time.Now()
				if !force && !lastUpdate.IsZero() && now.Sub(lastUpdate) < bashUpdateThrottle {
					return
				}
				lastUpdate = now
				onUpdate(bashProgressResult(progress(), false))
			}

			capture, err := harnessutils.ExecuteShellWithCapture(ctx, env, execution.Command, harnessutils.ShellCaptureOptions{
				CWD:                   execution.CWD,
				Env:                   execution.Env,
				InheritEnv:            execution.InheritEnv,
				Timeout:               timeout,
				ReturnExecutionErrors: true,
				OnChunk: func(_ string, progress func() harnessutils.ShellCaptureProgress) {
					emitProgress(progress, false)
				},
			})
			updateMu.Lock()
			settled = true
			updateMu.Unlock()
			if err != nil {
				return core.AgentToolResult{}, err
			}
			if onUpdate != nil {
				emitProgress(func() harnessutils.ShellCaptureProgress {
					return capture.ShellCaptureProgress
				}, true)
			}

			output, details := formatBashCapture(capture.ShellCaptureProgress)
			appendStatus := func(status string) string {
				if output == "" {
					return status
				}
				return output + "\n\n" + status
			}
			switch {
			case capture.Cancelled:
				return core.AgentToolResult{}, fmt.Errorf("%s", appendStatus("Command aborted"))
			case capture.ExecutionError != nil && capture.ExecutionError.Code == "timeout":
				timeoutLabel := fmt.Sprintf("%g", timeoutSeconds)
				if !hasTimeout && capture.ExecutionError.Err != nil {
					if encoded, ok := strings.CutPrefix(capture.ExecutionError.Err.Error(), "timeout:"); ok {
						timeoutLabel = encoded
					}
				}
				return core.AgentToolResult{}, fmt.Errorf("%s", appendStatus(fmt.Sprintf("Command timed out after %s seconds", timeoutLabel)))
			case capture.ExecutionError != nil:
				return core.AgentToolResult{}, capture.ExecutionError
			case capture.ExitCode != nil && *capture.ExitCode != 0:
				return core.AgentToolResult{}, fmt.Errorf("%s", appendStatus(fmt.Sprintf("Command exited with code %d", *capture.ExitCode)))
			}
			if output == "" {
				output = "(no output)"
			}
			result := core.AgentToolResult{Content: []llm.ContentPart{llm.Text(output)}}
			if details != nil {
				result.Details = details
			}
			return result, nil
		},
	}
}

func resolveBashTimeout(timeout float64, set bool) (time.Duration, error) {
	if !set {
		return 0, nil
	}
	if math.IsNaN(timeout) || math.IsInf(timeout, 0) || timeout <= 0 {
		return 0, fmt.Errorf("Invalid timeout: must be a finite number of seconds")
	}
	if timeout > maxBashTimeoutSeconds {
		return 0, fmt.Errorf("Invalid timeout: maximum is %g seconds", maxBashTimeoutSeconds)
	}
	return time.Duration(timeout * float64(time.Second)), nil
}

func bashProgressResult(progress harnessutils.ShellCaptureProgress, includeSummary bool) core.AgentToolResult {
	output := progress.Output
	var details *BashToolDetails
	if includeSummary {
		output, details = formatBashCapture(progress)
	} else if progress.Truncation.Truncated || progress.FullOutputPath != "" {
		details = &BashToolDetails{FullOutputPath: progress.FullOutputPath}
		if progress.Truncation.Truncated {
			truncation := progress.Truncation
			details.Truncation = &truncation
		}
	}
	result := core.AgentToolResult{Content: []llm.ContentPart{llm.Text(output)}}
	if details != nil {
		result.Details = details
	}
	return result
}

func formatBashCapture(progress harnessutils.ShellCaptureProgress) (string, *BashToolDetails) {
	if !progress.Truncation.Truncated {
		return progress.Output, nil
	}
	truncation := progress.Truncation
	details := &BashToolDetails{
		Truncation:     &truncation,
		FullOutputPath: progress.FullOutputPath,
	}
	startLine := truncation.TotalLines - truncation.OutputLines + 1
	endLine := truncation.TotalLines
	var summary string
	switch {
	case truncation.LastLinePartial:
		summary = fmt.Sprintf(
			"[Showing last %s of line %d (line is %s). Full output: %s]",
			formatSize(truncation.OutputBytes),
			endLine,
			formatSize(progress.LastLineBytes),
			progress.FullOutputPath,
		)
	case truncation.TruncatedBy == agentharness.TruncatedByLines:
		summary = fmt.Sprintf(
			"[Showing lines %d-%d of %d. Full output: %s]",
			startLine,
			endLine,
			truncation.TotalLines,
			progress.FullOutputPath,
		)
	default:
		summary = fmt.Sprintf(
			"[Showing lines %d-%d of %d (%s limit). Full output: %s]",
			startLine,
			endLine,
			truncation.TotalLines,
			formatSize(agentharness.DefaultMaxBytes),
			progress.FullOutputPath,
		)
	}
	output := progress.Output
	if strings.TrimSpace(output) != "" {
		output += "\n\n"
	}
	return output + summary, details
}
