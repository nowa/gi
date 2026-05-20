package gicodingagent

import (
	"strings"
	"testing"

	gitui "github.com/nowa/gi/gi-tui"
)

func TestBashExecutionCollapsedPreviewLinesRespectRenderTimeWidth(t *testing.T) {
	wideWidth := 200
	narrowWidth := 80
	component := NewBashExecutionComponent("pwd")

	longLine := strings.Repeat("x", 150)
	component.AppendOutput(longLine + "\n" + longLine + "\n")
	component.SetComplete(0, false)

	for i, line := range component.Render(narrowWidth) {
		if got := gitui.VisibleWidth(line); got > narrowWidth {
			t.Fatalf("line %d visible width = %d, want <= %d: %q", i, got, narrowWidth, line)
		}
	}

	for i, line := range component.Render(wideWidth) {
		if got := gitui.VisibleWidth(line); got > wideWidth {
			t.Fatalf("wide line %d visible width = %d, want <= %d: %q", i, got, wideWidth, line)
		}
	}
}

func TestBashExecutionRecomputesLinesWhenWidthChangesBetweenRenders(t *testing.T) {
	component := NewBashExecutionComponent("echo hello")
	longLine := strings.Repeat("abcdefghij", 20)
	component.AppendOutput(longLine + "\n")
	component.SetComplete(0, false)

	for i, line := range component.Render(200) {
		if got := gitui.VisibleWidth(line); got > 200 {
			t.Fatalf("line %d visible width = %d, want <= 200: %q", i, got, line)
		}
	}
	for i, line := range component.Render(60) {
		if got := gitui.VisibleWidth(line); got > 60 {
			t.Fatalf("line %d visible width = %d, want <= 60: %q", i, got, line)
		}
	}
}
