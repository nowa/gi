package gicodingagent

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestBashToolPiFullOutputForTruncatedErrors(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name    string
		err     error
		message string
	}{
		{name: "timeout", err: errors.New("timeout:5"), message: "Command timed out after 5 seconds"},
		{name: "aborted", err: errors.New("aborted"), message: "Command aborted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bash := NewBashTool(dir, BashToolOptions{Operations: BashOperations{
				Exec: func(_ string, _ string, options BashExecOptions) (BashOperationResult, error) {
					for i := 1; i <= 3000; i++ {
						options.OnData([]byte(fmt.Sprintf("%d\n", i)))
					}
					return BashOperationResult{}, tc.err
				},
			}})

			_, err := bash.Execute("test-call-"+tc.name, BashToolInput{Command: "chatty-fail"})
			if err == nil {
				t.Fatal("expected error")
			}
			message := err.Error()
			if !strings.Contains(message, tc.message) {
				t.Fatalf("error = %q, want %q", message, tc.message)
			}
			fullOutputPath := extractBashFullOutputPath(t, message)
			fullOutput := readBashFullOutput(t, fullOutputPath)
			if !strings.Contains(fullOutput, "1\n2\n3") || !strings.Contains(fullOutput, "2998\n2999\n3000") {
				t.Fatalf("full output = %q", fullOutput)
			}
		})
	}
}

func TestBashToolPiAdvancedOperations(t *testing.T) {
	dir := t.TempDir()

	spawnErrTool := NewBashTool(dir, BashToolOptions{Operations: BashOperations{
		Exec: func(_ string, _ string, _ BashExecOptions) (BashOperationResult, error) {
			return BashOperationResult{}, errors.New("ENOENT: spawn failed")
		},
	}})
	if _, err := spawnErrTool.Execute("test-call-12", BashToolInput{Command: "echo test"}); err == nil || !strings.Contains(err.Error(), "ENOENT") {
		t.Fatalf("spawn err = %v", err)
	}

	customShellWithInjectedOps := NewBashTool(dir, BashToolOptions{
		ShellPath: "/custom/bash",
		Operations: BashOperations{
			Exec: func(_ string, _ string, options BashExecOptions) (BashOperationResult, error) {
				options.OnData([]byte("ok\n"))
				return BashOperationResult{ExitCode: 0}, nil
			},
		},
	})
	if result, err := customShellWithInjectedOps.Execute("test-call-12b", BashToolInput{Command: "echo test"}); err != nil || strings.TrimSpace(readToolText(result)) != "ok" {
		t.Fatalf("custom shell injected result = %#v, err = %v", result, err)
	}

	ops := CreateLocalBashOperations(BashLocalOperationsOptions{ShellPath: "/custom/bash"})
	if _, err := ops.Exec("echo test", dir, BashExecOptions{OnData: func([]byte) {}}); err == nil || !strings.Contains(err.Error(), "Custom shell path not found: /custom/bash") {
		t.Fatalf("custom shell err = %v", err)
	}
}

