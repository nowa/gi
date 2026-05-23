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

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
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
	ExitCode  int  `json:"exitCode"`
	Cancelled bool `json:"cancelled"`
}

type BashOperations struct {
	Exec func(command, cwd string, options BashExecOptions) (BashOperationResult, error)
}

type BashLocalOperationsOptions struct {
	ShellPath string
}

type BashResult struct {
	Output          string `json:"output"`
	ExitCode        int    `json:"exitCode"`
	Cancelled       bool   `json:"cancelled"`
	Truncated       bool   `json:"truncated"`
	TruncatedBy     string `json:"truncatedBy,omitempty"`
	FullOutputPath  string `json:"fullOutputPath,omitempty"`
	TotalLines      int    `json:"totalLines,omitempty"`
	TotalBytes      int    `json:"totalBytes,omitempty"`
	OutputLines     int    `json:"outputLines,omitempty"`
	OutputBytes     int    `json:"outputBytes,omitempty"`
	LastLinePartial bool   `json:"lastLinePartial,omitempty"`
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
	outputAccumulator := newBashOutputAccumulator(bashOutputAccumulatorOptions{
		MaxLines:       defaultBashOutputLineLimit,
		MaxBytes:       agentharness.DefaultMaxBytes,
		TempFilePrefix: "gi-bash-output",
	})
	operationResult, err := operations.Exec(command, cwd, BashExecOptions{
		Context:        ctx,
		ExitStdioGrace: grace,
		OnData: func(data []byte) {
			if len(data) == 0 {
				return
			}
			copied := append([]byte(nil), data...)
			mu.Lock()
			outputAccumulator.Append(copied)
			mu.Unlock()
			if opts.OnChunk != nil {
				opts.OnChunk(sanitizeBashOutputBytes(copied))
			}
		},
	})

	mu.Lock()
	snapshot := outputAccumulator.Snapshot(true)
	outputAccumulator.Close()
	mu.Unlock()
	output, truncation := formatBashOutputSnapshot(snapshot)
	result := BashResult{
		Output:          output,
		ExitCode:        operationResult.ExitCode,
		Cancelled:       operationResult.Cancelled,
		Truncated:       truncation.Truncated,
		TruncatedBy:     truncation.TruncatedBy,
		FullOutputPath:  truncation.FullOutputPath,
		TotalLines:      truncation.TotalLines,
		TotalBytes:      truncation.TotalBytes,
		OutputLines:     truncation.OutputLines,
		OutputBytes:     truncation.OutputBytes,
		LastLinePartial: truncation.LastLinePartial,
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
	configureLocalBashCommand(cmd)
	cmd.Cancel = func() error {
		return cancelLocalBashCommand(cmd.Process)
	}
	if len(options.Env) > 0 {
		cmd.Env = mergeBashEnv(options.Env)
	}
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		return BashOperationResult{}, err
	}
	defer stdoutReader.Close()
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		_ = stdoutWriter.Close()
		return BashOperationResult{}, err
	}
	defer stderrReader.Close()
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter
	if err := cmd.Start(); err != nil {
		_ = stdoutWriter.Close()
		_ = stderrWriter.Close()
		if os.IsNotExist(err) {
			return BashOperationResult{}, fmt.Errorf("ENOENT: %w", err)
		}
		return BashOperationResult{}, err
	}
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()

	readDone := make(chan struct{}, 2)
	readPipe := func(pipe io.ReadCloser) {
		defer func() { readDone <- struct{}{} }()
		buffer := make([]byte, 4096)
		for {
			n, err := pipe.Read(buffer)
			if n > 0 {
				if options.OnData != nil {
					options.OnData(append([]byte(nil), buffer[:n]...))
				}
			}
			if err != nil {
				return
			}
		}
	}
	go readPipe(stdoutReader)
	go readPipe(stderrReader)

	waitErr := cmd.Wait()
	waitForBashPipesOrClose(stdoutReader, stderrReader, readDone, grace)

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
	Truncated       bool
	TruncatedBy     string
	FullOutputPath  string
	TotalLines      int
	TotalBytes      int
	OutputLines     int
	OutputBytes     int
	LastLinePartial bool
}

func sanitizeBashOutputBytes(data []byte) string {
	text := string(bytes.ToValidUTF8(data, []byte{}))
	text = StripAnsi(text)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return text
}

func formatBashOutput(fullOutput string) (string, bashOutputTruncation) {
	truncation := agentharness.TruncateTail(fullOutput, agentharness.TruncationOptions{
		MaxLines: defaultBashOutputLineLimit,
		MaxBytes: agentharness.DefaultMaxBytes,
	})
	fullOutputPath := ""
	if truncation.Truncated {
		fullOutputPath = persistBashFullOutput(fullOutput)
	}
	return formatBashOutputSnapshot(bashOutputSnapshot{
		Content:        truncation.Content,
		Truncation:     truncation,
		FullOutputPath: fullOutputPath,
		LastLineBytes:  lastBashLineBytes(fullOutput),
	})
}

func formatBashOutputSnapshot(snapshot bashOutputSnapshot) (string, bashOutputTruncation) {
	truncation := snapshot.Truncation
	if !truncation.Truncated {
		return snapshot.Content, bashOutputTruncation{
			TotalLines:  truncation.TotalLines,
			TotalBytes:  truncation.TotalBytes,
			OutputLines: truncation.OutputLines,
			OutputBytes: truncation.OutputBytes,
		}
	}

	startLine := truncation.TotalLines - truncation.OutputLines + 1
	endLine := truncation.TotalLines
	var summary string
	switch {
	case truncation.LastLinePartial:
		summary = fmt.Sprintf(
			"[Showing last %s of line %d (line is %s). Full output: %s]",
			formatBashOutputSize(truncation.OutputBytes),
			endLine,
			formatBashOutputSize(snapshot.LastLineBytes),
			snapshot.FullOutputPath,
		)
	case truncation.TruncatedBy == agentharness.TruncatedByLines:
		summary = fmt.Sprintf("[Showing lines %d-%d of %d. Full output: %s]", startLine, endLine, truncation.TotalLines, snapshot.FullOutputPath)
	default:
		summary = fmt.Sprintf(
			"[Showing lines %d-%d of %d (%s limit). Full output: %s]",
			startLine,
			endLine,
			truncation.TotalLines,
			formatBashOutputSize(truncation.MaxBytes),
			snapshot.FullOutputPath,
		)
	}
	display := truncation.Content
	if display != "" {
		display += "\n\n"
	}
	display += summary
	return display, bashOutputTruncation{
		Truncated:       true,
		TruncatedBy:     truncation.TruncatedBy,
		FullOutputPath:  snapshot.FullOutputPath,
		TotalLines:      truncation.TotalLines,
		TotalBytes:      truncation.TotalBytes,
		OutputLines:     truncation.OutputLines,
		OutputBytes:     truncation.OutputBytes,
		LastLinePartial: truncation.LastLinePartial,
	}
}

func formatBashOutputSize(bytes int) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%dB", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	}
}

func lastBashLineBytes(output string) int {
	index := strings.LastIndex(output, "\n")
	if index < 0 {
		return len([]byte(output))
	}
	return len([]byte(output[index+1:]))
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
