package gicodingagent

import (
	"path/filepath"
	"strings"
	"testing"

	gitui "github.com/nowa/gi/gi-tui"
)

func TestRenderToolPathStatesAndHyperlinkPolicy(t *testing.T) {
	gitui.SetCapabilities(gitui.TerminalCapabilities{Hyperlinks: false})
	t.Cleanup(gitui.ResetCapabilitiesCache)

	cwd := t.TempDir()
	rawPath := filepath.Join("folder", "a b.txt")
	rendered := renderToolPath(&rawPath, cwd)
	if got := StripAnsi(rendered); got != filepath.ToSlash(rawPath) {
		t.Fatalf("plain rendered path = %q", got)
	}
	if strings.Contains(rendered, "\x1b]8;;") {
		t.Fatalf("plain rendered path contains hyperlink: %q", rendered)
	}
	if got := StripAnsi(renderToolPath(nil, cwd)); got != "[invalid arg]" {
		t.Fatalf("invalid path = %q", got)
	}
	empty := ""
	if got := StripAnsi(renderToolPath(&empty, cwd)); got != "..." {
		t.Fatalf("empty path = %q", got)
	}
	if got := StripAnsi(renderToolPath(
		&empty,
		cwd,
		toolPathRenderOptions{emptyFallback: "."},
	)); got != "." {
		t.Fatalf("fallback path = %q", got)
	}

	gitui.SetCapabilities(gitui.TerminalCapabilities{Hyperlinks: true})
	rendered = renderToolPath(&rawPath, cwd)
	if !strings.Contains(rendered, "\x1b]8;;file://") ||
		!strings.Contains(rendered, "folder/a%20b.txt") ||
		!strings.Contains(rendered, filepath.ToSlash(rawPath)) {
		t.Fatalf("linked rendered path = %q", rendered)
	}
}

func TestToolPathArgumentPreservesProtocolStringStates(t *testing.T) {
	if got := toolPathArgument(map[string]any{}, "", "file_path", "path"); got == nil || *got != "" {
		t.Fatalf("missing argument = %#v", got)
	}
	if got := toolPathArgument(map[string]any{
		"file_path": nil,
		"path":      "current.txt",
	}, "", "file_path", "path"); got == nil || *got != "current.txt" {
		t.Fatalf("null-coalesced argument = %#v", got)
	}
	if got := toolPathArgument(map[string]any{
		"file_path": "",
		"path":      "current.txt",
	}, "", "file_path", "path"); got == nil || *got != "" {
		t.Fatalf("empty legacy argument = %#v", got)
	}
	if got := toolPathArgument(map[string]any{"path": 42}, "", "path"); got != nil {
		t.Fatalf("invalid argument = %#v", got)
	}
}

func TestBuiltInFileToolCallsUseUnifiedPathRenderer(t *testing.T) {
	gitui.SetCapabilities(gitui.TerminalCapabilities{Hyperlinks: true})
	t.Cleanup(gitui.ResetCapabilitiesCache)
	cwd := t.TempDir()

	for _, test := range []struct {
		name string
		call func(any, ToolRenderContext) []string
		args any
	}{
		{name: "read", call: renderReadToolCall, args: map[string]any{"path": "a b.txt"}},
		{name: "write", call: renderWriteToolCall, args: map[string]any{"path": "a b.txt"}},
		{name: "edit", call: renderEditToolCall, args: map[string]any{"path": "a b.txt"}},
		{name: "ls", call: renderLsToolCall, args: map[string]any{"path": "a b.txt"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			lines := test.call(test.args, ToolRenderContext{CWD: cwd})
			if len(lines) == 0 ||
				!strings.Contains(lines[0], "\x1b]8;;file://") ||
				!strings.Contains(lines[0], "a%20b.txt") {
				t.Fatalf("rendered call = %#v", lines)
			}
		})
	}

	lines := renderReadToolCall(map[string]any{"path": 42}, ToolRenderContext{CWD: cwd})
	if len(lines) != 1 || StripAnsi(lines[0]) != "read [invalid arg]" {
		t.Fatalf("invalid read call = %#v", lines)
	}
}
