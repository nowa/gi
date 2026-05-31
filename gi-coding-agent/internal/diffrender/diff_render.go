package diffrender

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type Theme struct {
	Context func(string) string
	Removed func(string) string
	Added   func(string) string
	Inverse func(string) string
}

type parsedDiffLine struct {
	prefix  byte
	lineNum string
	content string
}

type diffWordOp struct {
	kind  string
	value string
}

func Render(diffText string, theme Theme) string {
	if diffText == "" {
		return ""
	}
	lines := strings.Split(diffText, "\n")
	result := make([]string, 0, len(lines))
	for index := 0; index < len(lines); {
		line := lines[index]
		parsed, ok := parseDiffLine(line)
		if !ok {
			result = append(result, style(theme.Context, line))
			index++
			continue
		}
		if parsed.prefix == '-' {
			removed := make([]parsedDiffLine, 0, 1)
			for index < len(lines) {
				candidate, ok := parseDiffLine(lines[index])
				if !ok || candidate.prefix != '-' {
					break
				}
				removed = append(removed, candidate)
				index++
			}
			added := make([]parsedDiffLine, 0, 1)
			for index < len(lines) {
				candidate, ok := parseDiffLine(lines[index])
				if !ok || candidate.prefix != '+' {
					break
				}
				added = append(added, candidate)
				index++
			}
			if len(removed) == 1 && len(added) == 1 {
				removedLine, addedLine := renderIntraLineDiff(replaceDiffTabs(removed[0].content), replaceDiffTabs(added[0].content), theme)
				result = append(result, style(theme.Removed, "-"+removed[0].lineNum+" "+removedLine))
				result = append(result, style(theme.Added, "+"+added[0].lineNum+" "+addedLine))
				continue
			}
			for _, line := range removed {
				result = append(result, style(theme.Removed, "-"+line.lineNum+" "+replaceDiffTabs(line.content)))
			}
			for _, line := range added {
				result = append(result, style(theme.Added, "+"+line.lineNum+" "+replaceDiffTabs(line.content)))
			}
			continue
		}
		if parsed.prefix == '+' {
			result = append(result, style(theme.Added, "+"+parsed.lineNum+" "+replaceDiffTabs(parsed.content)))
			index++
			continue
		}
		result = append(result, style(theme.Context, " "+parsed.lineNum+" "+replaceDiffTabs(parsed.content)))
		index++
	}
	return strings.Join(result, "\n")
}

func style(fn func(string) string, text string) string {
	if fn == nil {
		return text
	}
	return fn(text)
}

func parseDiffLine(line string) (parsedDiffLine, bool) {
	if len(line) < 3 {
		return parsedDiffLine{}, false
	}
	prefix := line[0]
	if prefix != '+' && prefix != '-' && prefix != ' ' {
		return parsedDiffLine{}, false
	}
	rest := line[1:]
	leadingSpaces := 0
	for leadingSpaces < len(rest) && rest[leadingSpaces] == ' ' {
		leadingSpaces++
	}
	digitEnd := leadingSpaces
	for digitEnd < len(rest) && rest[digitEnd] >= '0' && rest[digitEnd] <= '9' {
		digitEnd++
	}
	if digitEnd > leadingSpaces {
		if digitEnd >= len(rest) || rest[digitEnd] != ' ' {
			return parsedDiffLine{}, false
		}
		return parsedDiffLine{
			prefix:  prefix,
			lineNum: rest[:digitEnd],
			content: rest[digitEnd+1:],
		}, true
	}
	if leadingSpaces == 0 {
		return parsedDiffLine{}, false
	}
	return parsedDiffLine{
		prefix:  prefix,
		lineNum: rest[:leadingSpaces-1],
		content: rest[leadingSpaces:],
	}, true
}

func replaceDiffTabs(text string) string {
	return strings.ReplaceAll(text, "\t", "   ")
}

func renderIntraLineDiff(oldContent, newContent string, theme Theme) (string, string) {
	ops := diffWordTokens(tokenizeDiffWords(oldContent), tokenizeDiffWords(newContent))
	var removedLine strings.Builder
	var addedLine strings.Builder
	firstRemoved := true
	firstAdded := true
	for _, op := range ops {
		switch op.kind {
		case "removed":
			value := op.value
			if firstRemoved {
				leading := leadingWhitespace(value)
				value = value[len(leading):]
				removedLine.WriteString(leading)
				firstRemoved = false
			}
			if value != "" {
				removedLine.WriteString(style(theme.Inverse, value))
			}
		case "added":
			value := op.value
			if firstAdded {
				leading := leadingWhitespace(value)
				value = value[len(leading):]
				addedLine.WriteString(leading)
				firstAdded = false
			}
			if value != "" {
				addedLine.WriteString(style(theme.Inverse, value))
			}
		default:
			removedLine.WriteString(op.value)
			addedLine.WriteString(op.value)
		}
	}
	return removedLine.String(), addedLine.String()
}

func tokenizeDiffWords(text string) []string {
	var tokens []string
	for len(text) > 0 {
		first, size := utf8Rune(text)
		class := diffRuneClass(first)
		end := size
		for end < len(text) {
			next, nextSize := utf8Rune(text[end:])
			if diffRuneClass(next) != class {
				break
			}
			end += nextSize
			if class == "punct" {
				break
			}
		}
		tokens = append(tokens, text[:end])
		text = text[end:]
	}
	return tokens
}

func diffRuneClass(r rune) string {
	switch {
	case unicode.IsSpace(r):
		return "space"
	case unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_':
		return "word"
	default:
		return "punct"
	}
}

func utf8Rune(text string) (rune, int) {
	r, size := utf8.DecodeRuneInString(text)
	if size > 0 {
		return r, size
	}
	return 0, 0
}

func diffWordTokens(oldTokens, newTokens []string) []diffWordOp {
	lcs := make([][]int, len(oldTokens)+1)
	for index := range lcs {
		lcs[index] = make([]int, len(newTokens)+1)
	}
	for i := len(oldTokens) - 1; i >= 0; i-- {
		for j := len(newTokens) - 1; j >= 0; j-- {
			if oldTokens[i] == newTokens[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var ops []diffWordOp
	i, j := 0, 0
	for i < len(oldTokens) || j < len(newTokens) {
		switch {
		case i < len(oldTokens) && j < len(newTokens) && oldTokens[i] == newTokens[j]:
			ops = appendDiffWordOp(ops, "equal", oldTokens[i])
			i++
			j++
		case i < len(oldTokens) && (j == len(newTokens) || lcs[i+1][j] >= lcs[i][j+1]):
			ops = appendDiffWordOp(ops, "removed", oldTokens[i])
			i++
		case j < len(newTokens):
			ops = appendDiffWordOp(ops, "added", newTokens[j])
			j++
		}
	}
	return ops
}

func appendDiffWordOp(ops []diffWordOp, kind, value string) []diffWordOp {
	if value == "" {
		return ops
	}
	if len(ops) > 0 && ops[len(ops)-1].kind == kind {
		ops[len(ops)-1].value += value
		return ops
	}
	return append(ops, diffWordOp{kind: kind, value: value})
}

func leadingWhitespace(text string) string {
	for index, r := range text {
		if !unicode.IsSpace(r) {
			return text[:index]
		}
	}
	return text
}
