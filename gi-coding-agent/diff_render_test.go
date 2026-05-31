package gicodingagent

import (
	"strings"
	"testing"
)

func TestRenderDiffPiStyle(t *testing.T) {
	rendered := RenderDiff("-1 Hello world\n+1 Hello universe\n 2 unchanged")
	plain := StripAnsi(rendered)
	for _, want := range []string{"-1 Hello world", "+1 Hello universe", " 2 unchanged"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("rendered diff missing %q:\n%s", want, plain)
		}
	}
	for _, want := range []string{"\x1b[7mworld\x1b[27m", "\x1b[7muniverse\x1b[27m"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered diff missing inverse segment %q:\n%q", want, rendered)
		}
	}
	if !strings.Contains(rendered, activeTUITheme.fg["toolDiffRemoved"]) ||
		!strings.Contains(rendered, activeTUITheme.fg["toolDiffAdded"]) ||
		!strings.Contains(rendered, activeTUITheme.fg["toolDiffContext"]) {
		t.Fatalf("rendered diff missing Pi tool diff colors:\n%q", rendered)
	}
}

func TestRenderDiffPiStyleTabsAndMultipleLineChanges(t *testing.T) {
	rendered := RenderDiff("-1 a\tb\n-2 removed\n+1 a\tc\n+2 added\n     ...")
	plain := StripAnsi(rendered)
	for _, want := range []string{"-1 a   b", "+1 a   c", "     ..."} {
		if !strings.Contains(plain, want) {
			t.Fatalf("rendered diff missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(rendered, "\x1b[7m") {
		t.Fatalf("multi-line replacements should not use intra-line inverse highlighting:\n%q", rendered)
	}
}
