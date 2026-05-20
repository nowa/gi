package gicodingagent

import (
	"strings"
	"testing"
)

func TestExportHTMLPiMarkdownLinkSanitization(t *testing.T) {
	if got := RenderExportHTMLMarkdownLink("javascript:alert(1)", "click"); got != "click" {
		t.Fatalf("javascript link = %q", got)
	}
	if got := RenderExportHTMLMarkdownLink("vbscript:msgbox(1)", "click"); got != "click" {
		t.Fatalf("vbscript link = %q", got)
	}
	got := RenderExportHTMLMarkdownLink(`https://example.com/?q=" onmouseover="x`, `<click>`)
	if strings.Contains(got, `" onmouseover="`) || !strings.Contains(got, "&#34;") || !strings.Contains(got, "&lt;click&gt;") {
		t.Fatalf("escaped link = %s", got)
	}
}

func TestExportHTMLPiMarkdownImageSanitization(t *testing.T) {
	if got := RenderExportHTMLMarkdownImage("javascript:alert(1)", "x"); got != "" {
		t.Fatalf("javascript image = %q", got)
	}
	got := RenderExportHTMLMarkdownImage(`https://example.com/i.png?x=" onerror="x`, `<alt>`)
	if strings.Contains(got, `" onerror="`) || !strings.Contains(got, "&#34;") || !strings.Contains(got, "&lt;alt&gt;") {
		t.Fatalf("escaped image = %s", got)
	}
}

func TestExportHTMLPiInlineImageEscapesAttributes(t *testing.T) {
	got := RenderExportHTMLInlineImage(`image/png" onerror="x`, `abc" onerror="x`)
	if strings.Contains(got, `" onerror="`) || !strings.Contains(got, "&#34;") {
		t.Fatalf("inline image = %s", got)
	}
}

func TestExportHTMLPiEscapesSessionEntryAttributes(t *testing.T) {
	got := RenderExportHTMLSessionEntryAttrs(`id" onclick="x`)
	if strings.Contains(got, `" onclick="`) || !strings.Contains(got, "entry-id&#34;") || !strings.Contains(got, `data-entry-id="id&#34;`) {
		t.Fatalf("entry attrs = %s", got)
	}
}

func TestExportHTMLPiEscapesTreeMetadata(t *testing.T) {
	got := RenderExportHTMLTreeMetadata(map[string]string{
		"tool":     `read"><script>`,
		"role":     `assistant<script>`,
		"model":    `model" onclick="x`,
		"thinking": `<high>`,
		"type":     `message"`,
	})
	if strings.Contains(got, "<script>") || strings.Contains(got, `" onclick="`) || !strings.Contains(got, "&lt;high&gt;") {
		t.Fatalf("tree metadata = %s", got)
	}
}

func TestExportHTMLPiEscapesHeaderModelNames(t *testing.T) {
	got := RenderExportHTMLHeaderModels([]string{`gpt" onclick="x`, `<script>`})
	if strings.Contains(got, `" onclick="`) || strings.Contains(got, "<script>") || !strings.Contains(got, "&lt;script&gt;") {
		t.Fatalf("header models = %s", got)
	}
	if got := RenderExportHTMLHeaderModels(nil); got != "unknown" {
		t.Fatalf("empty header models = %q", got)
	}
}
