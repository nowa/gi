// Package jsonutil contains the coding-agent JSON-with-comments compatibility
// boundary. It intentionally supports only Pi's line comments and trailing
// commas rather than accepting a broader JSON5 grammar.
package jsonutil

import "strings"

// StripCommentsAndTrailingCommas removes // line comments and commas directly
// before an object or array close. JSON string contents are copied byte for
// byte, including comment markers and comma-close sequences.
func StripCommentsAndTrailingCommas(input string) string {
	return stripTrailingCommas(stripLineComments(input))
}

func stripLineComments(input string) string {
	var output strings.Builder
	output.Grow(len(input))
	inString := false
	escaped := false
	for index := 0; index < len(input); index++ {
		current := input[index]
		if inString {
			output.WriteByte(current)
			switch {
			case escaped:
				escaped = false
			case current == '\\':
				escaped = true
			case current == '"':
				inString = false
			}
			continue
		}
		switch {
		case current == '"':
			inString = true
			output.WriteByte(current)
		case current == '/' &&
			index+1 < len(input) &&
			input[index+1] == '/':
			for index < len(input) &&
				input[index] != '\n' {
				index++
			}
			if index < len(input) {
				output.WriteByte('\n')
			}
		default:
			output.WriteByte(current)
		}
	}
	return output.String()
}

func stripTrailingCommas(input string) string {
	var output strings.Builder
	output.Grow(len(input))
	inString := false
	escaped := false
	for index := 0; index < len(input); index++ {
		current := input[index]
		if inString {
			output.WriteByte(current)
			switch {
			case escaped:
				escaped = false
			case current == '\\':
				escaped = true
			case current == '"':
				inString = false
			}
			continue
		}
		if current == '"' {
			inString = true
			output.WriteByte(current)
			continue
		}
		if current == ',' {
			next := index + 1
			for next < len(input) &&
				isJSONWhitespace(input[next]) {
				next++
			}
			if next < len(input) &&
				(input[next] == '}' || input[next] == ']') {
				continue
			}
		}
		output.WriteByte(current)
	}
	return output.String()
}

func isJSONWhitespace(value byte) bool {
	switch value {
	case ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}
