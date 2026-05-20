package gicodingagent

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	llm "github.com/nowa/gi/gi-llm-provider"
)

const (
	defaultReadToolLineLimit = 2000
	defaultReadToolByteLimit = 50 * 1024
)

type FileToolOperations struct {
	Access      func(path string) error
	ReadFile    func(path string) ([]byte, error)
	WriteFile   func(path string, content []byte) error
	MkdirAll    func(path string) error
	ResizeImage func(part llm.ContentPart, options ImageResizeOptions) *ResizedImage
}

type Edit struct {
	OldText string
	NewText string
}

type EditToolInput struct {
	Path  string
	Edits []Edit
}

type WriteToolInput struct {
	Path    string
	Content string
}

type FileToolResult struct {
	Text    string
	Content []llm.ContentPart
	Details *FileToolDetails
}

type FileToolDetails struct {
	Truncation *ReadToolTruncation
	Diff       string
}

type ReadToolTruncation struct {
	Truncated   bool
	TruncatedBy string
	TotalLines  int
	OutputLines int
}

type EditTool struct {
	cwd string
	ops FileToolOperations
}

type WriteTool struct {
	cwd string
	ops FileToolOperations
}

type ReadTool struct {
	cwd              string
	ops              FileToolOperations
	autoResizeImages bool
}

type ReadToolInput struct {
	Path   string
	Offset int
	Limit  int
}

type ReadToolOptions struct {
	AutoResizeImages *bool
}

func NewEditTool(cwd string, operations ...FileToolOperations) EditTool {
	return EditTool{cwd: cwd, ops: normalizeFileToolOperations(operations...)}
}

func NewWriteTool(cwd string, operations ...FileToolOperations) WriteTool {
	return WriteTool{cwd: cwd, ops: normalizeFileToolOperations(operations...)}
}

func NewReadTool(cwd string, operations ...FileToolOperations) ReadTool {
	return NewReadToolWithOptions(cwd, ReadToolOptions{}, operations...)
}

func NewReadToolWithOptions(cwd string, options ReadToolOptions, operations ...FileToolOperations) ReadTool {
	autoResizeImages := true
	if options.AutoResizeImages != nil {
		autoResizeImages = *options.AutoResizeImages
	}
	return ReadTool{cwd: cwd, ops: normalizeFileToolOperations(operations...), autoResizeImages: autoResizeImages}
}

func (t EditTool) Execute(_ string, input EditToolInput) (FileToolResult, error) {
	if strings.TrimSpace(input.Path) == "" {
		return FileToolResult{}, fmt.Errorf("edit path is required")
	}
	if len(input.Edits) == 0 {
		return FileToolResult{}, fmt.Errorf("edits must contain at least one replacement")
	}
	absolutePath := ResolveToCwd(input.Path, t.cwd)
	var result FileToolResult
	err := WithFileMutationQueue(absolutePath, func() error {
		if err := t.ops.Access(absolutePath); err != nil {
			return formatEditAccessError(input.Path, err)
		}
		content, err := t.ops.ReadFile(absolutePath)
		if err != nil {
			return err
		}
		editResult, err := applyEditsAgainstOriginal(string(content), input.Edits)
		if err != nil {
			return err
		}
		if err := t.ops.WriteFile(absolutePath, []byte(editResult.Content)); err != nil {
			return formatEditAccessError(input.Path, err)
		}
		text := "Successfully replaced"
		if len(input.Edits) > 1 {
			text = fmt.Sprintf("Successfully replaced %d block(s)", len(input.Edits))
		}
		result = FileToolResult{
			Text:    text,
			Content: []llm.ContentPart{llm.Text(text)},
			Details: &FileToolDetails{Diff: editResult.Diff},
		}
		return nil
	})
	return result, err
}

func (t WriteTool) Execute(_ string, input WriteToolInput) (FileToolResult, error) {
	if strings.TrimSpace(input.Path) == "" {
		return FileToolResult{}, fmt.Errorf("write path is required")
	}
	absolutePath := ResolveToCwd(input.Path, t.cwd)
	err := WithFileMutationQueue(absolutePath, func() error {
		if err := t.ops.MkdirAll(filepath.Dir(absolutePath)); err != nil {
			return err
		}
		return t.ops.WriteFile(absolutePath, []byte(input.Content))
	})
	if err != nil {
		return FileToolResult{}, err
	}
	text := fmt.Sprintf("Successfully wrote %d bytes to %s", len(input.Content), input.Path)
	return FileToolResult{Text: text, Content: []llm.ContentPart{llm.Text(text)}}, nil
}

