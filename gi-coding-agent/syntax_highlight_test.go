package gicodingagent

import (
	"strings"
	"testing"
)

func TestSyntaxHighlightRenderer(t *testing.T) {
	t.Run("renders highlighted spans with the provided theme", func(t *testing.T) {
		rendered := RenderHighlightedHTML(`<span class="hljs-keyword">const</span> value`, HighlightTheme{
			"keyword": func(text string) string { return "[keyword:" + text + "]" },
		})
		if rendered != "[keyword:const] value" {
			t.Fatalf("rendered = %q", rendered)
		}
	})

	t.Run("decodes HTML entities emitted by highlight.js", func(t *testing.T) {
		rendered := RenderHighlightedHTML(`&lt;tag attr=&quot;value&quot;&gt;&amp;#x41;&#65;&lt;/tag&gt;`, nil)
		if rendered != `<tag attr="value">&#x41;A</tag>` {
			t.Fatalf("rendered = %q", rendered)
		}
	})

	t.Run("inherits parent formatting for unmapped nested scopes", func(t *testing.T) {
		interpolation := "$" + "{x}"
		rendered := RenderHighlightedHTML(`<span class="hljs-string">a<span class="hljs-subst">`+interpolation+`</span>b</span>`, HighlightTheme{
			"string": func(text string) string { return "[string:" + text + "]" },
		})
		want := "[string:a][string:" + interpolation + "][string:b]"
		if rendered != want {
			t.Fatalf("rendered = %q, want %q", rendered, want)
		}
	})

	t.Run("keeps parent formatting across unscoped nested spans", func(t *testing.T) {
		rendered := RenderHighlightedHTML(`<span class="hljs-string">a<span class="language-xml">b</span>c</span>`, HighlightTheme{
			"string": func(text string) string { return "[string:" + text + "]" },
		})
		if rendered != "[string:a][string:b][string:c]" {
			t.Fatalf("rendered = %q", rendered)
		}
	})

	t.Run("highlights code through highlight.js", func(t *testing.T) {
		if !SupportsLanguage("typescript") {
			t.Fatalf("typescript should be supported")
		}
		rendered := Highlight("const value = 1", HighlightOptions{
			Language:       "typescript",
			IgnoreIllegals: true,
			Theme: HighlightTheme{
				"keyword": func(text string) string { return "[keyword:" + text + "]" },
				"number":  func(text string) string { return "[number:" + text + "]" },
			},
		})
		if !strings.Contains(rendered, "[keyword:const]") || !strings.Contains(rendered, "[number:1]") {
			t.Fatalf("rendered = %q", rendered)
		}
	})

	t.Run("colors diff additions and deletions in fenced diff blocks", func(t *testing.T) {
		lines := highlightCode("-old\n+new\n", "diff")
		if len(lines) != 3 {
			t.Fatalf("lines = %#v", lines)
		}
		if want := tuiThemeFG("toolDiffRemoved", "-old"); lines[0] != want {
			t.Fatalf("deletion = %q, want %q", lines[0], want)
		}
		if want := tuiThemeFG("toolDiffAdded", "+new"); lines[1] != want {
			t.Fatalf("addition = %q, want %q", lines[1], want)
		}
	})

	t.Run("keeps cli-highlight default styled scopes mapped to theme styles", func(t *testing.T) {
		javascript := highlightCode(
			"const re = /foo+/gi;",
			"javascript",
		)[0]
		if want := tuiThemeFG(
			"syntaxString",
			"/foo+/gi",
		); !strings.Contains(javascript, want) {
			t.Fatalf("javascript = %q, want %q", javascript, want)
		}

		python := highlightCode("@decorator", "python")[0]
		if want := tuiThemeFG("muted", "@decorator"); python != want {
			t.Fatalf("python = %q, want %q", python, want)
		}

		html := highlightCode("<div></div>", "html")[0]
		if want := tuiThemeFG(
			"syntaxKeyword",
			"div",
		); !strings.Contains(html, want) {
			t.Fatalf("html = %q, want %q", html, want)
		}
	})
}
