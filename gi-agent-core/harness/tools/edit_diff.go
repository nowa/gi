package tools

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

type Edit struct {
	OldText string `json:"oldText"`
	NewText string `json:"newText"`
}

type AppliedEditsResult struct {
	BaseContent      string
	NewContent       string
	Diff             string
	Patch            string
	FirstChangedLine int
}

type editRange struct {
	start       int
	end         int
	index       int
	replacement string
}

type editOccurrence struct {
	start int
	end   int
}

func DetectLineEnding(content string) string {
	crlfIndex := strings.Index(content, "\r\n")
	lfIndex := strings.Index(content, "\n")
	if lfIndex < 0 || crlfIndex < 0 {
		return "\n"
	}
	if crlfIndex < lfIndex {
		return "\r\n"
	}
	return "\n"
}

func NormalizeToLF(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	return strings.ReplaceAll(content, "\r", "\n")
}

func RestoreLineEndings(content, ending string) string {
	if ending == "\r\n" {
		return strings.ReplaceAll(content, "\n", "\r\n")
	}
	return content
}

func StripBOM(content string) (bom, text string) {
	if strings.HasPrefix(content, "\ufeff") {
		return "\ufeff", strings.TrimPrefix(content, "\ufeff")
	}
	return "", content
}

// NormalizeForFuzzyMatch normalizes the compatibility characters accepted by
// Pi's edit tool and strips trailing whitespace from each line.
func NormalizeForFuzzyMatch(content string) string {
	normalized, _ := normalizeEditText(content)
	return normalized
}

// ApplyEditsToNormalizedContent matches every edit against the same original
// LF-normalized content and applies non-overlapping replacements atomically.
func ApplyEditsToNormalizedContent(content string, edits []Edit, path string) (AppliedEditsResult, error) {
	normalizedEdits := make([]Edit, len(edits))
	for i, edit := range edits {
		normalizedEdits[i] = Edit{
			OldText: NormalizeToLF(edit.OldText),
			NewText: NormalizeToLF(edit.NewText),
		}
		if normalizedEdits[i].OldText == "" {
			return AppliedEditsResult{}, editEmptyOldTextError(path, i, len(edits))
		}
	}

	ranges := make([]editRange, 0, len(normalizedEdits))
	for index, edit := range normalizedEdits {
		exactOccurrences := findExactOccurrences(content, edit.OldText)
		fuzzyOccurrences := findFuzzyOccurrences(content, edit.OldText)
		var occurrence editOccurrence
		switch {
		case len(exactOccurrences) > 0:
			if len(fuzzyOccurrences) > 1 {
				return AppliedEditsResult{}, editDuplicateError(path, index, len(edits), len(fuzzyOccurrences))
			}
			occurrence = exactOccurrences[0]
		case len(fuzzyOccurrences) == 0:
			return AppliedEditsResult{}, editNotFoundError(path, index, len(edits))
		case len(fuzzyOccurrences) > 1:
			return AppliedEditsResult{}, editDuplicateError(path, index, len(edits), len(fuzzyOccurrences))
		default:
			occurrence = fuzzyOccurrences[0]
		}
		ranges = append(ranges, editRange{
			start:       occurrence.start,
			end:         occurrence.end,
			index:       index,
			replacement: edit.NewText,
		})
	}

	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].start < ranges[j].start
	})
	for i := 1; i < len(ranges); i++ {
		if ranges[i].start < ranges[i-1].end {
			return AppliedEditsResult{}, fmt.Errorf(
				"edits[%d] and edits[%d] overlap in %s. Merge them into one edit or target disjoint regions.",
				ranges[i-1].index,
				ranges[i].index,
				path,
			)
		}
	}

	var builder strings.Builder
	cursor := 0
	for _, editRange := range ranges {
		builder.WriteString(content[cursor:editRange.start])
		builder.WriteString(editRange.replacement)
		cursor = editRange.end
	}
	builder.WriteString(content[cursor:])
	newContent := builder.String()
	if newContent == content {
		return AppliedEditsResult{}, editNoChangeError(path, len(edits))
	}
	return AppliedEditsResult{
		BaseContent:      content,
		NewContent:       newContent,
		Diff:             generateDiffStringWithRanges(content, newContent, ranges, 4),
		Patch:            generateUnifiedPatchWithRanges(path, content, newContent, ranges, 4),
		FirstChangedLine: firstChangedLine(content, ranges),
	}, nil
}

