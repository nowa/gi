package gitui

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	terminalwidth "github.com/nowa/gi/gi-tui/internal/width"
)

// VisibleWidth returns the number of terminal cells occupied by s.
// It strips ANSI escape sequences, treats tabs as three cells, and uses a
// conservative width table for combining marks, CJK, and emoji.
func VisibleWidth(s string) int {
	if s == "" {
		return 0
	}
	clean := stripANSI(s)
	return visibleWidthPlain(clean)
}

func visibleWidthPlain(clean string) int {
	return terminalwidth.VisibleWidthPlain(clean)
}

func isWideRune(r rune) bool {
	return terminalwidth.IsWideRune(r)
}

type graphemeSpan struct {
	start int
	end   int
}

func graphemeSpans(runes []rune) []graphemeSpan {
	source := terminalwidth.GraphemeSpans(runes)
	if len(source) == 0 {
		return nil
	}
	spans := make([]graphemeSpan, 0, len(source))
	for _, span := range source {
		spans = append(spans, graphemeSpan{start: span.Start, end: span.End})
	}
	return spans
}

func previousGraphemeBoundary(runes []rune, pos int) int {
	return terminalwidth.PreviousGraphemeBoundary(runes, pos)
}

func nextGraphemeBoundary(runes []rune, pos int) int {
	return terminalwidth.NextGraphemeBoundary(runes, pos)
}

func graphemeWidth(cluster []rune) int {
	return terminalwidth.GraphemeWidth(cluster)
}

func isGraphemeExtend(r rune) bool {
	return terminalwidth.IsGraphemeExtend(r)
}

func isRegionalIndicator(r rune) bool {
	return terminalwidth.IsRegionalIndicator(r)
}

// TruncateToWidth truncates text to maxWidth terminal cells. ANSI escapes are
// preserved when possible. The optional pad argument pads the result to maxWidth.
func TruncateToWidth(text string, maxWidth int, ellipsis string, pad ...bool) string {
	shouldPad := len(pad) > 0 && pad[0]
	if maxWidth <= 0 {
		return ""
	}
	originalWidth := VisibleWidth(text)
	if originalWidth <= maxWidth {
		if shouldPad {
			return text + strings.Repeat(" ", max(0, maxWidth-originalWidth))
		}
		return text
	}
	ellipsisWidth := VisibleWidth(ellipsis)
	if ellipsisWidth >= maxWidth {
		clipped, clippedWidth := truncateFragmentToWidth(ellipsis, maxWidth)
		if clippedWidth == 0 {
			if shouldPad {
				return strings.Repeat(" ", maxWidth)
			}
			return ""
		}
		result := "\x1b[0m" + clipped + "\x1b[0m"
		if shouldPad {
			result += strings.Repeat(" ", max(0, maxWidth-clippedWidth))
		}
		return result
	}
	limit := maxWidth - ellipsisWidth
	var b strings.Builder
	width := 0
	for _, segment := range terminalWidthSegments(text) {
		if segment.ansi {
			b.WriteString(segment.text)
			continue
		}
		if width+segment.width > limit {
			break
		}
		b.WriteString(segment.text)
		width += segment.width
	}
	result := b.String()
	if ellipsis != "" {
		result += "\x1b[0m" + ellipsis + "\x1b[0m"
		width += ellipsisWidth
	} else {
		result += "\x1b[0m"
	}
	if shouldPad {
		result += strings.Repeat(" ", max(0, maxWidth-width))
	}
	return result
}

func truncateFragmentToWidth(text string, maxWidth int) (string, int) {
	if maxWidth <= 0 {
		return "", 0
	}
	var b strings.Builder
	width := 0
	for _, segment := range terminalWidthSegments(text) {
		if segment.ansi {
			b.WriteString(segment.text)
			continue
		}
		if width+segment.width > maxWidth {
			break
		}
		b.WriteString(segment.text)
		width += segment.width
	}
	return b.String(), width
}

func runeWidth(r rune) int {
	switch {
	case r == '\t':
		return 3
	case r == '\n' || r == '\r' || unicode.IsControl(r) || unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r):
		return 0
	case isWideRune(r):
		return 2
	default:
		return 1
	}
}

