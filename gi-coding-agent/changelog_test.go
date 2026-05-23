package gicodingagent

import "testing"

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