func TestBashToolPiStreamingAndUTF8(t *testing.T) {
	dir := t.TempDir()
	chatty := NewBashTool(dir, BashToolOptions{Operations: BashOperations{
		Exec: func(_ string, _ string, options BashExecOptions) (BashOperationResult, error) {
			for i := 0; i < 5000; i++ {
				options.OnData([]byte(fmt.Sprintf("line %d\n", i)))
			}
			return BashOperationResult{ExitCode: 0}, nil
		},
	}})
	var updates []FileToolResult
	result, err := chatty.ExecuteWithUpdates("test-call-chatty-updates", BashToolInput{Command: "chatty"}, func(update FileToolResult) {
		updates = append(updates, update)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) >= 25 {
		t.Fatalf("updates = %d, want fewer than 25", len(updates))
	}
	if !strings.Contains(readToolText(result), "line 4999") {
		t.Fatalf("chatty output = %q", readToolText(result))
	}

	euro := []byte("€\n")
	splitUTF8 := NewBashTool(dir, BashToolOptions{Operations: BashOperations{
		Exec: func(_ string, _ string, options BashExecOptions) (BashOperationResult, error) {
			options.OnData(euro[:1])
			options.OnData(euro[1:])
			return BashOperationResult{ExitCode: 0}, nil
		},
	}})
	result, err = splitUTF8.Execute("test-call-split-utf8", BashToolInput{Command: "split-utf8"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(readToolText(result)) != "€" {
		t.Fatalf("split utf8 output = %q", readToolText(result))
	}
}

func TestLocalBashOperationsPiReuseAndSanitization(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test commands use POSIX shell syntax")
	}
	dir := t.TempDir()
	ops := CreateLocalBashOperations()
	var chunks bytes.Buffer
	result, err := ops.Exec("echo $TEST_LOCAL_BASH_OPS", dir, BashExecOptions{
		OnData: func(data []byte) {
			chunks.Write(data)
		},
		Env: map[string]string{"TEST_LOCAL_BASH_OPS": "from-local-ops"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || strings.TrimSpace(chunks.String()) != "from-local-ops" {
		t.Fatalf("local ops result = %#v, output = %q", result, chunks.String())
	}

	bashResult, err := ExecuteBashWithOperations("printf '\\033[31mred\\033[0m\\r\\n'", dir, ops)
	if err != nil {
		t.Fatal(err)
	}
	if bashResult.ExitCode != 0 || bashResult.Output != "red\n" {
		t.Fatalf("sanitized result = %#v", bashResult)
	}
}

func TestBashToolPiLineTruncationPersistsFullOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test command uses POSIX seq")
	}
	dir := t.TempDir()
	bash := NewBashTool(dir)
	result, err := bash.Execute("test-call-line-truncation", BashToolInput{Command: "seq 3000"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Details == nil || result.Details.Truncation == nil || !result.Details.Truncation.Truncated || result.Details.Truncation.TruncatedBy != "lines" {
		t.Fatalf("truncation details = %#v", result.Details)
	}
	fullOutputPath := result.Details.FullOutputPath
	if fullOutputPath == "" {
		t.Fatalf("missing full output path in %#v", result.Details)
	}
	output := readToolText(result)
	if !regexp.MustCompile(`\[Showing lines \d+-\d+ of \d+\. Full output: `).MatchString(output) {
		t.Fatalf("truncated output = %q", output)
	}
	fullOutput := readBashFullOutput(t, fullOutputPath)
	if !strings.Contains(fullOutput, "1\n2\n3") || !strings.Contains(fullOutput, "2998\n2999\n3000") {
		t.Fatalf("full output = %q", fullOutput)
	}

	bashResult, err := ExecuteBashWithOperations("seq 3000", dir, CreateLocalBashOperations())
	if err != nil {
		t.Fatal(err)
	}
	if !bashResult.Truncated || bashResult.FullOutputPath == "" {
		t.Fatalf("bash result = %#v", bashResult)
	}
	fullOutput = readBashFullOutput(t, bashResult.FullOutputPath)
	if !strings.Contains(fullOutput, "1\n2\n3") || !strings.Contains(fullOutput, "2998\n2999\n3000") {
		t.Fatalf("execute full output = %q", fullOutput)
	}
}

func extractBashFullOutputPath(t *testing.T, message string) string {
	t.Helper()
	re := regexp.MustCompile(`\[Showing lines \d+-\d+ of \d+\. Full output: ([^\]\n]+)\]`)
	matches := re.FindStringSubmatch(message)
	if len(matches) != 2 {
		t.Fatalf("message missing full output path: %q", message)
	}
	if strings.Contains(matches[1], "undefined") {
		t.Fatalf("full output path = %q", matches[1])
	}
	return matches[1]
}

func readBashFullOutput(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
