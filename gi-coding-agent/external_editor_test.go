package gicodingagent

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type externalEditorRunnerFunc func(context.Context, externalEditorInvocation) error

func (f externalEditorRunnerFunc) Run(ctx context.Context, invocation externalEditorInvocation) error {
	return f(ctx, invocation)
}

type externalEditorCapture struct {
	invocation    externalEditorInvocation
	original      string
	entries       []string
	directoryMode os.FileMode
}

func TestEditInExternalEditorPiCases(t *testing.T) {
	t.Run("edits a prompt inside a private temporary directory", func(t *testing.T) {
		var capture externalEditorCapture
		var output bytes.Buffer
		result, err := editInExternalEditor(
			context.Background(),
			ExternalEditorOptions{
				Command: "fake-editor --wait",
				Content: "original",
				Stdout:  &output,
				Stderr:  &output,
			},
			externalEditorRunnerFunc(func(_ context.Context, invocation externalEditorInvocation) error {
				capture = captureExternalEditorInvocation(t, invocation)
				return os.WriteFile(invocation.FilePath, []byte("edited\n"), 0o600)
			}),
		)
		if err != nil {
			t.Fatal(err)
		}
		if result != (ExternalEditorResult{Status: ExternalEditorStatusComplete, Content: "edited"}) {
			t.Fatalf("result = %#v", result)
		}

		directory := filepath.Dir(capture.invocation.FilePath)
		if filepath.Dir(directory) != filepath.Clean(os.TempDir()) {
			t.Fatalf("temporary directory parent = %q, want %q", filepath.Dir(directory), os.TempDir())
		}
		if !strings.HasPrefix(filepath.Base(directory), "gi-editor-") {
			t.Fatalf("temporary directory = %q", directory)
		}
		if filepath.Base(capture.invocation.FilePath) != "prompt.md" {
			t.Fatalf("editor file = %q", capture.invocation.FilePath)
		}
		if len(capture.entries) != 1 || capture.entries[0] != "prompt.md" {
			t.Fatalf("temporary directory entries = %#v", capture.entries)
		}
		if capture.original != "original" {
			t.Fatalf("initial editor content = %q", capture.original)
		}
		if runtime.GOOS != "windows" && capture.directoryMode.Perm()&0o077 != 0 {
			t.Fatalf("temporary directory mode = %o, want private", capture.directoryMode.Perm())
		}
		if capture.invocation.Command != "fake-editor" ||
			len(capture.invocation.Args) != 1 ||
			capture.invocation.Args[0] != "--wait" {
			t.Fatalf("editor invocation = %#v", capture.invocation)
		}
		if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("temporary directory still exists: %v", err)
		}
		if !strings.Contains(output.String(), "Launching external editor: fake-editor --wait") {
			t.Fatalf("launch output = %q", output.String())
		}
	})

	t.Run("keeps the original content when the editor exits unsuccessfully", func(t *testing.T) {
		var filePath string
		result, err := editInExternalEditor(
			context.Background(),
			ExternalEditorOptions{Command: "fake-editor", Content: "original", Stdout: &bytes.Buffer{}},
			externalEditorRunnerFunc(func(_ context.Context, invocation externalEditorInvocation) error {
				filePath = invocation.FilePath
				return errors.New("editor failed")
			}),
		)
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != ExternalEditorStatusFailed || result.Content != "" {
			t.Fatalf("result = %#v", result)
		}
		if _, err := os.Stat(filepath.Dir(filePath)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("temporary directory still exists: %v", err)
		}
	})

	t.Run("returns empty content when the editor clears the prompt", func(t *testing.T) {
		result, err := editInExternalEditor(
			context.Background(),
			ExternalEditorOptions{Command: "fake-editor", Content: "original", Stdout: &bytes.Buffer{}},
			externalEditorRunnerFunc(func(_ context.Context, invocation externalEditorInvocation) error {
				return os.WriteFile(invocation.FilePath, nil, 0o600)
			}),
		)
		if err != nil {
			t.Fatal(err)
		}
		if result != (ExternalEditorResult{Status: ExternalEditorStatusComplete, Content: ""}) {
			t.Fatalf("result = %#v", result)
		}
	})
}

func TestSettingsManagerExternalEditorPiCases(t *testing.T) {
	t.Run("resolves editor commands by precedence", func(t *testing.T) {
		t.Setenv("VISUAL", "vim")
		t.Setenv("EDITOR", "nano")
		if got := NewInMemorySettingsManager(map[string]any{"externalEditor": "code --wait"}).GetExternalEditorCommand(); got != "code --wait" {
			t.Fatalf("configured editor = %q", got)
		}
		if got := NewInMemorySettingsManager(nil).GetExternalEditorCommand(); got != "vim" {
			t.Fatalf("VISUAL editor = %q", got)
		}

		t.Setenv("VISUAL", "")
		t.Setenv("EDITOR", "emacs")
		if got := NewInMemorySettingsManager(nil).GetExternalEditorCommand(); got != "emacs" {
			t.Fatalf("EDITOR editor = %q", got)
		}
	})

	t.Run("falls back to platform defaults", func(t *testing.T) {
		emptyEnvironment := func(string) (string, bool) { return "", false }
		if got := resolveExternalEditorCommand("", emptyEnvironment, "windows"); got != "notepad" {
			t.Fatalf("Windows editor = %q", got)
		}
		if got := resolveExternalEditorCommand("", emptyEnvironment, "darwin"); got != "nano" {
			t.Fatalf("macOS editor = %q", got)
		}
		if got := resolveExternalEditorCommand("", emptyEnvironment, "linux"); got != "nano" {
			t.Fatalf("Linux editor = %q", got)
		}
	})
}

func captureExternalEditorInvocation(t *testing.T, invocation externalEditorInvocation) externalEditorCapture {
	t.Helper()
	content, err := os.ReadFile(invocation.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(invocation.FilePath))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(entries))
	for index, entry := range entries {
		names[index] = entry.Name()
	}
	info, err := os.Stat(filepath.Dir(invocation.FilePath))
	if err != nil {
		t.Fatal(err)
	}
	invocation.Args = append([]string(nil), invocation.Args...)
	return externalEditorCapture{
		invocation:    invocation,
		original:      string(content),
		entries:       names,
		directoryMode: info.Mode(),
	}
}
