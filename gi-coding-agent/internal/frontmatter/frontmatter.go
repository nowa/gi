package frontmatter

import (
	"fmt"
	"strings"
)

type Result struct {
	Frontmatter map[string]string
	Body        string
}

func Parse(content string) (Result, error) {
	normalized := NormalizeNewlines(content)
	if !strings.HasPrefix(normalized, "---") {
		return Result{Frontmatter: map[string]string{}, Body: normalized}, nil
	}
	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return Result{Frontmatter: map[string]string{}, Body: normalized}, nil
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return Result{Frontmatter: map[string]string{}, Body: normalized}, nil
	}

	values, err := parseSimpleYAML(lines[1:end])
	if err != nil {
		return Result{}, err
	}
	body := strings.TrimSpace(strings.Join(lines[end+1:], "\n"))
	return Result{Frontmatter: values, Body: body}, nil
}

func Strip(content string) string {
	parsed, err := Parse(content)
	if err != nil {
		return NormalizeNewlines(content)
	}
	if len(parsed.Frontmatter) == 0 && parsed.Body == NormalizeNewlines(content) {
		return parsed.Body
	}
	return strings.TrimSpace(parsed.Body)
}

func NormalizeNewlines(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	return strings.ReplaceAll(value, "\r", "\n")
}

func parseSimpleYAML(lines []string) (map[string]string, error) {
	values := map[string]string{}
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if isIndentedLine(line) {
			return nil, fmt.Errorf("invalid YAML frontmatter at line %d, column 1", i+1)
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
		if isBlockValue(value) {
			block, next := collectIndentedBlock(lines, i+1)
			if strings.HasPrefix(value, "|") {
				values[key] = strings.Join(block, "\n") + "\n"
			} else {
				values[key] = foldBlock(block)
			}
			i = next - 1
			continue
		}
		if value == "" {
			_, next := collectIndentedBlock(lines, i+1)
			i = next - 1
			values[key] = ""
			continue
		}
		values[key] = trimScalarQuotes(value)
	}
	return values, nil
}

func isIndentedLine(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}

func isBlockValue(value string) bool {
	return strings.HasPrefix(value, "|") || strings.HasPrefix(value, ">")
}

func collectIndentedBlock(lines []string, start int) ([]string, int) {
	var block []string
	i := start
	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			block = append(block, "")
			i++
			continue
		}
		if !isIndentedLine(line) {
			break
		}
		block = append(block, strings.TrimPrefix(strings.TrimPrefix(line, "  "), "\t"))
		i++
	}
	return trimTrailingEmptyLines(block), i
}

func foldBlock(lines []string) string {
	lines = trimTrailingEmptyLines(lines)
	if len(lines) == 0 {
		return ""
	}
	var paragraphs []string
	var current []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(current) > 0 {
				paragraphs = append(paragraphs, strings.Join(current, " "))
				current = nil
			}
			continue
		}
		current = append(current, trimmed)
	}
	if len(current) > 0 {
		paragraphs = append(paragraphs, strings.Join(current, " "))
	}
	return strings.Join(paragraphs, "\n")
}

func trimTrailingEmptyLines(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func trimScalarQuotes(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}
