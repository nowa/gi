package gicodingagent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

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

	var updateText strings.Builder
	lastUpdate := time.Now()
	result, err := ExecuteBashWithOperations(command, t.cwd, t.operations, BashExecutorOptions{
		Context: ctx,
		OnChunk: func(chunk string) {
			if onUpdate == nil {
				return
			}
			updateText.WriteString(chunk)
			if time.Since(lastUpdate) < 50*time.Millisecond {
				return
			}
			lastUpdate = time.Now()
			text := updateText.String()
			onUpdate(FileToolResult{Text: text, Content: []llm.ContentPart{llm.Text(text)}})
		},
	})
	if onUpdate != nil && updateText.Len() > 0 {
		text := updateText.String()
		onUpdate(FileToolResult{Text: text, Content: []llm.ContentPart{llm.Text(text)}})
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
