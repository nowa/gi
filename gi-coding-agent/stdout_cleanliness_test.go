package gicodingagent

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestRunCLIJSONHelpKeepsStdoutClean(t *testing.T) {
	stdout, stderr, code := runCLIWithStartupChatter([]string{"--mode", "json", "--help"})

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	assertCleanHelpOutput(t, stdout, stderr)
}

func TestRunCLIPrintHelpKeepsStdoutClean(t *testing.T) {
	stdout, stderr, code := runCLIWithStartupChatter([]string{"-p", "--help"})

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	assertCleanHelpOutput(t, stdout, stderr)
}

func runCLIWithStartupChatter(args []string) (string, string, int) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := RunCLI(CLIOptions{
		Args:   args,
		Stdout: &stdout,
		Stderr: &stderr,
		Startup: func(writer io.Writer) error {
			_, _ = fmt.Fprintln(writer, "changed 1 package in 471ms")
			_, _ = fmt.Fprintln(writer, "found 0 vulnerabilities")
			return nil
		},
	})
	return stdout.String(), stderr.String(), code
}

func assertCleanHelpOutput(t *testing.T, stdout, stderr string) {
	t.Helper()
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	for _, expected := range []string{"changed 1 package in 471ms", "found 0 vulnerabilities", "Usage:"} {
		if !strings.Contains(stderr, expected) {
			t.Fatalf("stderr = %q, want %q", stderr, expected)
		}
	}
}
