package harness

import (
	"fmt"
	"strings"
)

func parseFrontmatter(content string) (map[string]string, string, error) {
	normalized := strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return map[string]string{}, normalized, nil
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return map[string]string{}, normalized, nil
	}
	values, err := parseSimpleFrontmatter(lines[1:end])
	if err != nil {
		return nil, "", err
	}
	body := strings.TrimSpace(strings.Join(lines[end+1:], "\n"))
	return values, body, nil
}

func parseSimpleFrontmatter(lines []string) (map[string]string, error) {
	values := map[string]string{}
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if isIndentedFrontmatterLine(line) {
			return nil, fmt.Errorf("invalid frontmatter line: %s", line)
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid frontmatter line: %s", line)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			return nil, fmt.Errorf("invalid frontmatter line: %s", line)
		}
		if strings.Contains(value, "[") && !strings.Contains(value, "]") {
			return nil, fmt.Errorf("invalid frontmatter value for %s", key)
		}
		if isBlockScalarFrontmatterValue(value) {
			block, next := collectIndentedFrontmatterBlock(lines, i+1)
			if strings.HasPrefix(value, "|") {
				values[key] = strings.Join(block, "\n")
			} else {
				values[key] = foldFrontmatterBlock(block)
			}
			i = next - 1
			continue
		}
		if value == "" {
			_, next := collectIndentedFrontmatterBlock(lines, i+1)
			i = next - 1
			values[key] = ""
			continue
		}
		values[key] = strings.Trim(value, `"'`)
	}
	return values, nil
}

func isIndentedFrontmatterLine(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}

func isBlockScalarFrontmatterValue(value string) bool {
	return strings.HasPrefix(value, "|") || strings.HasPrefix(value, ">")
}

func collectIndentedFrontmatterBlock(lines []string, start int) ([]string, int) {
	var block []string
	i := start
	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			block = append(block, "")
			i++
			continue
		}
		if !isIndentedFrontmatterLine(line) {
			break
		}
		block = append(block, strings.TrimPrefix(strings.TrimPrefix(line, "  "), "\t"))
		i++
	}
	return trimTrailingEmptyFrontmatterLines(block), i
}

func foldFrontmatterBlock(lines []string) string {
	lines = trimTrailingEmptyFrontmatterLines(lines)
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

func trimTrailingEmptyFrontmatterLines(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func frontmatterBool(values map[string]string, key string) bool {
	return strings.EqualFold(values[key], "true")
}
