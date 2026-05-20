package gicodingagent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

const defaultBashExitStdioGrace = time.Second
const defaultBashOutputLineLimit = 2000

type BashExecutorOptions struct {
	Context        context.Context
	OnChunk        func(string)
	ExitStdioGrace time.Duration
}

type BashExecOptions struct {
	Context        context.Context
	OnData         func([]byte)
	Env            map[string]string
	ExitStdioGrace time.Duration
}

type BashOperationResult struct {
	ExitCode  int
	Cancelled bool
}

type BashOperations struct {
	Exec func(command, cwd string, options BashExecOptions) (BashOperationResult, error)
}

type BashLocalOperationsOptions struct {
	ShellPath string
}

type BashResult struct {
	Output         string
	ExitCode       int
	Cancelled      bool
	Truncated      bool
	TruncatedBy    string
	FullOutputPath string
	TotalLines     int
	OutputLines    int
}

func ExecuteBash(command, cwd string, options ...BashExecutorOptions) (BashResult, error) {
	return ExecuteBashWithOperations(command, cwd, CreateLocalBashOperations(), options...)
}

func ExecuteBashWithOperations(command, cwd string, operations BashOperations, options ...BashExecutorOptions) (BashResult, error) {
	opts := BashExecutorOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	grace := opts.ExitStdioGrace
	if grace <= 0 {
		grace = defaultBashExitStdioGrace
	}

	operations = normalizeBashOperations(operations)
	var mu sync.Mutex
	var rawOutput bytes.Buffer
	operationResult, err := operations.Exec(command, cwd, BashExecOptions{
		Context:        ctx,
		ExitStdioGrace: grace,
		OnData: func(data []byte) {
			if len(data) == 0 {
				return
			}
			copied := append([]byte(nil), data...)
			mu.Lock()
			rawOutput.Write(copied)
			mu.Unlock()
			if opts.OnChunk != nil {
				opts.OnChunk(sanitizeBashOutputBytes(copied))
			}
		},
	})

	mu.Lock()
	fullOutput := sanitizeBashOutputBytes(rawOutput.Bytes())
	mu.Unlock()
	output, truncation := formatBashOutput(fullOutput)
	result := BashResult{
		Output:         output,
		ExitCode:       operationResult.ExitCode,
		Cancelled:      operationResult.Cancelled,
		Truncated:      truncation.Truncated,
		TruncatedBy:    truncation.TruncatedBy,
		FullOutputPath: truncation.FullOutputPath,
		TotalLines:     truncation.TotalLines,
		OutputLines:    truncation.OutputLines,
	}
	return result, err
}

func CreateLocalBashOperations(options ...BashLocalOperationsOptions) BashOperations {
	opts := BashLocalOperationsOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	return BashOperations{
		Exec: func(command, cwd string, execOptions BashExecOptions) (BashOperationResult, error) {
			return execLocalBash(command, cwd, opts, execOptions)
		},
	}
}

func normalizeBashOperations(operations BashOperations) BashOperations {
	if operations.Exec == nil {
		return CreateLocalBashOperations()
	}
	return operations
}

func execLocalBash(command, cwd string, localOptions BashLocalOperationsOptions, options BashExecOptions) (BashOperationResult, error) {
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	grace := options.ExitStdioGrace
	if grace <= 0 {
		grace = defaultBashExitStdioGrace
	}
	shell := "sh"
	if strings.TrimSpace(localOptions.ShellPath) != "" {
		shell = localOptions.ShellPath
		if _, err := os.Stat(shell); err != nil {
			if os.IsNotExist(err) {
				return BashOperationResult{}, fmt.Errorf("Custom shell path not found: %s", shell)
			}
			return BashOperationResult{}, err
		}
	}

	cmd := exec.CommandContext(ctx, shell, "-c", command)
	cmd.Dir = cwd
	if len(options.Env) > 0 {
		cmd.Env = mergeBashEnv(options.Env)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return BashOperationResult{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return BashOperationResult{}, err
	}
	if err := cmd.Start(); err != nil {
		if os.IsNotExist(err) {
			return BashOperationResult{}, fmt.Errorf("ENOENT: %w", err)
		}
		return BashOperationResult{}, err
	}

	readDone := make(chan struct{}, 2)
	readPipe := func(pipe io.ReadCloser) {
		defer func() { readDone <- struct{}{} }()
		buffer := make([]byte, 4096)
		for {
			n, err := pipe.Read(buffer)
			if n > 0 {
				if options.OnData != nil {
					options.OnData(buffer[:n])
				}
			}
			if err != nil {
				return
			}
		}
	}
	go readPipe(stdout)
	go readPipe(stderr)

	waitErr := cmd.Wait()
	waitForBashPipesOrClose(stdout, stderr, readDone, grace)

	result := BashOperationResult{ExitCode: 0, Cancelled: errors.Is(ctx.Err(), context.Canceled)}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	if result.Cancelled {
		return result, nil
	}
	return result, waitErr
}

type bashOutputTruncation struct {
	Truncated      bool
	TruncatedBy    string
	FullOutputPath string
	TotalLines     int
	OutputLines    int
}

func sanitizeBashOutputBytes(data []byte) string {
	text := string(bytes.ToValidUTF8(data, []byte{}))
	text = StripAnsi(text)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return text
}

func formatBashOutput(fullOutput string) (string, bashOutputTruncation) {
	lines := splitBashOutputLines(fullOutput)
	totalLines := len(lines)
	if totalLines <= defaultBashOutputLineLimit {
		return fullOutput, bashOutputTruncation{TotalLines: totalLines, OutputLines: totalLines}
	}
	startLine := totalLines - defaultBashOutputLineLimit + 1
	outputLines := lines[startLine-1:]
	fullOutputPath := persistBashFullOutput(fullOutput)
	summary := fmt.Sprintf("[Showing lines %d-%d of %d. Full output: %s]", startLine, totalLines, totalLines, fullOutputPath)
	display := strings.Join(outputLines, "\n")
	if display != "" {
		display += "\n"
	}
	display += summary
	return display, bashOutputTruncation{
		Truncated:      true,
		TruncatedBy:    "lines",
		FullOutputPath: fullOutputPath,
		TotalLines:     totalLines,
		OutputLines:    defaultBashOutputLineLimit,
	}
}

func splitBashOutputLines(output string) []string {
	if output == "" {
		return nil
	}
	trimmed := strings.TrimSuffix(output, "\n")
	if trimmed == "" {
		return []string{""}
	}
	return strings.Split(trimmed, "\n")
}

func persistBashFullOutput(output string) string {
	file, err := os.CreateTemp("", "gi-bash-output-*.txt")
	if err != nil {
		return ""
	}
	path := file.Name()
	_, writeErr := file.WriteString(output)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return ""
	}
	return path
}

func mergeBashEnv(overrides map[string]string) []string {
	env := map[string]string{}
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			env[key] = value
		}
	}
	for key, value := range overrides {
		env[key] = value
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	merged := make([]string, 0, len(keys))
	for _, key := range keys {
		merged = append(merged, key+"="+env[key])
	}
	return merged
}

func waitForBashPipesOrClose(stdout, stderr io.Closer, readDone <-chan struct{}, grace time.Duration) {
	remaining := 2
	timer := time.NewTimer(grace)
	defer timer.Stop()
	for remaining > 0 {
		select {
		case <-readDone:
			remaining--
		case <-timer.C:
			_ = stdout.Close()
			_ = stderr.Close()
			for remaining > 0 {
				<-readDone
				remaining--
			}
		}
	}
}
