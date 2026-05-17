package harness

import (
	"fmt"
	"strings"
)

func parseFrontmatter(content string) (map[string]string, string, error) {
	normalized := strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\r", "\n")
	if !strings.HasPrefix(normalized, "---") {
		return map[string]string{}, normalized, nil
	}
	end := strings.Index(normalized[3:], "\n---")
	if end == -1 {
		return map[string]string{}, normalized, nil
	}
	yamlText := normalized[4 : 3+end]
	body := strings.TrimSpace(normalized[3+end+4:])
	values := map[string]string{}
	for _, line := range strings.Split(yamlText, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			return nil, "", fmt.Errorf("invalid frontmatter line: %s", line)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if strings.Contains(value, "[") && !strings.Contains(value, "]") {
			return nil, "", fmt.Errorf("invalid frontmatter value for %s", key)
		}
		value = strings.Trim(value, `"'`)
		values[key] = value
	}
	return values, body, nil
}

func frontmatterBool(values map[string]string, key string) bool {
	return strings.EqualFold(values[key], "true")
}