func (t ReadTool) Execute(_ string, input ReadToolInput) (FileToolResult, error) {
	if strings.TrimSpace(input.Path) == "" {
		return FileToolResult{}, fmt.Errorf("read path is required")
	}
	absolutePath, err := ResolveReadPath(input.Path, t.cwd)
	if err != nil {
		return FileToolResult{}, fmt.Errorf("could not read file: %s: %w", input.Path, err)
	}
	content, err := t.ops.ReadFile(absolutePath)
	if err != nil {
		return FileToolResult{}, err
	}
	if mimeType := detectSupportedImageMIMEType(content); mimeType != "" {
		imagePart := llm.Image(base64.StdEncoding.EncodeToString(content), mimeType)
		note := fmt.Sprintf("Read image file [%s]", mimeType)
		if t.autoResizeImages {
			resized := t.ops.ResizeImage(imagePart, ImageResizeOptions{})
			if resized == nil {
				return FileToolResult{
					Text:    note + "\n[Image omitted: could not be resized below the inline image size limit.]",
					Content: []llm.ContentPart{llm.Text(note + "\n[Image omitted: could not be resized below the inline image size limit.]")},
				}, nil
			}
			imagePart = llm.Image(resized.Data, resized.MIMEType)
			note = fmt.Sprintf("Read image file [%s]", resized.MIMEType)
			if dimensionNote := FormatDimensionNote(*resized); dimensionNote != "" {
				note += "\n" + dimensionNote
			}
		}
		return FileToolResult{
			Text: note,
			Content: []llm.ContentPart{
				llm.Text(note),
				imagePart,
			},
		}, nil
	}
	text, details, err := formatReadToolTextContent(string(content), input.Offset, input.Limit)
	if err != nil {
		return FileToolResult{}, err
	}
	return FileToolResult{Text: text, Content: []llm.ContentPart{llm.Text(text)}, Details: details}, nil
}

func normalizeFileToolOperations(operations ...FileToolOperations) FileToolOperations {
	ops := FileToolOperations{}
	if len(operations) > 0 {
		ops = operations[0]
	}
	if ops.Access == nil {
		ops.Access = func(path string) error {
			_, err := os.Stat(path)
			return err
		}
	}
	if ops.ReadFile == nil {
		ops.ReadFile = os.ReadFile
	}
	if ops.WriteFile == nil {
		ops.WriteFile = func(path string, content []byte) error {
			return os.WriteFile(path, content, 0o644)
		}
	}
	if ops.MkdirAll == nil {
		ops.MkdirAll = func(path string) error {
			return os.MkdirAll(path, 0o755)
		}
	}
	if ops.ResizeImage == nil {
		ops.ResizeImage = func(part llm.ContentPart, options ImageResizeOptions) *ResizedImage {
			return ResizeImage(part, options)
		}
	}
	return ops
}

type editApplyResult struct {
	Content string
	Diff    string
}

type editRange struct {
	start       int
	end         int
	edit        Edit
	replacement string
}

type editOccurrence struct {
	start int
	end   int
}

