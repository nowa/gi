package gicodingagent

import "github.com/nowa/gi/gi-coding-agent/internal/changelog"

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

func firstChangelogVersion(markdown string) string {
	return changelog.FirstVersion(markdown)
}

func displayPackageVersion(version string) string {
	return changelog.DisplayVersion(version)
}
