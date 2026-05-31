package changelog

import (
	"regexp"
	"strings"

	"github.com/nowa/gi/gi-coding-agent/internal/versioncheck"
)

type Entry struct {
	Version string
	Content string
}

var headingPattern = regexp.MustCompile(`^##\s+\[?(v?\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)\]?`)

func ParseEntries(markdown string) []Entry {
	lines := strings.Split(markdown, "\n")
	entries := []Entry{}
	var currentVersion string
	var currentLines []string
	flush := func() {
		if currentVersion == "" || len(currentLines) == 0 {
			return
		}
		content := strings.TrimSpace(strings.Join(currentLines, "\n"))
		if content != "" {
			entries = append(entries, Entry{Version: currentVersion, Content: content})
		}
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			flush()
			currentVersion = ""
			currentLines = nil
			if match := headingPattern.FindStringSubmatch(line); match != nil {
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

func NewEntries(entries []Entry, lastVersion string) []Entry {
	lastVersion = strings.TrimSpace(lastVersion)
	if lastVersion == "" {
		return nil
	}
	newEntries := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if comparison, ok := versioncheck.ComparePackageVersions(entry.Version, lastVersion); ok && comparison > 0 {
			newEntries = append(newEntries, entry)
		}
	}
	return newEntries
}

func EntriesMarkdown(entries []Entry) string {
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		content := strings.TrimSpace(entry.Content)
		if content != "" {
			parts = append(parts, content)
		}
	}
	return strings.Join(parts, "\n\n")
}

func FirstVersion(markdown string) string {
	for _, entry := range ParseEntries(markdown) {
		if strings.TrimSpace(entry.Version) != "" {
			return entry.Version
		}
	}
	return ""
}

func DisplayVersion(version string) string {
	return strings.TrimPrefix(strings.TrimSpace(version), "v")
}
