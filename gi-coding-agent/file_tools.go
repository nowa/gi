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

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
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
	Truncation           *ReadToolTruncation `json:"truncation,omitempty"`
	Diff                 string              `json:"diff,omitempty"`
	FirstChangedLine     int                 `json:"firstChangedLine,omitempty"`
	FullOutputPath       string              `json:"fullOutputPath,omitempty"`
	MatchLimitReached    int                 `json:"matchLimitReached,omitempty"`
	ResultLimitReached   int                 `json:"resultLimitReached,omitempty"`
	EntryLimitReached    int                 `json:"entryLimitReached,omitempty"`
	SearchLinesTruncated bool                `json:"searchLinesTruncated,omitempty"`
}

type EditDiffResult struct {
	Diff  string `json:"diff,omitempty"`
	Error string `json:"error,omitempty"`
}

type ReadToolTruncation struct {
	Truncated             bool   `json:"truncated"`
	TruncatedBy           string `json:"truncatedBy"`
	TotalLines            int    `json:"totalLines"`
	TotalBytes            int    `json:"totalBytes"`
	OutputLines           int    `json:"outputLines"`
	OutputBytes           int    `json:"outputBytes"`
	LastLinePartial       bool   `json:"lastLinePartial"`
	FirstLineExceedsLimit bool   `json:"firstLineExceedsLimit"`
	MaxLines              int    `json:"maxLines"`
	MaxBytes              int    `json:"maxBytes"`
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
			return formatEditAccessError(input.Path, err)
		}
		editResult, err := applyEditsAgainstOriginal(string(content), input.Edits, input.Path)
		if err != nil {
			return err
		}
		if err := t.ops.WriteFile(absolutePath, []byte(editResult.Content)); err != nil {
			return formatEditAccessError(input.Path, err)
		}
		text := fmt.Sprintf("Successfully replaced %d block(s) in %s.", len(input.Edits), input.Path)
		result = FileToolResult{
			Text:    text,
			Content: []llm.ContentPart{llm.Text(text)},
			Details: &FileToolDetails{Diff: editResult.Diff, FirstChangedLine: editResult.FirstChangedLine},
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

func ComputeEditsDiff(path string, edits []Edit, cwd string, operations ...FileToolOperations) EditDiffResult {
	if strings.TrimSpace(path) == "" {
		return EditDiffResult{Error: "edit path is required"}
	}
	if len(edits) == 0 {
		return EditDiffResult{Error: "edits must contain at least one replacement"}
	}
	ops := normalizeFileToolOperations(operations...)
	absolutePath := ResolveToCwd(path, cwd)
	if err := ops.Access(absolutePath); err != nil {
		return EditDiffResult{Error: formatEditAccessError(path, err).Error()}
	}
	content, err := ops.ReadFile(absolutePath)
	if err != nil {
		return EditDiffResult{Error: formatEditAccessError(path, err).Error()}
	}
	editResult, err := applyEditsAgainstOriginal(string(content), edits, path)
	if err != nil {
		return EditDiffResult{Error: err.Error()}
	}
	return EditDiffResult{Diff: editResult.Diff}
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
	text, details, err := formatReadToolTextContent(string(content), input.Path, input.Offset, input.Limit)
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
	Content          string
	Diff             string
	FirstChangedLine int
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

type editRangeLineInfo struct {
	editRange
	oldStartLine int
	oldEndLine   int
	newStartLine int
	newEndLine   int
}

type editDiffHunk struct {
	startLine int
	endLine   int
	ranges    []editRangeLineInfo
}

func applyEditsAgainstOriginal(content string, edits []Edit, path string) (editApplyResult, error) {
	bom, matchContent := splitUTF8BOM(content)
	ranges := make([]editRange, 0, len(edits))
	for index, edit := range edits {
		if edit.OldText == "" {
			return editApplyResult{}, editEmptyOldTextError(path, index, len(edits))
		}
		occurrences := findEditOccurrences(matchContent, edit.OldText)
		var occurrence editOccurrence
		fuzzy := false
		if len(occurrences) == 0 {
			fuzzyOccurrences := findFuzzyEditOccurrences(matchContent, edit.OldText)
			if len(fuzzyOccurrences) == 0 {
				return editApplyResult{}, editNotFoundError(path, index, len(edits))
			}
			if len(fuzzyOccurrences) > 1 {
				return editApplyResult{}, editDuplicateError(path, index, len(edits), len(fuzzyOccurrences))
			}
			occurrence = fuzzyOccurrences[0]
			fuzzy = true
		} else {
			if len(occurrences) > 1 {
				return editApplyResult{}, editDuplicateError(path, index, len(edits), len(occurrences))
			}
			if fuzzyOccurrences := findFuzzyEditOccurrences(matchContent, edit.OldText); len(fuzzyOccurrences) > 1 {
				return editApplyResult{}, editDuplicateError(path, index, len(edits), len(fuzzyOccurrences))
			}
			occurrence = editOccurrence{start: occurrences[0], end: occurrences[0] + len(edit.OldText)}
		}
		replacement := edit.NewText
		start := occurrence.start
		end := occurrence.end
		if fuzzy {
			if strings.Contains(matchContent[start:end], "\r\n") {
				replacement = strings.ReplaceAll(replacement, "\n", "\r\n")
			}
		}
		ranges = append(ranges, editRange{start: start, end: end, edit: edit, replacement: replacement})
	}
	sortEditRanges(ranges)
	for i := 1; i < len(ranges); i++ {
		if ranges[i].start < ranges[i-1].end {
			return editApplyResult{}, fmt.Errorf("edits[%d] and edits[%d] overlap in %s. Merge them into one edit or target disjoint regions.", i-1, i, path)
		}
	}

	var builder strings.Builder
	cursor := 0
	for _, r := range ranges {
		builder.WriteString(matchContent[cursor:r.start])
		builder.WriteString(r.replacement)
		cursor = r.end
	}
	builder.WriteString(matchContent[cursor:])
	newContent := builder.String()
	if newContent == matchContent {
		return editApplyResult{}, editNoChangeError(path, len(edits))
	}
	return editApplyResult{
		Content:          bom + newContent,
		Diff:             buildEditDiff(matchContent, newContent, ranges),
		FirstChangedLine: firstChangedLine(matchContent, ranges),
	}, nil
}

func splitUTF8BOM(content string) (string, string) {
	const bom = "\ufeff"
	if strings.HasPrefix(content, bom) {
		return bom, strings.TrimPrefix(content, bom)
	}
	return "", content
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

func buildEditDiff(content, newContent string, ranges []editRange) string {
	const contextLines = 4
	contentLines := splitEditDiffLines(content)
	newContentLines := splitEditDiffLines(newContent)
	totalLines := len(contentLines)
	if totalLines == 0 {
		totalLines = 1
	}
	maxLineNumber := totalLines
	if len(newContentLines) > maxLineNumber {
		maxLineNumber = len(newContentLines)
	}
	lineWidth := len(fmt.Sprintf("%d", maxLineNumber))
	infos := editRangeLineInfos(content, ranges)
	hunks := editDiffHunks(infos, totalLines, contextLines)
	var output []string
	previousEnd := 0
	for _, hunk := range hunks {
		if hunk.startLine > 1 && (previousEnd == 0 || hunk.startLine > previousEnd+1) {
			output = append(output, editDiffEllipsis(lineWidth))
		}
		firstRange := hunk.ranges[0]
		lastRange := hunk.ranges[len(hunk.ranges)-1]
		newStartLine := hunk.startLine + (firstRange.newStartLine - firstRange.oldStartLine)
		newEndLine := hunk.endLine + (lastRange.newEndLine - lastRange.oldEndLine)
		output = append(output, formatEditDiffWindow(
			contentLines,
			newContentLines,
			hunk.startLine,
			hunk.endLine,
			newStartLine,
			newEndLine,
			lineWidth,
		)...)
		previousEnd = hunk.endLine
	}
	if previousEnd > 0 && previousEnd < totalLines {
		output = append(output, editDiffEllipsis(lineWidth))
	}
	return strings.Join(output, "\n")
}

func editRangeLineInfos(content string, ranges []editRange) []editRangeLineInfo {
	infos := make([]editRangeLineInfo, 0, len(ranges))
	lineDelta := 0
	for _, r := range ranges {
		oldStartLine := strings.Count(content[:r.start], "\n") + 1
		oldLineCount := len(splitEditDiffLines(content[r.start:r.end]))
		newLineCount := len(splitEditDiffLines(r.replacement))
		if oldLineCount == 0 {
			oldLineCount = 1
		}
		newStartLine := oldStartLine + lineDelta
		newEndLine := newStartLine + newLineCount - 1
		if newLineCount == 0 {
			newEndLine = newStartLine - 1
		}
		infos = append(infos, editRangeLineInfo{
			editRange:    r,
			oldStartLine: oldStartLine,
			oldEndLine:   oldStartLine + oldLineCount - 1,
			newStartLine: newStartLine,
			newEndLine:   newEndLine,
		})
		lineDelta += newLineCount - oldLineCount
	}
	return infos
}

func editDiffHunks(infos []editRangeLineInfo, totalLines, contextLines int) []editDiffHunk {
	var hunks []editDiffHunk
	for _, info := range infos {
		start := info.oldStartLine - contextLines
		if start < 1 {
			start = 1
		}
		end := info.oldEndLine + contextLines
		if end > totalLines {
			end = totalLines
		}
		if len(hunks) == 0 || start > hunks[len(hunks)-1].endLine+1 {
			hunks = append(hunks, editDiffHunk{startLine: start, endLine: end, ranges: []editRangeLineInfo{info}})
			continue
		}
		last := &hunks[len(hunks)-1]
		if end > last.endLine {
			last.endLine = end
		}
		last.ranges = append(last.ranges, info)
	}
	return hunks
}

func splitEditDiffLines(value string) []string {
	if value == "" {
		return nil
	}
	trimmed := strings.TrimSuffix(value, "\n")
	if trimmed == "" {
		return []string{""}
	}
	return strings.Split(trimmed, "\n")
}

func formatEditDiffWindow(oldLines, newLines []string, oldStartLine, oldEndLine, newStartLine, newEndLine, width int) []string {
	oldWindow := editDiffLineWindow(oldLines, oldStartLine, oldEndLine)
	newWindow := editDiffLineWindow(newLines, newStartLine, newEndLine)
	operations := editDiffLineOperations(oldWindow, newWindow)
	output := make([]string, 0, len(operations))
	oldLine := oldStartLine
	newLine := newStartLine
	for _, operation := range operations {
		switch operation.kind {
		case "equal":
			output = append(output, formatEditDiffLine(" ", oldLine, width, operation.line))
			oldLine++
			newLine++
		case "remove":
			output = append(output, formatEditDiffLine("-", oldLine, width, operation.line))
			oldLine++
		case "add":
			output = append(output, formatEditDiffLine("+", newLine, width, operation.line))
			newLine++
		}
	}
	return output
}

func editDiffLineWindow(lines []string, startLine, endLine int) []string {
	if startLine < 1 {
		startLine = 1
	}
	if endLine < startLine {
		return nil
	}
	start := startLine - 1
	if start >= len(lines) {
		return nil
	}
	end := endLine
	if end > len(lines) {
		end = len(lines)
	}
	return lines[start:end]
}

type editDiffLineOperation struct {
	kind string
	line string
}

func editDiffLineOperations(oldLines, newLines []string) []editDiffLineOperation {
	lcs := make([][]int, len(oldLines)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(newLines)+1)
	}
	for i := len(oldLines) - 1; i >= 0; i-- {
		for j := len(newLines) - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var operations []editDiffLineOperation
	i, j := 0, 0
	for i < len(oldLines) || j < len(newLines) {
		switch {
		case i < len(oldLines) && j < len(newLines) && oldLines[i] == newLines[j]:
			operations = append(operations, editDiffLineOperation{kind: "equal", line: oldLines[i]})
			i++
			j++
		case j < len(newLines) && (i == len(oldLines) || lcs[i][j+1] > lcs[i+1][j]):
			operations = append(operations, editDiffLineOperation{kind: "add", line: newLines[j]})
			j++
		case i < len(oldLines):
			operations = append(operations, editDiffLineOperation{kind: "remove", line: oldLines[i]})
			i++
		default:
			j++
		}
	}
	return operations
}

func formatEditDiffLine(prefix string, lineNumber, width int, line string) string {
	return fmt.Sprintf("%s%*d %s", prefix, width, lineNumber, line)
}

func editDiffEllipsis(width int) string {
	return " " + strings.Repeat(" ", width) + " ..."
}

func firstChangedLine(content string, ranges []editRange) int {
	if len(ranges) == 0 {
		return 0
	}
	return strings.Count(content[:ranges[0].start], "\n") + 1
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
		if r == '\r' {
			pendingSpaces = nil
			appendNormalized("\n", index)
			index += size
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
	case '\u0300', '\u0301', '\u0302', '\u0303', '\u0304', '\u0306', '\u0308', '\u030a', '\u0327':
		return "", false
	case '\u00a0', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006', '\u2007', '\u2008', '\u2009', '\u200a', '\u202f', '\u205f', '\u3000':
		return " ", true
	case '\u2018', '\u2019', '\u201a', '\u201b':
		return "'", true
	case '\u201c', '\u201d', '\u201e', '\u201f':
		return "\"", true
	case '\u2010', '\u2011', '\u2012', '\u2013', '\u2014', '\u2015', '\u2212':
		return "-", true
	case 'é', 'É':
		return "e", true
	}
	if r >= '\uff01' && r <= '\uff5e' {
		return string(r - 0xfee0), true
	}
	return string(r), true
}

func editNotFoundError(path string, editIndex, totalEdits int) error {
	if totalEdits == 1 {
		return fmt.Errorf("Could not find the exact text in %s. The old text must match exactly including all whitespace and newlines.", path)
	}
	return fmt.Errorf("Could not find edits[%d] in %s. The oldText must match exactly including all whitespace and newlines.", editIndex, path)
}

func editDuplicateError(path string, editIndex, totalEdits, occurrences int) error {
	if totalEdits == 1 {
		return fmt.Errorf("Found %d occurrences of the text in %s. The text must be unique. Please provide more context to make it unique.", occurrences, path)
	}
	return fmt.Errorf("Found %d occurrences of edits[%d] in %s. Each oldText must be unique. Please provide more context to make it unique.", occurrences, editIndex, path)
}

func editEmptyOldTextError(path string, editIndex, totalEdits int) error {
	if totalEdits == 1 {
		return fmt.Errorf("oldText must not be empty in %s.", path)
	}
	return fmt.Errorf("edits[%d].oldText must not be empty in %s.", editIndex, path)
}

func editNoChangeError(path string, totalEdits int) error {
	if totalEdits == 1 {
		return fmt.Errorf("No changes made to %s. The replacement produced identical content. This might indicate an issue with special characters or the text not existing as expected.", path)
	}
	return fmt.Errorf("No changes made to %s. The replacements produced identical content.", path)
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

func formatReadToolTextContent(content, path string, offset, limit int) (string, *FileToolDetails, error) {
	lines := strings.Split(content, "\n")
	totalLines := len(lines)
	start := 0
	if offset > 0 {
		start = offset - 1
	}
	if start >= totalLines {
		return "", nil, fmt.Errorf("Offset %d is beyond end of file (%d lines total)", offset, totalLines)
	}

	selectedEnd := totalLines
	userLimitedLines := 0
	if limit > 0 {
		selectedEnd = min(start+limit, totalLines)
		userLimitedLines = selectedEnd - start
	}
	selectedContent := strings.Join(lines[start:selectedEnd], "\n")
	truncation := agentharness.TruncateHead(selectedContent, agentharness.TruncationOptions{
		MaxLines: defaultReadToolLineLimit,
		MaxBytes: defaultReadToolByteLimit,
	})

	if truncation.FirstLineExceedsLimit {
		firstLineSize := formatBashOutputSize(len([]byte(lines[start])))
		output := fmt.Sprintf("[Line %d is %s, exceeds %s limit. Use bash: sed -n '%dp' %s | head -c %d]",
			start+1,
			firstLineSize,
			formatBashOutputSize(defaultReadToolByteLimit),
			start+1,
			path,
			defaultReadToolByteLimit,
		)
		return output, &FileToolDetails{Truncation: readToolTruncationFromHarness(truncation)}, nil
	}

	if truncation.Truncated {
		displayStart := start + 1
		displayEnd := displayStart + truncation.OutputLines - 1
		nextOffset := displayEnd + 1
		output := truncation.Content
		if truncation.TruncatedBy == agentharness.TruncatedByBytes {
			output += fmt.Sprintf("\n\n[Showing lines %d-%d of %d (%s limit). Use offset=%d to continue.]",
				displayStart, displayEnd, totalLines, formatBashOutputSize(defaultReadToolByteLimit), nextOffset)
		} else {
			output += fmt.Sprintf("\n\n[Showing lines %d-%d of %d. Use offset=%d to continue.]",
				displayStart, displayEnd, totalLines, nextOffset)
		}
		return output, &FileToolDetails{Truncation: readToolTruncationFromHarness(truncation)}, nil
	}

	if userLimitedLines > 0 && start+userLimitedLines < totalLines {
		remaining := totalLines - (start + userLimitedLines)
		nextOffset := start + userLimitedLines + 1
		return fmt.Sprintf("%s\n\n[%d more lines in file. Use offset=%d to continue.]", truncation.Content, remaining, nextOffset), nil, nil
	}

	return truncation.Content, nil, nil
}

func readToolTruncationFromHarness(result agentharness.TruncationResult) *ReadToolTruncation {
	return &ReadToolTruncation{
		Truncated:             result.Truncated,
		TruncatedBy:           result.TruncatedBy,
		TotalLines:            result.TotalLines,
		TotalBytes:            result.TotalBytes,
		OutputLines:           result.OutputLines,
		OutputBytes:           result.OutputBytes,
		LastLinePartial:       result.LastLinePartial,
		FirstLineExceedsLimit: result.FirstLineExceedsLimit,
		MaxLines:              result.MaxLines,
		MaxBytes:              result.MaxBytes,
	}
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