func applyEditsAgainstOriginal(content string, edits []Edit) (editApplyResult, error) {
	ranges := make([]editRange, 0, len(edits))
	for _, edit := range edits {
		if edit.OldText == "" {
			return editApplyResult{}, fmt.Errorf("oldText must not be empty")
		}
		occurrences := findEditOccurrences(content, edit.OldText)
		var occurrence editOccurrence
		fuzzy := false
		if len(occurrences) == 0 {
			fuzzyOccurrences := findFuzzyEditOccurrences(content, edit.OldText)
			if len(fuzzyOccurrences) == 0 {
				return editApplyResult{}, fmt.Errorf("Could not find the exact text to replace")
			}
			if len(fuzzyOccurrences) > 1 {
				return editApplyResult{}, fmt.Errorf("Found %d occurrences of oldText; expected exactly one", len(fuzzyOccurrences))
			}
			occurrence = fuzzyOccurrences[0]
			fuzzy = true
		} else {
			if len(occurrences) > 1 {
				return editApplyResult{}, fmt.Errorf("Found %d occurrences of oldText; expected exactly one", len(occurrences))
			}
			if strings.Contains(edit.OldText, "\n") {
				if fuzzyOccurrences := findFuzzyEditOccurrences(content, edit.OldText); len(fuzzyOccurrences) > 1 {
					return editApplyResult{}, fmt.Errorf("Found %d occurrences of oldText; expected exactly one", len(fuzzyOccurrences))
				}
			}
			occurrence = editOccurrence{start: occurrences[0], end: occurrences[0] + len(edit.OldText)}
		}
		replacement := edit.NewText
		start := occurrence.start
		end := occurrence.end
		if fuzzy {
			if strings.Contains(content[start:end], "\r\n") {
				replacement = strings.ReplaceAll(replacement, "\n", "\r\n")
			}
		}
		ranges = append(ranges, editRange{start: start, end: end, edit: edit, replacement: replacement})
	}
	sortEditRanges(ranges)
	for i := 1; i < len(ranges); i++ {
		if ranges[i].start < ranges[i-1].end {
			return editApplyResult{}, fmt.Errorf("edit regions overlap")
		}
	}

	var builder strings.Builder
	cursor := 0
	for _, r := range ranges {
		builder.WriteString(content[cursor:r.start])
		builder.WriteString(r.replacement)
		cursor = r.end
	}
	builder.WriteString(content[cursor:])
	return editApplyResult{Content: builder.String(), Diff: buildEditDiff(content, ranges)}, nil
}

func findEditOccurrences(content, oldText string) []int {
	var result []int
	searchFrom := 0
	for {
		index := strings.Index(content[searchFrom:], oldText)
		if index < 0 {
			return result
		}
		start := searchFrom + index
		result = append(result, start)
		searchFrom = start + len(oldText)
	}
}

func sortEditRanges(ranges []editRange) {
	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].start < ranges[j].start
	})
}

func buildEditDiff(content string, ranges []editRange) string {
	var lines []string
	previousEnd := 0
	for index, r := range ranges {
		if index > 0 && r.start-previousEnd > 200 {
			lines = append(lines, "...")
		}
		lines = append(lines, prefixEditDiffLines("-", content[r.start:r.end])...)
		lines = append(lines, prefixEditDiffLines("+", r.replacement)...)
		previousEnd = r.end
	}
	return strings.Join(lines, "\n")
}

func findFuzzyEditOccurrences(content, oldText string) []editOccurrence {
	normalizedContent, contentPositions := normalizeEditText(content)
	normalizedOldText, _ := normalizeEditText(oldText)
	if normalizedOldText == "" {
		return nil
	}
	var ranges []editOccurrence
	searchFrom := 0
	for {
		index := strings.Index(normalizedContent[searchFrom:], normalizedOldText)
		if index < 0 {
			return ranges
		}
		normalizedStart := searchFrom + index
		normalizedEnd := normalizedStart + len(normalizedOldText)
		ranges = append(ranges, editOccurrence{start: contentPositions[normalizedStart], end: contentPositions[normalizedEnd]})
		searchFrom = normalizedEnd
	}
}

func normalizeEditText(input string) (string, []int) {
	var builder strings.Builder
	var positions []int
	var pendingSpaces []int
	appendNormalized := func(value string, originalIndex int) {
		for range []byte(value) {
			positions = append(positions, originalIndex)
		}
		builder.WriteString(value)
	}
	flushSpaces := func() {
		for _, position := range pendingSpaces {
			appendNormalized(" ", position)
		}
		pendingSpaces = nil
	}

	for index := 0; index < len(input); {
		r, size := utf8.DecodeRuneInString(input[index:])
		if r == '\r' && index+size < len(input) && input[index+size] == '\n' {
			pendingSpaces = nil
			appendNormalized("\n", index)
			index += size + 1
			continue
		}
		if r == '\n' {
			pendingSpaces = nil
			appendNormalized("\n", index)
			index += size
			continue
		}
		normalized, keep := normalizeEditRune(r)
		if !keep {
			index += size
			continue
		}
		if normalized == " " || normalized == "\t" {
			pendingSpaces = append(pendingSpaces, index)
			index += size
			continue
		}
		flushSpaces()
		appendNormalized(normalized, index)
		index += size
	}
	flushSpaces()
	positions = append(positions, len(input))
	return builder.String(), positions
}

