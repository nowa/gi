package harness

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	FileKindFile      = "file"
	FileKindDirectory = "directory"
	FileKindSymlink   = "symlink"

	FileErrorAborted          = "aborted"
	FileErrorNotFound         = "not_found"
	FileErrorPermissionDenied = "permission_denied"
	FileErrorNotDirectory     = "not_directory"
	FileErrorIsDirectory      = "is_directory"
	FileErrorInvalid          = "invalid"
	FileErrorUnknown          = "unknown"

	ExecutionErrorAborted          = "aborted"
	ExecutionErrorTimeout          = "timeout"
	ExecutionErrorShellUnavailable = "shell_unavailable"
	ExecutionErrorSpawnError       = "spawn_error"
	ExecutionErrorCallbackError    = "callback_error"
	ExecutionErrorUnknown          = "unknown"
)

type FileInfo struct {
	Name    string
	Path    string
	Kind    string
	Size    int64
	ModTime time.Time
}

type FileError struct {
	Code string
	Path string
	Err  error
}

func (e *FileError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *FileError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type ExecutionError struct {
	Code string
	Err  error
}

func (e *ExecutionError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *ExecutionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type ExecOptions struct {
	CWD      string
	Env      map[string]string
	Timeout  time.Duration
	OnStdout func(string) error
	OnStderr func(string) error
}

type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type CreateDirOptions struct {
	Recursive bool
}

type RemoveOptions struct {
	Recursive bool
	Force     bool
}

type TempFileOptions struct {
	Prefix string
	Suffix string
}

type FileSystem interface {
	CWD() string
	AbsolutePath(path string) string
	JoinPath(parts ...string) string
	ReadTextFile(ctx context.Context, path string) (string, error)
	ReadTextLines(ctx context.Context, path string, maxLines int) ([]string, error)
	ReadBinaryFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, content []byte) error
	AppendFile(ctx context.Context, path string, content []byte) error
	FileInfo(ctx context.Context, path string) (FileInfo, error)
	ListDir(ctx context.Context, path string) ([]FileInfo, error)
	CanonicalPath(ctx context.Context, path string) (string, error)
	Exists(ctx context.Context, path string) (bool, error)
	CreateDir(ctx context.Context, path string, options CreateDirOptions) error
	Remove(ctx context.Context, path string, options RemoveOptions) error
	CreateTempDir(ctx context.Context, prefix string) (string, error)
	CreateTempFile(ctx context.Context, options TempFileOptions) (string, error)
	Cleanup(ctx context.Context) error
}

type Shell interface {
	Exec(ctx context.Context, command string, options ExecOptions) (ExecResult, error)
	Cleanup(ctx context.Context) error
}

type ExecutionEnv interface {
	FileSystem
	Shell
}

type LocalExecutionEnv struct {
	cwd       string
	shellPath string
}

type LocalExecutionOption func(*LocalExecutionEnv)

func WithShellPath(path string) LocalExecutionOption {
	return func(env *LocalExecutionEnv) {
		env.shellPath = path
	}
}

func NewLocalExecutionEnv(cwd string, options ...LocalExecutionOption) (*LocalExecutionEnv, error) {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return nil, err
	}
	env := &LocalExecutionEnv{cwd: filepath.Clean(abs), shellPath: "/bin/sh"}
	for _, option := range options {
		option(env)
	}
	return env, nil
}

func MustLocalExecutionEnv(cwd string, options ...LocalExecutionOption) *LocalExecutionEnv {
	env, err := NewLocalExecutionEnv(cwd, options...)
	if err != nil {
		panic(err)
	}
	return env
}

func (e *LocalExecutionEnv) CWD() string { return e.cwd }

func (e *LocalExecutionEnv) AbsolutePath(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(e.cwd, path)
}

func (e *LocalExecutionEnv) JoinPath(parts ...string) string {
	return filepath.Join(parts...)
}

