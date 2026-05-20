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
}

type BashToolOptions struct {
	CommandPrefix string
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
	return BashTool{cwd: cwd, commandPrefix: opts.CommandPrefix}
}

func (t BashTool) Execute(_ string, input BashToolInput) (FileToolResult, error) {
	if strings.TrimSpace(input.Command) == "" {
		return FileToolResult{}, fmt.Errorf("command is required")
	}
	if stat, err := os.Stat(t.cwd); err != nil || !stat.IsDir() {
		return FileToolResult{}, fmt.Errorf("Working directory does not exist: %s", t.cwd)
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

	result, err := ExecuteBash(command, t.cwd, BashExecutorOptions{Context: ctx})
	if ctx.Err() == context.DeadlineExceeded {
		return FileToolResult{}, fmt.Errorf("Command timed out after %d seconds", input.Timeout)
	}
	if err != nil && result.ExitCode == 0 {
		return FileToolResult{}, err
	}
	if result.ExitCode != 0 {
		return FileToolResult{}, fmt.Errorf("Command failed with code %d", result.ExitCode)
	}
	return FileToolResult{Text: result.Output, Content: []llm.ContentPart{llm.Text(result.Output)}}, nil
}