func normalizeEditRune(r rune) (string, bool) {
	switch r {
	case '\u0301':
		return "", false
	case '\u00a0', '\u202f':
		return " ", true
	case '\u2018', '\u2019':
		return "'", true
	case '\u201c', '\u201d':
		return "\"", true
	case '\u2010', '\u2011', '\u2012', '\u2013', '\u2014', '\u2212':
		return "-", true
	case 'é', 'É':
		return "e", true
	}
	if r >= '\uff01' && r <= '\uff5e' {
		return string(r - 0xfee0), true
	}
	return string(r), true
}

func prefixEditDiffLines(prefix, value string) []string {
	value = strings.TrimSuffix(value, "\n")
	if value == "" {
		return []string{prefix}
	}
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = prefix + " " + line
	}
	return lines
}

func formatEditAccessError(path string, err error) error {
	if os.IsNotExist(err) {
		return fmt.Errorf("Could not edit file: %s. Error code: ENOENT.", path)
	}
	if os.IsPermission(err) {
		return fmt.Errorf("Could not edit file: %s. Error code: EACCES.", path)
	}
	return fmt.Errorf("Could not edit file: %s. Error: %s.", path, err.Error())
}

func formatReadToolTextContent(content string, offset, limit int) (string, *FileToolDetails, error) {
	lines := strings.Split(content, "\n")
	totalLines := len(lines)
	start := 0
	if offset > 0 {
		start = offset - 1
	}
	if start >= totalLines {
		return "", nil, fmt.Errorf("Offset %d is beyond end of file (%d lines total)", offset, totalLines)
	}
	lineLimit := defaultReadToolLineLimit
	explicitLimit := false
	if limit > 0 {
		lineLimit = limit
		explicitLimit = true
	}

	capacity := lineLimit
	if remaining := totalLines - start; remaining < capacity {
		capacity = remaining
	}
	outputLines := make([]string, 0, capacity)
	outputBytes := 0
	truncatedBy := ""
	nextOffset := 0
	for index := start; index < totalLines; index++ {
		if len(outputLines) >= lineLimit {
			truncatedBy = "lines"
			if explicitLimit {
				truncatedBy = "limit"
			}
			nextOffset = index + 1
			break
		}
		line := lines[index]
		addedBytes := len(line)
		if len(outputLines) > 0 {
			addedBytes++
		}
		if outputBytes+addedBytes > defaultReadToolByteLimit && len(outputLines) > 0 {
			truncatedBy = "bytes"
			nextOffset = index + 1
			break
		}
		outputLines = append(outputLines, line)
		outputBytes += addedBytes
	}

	output := strings.Join(outputLines, "\n")
	if truncatedBy == "" {
		return output, nil, nil
	}

	displayStart := start + 1
	displayEnd := start + len(outputLines)
	remaining := totalLines - displayEnd
	if explicitLimit {
		output += fmt.Sprintf("\n[%d more lines in file. Use offset=%d to continue.]", remaining, nextOffset)
	} else if truncatedBy == "bytes" {
		output += fmt.Sprintf("\n[Showing lines %d-%d of %d (byte limit). Use offset=%d to continue.]", displayStart, displayEnd, totalLines, nextOffset)
	} else {
		output += fmt.Sprintf("\n[Showing lines %d-%d of %d. Use offset=%d to continue.]", displayStart, displayEnd, totalLines, nextOffset)
	}
	return output, &FileToolDetails{Truncation: &ReadToolTruncation{
		Truncated:   true,
		TruncatedBy: truncatedBy,
		TotalLines:  totalLines,
		OutputLines: len(outputLines),
	}}, nil
}

func detectSupportedImageMIMEType(content []byte) string {
	if len(content) >= 8 && bytes.Equal(content[:8], []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}) {
		return "image/png"
	}
	if len(content) >= 3 && content[0] == 0xff && content[1] == 0xd8 && content[2] == 0xff {
		return "image/jpeg"
	}
	if len(content) >= 6 && (string(content[:6]) == "GIF87a" || string(content[:6]) == "GIF89a") {
		return "image/gif"
	}
	if len(content) >= 12 && string(content[:4]) == "RIFF" && string(content[8:12]) == "WEBP" {
		return "image/webp"
	}
	return ""
}

func imageMIMETypeForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return ""
	}
}
