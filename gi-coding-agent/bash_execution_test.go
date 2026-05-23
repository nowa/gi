package gicodingagent

import (
	"fmt"
	"strings"
	"testing"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
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

func TestBashExecutionShowsRunningCancelHintPiStyle(t *testing.T) {
	component := NewBashExecutionComponent("sleep 10")

	output := strings.Join(component.Render(100), "\n")
	if !strings.Contains(output, "Running... (Esc to cancel)") {
		t.Fatalf("rendered bash output missing running cancel hint:\n%s", output)
	}
}

func TestBashExecutionShowsTruncatedFullOutputPathPiStyle(t *testing.T) {
	component := NewBashExecutionComponent("printf lots")
	component.AppendOutput("tail output\n")
	component.SetComplete(0, false, BashExecutionCompleteOptions{
		Truncated:      true,
		FullOutputPath: "/tmp/gi-bash-full.log",
	})

	output := strings.Join(component.Render(100), "\n")
	if !strings.Contains(output, "Output truncated. Full output: /tmp/gi-bash-full.log") {
		t.Fatalf("rendered bash output missing truncation path:\n%s", output)
	}
}

func TestBashExecutionAppliesContextTruncationPiStyle(t *testing.T) {
	component := NewBashExecutionComponent("printf many-lines")
	component.SetExpanded(true)
	var builder strings.Builder
	for i := 0; i < agentharness.DefaultMaxLines+5; i++ {
		fmt.Fprintf(&builder, "line-%04d\n", i)
	}
	component.AppendOutput(builder.String())
	component.SetComplete(0, false, BashExecutionCompleteOptions{
		FullOutputPath: "/tmp/full-bash-output.log",
	})

	output := strings.Join(component.Render(120), "\n")
	if strings.Contains(output, "line-0000") {
		t.Fatalf("rendered bash output kept head line despite tail truncation")
	}
	if !strings.Contains(output, fmt.Sprintf("line-%04d", agentharness.DefaultMaxLines+4)) {
		t.Fatalf("rendered bash output missing tail line after truncation")
	}
	if !strings.Contains(output, "Output truncated. Full output: /tmp/full-bash-output.log") {
		t.Fatalf("rendered bash output missing context truncation status")
	}
}
