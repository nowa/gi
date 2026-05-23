package gicodingagent

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type GrepTool struct {
	cwd string
}

type GrepToolInput struct {
	Pattern    string
	Path       string
	Glob       string
	IgnoreCase bool
	Literal    bool
	Limit      int
	Context    int
}

type FindTool struct {
	cwd string
}

type FindToolInput struct {
	Pattern string
	Path    string
	Limit   int
}

type LsTool struct {
	cwd string
}

type LsToolInput struct {
	Path  string
	Limit int
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
	matchesLine, err := newGrepMatcher(input)
	if err != nil {
		return FileToolResult{}, err
	}
	var files []string
	info, err := os.Stat(root)
	if err != nil {
		return FileToolResult{}, err
	}
	if info.IsDir() {
		ignoreSet := newScopedGitignoreSet(root)
		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if ignoreSet.Ignored(path, true) && path != root {
					return filepath.SkipDir
				}
				ignoreSet.LoadDir(path)
				return nil
			}
			if ignoreSet.Ignored(path, false) {
				return nil
			}
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			relative = filepath.ToSlash(relative)
			if input.Glob != "" && !findGlobMatch(input.Glob, relative) {
				return nil
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
			if !matchesLine(line) {
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
	details := &FileToolDetails{}
	if limitReached {
		lines = append(lines, fmt.Sprintf("[%d matches limit reached. Use limit=%d for more, or refine pattern]", limit, limit+1))
		details.MatchLimitReached = limit
	}
	text := strings.Join(lines, "\n")
	if !limitReached {
		details = nil
	}
	return FileToolResult{Text: text, Content: []llm.ContentPart{llm.Text(text)}, Details: details}, nil
}

func newGrepMatcher(input GrepToolInput) (func(string) bool, error) {
	pattern := input.Pattern
	if input.Literal {
		if input.IgnoreCase {
			loweredPattern := strings.ToLower(pattern)
			return func(line string) bool { return strings.Contains(strings.ToLower(line), loweredPattern) }, nil
		}
		return func(line string) bool { return strings.Contains(line, pattern) }, nil
	}
	if input.IgnoreCase {
		pattern = "(?i)" + pattern
	}
	expression, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("error parsing grep pattern: %w", err)
	}
	return expression.MatchString, nil
}

func (t FindTool) Execute(_ string, input FindToolInput) (FileToolResult, error) {
	root := ResolveToCwd(firstNonEmptyString(input.Path, "."), t.cwd)
	limit := input.Limit
	if limit <= 0 {
		limit = 1000
	}
	if err := validateFindGlob(input.Pattern); err != nil {
		return FileToolResult{}, fmt.Errorf("error parsing glob: %w", err)
	}
	ignoreSet := newScopedGitignoreSet(root)
	var matches []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if ignoreSet.Ignored(path, true) && path != root {
				return filepath.SkipDir
			}
			ignoreSet.LoadDir(path)
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if ignoreSet.Ignored(path, false) {
			return nil
		}
		if findGlobMatch(input.Pattern, relative) {
			matches = append(matches, relative)
		}
		return nil
	})
	if err != nil {
		return FileToolResult{}, err
	}
	sort.Strings(matches)
	limitReached := len(matches) > limit
	if limitReached {
		matches = matches[:limit]
	}
	text := strings.Join(matches, "\n")
	if text == "" {
		text = "No files found matching pattern"
	}
	var details *FileToolDetails
	if limitReached {
		text += fmt.Sprintf("\n\n[%d results limit reached. Use limit=%d for more, or refine pattern]", limit, limit*2)
		details = &FileToolDetails{ResultLimitReached: limit}
	}
	return FileToolResult{Text: text, Content: []llm.ContentPart{llm.Text(text)}, Details: details}, nil
}

