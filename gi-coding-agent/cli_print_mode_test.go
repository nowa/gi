package gicodingagent

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestRunCLIPrintModeUsesInjectedHost(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var factoryArgs Args
	host := newFakePrintModeHost(llm.Message{
		Role:       llm.RoleAssistant,
		Content:    []llm.ContentPart{llm.Text("done")},
		StopReason: llm.StopReasonStop,
	})

	code := RunCLI(CLIOptions{
		Args:   []string{"-p", "hello"},
		Stdout: &stdout,
		Stderr: &stderr,
		PrintModeHostFactory: func(args Args) (PrintModeRuntimeHost, error) {
			factoryArgs = args
			return host, nil
		},
	})

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.String() != "done" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if !factoryArgs.Print || len(factoryArgs.Messages) != 1 || factoryArgs.Messages[0] != "hello" {
		t.Fatalf("factory args = %#v", factoryArgs)
	}
	if len(host.session.prompts) != 1 || host.session.prompts[0].message != "hello" {
		t.Fatalf("prompts = %#v", host.session.prompts)
	}
}

func TestRunCLIJSONModeUsesPrintMode(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	host := newFakePrintModeHost(llm.Message{
		Role:       llm.RoleAssistant,
		Content:    []llm.ContentPart{llm.Text("done")},
		StopReason: llm.StopReasonStop,
	})

	code := RunCLI(CLIOptions{
		Args:   []string{"--mode", "json", "hello"},
		Stdout: &stdout,
		Stderr: &stderr,
		PrintModeHostFactory: func(Args) (PrintModeRuntimeHost, error) {
			return host, nil
		},
	})

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"Role":"assistant"`) || !strings.Contains(stdout.String(), `"done"`) {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if len(host.session.prompts) != 1 || host.session.prompts[0].message != "hello" {
		t.Fatalf("prompts = %#v", host.session.prompts)
	}
}

func TestRunCLIPrintModeFactoryError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := RunCLI(CLIOptions{
		Args:   []string{"-p", "hello"},
		Stdout: &stdout,
		Stderr: &stderr,
		PrintModeHostFactory: func(Args) (PrintModeRuntimeHost, error) {
			return nil, errors.New("factory failed")
		},
	})

	if code != 1 {
		t.Fatalf("exit code = %d", code)
	}
	if stdout.String() != "" || stderr.String() != "factory failed\n" {
		t.Fatalf("stdout = %q stderr = %q", stdout.String(), stderr.String())
	}
}

func TestRunCLIDefaultOfflinePrintMode(t *testing.T) {
	tempDir := t.TempDir()
	agentDir := filepath.Join(tempDir, "agent")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := RunCLI(CLIOptions{
		Args:     []string{"--offline", "--no-session", "--model", "openai/gpt-4o-mini", "-p", "hello"},
		Stdout:   &stdout,
		Stderr:   &stderr,
		CWD:      tempDir,
		AgentDir: agentDir,
	})

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.String() != "Response to: hello" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if _, err := os.Stat(agentDir); !os.IsNotExist(err) {
		t.Fatalf("agent dir stat err = %v, want not exist", err)
	}
}