func ansiAt(s string) (string, int) {
	if code, length := c1ANSIAt(s); length > 0 {
		return code, length
	}
	if s == "" || s[0] != '\x1b' {
		return "", 0
	}
	if len(s) < 2 {
		return "", 0
	}
	switch s[1] {
	case '[':
		for i := 2; i < len(s); i++ {
			if s[i] >= 0x40 && s[i] <= 0x7e {
				return s[:i+1], i + 1
			}
		}
	case ']', '_':
		if end := ansiStringSequenceEnd(s, 2, true); end > 0 {
			return s[:end], end
		}
	case 'P', '^', 'X':
		if end := ansiStringSequenceEnd(s, 2, false); end > 0 {
			return s[:end], end
		}
	case '(', ')':
		if len(s) >= 3 && s[2] >= 0x20 && s[2] < 0x7f {
			return s[:3], 3
		}
	}
	return "", 0
}

func c1ANSIAt(s string) (string, int) {
	code, prefixLen, ok := c1Prefix(s)
	if !ok {
		return "", 0
	}
	switch code {
	case 0x8e, 0x8f:
		if len(s) >= prefixLen+1 {
			return s[:prefixLen+1], prefixLen + 1
		}
	case 0x90, 0x98, 0x9e:
		if end := c1StringTerminatedLength(s, prefixLen, false); end > 0 {
			return s[:end], end
		}
	case 0x9b:
		for i := prefixLen; i < len(s); i++ {
			if s[i] >= 0x40 && s[i] <= 0x7e {
				return s[:i+1], i + 1
			}
		}
	case 0x9d, 0x9f:
		if end := c1StringTerminatedLength(s, prefixLen, true); end > 0 {
			return s[:end], end
		}
	default:
		return s[:prefixLen], prefixLen
	}
	return "", 0
}

func ansiStringSequenceEnd(s string, start int, allowBEL bool) int {
	if _, sequenceEnd, ok := controlStringTerminator(s, start, allowBEL); ok {
		return sequenceEnd
	}
	return 0
}

type ANSICode struct {
	Code   string
	Length int
}

func ExtractAnsiCode(str string, pos int) (ANSICode, bool) {
	if pos < 0 || pos >= len(str) {
		return ANSICode{}, false
	}
	code, length := ansiAt(str[pos:])
	if length == 0 {
		return ANSICode{}, false
	}
	return ANSICode{Code: code, Length: length}, true
}

