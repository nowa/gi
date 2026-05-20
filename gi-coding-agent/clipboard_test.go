package gicodingagent

import (
	"errors"
	"strings"
	"testing"
)

type clipboardExecCall struct {
	command string
	options ClipboardTextCommandOptions
}

type clipboardSpawnCall struct {
	command string
	args    []string
	input   string
}

func TestCopyToClipboardLocalNativeSuccessSkipsFallbacks(t *testing.T) {
	var nativeText string
	var execCalls []clipboardExecCall
	var spawnCalls []clipboardSpawnCall
	var stdoutWrites []string
	ops := ClipboardCopyOperations{
		SetText: func(text string) error {
			nativeText = text
			return nil
		},
		ExecCommand: func(command string, options ClipboardTextCommandOptions) error {
			execCalls = append(execCalls, clipboardExecCall{command: command, options: options})
			return nil
		},
		SpawnWithInput: func(command string, args []string, input string) error {
			spawnCalls = append(spawnCalls, clipboardSpawnCall{command: command, args: args, input: input})
			return nil
		},
		WriteStdout: func(text string) error {
			stdoutWrites = append(stdoutWrites, text)
			return nil
		},
	}

	if err := CopyToClipboard("hello", ClipboardCopyOptions{Platform: "darwin", Env: map[string]string{}, Operations: ops}); err != nil {
		t.Fatalf("CopyToClipboard returned error: %v", err)
	}
	if nativeText != "hello" {
		t.Fatalf("native text = %q, want hello", nativeText)
	}
	if len(execCalls) != 0 || len(spawnCalls) != 0 || len(osc52Writes(stdoutWrites)) != 0 {
		t.Fatalf("fallbacks exec=%#v spawn=%#v stdout=%#v", execCalls, spawnCalls, stdoutWrites)
	}
}

func TestCopyToClipboardRemoteNativeSuccessEmitsOSC52AfterNative(t *testing.T) {
	nativeResolved := false
	var stdoutWrites []string
	ops := ClipboardCopyOperations{
		SetText: func(text string) error {
			if text != "hello" {
				t.Fatalf("native text = %q, want hello", text)
			}
			if len(stdoutWrites) != 0 {
				t.Fatal("OSC 52 should not be emitted before native write")
			}
			nativeResolved = true
			return nil
		},
		ExecCommand: func(command string, options ClipboardTextCommandOptions) error {
			t.Fatalf("exec should not be called after native success: %s", command)
			return nil
		},
		WriteStdout: func(text string) error {
			stdoutWrites = append(stdoutWrites, text)
			return nil
		},
	}

	err := CopyToClipboard("hello", ClipboardCopyOptions{
		Platform:   "darwin",
		Env:        map[string]string{"SSH_CONNECTION": "client server"},
		Operations: ops,
	})
	if err != nil {
		t.Fatalf("CopyToClipboard returned error: %v", err)
	}
	if !nativeResolved {
		t.Fatal("native write did not run")
	}
	if len(osc52Writes(stdoutWrites)) != 1 {
		t.Fatalf("OSC 52 writes = %#v", stdoutWrites)
	}
}

func TestCopyToClipboardLocalShellFallbackSuccessSkipsOSC52(t *testing.T) {
	var execCalls []clipboardExecCall
	var stdoutWrites []string
	ops := ClipboardCopyOperations{
		SetText: func(string) error { return errors.New("native failed") },
		ExecCommand: func(command string, options ClipboardTextCommandOptions) error {
			execCalls = append(execCalls, clipboardExecCall{command: command, options: options})
			return nil
		},
		WriteStdout: func(text string) error {
			stdoutWrites = append(stdoutWrites, text)
			return nil
		},
	}

	if err := CopyToClipboard("hello", ClipboardCopyOptions{Platform: "darwin", Env: map[string]string{}, Operations: ops}); err != nil {
		t.Fatalf("CopyToClipboard returned error: %v", err)
	}
	if len(execCalls) != 1 || execCalls[0].command != "pbcopy" || execCalls[0].options.Input != "hello" || execCalls[0].options.Timeout != defaultClipboardCopyTimeout {
		t.Fatalf("exec calls = %#v", execCalls)
	}
	if len(osc52Writes(stdoutWrites)) != 0 {
		t.Fatalf("OSC 52 writes = %#v", stdoutWrites)
	}
}

func TestCopyToClipboardUsesOSC52FallbackWhenNativeAndShellFail(t *testing.T) {
	var stdoutWrites []string
	ops := ClipboardCopyOperations{
		SetText:     func(string) error { return errors.New("native failed") },
		ExecCommand: func(string, ClipboardTextCommandOptions) error { return errors.New("shell failed") },
		WriteStdout: func(text string) error {
			stdoutWrites = append(stdoutWrites, text)
			return nil
		},
	}

	if err := CopyToClipboard("hello", ClipboardCopyOptions{Platform: "darwin", Env: map[string]string{}, Operations: ops}); err != nil {
		t.Fatalf("CopyToClipboard returned error: %v", err)
	}
	writes := osc52Writes(stdoutWrites)
	if len(writes) != 1 || writes[0] != "\x1b]52;c;aGVsbG8=\x07" {
		t.Fatalf("OSC 52 writes = %#v", stdoutWrites)
	}
}

func TestCopyToClipboardRejectsOversizedOSC52Payloads(t *testing.T) {
	var stdoutWrites []string
	ops := ClipboardCopyOperations{
		SetText:     func(string) error { return errors.New("native failed") },
		ExecCommand: func(string, ClipboardTextCommandOptions) error { return errors.New("shell failed") },
		WriteStdout: func(text string) error {
			stdoutWrites = append(stdoutWrites, text)
			return nil
		},
	}

	err := CopyToClipboard(strings.Repeat("x", 80000), ClipboardCopyOptions{Platform: "darwin", Env: map[string]string{}, Operations: ops})
	if err == nil || err.Error() != "Failed to copy to clipboard" {
		t.Fatalf("error = %v, want failed copy", err)
	}
	if len(osc52Writes(stdoutWrites)) != 0 {
		t.Fatalf("OSC 52 writes = %#v", stdoutWrites)
	}
}

func osc52Writes(writes []string) []string {
	var result []string
	for _, write := range writes {
		if strings.HasPrefix(write, "\x1b]52;c;") {
			result = append(result, write)
		}
	}
	return result
}
