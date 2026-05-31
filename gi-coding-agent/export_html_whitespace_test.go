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

	if got := AnsiLinesToHTML([]string{""}); got != `<div class="ansi-line">&nbsp;</div>` {
		t.Fatalf("empty ANSI line = %q", got)
	}
}

func TestAnsiLinesToHTMLPiSGRStyles(t *testing.T) {
	got := AnsiLinesToHTML([]string{
		"\x1b[1;3;4;31;44mstyled & '<'\x1b[22;23;24;39;49m plain",
		"\x1b[38;5;196mred256\x1b[0m \x1b[38;2;1;2;3;48;2;4;5;6mrgb\x1b[0m",
	})
	want := `<div class="ansi-line"><span style="color:#800000;background-color:#000080;font-weight:bold;font-style:italic;text-decoration:underline">styled &amp; &#039;&lt;&#039;</span> plain</div>` +
		`<div class="ansi-line"><span style="color:#ff0000">red256</span> <span style="color:rgb(1,2,3);background-color:rgb(4,5,6)">rgb</span></div>`
	if got != want {
		t.Fatalf("ANSI SGR HTML = %q, want %q", got, want)
	}
}

func TestRenderCustomToolResultHTMLPiTrimsSpacingLines(t *testing.T) {
	got := RenderCustomToolResultHTML([]string{"", "\x1b[31mone\x1b[0m", "two", ""})
	want := `<div class="ansi-line"><span style="color:#800000">one</span></div><div class="ansi-line">two</div>`
	if got != want {
		t.Fatalf("custom tool result HTML = %q, want %q", got, want)
	}

	got = RenderCustomToolResultHTML([]string{"\x1b[31m\x1b[0m", "\x1b[2m  \x1b[0m", "body", "\x1b[34m\x1b[0m"})
	want = `<div class="ansi-line">body</div>`
	if got != want {
		t.Fatalf("custom tool result with ANSI-only lines = %q, want %q", got, want)
	}
}
