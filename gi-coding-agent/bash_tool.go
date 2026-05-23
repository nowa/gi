package gicodingagent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
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
	Timeout int
}

const bashUpdateThrottle = 100 * time.Millisecond

func NewBashTool(cwd string, options ...BashToolOptions) BashTool {
	opts := BashToolOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	operations := opts.Operations
	if operations.Exec == nil {
		operations = CreateLocalBashOperations(BashLocalOperationsOptions{ShellPath: opts.ShellPath})
	}
	return BashTool{cwd: cwd, commandPrefix: opts.CommandPrefix, operations: operations}
}

func (t BashTool) Execute(_ string, input BashToolInput) (FileToolResult, error) {
	return t.ExecuteWithUpdates("", input, nil)
}

func (t BashTool) ExecuteWithUpdates(_ string, input BashToolInput, onUpdate func(FileToolResult)) (FileToolResult, error) {
	if strings.TrimSpace(input.Command) == "" {
		return FileToolResult{}, fmt.Errorf("command is required")
	}
	if stat, err := os.Stat(t.cwd); err != nil || !stat.IsDir() {
		return FileToolResult{}, fmt.Errorf("Working directory does not exist: %s", t.cwd)
	}
	if onUpdate != nil {
		onUpdate(FileToolResult{})
	}
	command := input.Command
	if strings.TrimSpace(t.commandPrefix) != "" {
		command = t.commandPrefix + "\n" + command
	}

	ctx := context.Background()
	cancel := func() {}
	if input.Timeout > 0 {
		var cancelFunc context.CancelFunc
		ctx, cancelFunc = context.WithTimeout(ctx, time.Duration(input.Timeout)*time.Second)
		cancel = cancelFunc
	}
	defer cancel()

	var updateAccumulator *bashOutputAccumulator
	if onUpdate != nil {
		updateAccumulator = newBashOutputAccumulator(bashOutputAccumulatorOptions{
			MaxLines:       defaultBashOutputLineLimit,
			MaxBytes:       agentharness.DefaultMaxBytes,
			TempFilePrefix: "gi-bash-update",
		})
		defer updateAccumulator.Close()
	}
	lastUpdate := time.Now()
	emitUpdate := func() {
		if onUpdate == nil || updateAccumulator == nil {
			return
		}
		snapshot := updateAccumulator.Snapshot(true)
		onUpdate(bashSnapshotFileToolResult(snapshot))
	}
	result, err := ExecuteBashWithOperations(command, t.cwd, t.operations, BashExecutorOptions{
		Context: ctx,
		OnChunk: func(chunk string) {
			if onUpdate == nil || updateAccumulator == nil {
				return
			}
			updateAccumulator.Append([]byte(chunk))
			if time.Since(lastUpdate) < bashUpdateThrottle {
				return
			}
			lastUpdate = time.Now()
			emitUpdate()
		},
	})
	if onUpdate != nil && updateAccumulator != nil && updateAccumulator.totalRawBytes > 0 {
		emitUpdate()
	}
	if ctx.Err() == context.DeadlineExceeded {
		return FileToolResult{}, formatBashToolError(fmt.Sprintf("Command timed out after %d seconds", input.Timeout), result)
	}
	if err != nil && result.ExitCode == 0 {
		return FileToolResult{}, formatBashToolError(formatBashOperationError(err), result)
	}
	if result.ExitCode != 0 {
		return FileToolResult{}, formatBashToolError(fmt.Sprintf("Command failed with code %d", result.ExitCode), result)
	}
	details := bashToolDetails(result)
	return FileToolResult{Text: result.Output, Content: []llm.ContentPart{llm.Text(result.Output)}, Details: details}, nil
}

func bashSnapshotFileToolResult(snapshot bashOutputSnapshot) FileToolResult {
	text := snapshot.Content
	result := FileToolResult{Text: text, Content: []llm.ContentPart{llm.Text(text)}}
	details := bashToolDetails(BashResult{
		Truncated:      snapshot.Truncation.Truncated,
		TruncatedBy:    snapshot.Truncation.TruncatedBy,
		FullOutputPath: snapshot.FullOutputPath,
		TotalLines:     snapshot.Truncation.TotalLines,
		OutputLines:    snapshot.Truncation.OutputLines,
	})
	if details != nil {
		result.Details = details
	}
	return result
}

func formatBashOperationError(err error) string {
	message := err.Error()
	if strings.HasPrefix(message, "timeout:") {
		seconds := strings.TrimPrefix(message, "timeout:")
		return "Command timed out after " + seconds + " seconds"
	}
	if message == "aborted" {
		return "Command aborted"
	}
	return message
}

func formatBashToolError(message string, result BashResult) error {
	if strings.TrimSpace(result.Output) != "" {
		return fmt.Errorf("%s\n%s", message, result.Output)
	}
	return fmt.Errorf("%s", message)
}

func bashToolDetails(result BashResult) *FileToolDetails {
	if !result.Truncated && result.FullOutputPath == "" {
		return nil
	}
	details := &FileToolDetails{FullOutputPath: result.FullOutputPath}
	if result.Truncated {
		details.Truncation = &ReadToolTruncation{
			Truncated:   true,
			TruncatedBy: result.TruncatedBy,
			TotalLines:  result.TotalLines,
			OutputLines: result.OutputLines,
		}
	}
	return details
}
