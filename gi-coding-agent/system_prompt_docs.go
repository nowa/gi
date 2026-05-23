package gicodingagent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func defaultGiDocumentationPaths(cwd string) []SystemPromptDocumentationPath {
	roots := make([]string, 0, 2)
	if root, ok := findGiDocumentationRoot(cwd); ok {
		roots = append(roots, root)
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		if root, ok := findGiDocumentationRoot(filepath.Dir(file)); ok {
			roots = append(roots, root)
		}
	}
	if len(roots) == 0 {
		return nil
	}
	root := roots[0]
	return []SystemPromptDocumentationPath{
		{Label: "Main documentation", Path: slashPath(filepath.Join(root, "README.md"))},
		{Label: "Protocol overview", Path: slashPath(filepath.Join(root, "protocol", "README.md")), Detail: "extension/package host protocol"},
		{Label: "Extension protocol spec", Path: slashPath(filepath.Join(root, "protocol", "spec", "gi-extension-protocol.md")), Detail: "RPC, ViewTree, capabilities, official packages"},
		{Label: "Pi compatibility status", Path: slashPath(filepath.Join(root, "PI_COMPATIBILITY.md")), Detail: "migration and parity notes"},
	}
}

func findGiDocumentationRoot(start string) (string, bool) {
	if strings.TrimSpace(start) == "" {
		return "", false
	}
	dir, err := filepath.Abs(start)
	if err != nil {
		dir = filepath.Clean(start)
	}
	if stat, err := os.Stat(dir); err == nil && !stat.IsDir() {
		dir = filepath.Dir(dir)
	}
	for {
		goModPath := filepath.Join(dir, "go.mod")
		content, err := os.ReadFile(goModPath)
		if err == nil && strings.Contains(string(content), "module github.com/nowa/gi") {
			if hasGiDocumentationFiles(dir) {
				return dir, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func hasGiDocumentationFiles(root string) bool {
	for _, path := range []string{
		"README.md",
		filepath.Join("protocol", "README.md"),
		filepath.Join("protocol", "spec", "gi-extension-protocol.md"),
		"PI_COMPATIBILITY.md",
	} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			return false
		}
	}
	return true
}

func slashPath(path string) string {
	return filepath.ToSlash(path)
}
