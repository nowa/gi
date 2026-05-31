package gitsource

import (
	"net/url"
	"regexp"
	"strings"
)

type GitSource struct {
	Type   string
	Repo   string
	Host   string
	Path   string
	Ref    string
	Pinned bool
}

var scpLikeGitPattern = regexp.MustCompile(`^git@([^:]+):(.+)$`)

func ParseGitURL(source string) (GitSource, bool) {
	trimmed := strings.TrimSpace(source)
	hasGitPrefix := strings.HasPrefix(trimmed, "git:")
	rawURL := trimmed
	if hasGitPrefix {
		rawURL = strings.TrimSpace(strings.TrimPrefix(trimmed, "git:"))
	}
	lower := strings.ToLower(rawURL)
	if !hasGitPrefix &&
		!strings.HasPrefix(lower, "https://") &&
		!strings.HasPrefix(lower, "http://") &&
		!strings.HasPrefix(lower, "ssh://") &&
		!strings.HasPrefix(lower, "git://") {
		return GitSource{}, false
	}
	return parseGenericGitURL(rawURL)
}

func parseGenericGitURL(rawURL string) (GitSource, bool) {
	repoWithoutRef, ref := splitGitRef(rawURL)
	repo := repoWithoutRef
	host := ""
	path := ""

	if match := scpLikeGitPattern.FindStringSubmatch(repoWithoutRef); match != nil {
		host = match[1]
		path = match[2]
	} else if strings.Contains(repoWithoutRef, "://") {
		parsed, err := url.Parse(repoWithoutRef)
		if err != nil || parsed.Host == "" {
			return GitSource{}, false
		}
		host = parsed.Hostname()
		path = strings.TrimLeft(parsed.EscapedPath(), "/")
		if unescaped, err := url.PathUnescape(path); err == nil {
			path = unescaped
		}
	} else {
		slashIndex := strings.Index(repoWithoutRef, "/")
		if slashIndex < 0 {
			return GitSource{}, false
		}
		host = repoWithoutRef[:slashIndex]
		path = repoWithoutRef[slashIndex+1:]
		if !strings.Contains(host, ".") && host != "localhost" {
			return GitSource{}, false
		}
		repo = "https://" + repoWithoutRef
	}

	normalizedPath := strings.TrimLeft(strings.TrimSuffix(path, ".git"), "/")
	if host == "" || normalizedPath == "" || len(strings.Split(normalizedPath, "/")) < 2 {
		return GitSource{}, false
	}
	return GitSource{
		Type:   "git",
		Repo:   repo,
		Host:   host,
		Path:   normalizedPath,
		Ref:    ref,
		Pinned: ref != "",
	}, true
}

func splitGitRef(rawURL string) (repo string, ref string) {
	if match := scpLikeGitPattern.FindStringSubmatch(rawURL); match != nil {
		pathWithRef := match[2]
		refSeparator := strings.Index(pathWithRef, "@")
		if refSeparator < 0 {
			return rawURL, ""
		}
		repoPath := pathWithRef[:refSeparator]
		ref := pathWithRef[refSeparator+1:]
		if repoPath == "" || ref == "" {
			return rawURL, ""
		}
		return "git@" + match[1] + ":" + repoPath, ref
	}

	if strings.Contains(rawURL, "://") {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return rawURL, ""
		}
		pathWithRef := strings.TrimLeft(parsed.Path, "/")
		refSeparator := strings.Index(pathWithRef, "@")
		if refSeparator < 0 {
			return rawURL, ""
		}
		repoPath := pathWithRef[:refSeparator]
		ref := pathWithRef[refSeparator+1:]
		if repoPath == "" || ref == "" {
			return rawURL, ""
		}
		parsed.Path = "/" + repoPath
		return strings.TrimRight(parsed.String(), "/"), ref
	}

	slashIndex := strings.Index(rawURL, "/")
	if slashIndex < 0 {
		return rawURL, ""
	}
	host := rawURL[:slashIndex]
	pathWithRef := rawURL[slashIndex+1:]
	refSeparator := strings.Index(pathWithRef, "@")
	if refSeparator < 0 {
		return rawURL, ""
	}
	repoPath := pathWithRef[:refSeparator]
	ref = pathWithRef[refSeparator+1:]
	if repoPath == "" || ref == "" {
		return rawURL, ""
	}
	return host + "/" + repoPath, ref
}
