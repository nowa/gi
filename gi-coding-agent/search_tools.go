package gicodingagent

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type GrepTool struct {
	cwd string
}

type GrepToolInput struct {
	Pattern string
	Path    string
	Limit   int
	Context int
}

type FindTool struct {
	cwd string
}

type FindToolInput struct {
	Pattern string
	Path    string
}

type LsTool struct {
	cwd string
}

type LsToolInput struct {
	Path string
}

func NewGrepTool(cwd string) GrepTool { return GrepTool{cwd: cwd} }
func NewFindTool(cwd string) FindTool { return FindTool{cwd: cwd} }
func NewLsTool(cwd string) LsTool     { return LsTool{cwd: cwd} }

func (t GrepTool) Execute(_ string, input GrepToolInput) (FileToolResult, error) {
	root := ResolveToCwd(firstNonEmptyString(input.Path, "."), t.cwd)
	limit := input.Limit
	if limit <= 0 {
		limit = 100
	}
	var files []string
	info, err := os.Stat(root)
	if err != nil {
		return FileToolResult{}, err
	}
	if info.IsDir() {
		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			return FileToolResult{}, err
		}
	} else {
		files = append(files, root)
	}
	sort.Strings(files)

	var lines []string
	matches := 0
	limitReached := false
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		fileLines := strings.Split(string(content), "\n")
		for index, line := range fileLines {
			if !strings.Contains(line, input.Pattern) {
				continue
			}
			if matches >= limit {
				limitReached = true
				break
			}
			name := filepath.Base(file)
			for contextIndex := maxInt(0, index-input.Context); contextIndex < index; contextIndex++ {
				lines = append(lines, fmt.Sprintf("%s-%d- %s", name, contextIndex+1, fileLines[contextIndex]))
			}
			lines = append(lines, fmt.Sprintf("%s:%d: %s", name, index+1, line))
			for contextIndex := index + 1; contextIndex <= minSearchInt(len(fileLines)-1, index+input.Context); contextIndex++ {
				lines = append(lines, fmt.Sprintf("%s-%d- %s", name, contextIndex+1, fileLines[contextIndex]))
			}
			matches++
		}
		if limitReached {
			break
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "No matches found")
	}
	if limitReached {
		lines = append(lines, fmt.Sprintf("[%d matches limit reached. Use limit=%d for more, or refine pattern]", limit, limit+1))
	}
	text := strings.Join(lines, "\n")
	return FileToolResult{Text: text, Content: []llm.ContentPart{llm.Text(text)}}, nil
}

func (t FindTool) Execute(_ string, input FindToolInput) (FileToolResult, error) {
	root := ResolveToCwd(firstNonEmptyString(input.Path, "."), t.cwd)
	if _, err := filepath.Match(input.Pattern, "probe"); err != nil && !strings.Contains(input.Pattern, "**") {
		return FileToolResult{}, fmt.Errorf("error parsing glob: %w", err)
	}
	ignored := readSimpleGitignore(root)
	var matches []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if ignored[filepath.Base(relative)] || ignored[relative] {
			return nil
		}
		if simpleGlobMatch(input.Pattern, relative) {
			matches = append(matches, relative)
		}
		return nil
	})
	if err != nil {
		return FileToolResult{}, err
	}
	sort.Strings(matches)
	text := strings.Join(matches, "\n")
	if text == "" {
		text = "No files found matching pattern"
	}
	return FileToolResult{Text: text, Content: []llm.ContentPart{llm.Text(text)}}, nil
}

func (t LsTool) Execute(_ string, input LsToolInput) (FileToolResult, error) {
	root := ResolveToCwd(firstNonEmptyString(input.Path, "."), t.cwd)
	entries, err := os.ReadDir(root)
	if err != nil {
		return FileToolResult{}, err
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		lines = append(lines, name)
	}
	sort.Strings(lines)
	text := strings.Join(lines, "\n")
	return FileToolResult{Text: text, Content: []llm.ContentPart{llm.Text(text)}}, nil
}

func readSimpleGitignore(root string) map[string]bool {
	content, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		return map[string]bool{}
	}
	result := map[string]bool{}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			result[line] = true
		}
	}
	return result
}

func simpleGlobMatch(pattern, relative string) bool {
	if strings.HasPrefix(pattern, "**/") {
		suffix := strings.TrimPrefix(pattern, "**/")
		if strings.HasPrefix(suffix, "*") {
			return strings.HasSuffix(relative, strings.TrimPrefix(suffix, "*"))
		}
	}
	matched, err := filepath.Match(pattern, filepath.Base(relative))
	return err == nil && matched
}

func minSearchInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
