package gicodingagent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gitui "github.com/nowa/gi/gi-tui"
)

func TestEditToolExecutionLargeDiffSettlesWithoutFullRedraw(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "large-edit.txt")
	if err := os.WriteFile(filePath, []byte(numberedEditLines(1000)), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(numberedEditLines(1000), "\n"), "\n")
	edits := createLargeRedrawEdits(lines)
	diff := ComputeEditsDiff(filePath, edits, dir)
	if diff.Error != "" {
		t.Fatalf("diff error: %s", diff.Error)
	}

	terminal := newToolExecutionFakeTerminal(80, 24)
	ui := gitui.NewTUI(terminal)
	root := gitui.NewContainer()
	for i := 0; i < 200; i++ {
		root.AddChild(gitui.NewText(fmt.Sprintf("history %d", i), 0, 0))
	}
	component := NewToolExecutionComponent(
		"edit",
		"tool-call-1",
		EditToolInput{Path: filePath, Edits: edits},
		CreateEditToolDefinition(dir),
		dir,
	)
	root.AddChild(component)
	ui.AddChild(root)
	ui.Start()

	component.SetArgsComplete()
	ui.RequestRender()
	callOnlyRender := strings.Join(component.Render(80), "\n")
	if !strings.Contains(callOnlyRender, "edit") || !strings.Contains(callOnlyRender, "line 50 changed") || !strings.Contains(callOnlyRender, "line 950 changed") {
		t.Fatalf("call preview render = %q", callOnlyRender)
	}

	redrawsBeforeResult := ui.FullRedraws()
	clearsBeforeResult := terminal.fullClearCount()
	component.UpdateResult(FileToolResult{
		Text:    fmt.Sprintf("Successfully replaced %d block(s) in %s.", len(edits), filePath),
		Details: &FileToolDetails{Diff: diff.Diff},
	}, false)
	ui.RequestRender()

	if got := ui.FullRedraws(); got != redrawsBeforeResult {
		t.Fatalf("full redraws = %d, want %d", got, redrawsBeforeResult)
	}
	if got := terminal.fullClearCount(); got != clearsBeforeResult {
		t.Fatalf("full clears = %d, want %d", got, clearsBeforeResult)
	}
	settledRender := strings.Join(component.Render(80), "\n")
	if !strings.Contains(settledRender, "line 50 changed") || !strings.Contains(settledRender, "line 950 changed") {
		t.Fatalf("settled render = %q", settledRender)
	}
	if strings.Contains(settledRender, "Successfully replaced") {
		t.Fatalf("settled render should not show success text: %q", settledRender)
	}
}

func TestEditToolExecutionReconstructsPreviewFromSettledResult(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "replay-edit.txt")
	if err := os.WriteFile(filePath, []byte(numberedEditLines(200)), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(numberedEditLines(200), "\n"), "\n")
	edits := createLargeRedrawEdits(lines)[:2]
	diff := ComputeEditsDiff(filePath, edits, dir)
	if diff.Error != "" {
		t.Fatalf("diff error: %s", diff.Error)
	}
	if err := os.Remove(filePath); err != nil {
		t.Fatalf("remove file: %v", err)
	}

	component := NewToolExecutionComponent(
		"edit",
		"tool-call-replay",
		EditToolInput{Path: filePath, Edits: edits},
		CreateEditToolDefinition(dir),
		dir,
	)
	component.UpdateResult(FileToolResult{
		Text:    fmt.Sprintf("Successfully replaced %d block(s) in %s.", len(edits), filePath),
		Details: &FileToolDetails{Diff: diff.Diff},
	}, false)

	rendered := strings.Join(component.Render(80), "\n")
	if !strings.Contains(rendered, "line 50 changed") || !strings.Contains(rendered, "line 150 changed") {
		t.Fatalf("rendered replay = %q", rendered)
	}
	if strings.Contains(rendered, "Successfully replaced") {
		t.Fatalf("rendered replay should not show success text: %q", rendered)
	}
}

func TestEditToolExecutionShowsPreflightErrorWithoutDiff(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "missing-edit.txt")
	if err := os.WriteFile(filePath, []byte("line 0\nline 1\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	component := NewToolExecutionComponent(
		"edit",
		"tool-call-2",
		EditToolInput{Path: filePath, Edits: []Edit{{OldText: "does not exist", NewText: "replacement"}}},
		CreateEditToolDefinition(dir),
		dir,
	)

	component.SetArgsComplete()
	rendered := strings.Join(component.Render(80), "\n")

	if !strings.Contains(rendered, "Could not find") {
		t.Fatalf("rendered preflight = %q", rendered)
	}
	if strings.Contains(rendered, "+1 ") || strings.Contains(rendered, "-1 ") {
		t.Fatalf("rendered preflight should not contain diff markers: %q", rendered)
	}
}

func numberedEditLines(count int) string {
	var builder strings.Builder
	for i := 0; i < count; i++ {
		builder.WriteString(fmt.Sprintf("line %d\n", i))
	}
	return builder.String()
}

func createLargeRedrawEdits(lines []string) []Edit {
	targets := []int{50, 150, 250, 350, 450, 550, 650, 750, 850, 950}
	edits := make([]Edit, 0, len(targets))
	for _, lineNumber := range targets {
		if lineNumber+1 >= len(lines) {
			continue
		}
		edits = append(edits, Edit{
			OldText: strings.Join([]string{
				lines[lineNumber-1],
				lines[lineNumber],
				lines[lineNumber+1],
			}, "\n"),
			NewText: strings.Join([]string{
				lines[lineNumber-1],
				lines[lineNumber] + " changed",
				lines[lineNumber+1],
			}, "\n"),
		})
	}
	return edits
}

type toolExecutionFakeTerminal struct {
	cols   int
	rows   int
	writes []string
}

func newToolExecutionFakeTerminal(cols, rows int) *toolExecutionFakeTerminal {
	return &toolExecutionFakeTerminal{cols: cols, rows: rows}
}

func (f *toolExecutionFakeTerminal) Start(func(string), func()) {}
func (f *toolExecutionFakeTerminal) Stop()                      {}
func (f *toolExecutionFakeTerminal) DrainInput(_, _ time.Duration) error {
	return nil
}
func (f *toolExecutionFakeTerminal) Write(data string) error {
	f.writes = append(f.writes, data)
	return nil
}
func (f *toolExecutionFakeTerminal) Columns() int              { return f.cols }
func (f *toolExecutionFakeTerminal) Rows() int                 { return f.rows }
func (f *toolExecutionFakeTerminal) KittyProtocolActive() bool { return true }
func (f *toolExecutionFakeTerminal) MoveBy(int) error          { return nil }
func (f *toolExecutionFakeTerminal) HideCursor() error         { return nil }
func (f *toolExecutionFakeTerminal) ShowCursor() error         { return nil }
func (f *toolExecutionFakeTerminal) ClearLine() error          { return nil }
func (f *toolExecutionFakeTerminal) ClearFromCursor() error    { return nil }
func (f *toolExecutionFakeTerminal) ClearScreen() error        { return nil }
func (f *toolExecutionFakeTerminal) SetTitle(string) error     { return nil }
func (f *toolExecutionFakeTerminal) SetProgress(bool) error    { return nil }
func (f *toolExecutionFakeTerminal) fullClearCount() int {
	count := 0
	for _, write := range f.writes {
		if strings.Contains(write, "\x1b[2J\x1b[H\x1b[3J") {
			count++
		}
	}
	return count
}
