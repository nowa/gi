package harness

import "unicode/utf8"

const (
	DefaultMaxLines    = 2000
	DefaultMaxBytes    = 50 * 1024
	GrepMaxLineLength  = 500
	TruncatedByLines   = "lines"
	TruncatedByBytes   = "bytes"
	TruncatedByNothing = ""
)

type TruncationOptions struct {
	MaxLines int
	MaxBytes int
}

type TruncationResult struct {
	Content               string
	Truncated             bool
	TruncatedBy           string
	TotalLines            int
	TotalBytes            int
	OutputLines           int
	OutputBytes           int
	LastLinePartial       bool
	FirstLineExceedsLimit bool
	MaxLines              int
	MaxBytes              int
}

func normalizeTruncationOptions(options TruncationOptions) TruncationOptions {
	if options.MaxLines == 0 && options.MaxBytes == 0 {
		return TruncationOptions{MaxLines: DefaultMaxLines, MaxBytes: DefaultMaxBytes}
	}
	if options.MaxLines == 0 {
		options.MaxLines = DefaultMaxLines
	}
	return options
}

func TruncateHead(content string, options TruncationOptions) TruncationResult {
	options = normalizeTruncationOptions(options)
	totalBytes := len([]byte(content))
	lines := splitLines(content)
	totalLines := len(lines)
	if totalLines <= options.MaxLines && totalBytes <= options.MaxBytes {
		return truncationResult(content, false, TruncatedByNothing, totalLines, totalBytes, totalLines, totalBytes, false, false, options)
	}

	if len([]byte(lines[0])) > options.MaxBytes {
		return truncationResult("", true, TruncatedByBytes, totalLines, totalBytes, 0, 0, false, true, options)
	}

	output := make([]string, 0, min(totalLines, options.MaxLines))
	outputBytes := 0
	truncatedBy := TruncatedByLines
	for i := 0; i < totalLines && i < options.MaxLines; i++ {
		lineBytes := len([]byte(lines[i]))
		if i > 0 {
			lineBytes++
		}
		if outputBytes+lineBytes > options.MaxBytes {
			truncatedBy = TruncatedByBytes
			break
		}
		output = append(output, lines[i])
		outputBytes += lineBytes
	}
	contentOut := joinLines(output)
	return truncationResult(contentOut, true, truncatedBy, totalLines, totalBytes, len(output), len([]byte(contentOut)), false, false, options)
}

func TruncateTail(content string, options TruncationOptions) TruncationResult {
	options = normalizeTruncationOptions(options)
	totalBytes := len([]byte(content))
	lines := splitLines(content)
	totalLines := len(lines)
	if totalLines <= options.MaxLines && totalBytes <= options.MaxBytes {
		return truncationResult(content, false, TruncatedByNothing, totalLines, totalBytes, totalLines, totalBytes, false, false, options)
	}

	output := []string{}
	outputBytes := 0
	truncatedBy := TruncatedByLines
	lastLinePartial := false
	for i := len(lines) - 1; i >= 0 && len(output) < options.MaxLines; i-- {
		lineBytes := len([]byte(lines[i]))
		if len(output) > 0 {
			lineBytes++
		}
		if outputBytes+lineBytes > options.MaxBytes {
			truncatedBy = TruncatedByBytes
			if len(output) == 0 {
				truncated := truncateStringToBytesFromEnd(lines[i], options.MaxBytes)
				output = append([]string{truncated}, output...)
				outputBytes = len([]byte(truncated))
				lastLinePartial = true
			}
			break
		}
		output = append([]string{lines[i]}, output...)
		outputBytes += lineBytes
	}
	contentOut := joinLines(output)
	return truncationResult(contentOut, true, truncatedBy, totalLines, totalBytes, len(output), len([]byte(contentOut)), lastLinePartial, false, options)
}

func TruncateLine(line string, maxChars int) (string, bool) {
	if maxChars == 0 {
		maxChars = GrepMaxLineLength
	}
	runes := []rune(line)
	if len(runes) <= maxChars {
		return line, false
	}
	return string(runes[:maxChars]) + "... [truncated]", true
}

func truncationResult(content string, truncated bool, truncatedBy string, totalLines, totalBytes, outputLines, outputBytes int, lastLinePartial, firstLineExceedsLimit bool, options TruncationOptions) TruncationResult {
	return TruncationResult{
		Content:               content,
		Truncated:             truncated,
		TruncatedBy:           truncatedBy,
		TotalLines:            totalLines,
		TotalBytes:            totalBytes,
		OutputLines:           outputLines,
		OutputBytes:           outputBytes,
		LastLinePartial:       lastLinePartial,
		FirstLineExceedsLimit: firstLineExceedsLimit,
		MaxLines:              options.MaxLines,
		MaxBytes:              options.MaxBytes,
	}
}

func splitLines(s string) []string {
	lines := []string{""}
	start := 0
	for i, r := range s {
		if r == '\n' {
			lines[len(lines)-1] = s[start:i]
			lines = append(lines, "")
			start = i + 1
		}
	}
	lines[len(lines)-1] = s[start:]
	return lines
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	total := 0
	for _, line := range lines {
		total += len(line)
	}
	total += len(lines) - 1
	bytes := make([]byte, 0, total)
	for i, line := range lines {
		if i > 0 {
			bytes = append(bytes, '\n')
		}
		bytes = append(bytes, line...)
	}
	return string(bytes)
}

func truncateStringToBytesFromEnd(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	bytes := []byte(s)
	if len(bytes) <= maxBytes {
		return s
	}
	start := len(bytes) - maxBytes
	for start < len(bytes) && !utf8.RuneStart(bytes[start]) {
		start++
	}
	if start >= len(bytes) {
		return ""
	}
	return string(bytes[start:])
}