func (e *LocalExecutionEnv) ReadTextFile(ctx context.Context, path string) (string, error) {
	data, err := e.ReadBinaryFile(ctx, path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (e *LocalExecutionEnv) ReadTextLines(ctx context.Context, path string, maxLines int) ([]string, error) {
	if err := checkContext(ctx, ""); err != nil {
		return nil, err
	}
	file, err := os.Open(e.AbsolutePath(path))
	if err != nil {
		return nil, fileErrorFrom(path, e.AbsolutePath(path), err)
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if maxLines > 0 && len(lines) >= maxLines {
			return lines, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fileErrorFrom(path, e.AbsolutePath(path), err)
	}
	return lines, nil
}

func (e *LocalExecutionEnv) ReadBinaryFile(ctx context.Context, path string) ([]byte, error) {
	if err := checkContext(ctx, e.AbsolutePath(path)); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(e.AbsolutePath(path))
	if err != nil {
		return nil, fileErrorFrom(path, e.AbsolutePath(path), err)
	}
	return data, nil
}

func (e *LocalExecutionEnv) WriteFile(ctx context.Context, path string, content []byte) error {
	abs := e.AbsolutePath(path)
	if err := checkContext(ctx, abs); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return fileErrorFrom(path, abs, err)
	}
	if err := os.WriteFile(abs, content, 0o644); err != nil {
		return fileErrorFrom(path, abs, err)
	}
	return nil
}

func (e *LocalExecutionEnv) AppendFile(ctx context.Context, path string, content []byte) error {
	abs := e.AbsolutePath(path)
	if err := checkContext(ctx, abs); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return fileErrorFrom(path, abs, err)
	}
	file, err := os.OpenFile(abs, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fileErrorFrom(path, abs, err)
	}
	defer file.Close()
	if _, err := file.Write(content); err != nil {
		return fileErrorFrom(path, abs, err)
	}
	return nil
}

func (e *LocalExecutionEnv) FileInfo(ctx context.Context, path string) (FileInfo, error) {
	abs := e.AbsolutePath(path)
	if err := checkContext(ctx, abs); err != nil {
		return FileInfo{}, err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return FileInfo{}, fileErrorFrom(path, abs, err)
	}
	return fileInfoFrom(abs, info), nil
}

func (e *LocalExecutionEnv) ListDir(ctx context.Context, path string) ([]FileInfo, error) {
	abs := e.AbsolutePath(path)
	if err := checkContext(ctx, abs); err != nil {
		return nil, err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return nil, fileErrorFrom(path, abs, err)
	}
	if !info.IsDir() {
		return nil, &FileError{Code: FileErrorNotDirectory, Path: abs, Err: fmt.Errorf("%s is not a directory", abs)}
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, fileErrorFrom(path, abs, err)
	}
	result := make([]FileInfo, 0, len(entries))
	for _, entry := range entries {
		childPath := filepath.Join(abs, entry.Name())
		childInfo, err := os.Lstat(childPath)
		if err != nil {
			return nil, fileErrorFrom(entry.Name(), childPath, err)
		}
		result = append(result, fileInfoFrom(childPath, childInfo))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (e *LocalExecutionEnv) CanonicalPath(ctx context.Context, path string) (string, error) {
	abs := e.AbsolutePath(path)
	if err := checkContext(ctx, abs); err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fileErrorFrom(path, abs, err)
	}
	return resolved, nil
}

func (e *LocalExecutionEnv) Exists(ctx context.Context, path string) (bool, error) {
	abs := e.AbsolutePath(path)
	if err := checkContext(ctx, abs); err != nil {
		return false, err
	}
	_, err := os.Lstat(abs)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fileErrorFrom(path, abs, err)
}

func (e *LocalExecutionEnv) CreateDir(ctx context.Context, path string, options CreateDirOptions) error {
	abs := e.AbsolutePath(path)
	if err := checkContext(ctx, abs); err != nil {
		return err
	}
	if options.Recursive {
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return fileErrorFrom(path, abs, err)
		}
		return nil
	}
	if err := os.Mkdir(abs, 0o755); err != nil {
		return fileErrorFrom(path, abs, err)
	}
	return nil
}

func (e *LocalExecutionEnv) Remove(ctx context.Context, path string, options RemoveOptions) error {
	abs := e.AbsolutePath(path)
	if err := checkContext(ctx, abs); err != nil {
		return err
	}
	var err error
	if options.Recursive {
		err = os.RemoveAll(abs)
	} else {
		err = os.Remove(abs)
	}
	if err != nil {
		if options.Force && errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fileErrorFrom(path, abs, err)
	}
	return nil
}

func (e *LocalExecutionEnv) CreateTempDir(ctx context.Context, prefix string) (string, error) {
	if prefix == "" {
		prefix = "tmp-"
	}
	if err := checkContext(ctx, e.cwd); err != nil {
		return "", err
	}
	path, err := os.MkdirTemp(e.cwd, prefix)
	if err != nil {
		return "", fileErrorFrom(prefix, e.cwd, err)
	}
	return path, nil
}

func (e *LocalExecutionEnv) CreateTempFile(ctx context.Context, options TempFileOptions) (string, error) {
	if err := checkContext(ctx, e.cwd); err != nil {
		return "", err
	}
	pattern := options.Prefix + "*" + options.Suffix
	file, err := os.CreateTemp(e.cwd, pattern)
	if err != nil {
		return "", fileErrorFrom(pattern, e.cwd, err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		return "", fileErrorFrom(pattern, path, err)
	}
	return path, nil
}

func (e *LocalExecutionEnv) Cleanup(context.Context) error { return nil }

func (e *LocalExecutionEnv) Exec(ctx context.Context, command string, options ExecOptions) (ExecResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ExecResult{}, &ExecutionError{Code: ExecutionErrorAborted, Err: err}
	}
	if err := checkShell(e.shellPath); err != nil {
		return ExecResult{}, err
	}
	var runCtx context.Context
	var cancel context.CancelFunc
	timedOut := false
	if options.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, options.Timeout)
	} else {
		runCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	cwd := options.CWD
	if cwd == "" {
		cwd = e.cwd
	} else {
		cwd = e.AbsolutePath(cwd)
	}
	cmd := exec.CommandContext(runCtx, e.shellPath, "-c", command)
	cmd.Dir = cwd
	cmd.Env = os.Environ()
	for key, value := range options.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return ExecResult{}, &ExecutionError{Code: ExecutionErrorSpawnError, Err: err}
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return ExecResult{}, &ExecutionError{Code: ExecutionErrorSpawnError, Err: err}
	}
	if err := cmd.Start(); err != nil {
		return ExecResult{}, &ExecutionError{Code: ExecutionErrorSpawnError, Err: err}
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	callbackErr := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go readExecPipe(stdoutPipe, &stdout, options.OnStdout, callbackErr, cancel, &wg)
	go readExecPipe(stderrPipe, &stderr, options.OnStderr, callbackErr, cancel, &wg)

	waitErr := cmd.Wait()
	wg.Wait()
	close(callbackErr)
	for err := range callbackErr {
		if err != nil {
			_ = cmd.Process.Kill()
			return ExecResult{}, &ExecutionError{Code: ExecutionErrorCallbackError, Err: err}
		}
	}
	if runCtx.Err() != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
			timedOut = true
		}
		if timedOut {
			return ExecResult{}, &ExecutionError{Code: ExecutionErrorTimeout, Err: runCtx.Err()}
		}
		return ExecResult{}, &ExecutionError{Code: ExecutionErrorAborted, Err: runCtx.Err()}
	}

	result := ExecResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		return ExecResult{}, &ExecutionError{Code: ExecutionErrorSpawnError, Err: waitErr}
	}
	return result, nil
}

