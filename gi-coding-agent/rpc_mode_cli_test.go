package gicodingagent

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCLIRPCModeProcessesJSONLCommands(t *testing.T) {
	tempDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := RunCLI(CLIOptions{
		Args:     []string{"--offline", "--no-session", "--model", "openai/gpt-4o-mini", "--mode", "rpc"},
		Stdin:    strings.NewReader(`{"id":"state-1","type":"get_state"}` + "\n"),
		Stdout:   &stdout,
		Stderr:   &stderr,
		CWD:      tempDir,
		AgentDir: filepath.Join(tempDir, "agent"),
	})
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q", stderr.String())
	}
	var response RPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &response); err != nil {
		t.Fatalf("stdout = %q err=%v", stdout.String(), err)
	}
	if response.ID != "state-1" || response.Command != RPCCommandGetState || !response.Success || len(response.Data) == 0 {
		t.Fatalf("response = %#v", response)
	}
}

func TestRunCLIRPCModeRejectsFileArguments(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := RunCLI(CLIOptions{
		Args:   []string{"--mode", "rpc", "@prompt.md"},
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if code != 1 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if stdout.String() != "" || !strings.Contains(stderr.String(), "@file arguments are not supported in RPC mode") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}
