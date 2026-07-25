package gicodingagent

import (
	"strings"
	"testing"
)

func TestChangelogEntriesFollowPiStartupParsing(t *testing.T) {
	entries := parseChangelogEntries(`# Changelog

## 0.3.0

- Added startup notices

## [0.2.0]

- Previous entry

## Unreleased

- Ignored
`)
	if len(entries) != 2 {
		t.Fatalf("entries = %#v", entries)
	}
	newEntries := newChangelogEntries(entries, "0.2.0")
	if len(newEntries) != 1 || newEntries[0].Version != "0.3.0" || newEntries[0].Content == "" {
		t.Fatalf("new entries = %#v", newEntries)
	}
	if markdown := changelogEntriesMarkdown(newEntries); markdown == "" || firstChangelogVersion(markdown) != "0.3.0" {
		t.Fatalf("markdown = %q", markdown)
	}
}

func TestNormalizeChangelogLinksPinsPackageRelativeTargets(t *testing.T) {
	markdown := strings.Join([]string{
		"[Project Trust](README.md#project-trust)",
		"[Extensions](docs/extensions.md?plain=1#project_trust)",
		"[Examples](examples/extensions/)",
		"[Root README](../README.md#supply-chain-hardening)",
		`[Windows path](docs\guide.md)`,
		`![Diagram](docs/agent-flow.png "Agent flow")`,
		"[Outside](../../secrets.txt)",
	}, "\n")

	got := normalizeChangelogLinks(markdown, "0.79.0")
	want := strings.Join([]string{
		"[Project Trust](https://github.com/nowa/gi/blob/v0.79.0/gi-coding-agent/README.md#project-trust)",
		"[Extensions](https://github.com/nowa/gi/blob/v0.79.0/gi-coding-agent/docs/extensions.md?plain=1#project_trust)",
		"[Examples](https://github.com/nowa/gi/tree/v0.79.0/gi-coding-agent/examples/extensions/)",
		"[Root README](https://github.com/nowa/gi/blob/v0.79.0/README.md#supply-chain-hardening)",
		"[Windows path](https://github.com/nowa/gi/blob/v0.79.0/gi-coding-agent/docs/guide.md)",
		`![Diagram](https://github.com/nowa/gi/blob/v0.79.0/gi-coding-agent/docs/agent-flow.png "Agent flow")`,
		"[Outside](../../secrets.txt)",
	}, "\n")
	if got != want {
		t.Fatalf("normalized links:\n%s\nwant:\n%s", got, want)
	}
}

func TestNormalizeChangelogLinksCanonicalizesOldRepositoryURLsWithoutChangingExternalLinks(t *testing.T) {
	markdown := strings.Join([]string{
		"[#5167](https://github.com/earendil-works/pi-mono/pull/5167)",
		"[#4163](https://github.com/badlogic/pi-mono/issues/4163)",
		"[Agent README](https://github.com/badlogic/pi-mono/blob/main/packages/agent/README.md)",
		"[Gi README](https://github.com/nowa/gi/tree/master/gi-coding-agent/docs/)",
		"[External](https://example.com/docs)",
		"[Uppercase scheme](HTTPS://example.com/docs)",
		"[Protocol relative](//example.com/docs)",
		"[Local anchor](#settings)",
	}, "\n")

	got := normalizeChangelogLinks(markdown, "v0.79.0")
	want := strings.Join([]string{
		"[#5167](https://github.com/earendil-works/pi/pull/5167)",
		"[#4163](https://github.com/earendil-works/pi/issues/4163)",
		"[Agent README](https://github.com/earendil-works/pi/blob/v0.79.0/packages/agent/README.md)",
		"[Gi README](https://github.com/nowa/gi/tree/v0.79.0/gi-coding-agent/docs/)",
		"[External](https://example.com/docs)",
		"[Uppercase scheme](HTTPS://example.com/docs)",
		"[Protocol relative](//example.com/docs)",
		"[Local anchor](#settings)",
	}, "\n")
	if got != want {
		t.Fatalf("normalized links:\n%s\nwant:\n%s", got, want)
	}
}

func TestAllChangelogEntriesMarkdownNormalizesAndReversesEntries(t *testing.T) {
	markdown := "## 0.2.0\n\n[New](README.md)\n\n## 0.1.0\n\n[Old](docs/)"
	got := allChangelogEntriesMarkdown(markdown)
	oldIndex := strings.Index(got, "tree/v0.1.0/gi-coding-agent/docs/")
	newIndex := strings.Index(got, "blob/v0.2.0/gi-coding-agent/README.md")
	if oldIndex < 0 || newIndex < 0 || oldIndex >= newIndex {
		t.Fatalf("normalized changelog order = %q", got)
	}
}