type CapturedShellResult struct {
	Output         string
	FullOutputPath string
	Truncated      bool
	ExecResult     ExecResult
}

func ExecuteShellWithCapture(ctx context.Context, env *LocalExecutionEnv, command string, maxInlineBytes int) (CapturedShellResult, error) {
	if maxInlineBytes <= 0 {
		maxInlineBytes = 64 * 1024
	}
	var full bytes.Buffer
	result, err := env.Exec(ctx, command, ExecOptions{
		OnStdout: func(chunk string) error {
			full.WriteString(chunk)
			return nil
		},
		OnStderr: func(chunk string) error {
			full.WriteString(chunk)
			return nil
		},
	})
	if err != nil {
		return CapturedShellResult{}, err
	}
	output := full.String()
	if output == "" {
		output = result.Stdout + result.Stderr
	}
	captured := CapturedShellResult{Output: output, ExecResult: result}
	if len(output) <= maxInlineBytes {
		return captured, nil
	}
	path, err := env.CreateTempFile(ctx, TempFileOptions{Prefix: "shell-output-", Suffix: ".txt"})
	if err != nil {
		return CapturedShellResult{}, err
	}
	if err := env.WriteFile(ctx, path, []byte(output)); err != nil {
		return CapturedShellResult{}, err
	}
	captured.Output = output[:maxInlineBytes]
	captured.FullOutputPath = path
	captured.Truncated = true
	return captured, nil
}

func readExecPipe(pipe io.Reader, buffer *bytes.Buffer, callback func(string) error, callbackErr chan<- error, cancel context.CancelFunc, wg *sync.WaitGroup) {
	defer wg.Done()
	chunk := make([]byte, 8192)
	for {
		n, err := pipe.Read(chunk)
		if n > 0 {
			text := string(chunk[:n])
			buffer.WriteString(text)
			if callback != nil {
				if callbackErrValue := callback(text); callbackErrValue != nil {
					callbackErr <- callbackErrValue
					cancel()
					io.Copy(io.Discard, pipe)
					return
				}
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) {
				callbackErr <- err
			}
			return
		}
	}
}

func checkShell(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &ExecutionError{Code: ExecutionErrorShellUnavailable, Err: err}
		}
		return &ExecutionError{Code: ExecutionErrorSpawnError, Err: err}
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return &ExecutionError{Code: ExecutionErrorSpawnError, Err: fmt.Errorf("%s is not executable", path)}
	}
	return nil
}

func checkContext(ctx context.Context, path string) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return &FileError{Code: FileErrorAborted, Path: path, Err: err}
	}
	return nil
}

func fileInfoFrom(path string, info os.FileInfo) FileInfo {
	kind := FileKindFile
	if info.Mode()&os.ModeSymlink != 0 {
		kind = FileKindSymlink
	} else if info.IsDir() {
		kind = FileKindDirectory
	}
	return FileInfo{Name: filepath.Base(path), Path: path, Kind: kind, Size: info.Size(), ModTime: info.ModTime()}
}

func fileErrorFrom(inputPath, absolutePath string, err error) error {
	if err == nil {
		return nil
	}
	code := FileErrorUnknown
	if errors.Is(err, os.ErrNotExist) {
		code = FileErrorNotFound
	} else if errors.Is(err, os.ErrPermission) {
		code = FileErrorPermissionDenied
	} else if errors.Is(err, os.ErrInvalid) {
		code = FileErrorInvalid
	}
	if absolutePath == "" {
		absolutePath = inputPath
	}
	return &FileError{Code: code, Path: absolutePath, Err: err}
}
