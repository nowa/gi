package gicodingagent

import (
	"regexp"
	"testing"
)

func TestExportHTMLPiWhitespaceCSS(t *testing.T) {
	if !regexp.MustCompile(`\.output-preview > div:not\(\.expand-hint\),\s*\.output-full > div:not\(\.expand-hint\) \{[\s\S]*?white-space:\s*pre-wrap;`).MatchString(ExportHTMLTemplateCSS) {
		t.Fatalf("template CSS missing scoped pre-wrap rule:\n%s", ExportHTMLTemplateCSS)
	}
	if !regexp.MustCompile(`\.ansi-line\s*\{[\s\S]*?white-space:\s*pre;`).MatchString(ExportHTMLTemplateCSS) {
		t.Fatalf("template CSS missing ansi pre rule:\n%s", ExportHTMLTemplateCSS)
	}
	if regexp.MustCompile(`\.output-preview,\s*\.output-full\s*\{[\s\S]*?white-space:\s*pre-wrap;`).MatchString(ExportHTMLTemplateCSS) {
		t.Fatalf("template CSS should not apply pre-wrap to output containers:\n%s", ExportHTMLTemplateCSS)
	}
}

func TestAnsiLinesToHTMLPiWhitespace(t *testing.T) {
	got := AnsiLinesToHTML([]string{"one", "two"})
	want := `<div class="ansi-line">one</div><div class="ansi-line">two</div>`
	if got != want {
		t.Fatalf("AnsiLinesToHTML = %q, want %q", got, want)
	}
}

func TestRenderCustomToolResultHTMLPiTrimsSpacingLines(t *testing.T) {
	got := RenderCustomToolResultHTML([]string{"", "\x1b[31mone\x1b[0m", "two", ""})
	want := `<div class="ansi-line"><span style="color:#800000">one</span></div><div class="ansi-line">two</div>`
	if got != want {
		t.Fatalf("custom tool result HTML = %q, want %q", got, want)
	}
}