func GenerateDiffString(oldContent, newContent string, contextLines ...int) (string, int) {
	contextLineCount := optionalContextLines(contextLines)
	ranges := []editRange{{start: 0, end: len(oldContent), replacement: newContent}}
	return generateDiffStringWithRanges(oldContent, newContent, ranges, contextLineCount), firstChangedLine(oldContent, ranges)
}

func GenerateUnifiedPatch(path, oldContent, newContent string, contextLines ...int) string {
	ranges := []editRange{{start: 0, end: len(oldContent), replacement: newContent}}
	return generateUnifiedPatchWithRanges(path, oldContent, newContent, ranges, optionalContextLines(contextLines))
}

func optionalContextLines(values []int) int {
	if len(values) > 0 && values[0] >= 0 {
		return values[0]
	}
	return 4
}

func findExactOccurrences(content, oldText string) []editOccurrence {
	var result []editOccurrence
	for searchFrom := 0; searchFrom <= len(content)-len(oldText); {
		index := strings.Index(content[searchFrom:], oldText)
		if index < 0 {
			break
		}
		start := searchFrom + index
		result = append(result, editOccurrence{start: start, end: start + len(oldText)})
		searchFrom = start + len(oldText)
	}
	return result
}

func findFuzzyOccurrences(content, oldText string) []editOccurrence {
	normalizedContent, positions := normalizeEditText(content)
	normalizedOldText, _ := normalizeEditText(oldText)
	if normalizedOldText == "" {
		return nil
	}
	var result []editOccurrence
	for searchFrom := 0; searchFrom <= len(normalizedContent)-len(normalizedOldText); {
		index := strings.Index(normalizedContent[searchFrom:], normalizedOldText)
		if index < 0 {
			break
		}
		start := searchFrom + index
		end := start + len(normalizedOldText)
		result = append(result, editOccurrence{start: positions[start], end: positions[end]})
		searchFrom = end
	}
	return result
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
		if r == '\r' || r == '\n' {
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

func editRangeLineInfos(content string, ranges []editRange) []editRangeLineInfo {
	infos := make([]editRangeLineInfo, 0, len(ranges))
	lineDelta := 0
	for _, editRange := range ranges {
		oldStartLine := strings.Count(content[:editRange.start], "\n") + 1
		oldLineCount := len(splitDiffLines(content[editRange.start:editRange.end]))
		newLineCount := len(splitDiffLines(editRange.replacement))
		if oldLineCount == 0 {
			oldLineCount = 1
		}
		newStartLine := oldStartLine + lineDelta
		newEndLine := newStartLine + newLineCount - 1
		if newLineCount == 0 {
			newEndLine = newStartLine - 1
		}
		infos = append(infos, editRangeLineInfo{
			editRange:    editRange,
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
		start := max(1, info.oldStartLine-contextLines)
		end := min(totalLines, info.oldEndLine+contextLines)
		if len(hunks) == 0 || start > hunks[len(hunks)-1].endLine+1 {
			hunks = append(hunks, editDiffHunk{
				startLine: start,
				endLine:   end,
				ranges:    []editRangeLineInfo{info},
			})
			continue
		}
		last := &hunks[len(hunks)-1]
		last.endLine = max(last.endLine, end)
		last.ranges = append(last.ranges, info)
	}
	return hunks
}

type diffLineOperation struct {
	kind string
	line string
}

func diffLineOperations(oldLines, newLines []string) []diffLineOperation {
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
	var operations []diffLineOperation
	for i, j := 0, 0; i < len(oldLines) || j < len(newLines); {
		switch {
		case i < len(oldLines) && j < len(newLines) && oldLines[i] == newLines[j]:
			operations = append(operations, diffLineOperation{kind: "equal", line: oldLines[i]})
			i++
			j++
		case j < len(newLines) && (i == len(oldLines) || lcs[i][j+1] > lcs[i+1][j]):
			operations = append(operations, diffLineOperation{kind: "add", line: newLines[j]})
			j++
		case i < len(oldLines):
			operations = append(operations, diffLineOperation{kind: "remove", line: oldLines[i]})
			i++
		}
	}
	return operations
}

func generateDiffStringWithRanges(oldContent, newContent string, ranges []editRange, contextLines int) string {
	oldLines := splitDiffLines(oldContent)
	newLines := splitDiffLines(newContent)
	totalLines := max(1, len(oldLines))
	width := len(fmt.Sprintf("%d", max(totalLines, len(newLines))))
	infos := editRangeLineInfos(oldContent, ranges)
	hunks := editDiffHunks(infos, totalLines, contextLines)
	var output []string
	previousEnd := 0
	for _, hunk := range hunks {
		if hunk.startLine > 1 && (previousEnd == 0 || hunk.startLine > previousEnd+1) {
			output = append(output, " "+strings.Repeat(" ", width)+" ...")
		}
		firstRange := hunk.ranges[0]
		lastRange := hunk.ranges[len(hunk.ranges)-1]
		newStartLine := hunk.startLine + (firstRange.newStartLine - firstRange.oldStartLine)
		newEndLine := hunk.endLine + (lastRange.newEndLine - lastRange.oldEndLine)
		operations := diffLineOperations(
			lineWindow(oldLines, hunk.startLine, hunk.endLine),
			lineWindow(newLines, newStartLine, newEndLine),
		)
		oldLine, newLine := hunk.startLine, newStartLine
		for _, operation := range operations {
			switch operation.kind {
			case "equal":
				output = append(output, fmt.Sprintf(" %*d %s", width, oldLine, operation.line))
				oldLine++
				newLine++
			case "remove":
				output = append(output, fmt.Sprintf("-%*d %s", width, oldLine, operation.line))
				oldLine++
			case "add":
				output = append(output, fmt.Sprintf("+%*d %s", width, newLine, operation.line))
				newLine++
			}
		}
		previousEnd = hunk.endLine
	}
	if previousEnd > 0 && previousEnd < totalLines {
		output = append(output, " "+strings.Repeat(" ", width)+" ...")
	}
	return strings.Join(output, "\n")
}

func generateUnifiedPatchWithRanges(path, oldContent, newContent string, ranges []editRange, contextLines int) string {
	oldLines := splitDiffLines(oldContent)
	newLines := splitDiffLines(newContent)
	totalLines := max(1, len(oldLines))
	infos := editRangeLineInfos(oldContent, ranges)
	hunks := editDiffHunks(infos, totalLines, contextLines)
	output := []string{"--- " + path, "+++ " + path}
	for _, hunk := range hunks {
		firstRange := hunk.ranges[0]
		lastRange := hunk.ranges[len(hunk.ranges)-1]
		newStartLine := hunk.startLine + (firstRange.newStartLine - firstRange.oldStartLine)
		newEndLine := hunk.endLine + (lastRange.newEndLine - lastRange.oldEndLine)
		oldWindow := lineWindow(oldLines, hunk.startLine, hunk.endLine)
		newWindow := lineWindow(newLines, newStartLine, newEndLine)
		output = append(output, fmt.Sprintf(
			"@@ -%s +%s @@",
			formatPatchRange(hunk.startLine, len(oldWindow)),
			formatPatchRange(newStartLine, len(newWindow)),
		))
		for _, operation := range diffLineOperations(oldWindow, newWindow) {
			prefix := " "
			if operation.kind == "remove" {
				prefix = "-"
			} else if operation.kind == "add" {
				prefix = "+"
			}
			output = append(output, prefix+operation.line)
		}
	}
	return strings.Join(output, "\n") + "\n"
}

func formatPatchRange(start, count int) string {
	if count == 0 {
		return fmt.Sprintf("%d,0", max(0, start-1))
	}
	if count == 1 {
		return fmt.Sprintf("%d", start)
	}
	return fmt.Sprintf("%d,%d", start, count)
}

func splitDiffLines(content string) []string {
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	if strings.HasSuffix(content, "\n") {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func lineWindow(lines []string, startLine, endLine int) []string {
	if startLine < 1 {
		startLine = 1
	}
	if endLine < startLine || startLine > len(lines) {
		return nil
	}
	return lines[startLine-1 : min(endLine, len(lines))]
}

func firstChangedLine(content string, ranges []editRange) int {
	if len(ranges) == 0 {
		return 0
	}
	return strings.Count(content[:ranges[0].start], "\n") + 1
}
