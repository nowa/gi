package tools

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	harnessenv "github.com/nowa/gi/gi-agent-core/harness/env"
)

func TestPathExistsAndResolveReadToolPathShareEnvironmentBoundary(t *testing.T) {
	env := harnessenv.MustLocalExecutionEnv(t.TempDir())
	ctx := context.Background()
	name := "Screenshot 2024-01-01 at 10.00.00\u202fAM.png"
	if err := os.WriteFile(env.AbsolutePath(name), []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}

	exists, err := PathExists(ctx, env, name)
	if err != nil || !exists {
		t.Fatalf("existing path = %v, %v", exists, err)
	}
	exists, err = PathExists(ctx, env, "missing.txt")
	if err != nil || exists {
		t.Fatalf("missing path = %v, %v", exists, err)
	}

	resolved, err := ResolveReadToolPath(
		ctx,
		env,
		"Screenshot 2024-01-01 at 10.00.00 AM.png",
	)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != filepath.Join(env.CWD(), name) {
		t.Fatalf("resolved path = %q", resolved)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := PathExists(cancelled, env, name); err == nil {
		t.Fatal("cancelled path lookup returned nil error")
	}
	if _, err := PathExists(ctx, nil, name); err == nil {
		t.Fatal("nil environment path lookup returned nil error")
	}
}

func TestIsMissingPathErrorClassifiesPortableFileErrors(t *testing.T) {
	for _, err := range []error{
		fs.ErrNotExist,
		&os.PathError{Op: "realpath", Path: "missing", Err: syscall.ENOTDIR},
		&harnessenv.FileError{Code: harnessenv.FileErrorNotFound, Err: fs.ErrNotExist},
		&harnessenv.FileError{Code: harnessenv.FileErrorNotDirectory, Err: syscall.ENOTDIR},
		&harnessenv.FileError{Code: harnessenv.FileErrorUnknown, Err: syscall.ENOTDIR},
	} {
		if !isMissingPathError(err) {
			t.Fatalf("missing path error not recognized: %#v", err)
		}
	}
	if isMissingPathError(nil) ||
		isMissingPathError(fs.ErrPermission) ||
		isMissingPathError(&harnessenv.FileError{
			Code: harnessenv.FileErrorPermissionDenied,
			Err:  fs.ErrPermission,
		}) {
		t.Fatal("non-missing error classified as missing")
	}
	if !errors.Is(
		&harnessenv.FileError{Code: harnessenv.FileErrorNotFound, Err: fs.ErrNotExist},
		fs.ErrNotExist,
	) {
		t.Fatal("file error no longer unwraps its cause")
	}
}
