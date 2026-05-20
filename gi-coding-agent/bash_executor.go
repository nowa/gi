package gicodingagent

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const defaultBashExitStdioGrace = 100 * time.Millisecond

type BashExecutorOptions struct {
	Context        context.Context
	OnChunk        func(string)
	ExitStdioGrace time.Duration
}

type BashResult struct {
	Output    string
	ExitCode  int
	Cancelled bool
}

func ExecuteBash(command, cwd string, options ...BashExecutorOptions) (BashResult, error) {
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

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = cwd
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return BashResult{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return BashResult{}, err
	}
	if err := cmd.Start(); err != nil {
		return BashResult{}, err
	}

	var mu sync.Mutex
	var output strings.Builder
	readDone := make(chan struct{}, 2)
	readPipe := func(pipe io.ReadCloser) {
		defer func() { readDone <- struct{}{} }()
		buffer := make([]byte, 4096)
		for {
			n, err := pipe.Read(buffer)
			if n > 0 {
				chunk := strings.ReplaceAll(StripAnsi(string(buffer[:n])), "\r", "")
				mu.Lock()
				output.WriteString(chunk)
				mu.Unlock()
				if opts.OnChunk != nil {
					opts.OnChunk(chunk)
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

	mu.Lock()
	text := output.String()
	mu.Unlock()
	result := BashResult{Output: text, ExitCode: 0, Cancelled: errors.Is(ctx.Err(), context.Canceled)}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	if result.Cancelled {
		return result, nil
	}
	return result, waitErr
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
