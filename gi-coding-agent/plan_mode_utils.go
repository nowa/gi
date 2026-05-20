package gicodingagent

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type PlanTodoItem struct {
	Step      int
	Text      string
	Completed bool
}

var (
	planMarkdownCodePattern   = regexp.MustCompile("`([^`]+)`")
	planMarkdownStrongPattern = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	planMarkdownEmPattern     = regexp.MustCompile(`\*([^*]+)\*`)
	planWhitespacePattern     = regexp.MustCompile(`\s+`)
	planItemPattern           = regexp.MustCompile(`^\s*(\d+)[.)]\s+(.+?)\s*$`)
	planDonePattern           = regexp.MustCompile(`(?i)\[DONE:(\d+)\]`)
)

func IsSafePlanCommand(command string) bool {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" || strings.Contains(trimmed, ">") {
		return false
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "ls", "cat", "head", "tail", "grep", "find", "pwd", "echo", "wc", "du", "df":
		return true
	case "git":
		return len(fields) >= 2 && containsString([]string{"status", "log", "diff", "branch"}, fields[1])
	case "npm":
		return len(fields) >= 2 && containsString([]string{"list", "outdated"}, fields[1])
	case "yarn":
		return len(fields) >= 2 && fields[1] == "info"
	default:
		return false
	}
}

func CleanPlanStepText(text string) string {
	cleaned := strings.TrimSpace(text)
	cleaned = planMarkdownStrongPattern.ReplaceAllString(cleaned, "$1")
	cleaned = planMarkdownEmPattern.ReplaceAllString(cleaned, "$1")
	cleaned = planMarkdownCodePattern.ReplaceAllString(cleaned, "$1")
	cleaned = planWhitespacePattern.ReplaceAllString(cleaned, " ")
	words := strings.Fields(cleaned)
	for len(words) > 0 && isPlanActionWord(words[0]) {
		words = words[1:]
	}
	if len(words) > 0 && strings.EqualFold(words[0], "the") {
		words = words[1:]
	}
	cleaned = strings.Join(words, " ")
	cleaned = capitalizeFirst(cleaned)
	if len([]rune(cleaned)) > 50 {
		runes := []rune(cleaned)
		cleaned = string(runes[:47]) + "..."
	}
	return cleaned
}

func ExtractPlanTodoItems(message string) []PlanTodoItem {
	lines := strings.Split(message, "\n")
	inPlan := false
	var items []PlanTodoItem
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		header := strings.Trim(trimmed, "*")
		if strings.EqualFold(header, "Plan:") {
			inPlan = true
			continue
		}
		if !inPlan {
			continue
		}
		match := planItemPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		rawText := strings.TrimSpace(match[2])
		if strings.HasPrefix(rawText, "`") && strings.HasSuffix(rawText, "`") {
			continue
		}
		cleaned := CleanPlanStepText(rawText)
		if len([]rune(cleaned)) < 4 {
			continue
		}
		step, _ := strconv.Atoi(match[1])
		items = append(items, PlanTodoItem{Step: step, Text: cleaned})
	}
	return items
}

func ExtractDoneSteps(message string) []int {
	matches := planDonePattern.FindAllStringSubmatch(message, -1)
	steps := make([]int, 0, len(matches))
	for _, match := range matches {
		step, err := strconv.Atoi(match[1])
		if err == nil {
			steps = append(steps, step)
		}
	}
	return steps
}

func MarkCompletedPlanSteps(message string, items []PlanTodoItem) int {
	steps := ExtractDoneSteps(message)
	for _, step := range steps {
		for index := range items {
			if items[index].Step == step {
				items[index].Completed = true
			}
		}
	}
	return len(steps)
}

func isPlanActionWord(word string) bool {
	switch strings.ToLower(strings.Trim(word, ":")) {
	case "create", "run", "check", "update":
		return true
	default:
		return false
	}
}

func capitalizeFirst(text string) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return ""
	}
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
