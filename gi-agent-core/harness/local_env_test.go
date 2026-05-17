package harness

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLocalExecutionEnvFiles(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	env := MustLocalExecutionEnv(root)

	if got := env.AbsolutePath("nested/child"); got != filepath.Join(root, "nested/child") {
		t.Fatalf("absolute path = %s", got)
	}
	if got := env.JoinPath(root, "nested", "child"); got != filepath.Join(root, "nested", "child") {
		t.Fatalf("join path = %s", got)
	}
	if err := env.CreateDir(ctx, "nested/child", CreateDirOptions{Recursive: true}); err != nil {
		t.Fatal(err)
	}
	if err := env.WriteFile(ctx, "nested/child/file.txt", []byte("hel")); err != nil {
		t.Fatal(err)
	}
	if err := env.AppendFile(ctx, "nested/child/file.txt", []byte("lo")); err != nil {
		t.Fatal(err)
	}
	text, err := env.ReadTextFile(ctx, "nested/child/file.txt")
	if err != nil || text != "hello" {
		t.Fatalf("read text = %q err=%v", text, err)
	}
	lines, err := env.ReadTextLines(ctx, "nested/child/file.txt", 1)
	if err != nil || !reflect.DeepEqual(lines, []string{"hello"}) {
		t.Fatalf("read lines = %#v err=%v", lines, err)
	}
	binary, err := env.ReadBinaryFile(ctx, "nested/child/file.txt")
	if err != nil || string(binary) != "hello" {
		t.Fatalf("read binary = %q err=%v", string(binary), err)
	}
	entries, err := env.ListDir(ctx, "nested/child")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "file.txt" || entries[0].Kind != FileKindFile || entries[0].Size != 5 {
		t.Fatalf("entries = %#v", entries)
	}
	exists, err := env.Exists(ctx, "nested/child/file.txt")
	if err != nil || !exists {
		t.Fatalf("exists = %v err=%v", exists, err)
	}
	if err := env.Remove(ctx, "nested/child/file.txt", RemoveOptions{}); err != nil {
		t.Fatal(err)
	}
	exists, err = env.Exists(ctx, "nested/child/file.txt")
	if err != nil || exists {
		t.Fatalf("exists after remove = %v err=%v", exists, err)
	}
}

