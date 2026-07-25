package gicodingagent

import (
	"slices"

	"github.com/nowa/gi/gi-coding-agent/internal/changelog"
)

type changelogEntry = changelog.Entry

func parseChangelogEntries(markdown string) []changelogEntry {
	return changelog.ParseEntries(markdown)
}

func newChangelogEntries(entries []changelogEntry, lastVersion string) []changelogEntry {
	return changelog.NewEntries(entries, lastVersion)
}

func changelogEntriesMarkdown(entries []changelogEntry) string {
	return changelog.EntriesMarkdown(entries)
}

func allChangelogEntriesMarkdown(markdown string) string {
	entries := parseChangelogEntries(markdown)
	slices.Reverse(entries)
	return changelogEntriesMarkdown(entries)
}

func normalizeChangelogLinks(markdown, version string) string {
	return changelog.NormalizeLinks(markdown, version)
}

func firstChangelogVersion(markdown string) string {
	return changelog.FirstVersion(markdown)
}

func displayPackageVersion(version string) string {
	return changelog.DisplayVersion(version)
}
