package utils

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	harnessenv "github.com/nowa/gi/gi-agent-core/harness/env"
)

type ShellCaptureProgress struct {
	Output         string
	Truncation     TruncationResult
	FullOutputPath string
	LastLineBytes  int
}

type ShellCaptureOptions struct {
	CWD                   string
	Env                   map[string]string
	InheritEnv            bool
	Timeout               time.Duration
	OnChunk               func(chunk string, progress func() ShellCaptureProgress)
	ReturnExecutionErrors bool
}

type ShellCaptureResult struct {
	ShellCaptureProgress
	ExitCode       *int
	Cancelled      bool
	ExecutionError *harnessenv.ExecutionError
}

type shellCaptureState struct {
	mu sync.Mutex

	env        harnessenv.ExecutionEnv
	persistCtx context.Context
	accepting  bool

	tail             []byte
	fullBuffer       []byte
	fullOutputPath   string
	totalBytes       int
	completedLines   int
	hasOpenLine      bool
	currentLineBytes int
	err              error
}

func ExecuteShellWithCapture(ctx context.Context, env harnessenv.ExecutionEnv, command string, options ShellCaptureOptions) (ShellCaptureResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	state := &shellCaptureState{
		env:        env,
		persistCtx: context.WithoutCancel(ctx),
		accepting:  true,
	}
	onChunk := func(chunk string) error {
		accepted, err := state.append(chunk)
		if err != nil {
			return err
		}
		if accepted && options.OnChunk != nil {
			options.OnChunk(SanitizeBinaryOutput(chunk), state.progress)
		}
		return nil
	}
	inheritEnv := options.InheritEnv
	result, execErr := env.Exec(ctx, command, harnessenv.ExecOptions{
		CWD:        options.CWD,
		Env:        cloneStringMap(options.Env),
		InheritEnv: &inheritEnv,
		Timeout:    options.Timeout,
		OnStdout:   onChunk,
		OnStderr:   onChunk,
	})
	state.stopAccepting()

	if state.totalOutputBytes() == 0 && (result.Stdout != "" || result.Stderr != "") {
		if err := state.appendFallback(result.Stdout + result.Stderr); err != nil && execErr == nil {
			execErr = err
		}
	}
	progress, captureErr := state.finalize()
	if captureErr != nil {
		return ShellCaptureResult{}, captureErr
	}

	captured := ShellCaptureResult{ShellCaptureProgress: progress}
	if execErr != nil {
		var executionError *harnessenv.ExecutionError
		if !errors.As(execErr, &executionError) {
			executionError = &harnessenv.ExecutionError{
				Code: harnessenv.ExecutionErrorUnknown,
				Err:  execErr,
			}
		}
		if executionError.Code == harnessenv.ExecutionErrorAborted || errors.Is(ctx.Err(), context.Canceled) {
			captured.Cancelled = true
			return captured, nil
		}
		if options.ReturnExecutionErrors {
			captured.ExecutionError = executionError
			return captured, nil
		}
		return ShellCaptureResult{}, executionError
	}
	exitCode := result.ExitCode
	captured.ExitCode = &exitCode
	if errors.Is(ctx.Err(), context.Canceled) {
		captured.ExitCode = nil
		captured.Cancelled = true
	}
	return captured, nil
}

