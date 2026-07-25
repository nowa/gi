package pathutil

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

func ExpandPath(path string) string {
	path = strings.TrimPrefix(NormalizeUserPathText(path), "@")
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func ResolveToCwd(path, cwd string) string {
	path = ExpandPath(path)
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(cwd, path))
}

func ResolveReadPath(path, cwd string) (string, error) {
	resolved := ResolveToCwd(path, cwd)
	if _, err := os.Stat(resolved); err == nil {
		return resolved, nil
	}
	dir := filepath.Dir(resolved)
	base := filepath.Base(resolved)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return resolved, err
	}
	targetExact := ComparableUserPathTextWithCase(base)
	for _, entry := range entries {
		if ComparableUserPathTextWithCase(entry.Name()) == targetExact {
			return filepath.Join(dir, entry.Name()), nil
		}
	}
	target := ComparableUserPathTextFolded(base)
	for _, entry := range entries {
		if ComparableUserPathTextFolded(entry.Name()) == target {
			return filepath.Join(dir, entry.Name()), nil
		}
	}
	return resolved, os.ErrNotExist
}

func CanonicalizePath(path string) string {
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	return realPath
}

func GetCwdRelativePath(path, cwd string) (string, bool) {
	resolvedCwd, err := filepath.Abs(cwd)
	if err != nil {
		resolvedCwd = filepath.Clean(cwd)
	}
	var resolvedPath string
	if filepath.IsAbs(path) {
		resolvedPath = filepath.Clean(path)
	} else {
		resolvedPath = filepath.Clean(filepath.Join(resolvedCwd, path))
	}
	relativePath, err := filepath.Rel(resolvedCwd, resolvedPath)
	if err != nil {
		return "", false
	}
	if relativePath == "" {
		return ".", true
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) || filepath.IsAbs(relativePath) {
		return "", false
	}
	return relativePath, true
}

func IsLocalPath(value string) bool {
	trimmed := strings.TrimSpace(value)
	for _, prefix := range []string{"npm:", "git:", "github:", "http:", "https:", "ssh:"} {
		if strings.HasPrefix(trimmed, prefix) {
			return false
		}
	}
	return true
}

func NormalizeUserPathText(value string) string {
	value = strings.Map(func(r rune) rune {
		switch {
		case r == '\u00a0',
			r >= '\u2000' && r <= '\u200a',
			r == '\u202f',
			r == '\u205f',
			r == '\u3000':
			return ' '
		default:
			return r
		}
	}, value)
	return value
}

func ComparableUserPathText(value string) string {
	return ComparableUserPathTextFolded(value)
}

func ComparableUserPathTextWithCase(value string) string {
	value = NormalizeUserPathText(value)
	value = strings.NewReplacer("\u2018", "'", "\u2019", "'", "\u00e9", "e").Replace(value)
	value = strings.Map(func(r rune) rune {
		if r == '\u0301' {
			return -1
		}
		return r
	}, value)
	return value
}

func ComparableUserPathTextFolded(value string) string {
	return strings.Map(func(r rune) rune {
		return unicode.ToLower(r)
	}, ComparableUserPathTextWithCase(value))
}
