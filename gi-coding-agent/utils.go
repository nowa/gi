package gicodingagent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

type FrontmatterResult struct {
	Frontmatter map[string]string
	Body        string
}

func ParseFrontmatter(content string) (FrontmatterResult, error) {
	normalized := normalizeNewlines(content)
	if !strings.HasPrefix(normalized, "---") {
		return FrontmatterResult{Frontmatter: map[string]string{}, Body: normalized}, nil
	}
	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return FrontmatterResult{Frontmatter: map[string]string{}, Body: normalized}, nil
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return FrontmatterResult{Frontmatter: map[string]string{}, Body: normalized}, nil
	}

	values, err := parseSimpleYAMLFrontmatter(lines[1:end])
	if err != nil {
		return FrontmatterResult{}, err
	}
	body := strings.TrimSpace(strings.Join(lines[end+1:], "\n"))
	return FrontmatterResult{Frontmatter: values, Body: body}, nil
}

func StripFrontmatter(content string) string {
	parsed, err := ParseFrontmatter(content)
	if err != nil {
		return normalizeNewlines(content)
	}
	if len(parsed.Frontmatter) == 0 && parsed.Body == normalizeNewlines(content) {
		return parsed.Body
	}
	return strings.TrimSpace(parsed.Body)
}

func parseSimpleYAMLFrontmatter(lines []string) (map[string]string, error) {
	values := map[string]string{}
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid YAML frontmatter at line %d, column 1", i+1)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			return nil, fmt.Errorf("invalid YAML frontmatter at line %d, column 1", i+1)
		}
		if strings.HasPrefix(value, "[") && !strings.Contains(value, "]") {
			return nil, fmt.Errorf("invalid YAML frontmatter at line %d, column %d", i+1, len(trimmed)+1)
		}
		if value == "|" {
			var block []string
			for i+1 < len(lines) {
				next := lines[i+1]
				if strings.TrimSpace(next) == "" {
					block = append(block, "")
					i++
					continue
				}
				if !strings.HasPrefix(next, " ") && !strings.HasPrefix(next, "\t") {
					break
				}
				block = append(block, strings.TrimPrefix(strings.TrimPrefix(next, "  "), "\t"))
				i++
			}
			values[key] = strings.Join(block, "\n") + "\n"
			continue
		}
		values[key] = trimYAMLScalarQuotes(value)
	}
	return values, nil
}

func trimYAMLScalarQuotes(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func normalizeNewlines(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	return strings.ReplaceAll(value, "\r", "\n")
}

func ExpandPath(path string) string {
	path = normalizeUserPathText(path)
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
	targetExact := comparableUserPathTextWithCase(base)
	for _, entry := range entries {
		if comparableUserPathTextWithCase(entry.Name()) == targetExact {
			return filepath.Join(dir, entry.Name()), nil
		}
	}
	target := comparableUserPathTextFolded(base)
	for _, entry := range entries {
		if comparableUserPathTextFolded(entry.Name()) == target {
			return filepath.Join(dir, entry.Name()), nil
		}
	}
	return resolved, os.ErrNotExist
}

func normalizeUserPathText(value string) string {
	replacer := strings.NewReplacer("\u00a0", " ", "\u202f", " ")
	return replacer.Replace(value)
}

func comparableUserPathText(value string) string {
	return comparableUserPathTextFolded(value)
}

func comparableUserPathTextWithCase(value string) string {
	value = normalizeUserPathText(value)
	value = strings.NewReplacer("\u2018", "'", "\u2019", "'", "\u00e9", "e").Replace(value)
	value = strings.Map(func(r rune) rune {
		if r == '\u0301' {
			return -1
		}
		return r
	}, value)
	return value
}

func comparableUserPathTextFolded(value string) string {
	return strings.Map(func(r rune) rune {
		return unicode.ToLower(r)
	}, comparableUserPathTextWithCase(value))
}

var ansiOSCStripPattern = regexp.MustCompile(`\x1b\][\s\S]*?(?:\x07|\x1b\\|\x{9c})`)

var ansiCSIStripPattern = func() *regexp.Regexp {
	re := regexp.MustCompile(`[\x1b\x{9b}][\[\]()#;?]*(?:[0-9]{1,4}(?:[;:][0-9]{0,4})*)?[0-9A-PR-TZcf-nq-uy=><~]`)
	re.Longest()
	return re
}()

func StripAnsi(value string) string {
	if !strings.Contains(value, "\x1b") && !strings.Contains(value, "\x9b") {
		return value
	}
	value = ansiOSCStripPattern.ReplaceAllString(value, "")
	return ansiCSIStripPattern.ReplaceAllString(value, "")
}