// WrapTextWithANSI wraps text into terminal-width lines while preserving ANSI
// sequences. Newlines always force a break.
func WrapTextWithANSI(text string, width int) []string {
	if width <= 0 {
		width = 1
	}
	text = strings.ReplaceAll(text, "\t", "   ")
	var lines []string
	tracker := ansiStyleTracker{}
	for _, paragraph := range strings.Split(text, "\n") {
		prefix := ""
		if len(lines) > 0 {
			prefix = tracker.activeCodes()
		}
		lines = append(lines, wrapParagraph(prefix+paragraph, width)...)
		updateANSITrackerFromText(paragraph, &tracker)
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func updateANSITrackerFromText(text string, tracker *ansiStyleTracker) {
	for _, segment := range terminalWidthSegments(text) {
		if segment.ansi {
			tracker.process(segment.text)
		}
	}
}

func wrapParagraph(text string, width int) []string {
	var lines []string
	var line strings.Builder
	tracker := ansiStyleTracker{}
	lineWidth := 0
	lastSpaceBytes := -1
	lastSpaceWidth := 0
	for _, segment := range terminalWidthSegments(text) {
		if segment.ansi {
			tracker.process(segment.text)
			line.WriteString(segment.text)
			continue
		}
		w := segment.width
		if segment.space {
			lastSpaceBytes = line.Len()
			lastSpaceWidth = lineWidth
		}
		if lineWidth+w > width && lineWidth > 0 {
			if lastSpaceBytes >= 0 {
				current := line.String()
				wrapped := strings.TrimRight(current[:lastSpaceBytes], " ")
				lines = append(lines, wrapped+lineBreakCloseCodes(wrapped))
				remainder := strings.TrimLeft(current[lastSpaceBytes:], " ")
				line.Reset()
				line.WriteString(tracker.activeCodes())
				line.WriteString(remainder)
				lineWidth = VisibleWidth(remainder)
			} else {
				wrapped := line.String()
				lines = append(lines, wrapped+lineBreakCloseCodes(wrapped))
				line.Reset()
				line.WriteString(tracker.activeCodes())
				lineWidth = 0
			}
			lastSpaceBytes = -1
			lastSpaceWidth = 0
			if segment.space {
				continue
			}
		}
		line.WriteString(segment.text)
		lineWidth += w
		if lastSpaceBytes >= 0 && lastSpaceWidth > lineWidth {
			lastSpaceBytes = -1
		}
	}
	lines = append(lines, strings.TrimRight(line.String(), " "))
	return lines
}

func lineBreakCloseCodes(fragment string) string {
	tracker := ansiStyleTracker{}
	for _, segment := range terminalWidthSegments(fragment) {
		if segment.ansi {
			tracker.process(segment.text)
		}
	}
	var out strings.Builder
	if tracker.underline {
		out.WriteString("\x1b[24m")
	}
	if tracker.activeHyperlink != nil {
		out.WriteString("\x1b]8;;")
		out.WriteString(tracker.activeHyperlink.terminator)
	}
	return out.String()
}

type terminalWidthSegment struct {
	text  string
	width int
	space bool
	ansi  bool
}

func terminalWidthSegments(text string) []terminalWidthSegment {
	var segments []terminalWidthSegment
	for i := 0; i < len(text); {
		if esc, n := ansiAt(text[i:]); n > 0 {
			segments = append(segments, terminalWidthSegment{text: esc, ansi: true})
			i += n
			continue
		}
		start := i
		for i < len(text) {
			if _, n := ansiAt(text[i:]); n > 0 {
				break
			}
			_, size := utf8.DecodeRuneInString(text[i:])
			if size == 0 {
				break
			}
			i += size
		}
		plain := text[start:i]
		runes := []rune(plain)
		byteOffsets := runeByteOffsets(plain)
		for _, span := range graphemeSpans(runes) {
			byteStart := byteOffsets[span.start]
			byteEnd := len(plain)
			if span.end < len(byteOffsets) {
				byteEnd = byteOffsets[span.end]
			}
			cluster := plain[byteStart:byteEnd]
			segments = append(segments, terminalWidthSegment{
				text:  cluster,
				width: graphemeWidth(runes[span.start:span.end]),
				space: graphemeIsSpace(runes[span.start:span.end]),
			})
		}
	}
	return segments
}

func runeByteOffsets(text string) []int {
	return terminalwidth.RuneByteOffsets(text)
}

func graphemeIsSpace(cluster []rune) bool {
	if len(cluster) == 0 {
		return false
	}
	for _, r := range cluster {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func firstGraphemeByteLen(text string) int {
	runes := []rune(text)
	spans := graphemeSpans(runes)
	if len(spans) == 0 {
		return 0
	}
	offsets := runeByteOffsets(text)
	return offsets[spans[0].end]
}

func IsWhitespaceChar(char rune) bool {
	return unicode.IsSpace(char)
}

func IsPunctuationChar(char rune) bool {
	return strings.ContainsRune(`(){}[]<>.,;:'"!?+-=*/\|&%^$#@~`+"`", char)
}

type ColumnSlice struct {
	Text  string
	Width int
}

func SliceByColumn(line string, startCol, length int, strict ...bool) string {
	return SliceWithWidth(line, startCol, length, strict...).Text
}

func SliceWithWidth(line string, startCol, length int, strict ...bool) ColumnSlice {
	if length <= 0 {
		return ColumnSlice{}
	}
	strictMode := len(strict) > 0 && strict[0]
	endCol := startCol + length
	var result strings.Builder
	var pendingANSI strings.Builder
	resultWidth := 0
	currentCol := 0
	for _, segment := range terminalWidthSegments(line) {
		if segment.ansi {
			if currentCol >= startCol && currentCol < endCol {
				result.WriteString(segment.text)
			} else if currentCol < startCol {
				pendingANSI.WriteString(segment.text)
			}
			continue
		}
		inRange := currentCol >= startCol && currentCol < endCol
		fits := !strictMode || currentCol+segment.width <= endCol
		if inRange && fits {
			if pendingANSI.Len() > 0 {
				result.WriteString(pendingANSI.String())
				pendingANSI.Reset()
			}
			result.WriteString(segment.text)
			resultWidth += segment.width
		}
		currentCol += segment.width
		if currentCol >= endCol {
			break
		}
	}
	return ColumnSlice{Text: result.String(), Width: resultWidth}
}

type ExtractedSegments struct {
	Before      string
	BeforeWidth int
	After       string
	AfterWidth  int
}

func ExtractSegments(line string, beforeEnd, afterStart, afterLen int, strictAfter ...bool) ExtractedSegments {
	strictMode := len(strictAfter) > 0 && strictAfter[0]
	afterEnd := afterStart + afterLen
	tracker := ansiStyleTracker{}
	var before, after, pendingANSIBefore strings.Builder
	beforeWidth := 0
	afterWidth := 0
	currentCol := 0
	afterStarted := false
	for _, segment := range terminalWidthSegments(line) {
		if segment.ansi {
			tracker.process(segment.text)
			if currentCol < beforeEnd {
				pendingANSIBefore.WriteString(segment.text)
			} else if currentCol >= afterStart && currentCol < afterEnd && afterStarted {
				after.WriteString(segment.text)
			}
			continue
		}
		if currentCol < beforeEnd {
			if pendingANSIBefore.Len() > 0 {
				before.WriteString(pendingANSIBefore.String())
				pendingANSIBefore.Reset()
			}
			before.WriteString(segment.text)
			beforeWidth += segment.width
		} else if currentCol >= afterStart && currentCol < afterEnd {
			fits := !strictMode || currentCol+segment.width <= afterEnd
			if fits {
				if !afterStarted {
					after.WriteString(tracker.activeCodes())
					afterStarted = true
				}
				after.WriteString(segment.text)
				afterWidth += segment.width
			}
		}
		currentCol += segment.width
		if (afterLen <= 0 && currentCol >= beforeEnd) || (afterLen > 0 && currentCol >= afterEnd) {
			break
		}
	}
	return ExtractedSegments{Before: before.String(), BeforeWidth: beforeWidth, After: after.String(), AfterWidth: afterWidth}
}

type activeHyperlink struct {
	params     string
	url        string
	terminator string
}

type ansiStyleTracker struct {
	bold, dim, italic, underline, blink, inverse, hidden, strikethrough bool
	fgColor, bgColor                                                    string
	activeHyperlink                                                     *activeHyperlink
}

func (t *ansiStyleTracker) process(code string) {
	if hyperlink, ok := parseOSC8Hyperlink(code); ok {
		t.activeHyperlink = hyperlink
		return
	}
	if !strings.HasSuffix(code, "m") || !strings.HasPrefix(code, "\x1b[") {
		return
	}
	params := strings.TrimSuffix(strings.TrimPrefix(code, "\x1b["), "m")
	if params == "" || params == "0" {
		t.reset()
		return
	}
	parts := ansiSGRTrackerParams(params)
	for idx := 0; idx < len(parts); {
		value, err := strconv.Atoi(sgrCode(parts[idx]))
		if err != nil {
			idx++
			continue
		}
		if (value == 38 || value == 48) && idx+2 < len(parts) && sgrCode(parts[idx+1]) == "5" {
			color := strings.Join(parts[idx:idx+3], ";")
			if value == 38 {
				t.fgColor = color
			} else {
				t.bgColor = color
			}
			idx += 3
			continue
		}
		if (value == 38 || value == 48) && idx+3 < len(parts) && sgrCode(parts[idx+1]) == "2" {
			start := idx + 2
			if start < len(parts) && parts[start] == "" {
				start++
			}
			if start+2 >= len(parts) {
				idx++
				continue
			}
			color := strings.Join([]string{parts[idx], parts[idx+1], parts[start], parts[start+1], parts[start+2]}, ";")
			if value == 38 {
				t.fgColor = color
			} else {
				t.bgColor = color
			}
			idx = start + 3
			continue
		}
		switch {
		case value == 0:
			t.reset()
		case value == 1:
			t.bold = true
		case value == 2:
			t.dim = true
		case value == 3:
			t.italic = true
		case value == 4:
			t.handleUnderlinePart(parts[idx])
		case value == 5:
			t.blink = true
		case value == 7:
			t.inverse = true
		case value == 8:
			t.hidden = true
		case value == 9:
			t.strikethrough = true
		case value == 21:
			t.bold = false
		case value == 22:
			t.bold, t.dim = false, false
		case value == 23:
			t.italic = false
		case value == 24:
			t.underline = false
		case value == 25:
			t.blink = false
		case value == 27:
			t.inverse = false
		case value == 28:
			t.hidden = false
		case value == 29:
			t.strikethrough = false
		case value == 39:
			t.fgColor = ""
		case value == 49:
			t.bgColor = ""
		case (value >= 30 && value <= 37) || (value >= 90 && value <= 97):
			t.fgColor = strconv.Itoa(value)
		case (value >= 40 && value <= 47) || (value >= 100 && value <= 107):
			t.bgColor = strconv.Itoa(value)
		}
		idx++
	}
}

func (t *ansiStyleTracker) reset() {
	t.bold, t.dim, t.italic, t.underline = false, false, false, false
	t.blink, t.inverse, t.hidden, t.strikethrough = false, false, false, false
	t.fgColor, t.bgColor = "", ""
}

func (t *ansiStyleTracker) handleUnderlinePart(part string) {
	if !strings.Contains(part, ":") {
		t.underline = true
		return
	}
	parts := strings.Split(part, ":")
	if len(parts) > 1 && parts[1] == "0" {
		t.underline = false
		return
	}
	t.underline = true
}

func ansiSGRTrackerParams(params string) []string {
	fields := strings.Split(params, ";")
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		if isColonExtendedColorField(field) {
			parts = append(parts, strings.Split(field, ":")...)
		} else {
			parts = append(parts, field)
		}
	}
	return parts
}

func (t *ansiStyleTracker) activeCodes() string {
	var codes []string
	if t.bold {
		codes = append(codes, "1")
	}
	if t.dim {
		codes = append(codes, "2")
	}
	if t.italic {
		codes = append(codes, "3")
	}
	if t.underline {
		codes = append(codes, "4")
	}
	if t.blink {
		codes = append(codes, "5")
	}
	if t.inverse {
		codes = append(codes, "7")
	}
	if t.hidden {
		codes = append(codes, "8")
	}
	if t.strikethrough {
		codes = append(codes, "9")
	}
	if t.fgColor != "" {
		codes = append(codes, t.fgColor)
	}
	if t.bgColor != "" {
		codes = append(codes, t.bgColor)
	}
	result := ""
	if len(codes) > 0 {
		result = "\x1b[" + strings.Join(codes, ";") + "m"
	}
	if t.activeHyperlink != nil {
		result += "\x1b]8;" + t.activeHyperlink.params + ";" + t.activeHyperlink.url + t.activeHyperlink.terminator
	}
	return result
}

func parseOSC8Hyperlink(code string) (*activeHyperlink, bool) {
	payload, terminator, ok := oscPayloadAndTerminator(code)
	if !ok || !strings.HasPrefix(payload, "8;") {
		return nil, false
	}
	body := strings.TrimPrefix(payload, "8;")
	separator := strings.Index(body, ";")
	if separator < 0 {
		return nil, true
	}
	params := body[:separator]
	url := body[separator+1:]
	if url == "" {
		return nil, true
	}
	return &activeHyperlink{params: params, url: url, terminator: terminator}, true
}

func oscPayloadAndTerminator(code string) (string, string, bool) {
	if strings.HasPrefix(code, "\x1b]") {
		payloadEnd, sequenceEnd, ok := controlStringTerminator(code, 2, true)
		if !ok || sequenceEnd != len(code) {
			return "", "", false
		}
		return code[2:payloadEnd], code[payloadEnd:sequenceEnd], true
	}
	c1Code, prefixLen, ok := c1Prefix(code)
	if !ok || c1Code != 0x9d {
		return "", "", false
	}
	payloadEnd, sequenceEnd, ok := controlStringTerminator(code, prefixLen, true)
	if !ok || sequenceEnd != len(code) {
		return "", "", false
	}
	return code[prefixLen:payloadEnd], code[payloadEnd:sequenceEnd], true
}

// ApplyBackgroundToLine pads line to width and passes it through bgFn.
func ApplyBackgroundToLine(line string, width int, bgFn func(string) string) string {
	if width > 0 {
		line += strings.Repeat(" ", max(0, width-VisibleWidth(line)))
	}
	if bgFn == nil {
		return line
	}
	return bgFn(line)
}

func stripANSI(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		if _, n := ansiAt(s[i:]); n > 0 {
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if size <= 0 {
			break
		}
		out.WriteRune(r)
		i += size
	}
	return out.String()
}

// NormalizeTerminalOutput rewrites codepoints that terminals commonly render
// as decomposed sequences. This keeps output width accounting aligned with what
// the terminal displays for Thai and Lao AM vowels.
func NormalizeTerminalOutput(s string) string {
	s = strings.ReplaceAll(s, "\u0e33", "\u0e4d\u0e32")
	s = strings.ReplaceAll(s, "\u0eb3", "\u0ecd\u0eb2")
	return s
}
