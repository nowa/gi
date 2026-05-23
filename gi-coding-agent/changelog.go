package gicodingagent

import (
	"regexp"
	"strings"
)

type changelogEntry struct {
	Version string
	Content string
}

var changelogHeadingPattern = regexp.MustCompile(`^##\s+\[?(v?\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)\]?`)

func parseChangelogEntries(markdown string) []changelogEntry {
	lines := strings.Split(markdown, "\n")
	entries := []changelogEntry{}
	var currentVersion string
	var currentLines []string
	flush := func() {
		if currentVersion == "" || len(currentLines) == 0 {
			return
		}
		content := strings.TrimSpace(strings.Join(currentLines, "\n"))
		if content != "" {
			entries = append(entries, changelogEntry{Version: currentVersion, Content: content})
		}
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			flush()
			currentVersion = ""
			currentLines = nil
			if match := changelogHeadingPattern.FindStringSubmatch(line); match != nil {
				currentVersion = strings.TrimSpace(match[1])
				currentLines = []string{line}
			}
			continue
		}
		if currentVersion != "" {
			currentLines = append(currentLines, line)
		}
	}
	flush()
	return entries
}

func newChangelogEntries(entries []changelogEntry, lastVersion string) []changelogEntry {
	lastVersion = strings.TrimSpace(lastVersion)
	if lastVersion == "" {
		return nil
	}
	newEntries := make([]changelogEntry, 0, len(entries))
	for _, entry := range entries {
		if comparison, ok := ComparePackageVersions(entry.Version, lastVersion); ok && comparison > 0 {
			newEntries = append(newEntries, entry)
		}
	}
	return newEntries
}

func changelogEntriesMarkdown(entries []changelogEntry) string {
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		content := strings.TrimSpace(entry.Content)
		if content != "" {
			parts = append(parts, content)
		}
	}
	return strings.Join(parts, "\n\n")
}

func firstChangelogVersion(markdown string) string {
	for _, entry := range parseChangelogEntries(markdown) {
		if strings.TrimSpace(entry.Version) != "" {
			return entry.Version
		}
	}
	return ""
}

func displayPackageVersion(version string) string {
	return strings.TrimPrefix(strings.TrimSpace(version), "v")
}
