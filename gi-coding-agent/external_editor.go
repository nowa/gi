package gicodingagent

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ExternalEditorStatus describes whether the editor produced replacement text.
type ExternalEditorStatus string

const (
	ExternalEditorStatusComplete ExternalEditorStatus = "complete"
	ExternalEditorStatusFailed   ExternalEditorStatus = "failed"
)

// ExternalEditorOptions is the immutable request passed from a UI surface to
// the external-editor process boundary.
type ExternalEditorOptions struct {
	Command string
	Content string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
}

// ExternalEditorResult is detached from the temporary file and safe for the UI
// owner to apply after the process boundary has closed.
type ExternalEditorResult struct {
	Status  ExternalEditorStatus
	Content string
}

type externalEditorInvocation struct {
	Command  string
	Args     []string
	FilePath string
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
}

type externalEditorRunner interface {
	Run(context.Context, externalEditorInvocation) error
}

type processExternalEditorRunner struct{}

func (processExternalEditorRunner) Run(ctx context.Context, invocation externalEditorInvocation) error {
	args := append([]string(nil), invocation.Args...)
	args = append(args, invocation.FilePath)
	command := exec.CommandContext(ctx, invocation.Command, args...)
	command.Stdin = invocation.Stdin
	command.Stdout = invocation.Stdout
	command.Stderr = invocation.Stderr
	return command.Run()
}

// EditInExternalEditor writes content to a private temporary workspace, waits
// for the configured editor, and returns a detached result. A failed editor
// process is a normal failed result; filesystem failures are returned as
// errors. The temporary workspace is always removed on a best-effort basis.
func EditInExternalEditor(ctx context.Context, options ExternalEditorOptions) (ExternalEditorResult, error) {
	return editInExternalEditor(ctx, options, processExternalEditorRunner{})
}

func editInExternalEditor(
	ctx context.Context,
	options ExternalEditorOptions,
	runner externalEditorRunner,
) (ExternalEditorResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if runner == nil {
		return ExternalEditorResult{Status: ExternalEditorStatusFailed}, nil
	}

	directory, err := os.MkdirTemp("", "gi-editor-")
	if err != nil {
		return ExternalEditorResult{}, err
	}
	defer func() {
		_ = os.RemoveAll(directory)
	}()

	filePath := filepath.Join(directory, "prompt.md")
	if err := os.WriteFile(filePath, []byte(options.Content), 0o600); err != nil {
		return ExternalEditorResult{}, err
	}

	name, args, ok := splitExternalEditorCommand(options.Command)
	if !ok {
		return ExternalEditorResult{Status: ExternalEditorStatusFailed}, nil
	}
	stdin := options.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	stdout := options.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := options.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	_, _ = fmt.Fprintf(
		stdout,
		"Launching external editor: %s\nGi will resume when the editor exits.\n",
		options.Command,
	)
	if err := runner.Run(ctx, externalEditorInvocation{
		Command:  name,
		Args:     args,
		FilePath: filePath,
		Stdin:    stdin,
		Stdout:   stdout,
		Stderr:   stderr,
	}); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ExternalEditorResult{Status: ExternalEditorStatusFailed}, ctxErr
		}
		return ExternalEditorResult{Status: ExternalEditorStatusFailed}, nil
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return ExternalEditorResult{}, err
	}
	return ExternalEditorResult{
		Status:  ExternalEditorStatusComplete,
		Content: strings.TrimSuffix(string(content), "\n"),
	}, nil
}

func splitExternalEditorCommand(command string) (string, []string, bool) {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return "", nil, false
	}
	return fields[0], fields[1:], true
}

func resolveExternalEditorCommand(
	configured string,
	lookup func(string) (string, bool),
	goos string,
) string {
	if strings.TrimSpace(configured) != "" {
		return configured
	}
	if lookup != nil {
		if command, ok := lookup("VISUAL"); ok && command != "" {
			return command
		}
		if command, ok := lookup("EDITOR"); ok && command != "" {
			return command
		}
	}
	if goos == "windows" {
		return "notepad"
	}
	return "nano"
}

func defaultExternalEditorCommand(configured string) string {
	return resolveExternalEditorCommand(configured, os.LookupEnv, runtime.GOOS)
}
