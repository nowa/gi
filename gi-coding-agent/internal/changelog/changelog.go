package changelog

import (
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/nowa/gi/gi-coding-agent/internal/versioncheck"
)

const (
	changelogRepositoryURL = "https://github.com/nowa/gi"
	changelogLinkBasePath  = "gi-coding-agent"
	upstreamRepositoryURL  = "https://github.com/earendil-works/pi"
)

type Entry struct {
	Version string
	Content string
}

type localLinkTarget struct {
	fragment string
	pathPart string
	query    string
}

var (
	headingPattern            = regexp.MustCompile(`^##\s+\[?(v?\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)\]?`)
	inlineMarkdownLinkPattern = regexp.MustCompile(`(!?\[[^\]\n]+\]\()([^\s)]+)((\s+[^)]*)?\))`)
	urlSchemePattern          = regexp.MustCompile(`(?i)^[a-z][a-z0-9+.-]*:`)
)

func entryVersion(entry Entry) string {
	return strings.TrimSpace(entry.Version)
}

func normalizeTag(version string) string {
	version = strings.TrimSpace(version)
	if strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}

func splitLocalTarget(target string) localLinkTarget {
	beforeHash, fragment, found := strings.Cut(target, "#")
	if found {
		fragment = "#" + fragment
	} else {
		beforeHash = target
		fragment = ""
	}
	pathPart, query, found := strings.Cut(beforeHash, "?")
	if found {
		query = "?" + query
	} else {
		pathPart = beforeHash
		query = ""
	}
	return localLinkTarget{fragment: fragment, pathPart: pathPart, query: query}
}

func normalizePathPart(value string) string {
	return strings.ReplaceAll(value, `\`, "/")
}

func resolveRepositoryPath(targetPath string) (string, bool) {
	normalizedTarget := normalizePathPart(targetPath)
	var repositoryPath string
	if strings.HasPrefix(normalizedTarget, "/") {
		repositoryPath = path.Clean(strings.TrimLeft(normalizedTarget, "/"))
	} else {
		repositoryPath = path.Clean(path.Join(changelogLinkBasePath, normalizedTarget))
	}
	if repositoryPath == "." || repositoryPath == ".." || strings.HasPrefix(repositoryPath, "../") {
		return "", false
	}
	if strings.HasSuffix(normalizedTarget, "/") && !strings.HasSuffix(repositoryPath, "/") {
		repositoryPath += "/"
	}
	return repositoryPath, true
}

func isDirectoryTarget(originalPath, repositoryPath string) bool {
	if strings.HasSuffix(originalPath, "/") {
		return true
	}
	return !strings.Contains(path.Base(repositoryPath), ".")
}

func normalizeChangelogLinkTarget(target, tag string) string {
	canonicalTarget := canonicalizeLegacyRepositoryURL(target)
	for _, repositoryURL := range []string{changelogRepositoryURL, upstreamRepositoryURL} {
		for _, route := range []string{"blob", "tree"} {
			for _, branch := range []string{"main", "master"} {
				prefix := repositoryURL + "/" + route + "/" + branch + "/"
				if strings.HasPrefix(canonicalTarget, prefix) {
					canonicalTarget = repositoryURL + "/" + route + "/" + tag + "/" + strings.TrimPrefix(canonicalTarget, prefix)
				}
			}
		}
	}

	if strings.HasPrefix(canonicalTarget, "#") ||
		strings.HasPrefix(canonicalTarget, "//") ||
		urlSchemePattern.MatchString(canonicalTarget) {
		return canonicalTarget
	}

	localTarget := splitLocalTarget(canonicalTarget)
	if localTarget.pathPart == "" {
		return canonicalTarget
	}
	repositoryPath, ok := resolveRepositoryPath(localTarget.pathPart)
	if !ok {
		return canonicalTarget
	}
	route := "blob"
	if isDirectoryTarget(localTarget.pathPart, repositoryPath) {
		route = "tree"
	}
	return changelogRepositoryURL + "/" + route + "/" + tag + "/" +
		escapeRepositoryPath(repositoryPath) + localTarget.query + localTarget.fragment
}

// NormalizeLinks converts package-relative changelog links into tag-pinned
// repository URLs while preserving external URLs, anchors, queries, and titles.
func NormalizeLinks(markdown, version string) string {
	tag := normalizeTag(version)
	return inlineMarkdownLinkPattern.ReplaceAllStringFunc(markdown, func(match string) string {
		parts := inlineMarkdownLinkPattern.FindStringSubmatch(match)
		if len(parts) < 4 {
			return match
		}
		return parts[1] + normalizeChangelogLinkTarget(parts[2], tag) + parts[3]
	})
}

func canonicalizeLegacyRepositoryURL(target string) string {
	for _, legacyURL := range []string{
		"https://github.com/badlogic/pi-mono",
		"https://github.com/earendil-works/pi-mono",
	} {
		if target == legacyURL || strings.HasPrefix(target, legacyURL+"/") {
			return upstreamRepositoryURL + strings.TrimPrefix(target, legacyURL)
		}
	}
	return target
}

func escapeRepositoryPath(repositoryPath string) string {
	escaped := (&url.URL{Path: "/" + repositoryPath}).EscapedPath()
	return strings.TrimPrefix(escaped, "/")
}

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
			parts = append(parts, NormalizeLinks(content, entryVersion(entry)))
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
