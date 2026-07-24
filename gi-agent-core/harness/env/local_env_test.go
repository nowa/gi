package env

import (
	"bytes"
	"errors"
	"os/exec"
	"reflect"
	"testing"
	"time"
)

func TestUsesStdinCommandTransportForLegacyWSLBashPaths(t *testing.T) {
	for _, path := range []string{
		`C:\Windows\System32\bash.exe`,
		`c:/WINDOWS/sysnative/bash.exe`,
	} {
		config := getBashShellConfig(path)
		if config.shell != path || !config.commandStdin || !reflect.DeepEqual(config.args, []string{"-s"}) {
			t.Fatalf("legacy WSL config for %q = %#v", path, config)
		}
	}
	config := getBashShellConfig(`/bin/bash`)
	if config.commandStdin || !reflect.DeepEqual(config.args, []string{"-c"}) {
		t.Fatalf("regular bash config = %#v", config)
	}
}

func TestResolveTimeout(t *testing.T) {
	if timeout, err := resolveTimeout(0); err != nil || timeout != 0 {
		t.Fatalf("unset timeout = %s, %v", timeout, err)
	}
	if _, err := resolveTimeout(-time.Second); err == nil {
		t.Fatal("negative timeout error = nil")
	} else {
		var executionErr *ExecutionError
		if !errors.As(err, &executionErr) || executionErr.Code != ExecutionErrorTimeout {
			t.Fatalf("negative timeout error = %#v", err)
		}
	}
	if _, err := resolveTimeout(maxExecutionTimeout + time.Millisecond); err == nil {
		t.Fatal("oversized timeout error = nil")
	}
}

func TestWaitForChildProcessBoundsInheritedStdio(t *testing.T) {
	command := exec.Command("/bin/sh", "-c", "sleep 0.5 & printf done")
	command.WaitDelay = exitStdioGrace
	var stdout bytes.Buffer
	command.Stdout = &stdout
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	startedAt := time.Now()
	if err := waitForChildProcess(command); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(startedAt); elapsed >= 400*time.Millisecond {
		t.Fatalf("wait elapsed = %s, want bounded stdio grace", elapsed)
	}
	if stdout.String() != "done" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}