func TestLocalExecutionEnvSymlinksAndErrors(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	env := MustLocalExecutionEnv(root)
	if err := env.WriteFile(ctx, "target.txt", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "target.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	info, err := env.FileInfo(ctx, "link.txt")
	if err != nil {
		t.Fatal(err)
	}
	if info.Kind != FileKindSymlink {
		t.Fatalf("link kind = %s", info.Kind)
	}
	canonical, err := env.CanonicalPath(ctx, "link.txt")
	if err != nil {
		t.Fatal(err)
	}
	targetPath, err := filepath.EvalSymlinks(filepath.Join(root, "target.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if canonical != targetPath {
		t.Fatalf("canonical = %s", canonical)
	}
	entries, err := env.ListDir(ctx, ".")
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]string{}
	for _, entry := range entries {
		kinds[entry.Name] = entry.Kind
	}
	if !reflect.DeepEqual(kinds, map[string]string{"link.txt": FileKindSymlink, "target.txt": FileKindFile}) {
		t.Fatalf("kinds = %#v", kinds)
	}

	_, err = env.FileInfo(ctx, "missing.txt")
	var fileErr *FileError
	if !errors.As(err, &fileErr) || fileErr.Code != FileErrorNotFound {
		t.Fatalf("missing error = %#v", err)
	}
	if _, err := env.ListDir(ctx, "target.txt"); !errors.As(err, &fileErr) || fileErr.Code != FileErrorNotDirectory {
		t.Fatalf("list file error = %#v", err)
	}
	if err := env.Remove(ctx, "missing", RemoveOptions{Force: true}); err != nil {
		t.Fatalf("force remove missing = %v", err)
	}
}

func TestLocalExecutionEnvPreAbortedFileOperations(t *testing.T) {
	root := t.TempDir()
	env := MustLocalExecutionEnv(root)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := env.WriteFile(ctx, "file.txt", []byte("hello"))
	var fileErr *FileError
	if !errors.As(err, &fileErr) || fileErr.Code != FileErrorAborted {
		t.Fatalf("write aborted err = %#v", err)
	}
	if _, err := env.ListDir(ctx, "."); !errors.As(err, &fileErr) || fileErr.Code != FileErrorAborted {
		t.Fatalf("list aborted err = %#v", err)
	}
}

func TestLocalExecutionEnvExec(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	env := MustLocalExecutionEnv(root)

	result, err := env.Exec(ctx, `printf '%s:%s' "$PWD" "$GI_ENV_TEST"`, ExecOptions{Env: map[string]string{"GI_ENV_TEST": "ok"}})
	if err != nil {
		t.Fatal(err)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != realRoot+":ok" || result.Stderr != "" || result.ExitCode != 0 {
		t.Fatalf("exec result = %#v", result)
	}

	var stdout, stderr strings.Builder
	result, err = env.Exec(ctx, "printf out; printf err >&2", ExecOptions{
		OnStdout: func(chunk string) error {
			stdout.WriteString(chunk)
			return nil
		},
		OnStderr: func(chunk string) error {
			stderr.WriteString(chunk)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "out" || result.Stderr != "err" || stdout.String() != "out" || stderr.String() != "err" {
		t.Fatalf("stream result=%#v stdout=%q stderr=%q", result, stdout.String(), stderr.String())
	}

	result, err = env.Exec(ctx, "exit 7", ExecOptions{})
	if err != nil || result.ExitCode != 7 {
		t.Fatalf("exit result=%#v err=%v", result, err)
	}
}

func TestLocalExecutionEnvExecErrors(t *testing.T) {
	root := t.TempDir()
	env := MustLocalExecutionEnv(root)
	_, err := env.Exec(context.Background(), "sleep 5", ExecOptions{Timeout: 10 * time.Millisecond})
	var execErr *ExecutionError
	if !errors.As(err, &execErr) || execErr.Code != ExecutionErrorTimeout {
		t.Fatalf("timeout err = %#v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = env.Exec(ctx, "sleep 5", ExecOptions{})
	if !errors.As(err, &execErr) || execErr.Code != ExecutionErrorAborted {
		t.Fatalf("aborted err = %#v", err)
	}

	_, err = env.Exec(context.Background(), "printf out", ExecOptions{OnStdout: func(string) error {
		return errors.New("callback failed")
	}})
	if !errors.As(err, &execErr) || execErr.Code != ExecutionErrorCallbackError || err.Error() != "callback failed" {
		t.Fatalf("callback err = %#v", err)
	}

	missingShellEnv := MustLocalExecutionEnv(root, WithShellPath(filepath.Join(root, "missing-shell")))
	_, err = missingShellEnv.Exec(context.Background(), "printf ok", ExecOptions{})
	if !errors.As(err, &execErr) || execErr.Code != ExecutionErrorShellUnavailable {
		t.Fatalf("missing shell err = %#v", err)
	}

	shellPath := filepath.Join(root, "not-executable-shell")
	if err := os.WriteFile(shellPath, []byte("not executable"), 0o644); err != nil {
		t.Fatal(err)
	}
	spawnErrorEnv := MustLocalExecutionEnv(root, WithShellPath(shellPath))
	_, err = spawnErrorEnv.Exec(context.Background(), "printf ok", ExecOptions{})
	if !errors.As(err, &execErr) || execErr.Code != ExecutionErrorSpawnError {
		t.Fatalf("spawn err = %#v", err)
	}
}

func TestExecuteShellWithCapture(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	env := MustLocalExecutionEnv(root)
	result, err := ExecuteShellWithCapture(ctx, env, "yes line | head -n 15000", 1024)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Truncated || result.FullOutputPath == "" || len(result.Output) != 1024 {
		t.Fatalf("capture = %#v", result)
	}
	fullOutput, err := env.ReadTextFile(ctx, result.FullOutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(fullOutput, "\n") < 10000 || len(result.Output) >= len(fullOutput) {
		t.Fatalf("full output len=%d preview len=%d", len(fullOutput), len(result.Output))
	}
}