func (s *shellCaptureState) append(chunk string) (bool, error) {
	text := SanitizeBinaryOutput(chunk)
	if text == "" {
		return false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.accepting {
		return false, nil
	}
	return true, s.appendLocked([]byte(text))
}

func (s *shellCaptureState) appendFallback(chunk string) error {
	text := SanitizeBinaryOutput(chunk)
	if text == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.totalBytes != 0 {
		return nil
	}
	return s.appendLocked([]byte(text))
}

func (s *shellCaptureState) appendLocked(data []byte) error {
	if s.err != nil {
		return s.err
	}
	s.totalBytes += len(data)
	newlineCount := bytes.Count(data, []byte{'\n'})
	s.completedLines += newlineCount
	if lastNewline := bytes.LastIndexByte(data, '\n'); lastNewline >= 0 {
		s.currentLineBytes = len(data[lastNewline+1:])
		s.hasOpenLine = s.currentLineBytes > 0
	} else {
		s.currentLineBytes += len(data)
		s.hasOpenLine = true
	}

	s.tail = append(s.tail, data...)
	s.trimTail()
	if s.fullOutputPath == "" {
		s.fullBuffer = append(s.fullBuffer, data...)
		if s.isTruncatedLocked() {
			if err := s.startFullOutputLocked(); err != nil {
				s.err = err
				return err
			}
		}
		return nil
	}
	if err := s.env.AppendFile(s.persistCtx, s.fullOutputPath, data); err != nil {
		s.err = err
		return err
	}
	return nil
}

func (s *shellCaptureState) startFullOutputLocked() error {
	path, err := s.env.CreateTempFile(s.persistCtx, harnessenv.TempFileOptions{
		Prefix: "bash-",
		Suffix: ".log",
	})
	if err != nil {
		return err
	}
	if err := s.env.WriteFile(s.persistCtx, path, s.fullBuffer); err != nil {
		return err
	}
	s.fullOutputPath = path
	s.fullBuffer = nil
	return nil
}

func (s *shellCaptureState) trimTail() {
	maxTailBytes := DefaultMaxBytes * 2
	if len(s.tail) <= maxTailBytes {
		return
	}
	start := len(s.tail) - maxTailBytes
	for start < len(s.tail) && !utf8.RuneStart(s.tail[start]) {
		start++
	}
	s.tail = append([]byte(nil), s.tail[start:]...)
}

func (s *shellCaptureState) isTruncatedLocked() bool {
	totalLines := s.completedLines
	if s.hasOpenLine {
		totalLines++
	}
	return s.totalBytes > DefaultMaxBytes || totalLines > DefaultMaxLines
}

func (s *shellCaptureState) progress() ShellCaptureProgress {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.progressLocked()
}

func (s *shellCaptureState) progressLocked() ShellCaptureProgress {
	tailOutput := string(s.tail)
	truncation := TruncateTail(tailOutput, TruncationOptions{})
	totalLines := s.completedLines
	if s.hasOpenLine {
		totalLines++
	}
	truncated := s.totalBytes > DefaultMaxBytes || totalLines > DefaultMaxLines
	truncation.TotalLines = totalLines
	truncation.TotalBytes = s.totalBytes
	truncation.Truncated = truncated
	if truncated && truncation.TruncatedBy == TruncatedByNothing {
		if s.totalBytes > DefaultMaxBytes {
			truncation.TruncatedBy = TruncatedByBytes
		} else {
			truncation.TruncatedBy = TruncatedByLines
		}
	}
	output := tailOutput
	if truncated {
		output = truncation.Content
	}
	return ShellCaptureProgress{
		Output:         output,
		Truncation:     truncation,
		FullOutputPath: s.fullOutputPath,
		LastLineBytes:  s.currentLineBytes,
	}
}

func (s *shellCaptureState) stopAccepting() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accepting = false
}

func (s *shellCaptureState) totalOutputBytes() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.totalBytes
}

func (s *shellCaptureState) finalize() (ShellCaptureProgress, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return ShellCaptureProgress{}, s.err
	}
	if s.isTruncatedLocked() && s.fullOutputPath == "" {
		if err := s.startFullOutputLocked(); err != nil {
			return ShellCaptureProgress{}, err
		}
	}
	return s.progressLocked(), nil
}

func SanitizeBinaryOutput(text string) string {
	text = strings.ToValidUTF8(text, "")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "")
	return strings.Map(func(r rune) rune {
		switch r {
		case '\t', '\n':
			return r
		}
		if r <= 0x1f || r >= 0xfff9 && r <= 0xfffb {
			return -1
		}
		return r
	}, text)
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
