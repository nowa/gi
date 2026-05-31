package harness

import (
	"context"

	harnessenv "github.com/nowa/gi/gi-agent-core/harness/env"
)

const (
	FileKindFile      = harnessenv.FileKindFile
	FileKindDirectory = harnessenv.FileKindDirectory
	FileKindSymlink   = harnessenv.FileKindSymlink

	FileErrorAborted          = harnessenv.FileErrorAborted
	FileErrorNotFound         = harnessenv.FileErrorNotFound
	FileErrorPermissionDenied = harnessenv.FileErrorPermissionDenied
	FileErrorNotDirectory     = harnessenv.FileErrorNotDirectory
	FileErrorIsDirectory      = harnessenv.FileErrorIsDirectory
	FileErrorInvalid          = harnessenv.FileErrorInvalid
	FileErrorNotSupported     = harnessenv.FileErrorNotSupported
	FileErrorUnknown          = harnessenv.FileErrorUnknown

	ExecutionErrorAborted          = harnessenv.ExecutionErrorAborted
	ExecutionErrorTimeout          = harnessenv.ExecutionErrorTimeout
	ExecutionErrorShellUnavailable = harnessenv.ExecutionErrorShellUnavailable
	ExecutionErrorSpawnError       = harnessenv.ExecutionErrorSpawnError
	ExecutionErrorCallbackError    = harnessenv.ExecutionErrorCallbackError
	ExecutionErrorUnknown          = harnessenv.ExecutionErrorUnknown
)

type FileInfo = harnessenv.FileInfo
type FileError = harnessenv.FileError
type ExecutionError = harnessenv.ExecutionError

type ExecOptions = harnessenv.ExecOptions
type ExecResult = harnessenv.ExecResult
type CreateDirOptions = harnessenv.CreateDirOptions
type RemoveOptions = harnessenv.RemoveOptions
type TempFileOptions = harnessenv.TempFileOptions

type FileSystem = harnessenv.FileSystem
type Shell = harnessenv.Shell
type ExecutionEnv = harnessenv.ExecutionEnv

type LocalExecutionEnv = harnessenv.LocalExecutionEnv
type LocalExecutionOption = harnessenv.LocalExecutionOption

type CapturedShellResult = harnessenv.CapturedShellResult

func WithShellPath(path string) LocalExecutionOption {
	return harnessenv.WithShellPath(path)
}

func NewLocalExecutionEnv(cwd string, options ...LocalExecutionOption) (*LocalExecutionEnv, error) {
	return harnessenv.NewLocalExecutionEnv(cwd, options...)
}

func MustLocalExecutionEnv(cwd string, options ...LocalExecutionOption) *LocalExecutionEnv {
	return harnessenv.MustLocalExecutionEnv(cwd, options...)
}

func SanitizeBinaryOutput(str string) string {
	return harnessenv.SanitizeBinaryOutput(str)
}

func ExecuteShellWithCapture(ctx context.Context, env *LocalExecutionEnv, command string, maxInlineBytes int) (CapturedShellResult, error) {
	return harnessenv.ExecuteShellWithCapture(ctx, env, command, maxInlineBytes)
}
