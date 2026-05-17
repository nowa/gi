package harness

import (
	"path/filepath"
	"strconv"
	"strings"
)

func FormatSkillInvocation(skill Skill, additionalInstructions string) string {
	block := `<skill name="` + skill.Name + `" location="` + skill.FilePath + `">` + "\n" +
		"References are relative to " + filepath.Dir(skill.FilePath) + ".\n\n" +
		skill.Content + "\n</skill>"
	if additionalInstructions == "" {
		return block
	}
	return block + "\n\n" + additionalInstructions
}

func FormatSkillsForSystemPrompt(skills []Skill) string {
	visible := make([]Skill, 0, len(skills))
	for _, skill := range skills {
		if !skill.DisableModelInvocation {
			visible = append(visible, skill)
		}
	}
	if len(visible) == 0 {
		return ""
	}
	lines := []string{
		"The following skills provide specialized instructions for specific tasks.",
		"Read the full skill file when the task matches its description.",
		"When a skill file references a relative path, resolve it against the skill directory (parent of SKILL.md / dirname of the path) and use that absolute path in tool commands.",
		"",
		"<available_skills>",
	}
	for _, skill := range visible {
		lines = append(lines,
			"  <skill>",
			"    <name>"+escapeXML(skill.Name)+"</name>",
			"    <description>"+escapeXML(skill.Description)+"</description>",
			"    <location>"+escapeXML(skill.FilePath)+"</location>",
			"  </skill>",
		)
	}
	lines = append(lines, "</available_skills>")
	return strings.Join(lines, "\n")
}

func FormatPromptTemplateInvocation(template PromptTemplate, args []string) string {
	return SubstituteArgs(template.Content, args)
}

func ParseCommandArgs(input string) []string {
	args := []string{}
	current := strings.Builder{}
	var quote rune
	for _, r := range input {
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case ' ', '\t':
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
}

func SubstituteArgs(content string, args []string) string {
	result := content
	result = replaceDollarNumber(result, args)
	result = replaceAtSlices(result, args)
	all := strings.Join(args, " ")
	result = strings.ReplaceAll(result, "$ARGUMENTS", all)
	result = strings.ReplaceAll(result, "$@", all)
	return result
}

func replaceDollarNumber(content string, args []string) string {
	var out strings.Builder
	for i := 0; i < len(content); i++ {
		if content[i] == '$' && i+1 < len(content) && content[i+1] >= '0' && content[i+1] <= '9' {
			j := i + 1
			for j < len(content) && content[j] >= '0' && content[j] <= '9' {
				j++
			}
			n, _ := strconv.Atoi(content[i+1 : j])
			if n >= 1 && n <= len(args) {
				out.WriteString(args[n-1])
			}
			i = j - 1
			continue
		}
		out.WriteByte(content[i])
	}
	return out.String()
}

func replaceAtSlices(content string, args []string) string {
	var out strings.Builder
	for i := 0; i < len(content); i++ {
		if strings.HasPrefix(content[i:], "${@:") {
			end := strings.IndexByte(content[i:], '}')
			if end >= 0 {
				expr := content[i+4 : i+end]
				parts := strings.Split(expr, ":")
				start, err := strconv.Atoi(parts[0])
				if err == nil {
					if start < 1 {
						start = 1
					}
					from := start - 1
					to := len(args)
					if len(parts) == 2 {
						length, _ := strconv.Atoi(parts[1])
						to = min(len(args), from+length)
					}
					if from < len(args) {
						out.WriteString(strings.Join(args[from:to], " "))
					}
					i += end
					continue
				}
			}
		}
		out.WriteByte(content[i])
	}
	return out.String()
}

func escapeXML(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(value)
}