func (t LsTool) Execute(_ string, input LsToolInput) (FileToolResult, error) {
	root := ResolveToCwd(firstNonEmptyString(input.Path, "."), t.cwd)
	limit := input.Limit
	if limit <= 0 {
		limit = 500
	}
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
	sort.Slice(lines, func(i, j int) bool {
		left := strings.ToLower(lines[i])
		right := strings.ToLower(lines[j])
		if left == right {
			return lines[i] < lines[j]
		}
		return left < right
	})
	limitReached := len(lines) > limit
	if limitReached {
		lines = lines[:limit]
	}
	text := strings.Join(lines, "\n")
	if text == "" {
		text = "(empty directory)"
	}
	var details *FileToolDetails
	if limitReached {
		text += fmt.Sprintf("\n\n[%d entries limit reached. Use limit=%d for more]", limit, limit*2)
		details = &FileToolDetails{EntryLimitReached: limit}
	}
	return FileToolResult{Text: text, Content: []llm.ContentPart{llm.Text(text)}, Details: details}, nil
}

func minSearchInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type scopedGitignoreSet struct {
	root  string
	rules []scopedGitignoreRule
}

type scopedGitignoreRule struct {
	baseRel string
	pattern string
}

func newScopedGitignoreSet(root string) *scopedGitignoreSet {
	return &scopedGitignoreSet{root: filepath.Clean(root)}
}

func (s *scopedGitignoreSet) LoadDir(dir string) {
	if s == nil {
		return
	}
	content, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		return
	}
	baseRel, err := filepath.Rel(s.root, dir)
	if err != nil {
		return
	}
	baseRel = filepath.ToSlash(baseRel)
	if baseRel == "." {
		baseRel = ""
	}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		s.rules = append(s.rules, scopedGitignoreRule{baseRel: baseRel, pattern: filepath.ToSlash(line)})
	}
}

func (s *scopedGitignoreSet) Ignored(candidate string, isDir bool) bool {
	if s == nil {
		return false
	}
	relative, err := filepath.Rel(s.root, candidate)
	if err != nil || relative == "." {
		return false
	}
	relative = filepath.ToSlash(relative)
	for _, rule := range s.rules {
		if gitignoreRuleMatches(rule, relative, isDir) {
			return true
		}
	}
	return false
}

func gitignoreRuleMatches(rule scopedGitignoreRule, relative string, isDir bool) bool {
	target := relative
	if rule.baseRel != "" {
		prefix := strings.TrimSuffix(rule.baseRel, "/") + "/"
		if !strings.HasPrefix(relative, prefix) {
			return false
		}
		target = strings.TrimPrefix(relative, prefix)
	}
	pattern := strings.TrimSpace(rule.pattern)
	if pattern == "" {
		return false
	}
	dirOnly := strings.HasSuffix(pattern, "/")
	pattern = strings.Trim(pattern, "/")
	if pattern == "" || (dirOnly && !isDir && !strings.HasPrefix(target, pattern+"/")) {
		return false
	}
	if strings.Contains(pattern, "/") {
		return findGlobMatch(pattern, target)
	}
	matched, err := path.Match(pattern, path.Base(target))
	return err == nil && matched
}

func validateFindGlob(pattern string) error {
	for _, segment := range strings.Split(filepath.ToSlash(pattern), "/") {
		if segment == "" || segment == "**" {
			continue
		}
		if _, err := path.Match(segment, "probe"); err != nil {
			return err
		}
	}
	return nil
}

func findGlobMatch(pattern, relative string) bool {
	pattern = filepath.ToSlash(pattern)
	relative = filepath.ToSlash(relative)
	if !strings.Contains(pattern, "/") {
		matched, err := path.Match(pattern, path.Base(relative))
		return err == nil && matched
	}
	return matchGlobSegments(strings.Split(pattern, "/"), strings.Split(relative, "/"))
}

func matchGlobSegments(patternSegments, pathSegments []string) bool {
	if len(patternSegments) == 0 {
		return len(pathSegments) == 0
	}
	head := patternSegments[0]
	if head == "**" {
		if len(patternSegments) == 1 {
			return true
		}
		if matchGlobSegments(patternSegments[1:], pathSegments) {
			return true
		}
		return len(pathSegments) > 0 && matchGlobSegments(patternSegments, pathSegments[1:])
	}
	if len(pathSegments) == 0 {
		return false
	}
	matched, err := path.Match(head, pathSegments[0])
	return err == nil && matched && matchGlobSegments(patternSegments[1:], pathSegments[1:])
}
