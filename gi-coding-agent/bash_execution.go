package gicodingagent

import (
	"strings"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	gitui "github.com/nowa/gi/gi-tui"
)

const bashExecutionPreviewLines = 20

type BashExecutionComponent struct {
	command     string
	outputLines []string
	status      string
	exitCode    int
	expanded    bool
	truncated   bool
	fullOutput  string
}

type BashExecutionCompleteOptions struct {
	Truncated      bool
	FullOutputPath string
}

func NewBashExecutionComponent(command string) *BashExecutionComponent {
	return &BashExecutionComponent{command: command, status: "running"}
}

func (b *BashExecutionComponent) SetExpanded(expanded bool) {
	b.expanded = expanded
}

func (b *BashExecutionComponent) Invalidate() {}

func (b *BashExecutionComponent) AppendOutput(chunk string) {
	clean := strings.ReplaceAll(strings.ReplaceAll(StripAnsi(chunk), "\r\n", "\n"), "\r", "\n")
	newLines := strings.Split(clean, "\n")
	if len(b.outputLines) > 0 && len(newLines) > 0 {
		b.outputLines[len(b.outputLines)-1] += newLines[0]
		b.outputLines = append(b.outputLines, newLines[1:]...)
		return
	}
	b.outputLines = append(b.outputLines, newLines...)
}

func (b *BashExecutionComponent) SetComplete(exitCode int, cancelled bool, options ...BashExecutionCompleteOptions) {
	b.exitCode = exitCode
	if len(options) > 0 {
		b.truncated = options[0].Truncated
		b.fullOutput = options[0].FullOutputPath
	}
	switch {
	case cancelled:
		b.status = "cancelled"
	case exitCode != 0:
		b.status = "error"
	default:
		b.status = "complete"
	}
}

func (b *BashExecutionComponent) Render(width int) []string {
	if width <= 0 {
		return nil
	}
	lines := []string{
		strings.Repeat("─", width),
	}
	lines = append(lines, gitui.NewText("$ "+b.command, 1, 0).Render(width)...)

	availableLines, contextTruncated := b.availableOutputLines()
	if len(availableLines) > 0 {
		if b.expanded {
			displayText := "\n" + strings.Join(availableLines, "\n")
			lines = append(lines, gitui.NewText(displayText, 1, 0).Render(width)...)
		} else {
			previewLogicalLines := tailStrings(availableLines, bashExecutionPreviewLines)
			styledInput := "\n" + strings.Join(previewLogicalLines, "\n")
			truncated := TruncateToVisualLines(styledInput, bashExecutionPreviewLines, width, 1)
			lines = append(lines, truncated.VisualLines...)
		}
	}

	if b.status == "running" {
		lines = append(lines, gitui.NewText("Running... (Esc to cancel)", 1, 0).Render(width)...)
	} else {
		statusLines := b.statusLines(availableLines, contextTruncated)
		if len(statusLines) > 0 {
			lines = append(lines, gitui.NewText("\n"+strings.Join(statusLines, "\n"), 1, 0).Render(width)...)
		}
	}
	lines = append(lines, strings.Repeat("─", width))
	return lines
}

func (b *BashExecutionComponent) Output() string {
	return strings.Join(b.outputLines, "\n")
}

func (b *BashExecutionComponent) GetOutput() string {
	return b.Output()
}

func (b *BashExecutionComponent) GetCommand() string {
	return b.command
}

func (b *BashExecutionComponent) availableOutputLines() ([]string, bool) {
	if len(b.outputLines) == 0 {
		return nil, false
	}
	output := strings.Join(b.outputLines, "\n")
	truncated := agentharness.TruncateTail(output, agentharness.TruncationOptions{})
	if truncated.Content == "" {
		return nil, truncated.Truncated
	}
	return strings.Split(truncated.Content, "\n"), truncated.Truncated
}

func (b *BashExecutionComponent) statusLines(availableLines []string, contextTruncated bool) []string {
	hiddenLineCount := len(availableLines) - len(tailStrings(availableLines, bashExecutionPreviewLines))
	var lines []string
	if hiddenLineCount > 0 {
		if b.expanded {
			lines = append(lines, "(to collapse)")
		} else {
			lines = append(lines, "... "+formatFooterTokens(hiddenLineCount)+" more lines (to expand)")
		}
	}
	switch b.status {
	case "cancelled":
		lines = append(lines, "(cancelled)")
	case "error":
		lines = append(lines, "(exit "+formatFooterTokens(b.exitCode)+")")
	}
	if (b.truncated || contextTruncated) && strings.TrimSpace(b.fullOutput) != "" {
		lines = append(lines, "Output truncated. Full output: "+b.fullOutput)
	}
	return lines
}

type VisualTruncateResult struct {
	VisualLines  []string
	SkippedCount int
}

func TruncateToVisualLines(text string, maxVisualLines, width, paddingX int) VisualTruncateResult {
	if text == "" {
		return VisualTruncateResult{}
	}
	allVisualLines := gitui.NewText(text, paddingX, 0).Render(width)
	if len(allVisualLines) <= maxVisualLines {
		return VisualTruncateResult{VisualLines: allVisualLines}
	}
	return VisualTruncateResult{
		VisualLines:  append([]string(nil), allVisualLines[len(allVisualLines)-maxVisualLines:]...),
		SkippedCount: len(allVisualLines) - maxVisualLines,
	}
}

func tailStrings(values []string, maxItems int) []string {
	if maxItems <= 0 || len(values) <= maxItems {
		return append([]string(nil), values...)
	}
	return append([]string(nil), values[len(values)-maxItems:]...)
}
