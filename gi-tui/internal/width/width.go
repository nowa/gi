package width

import (
	"strings"
	"unicode"
)

type GraphemeSpan struct {
	Start int
	End   int
}

func VisibleWidthPlain(clean string) int {
	width := 0
	runes := []rune(clean)
	for _, span := range GraphemeSpans(runes) {
		width += GraphemeWidth(runes[span.Start:span.End])
	}
	return width
}

func TruncateFragmentToWidth(text string, maxWidth int) (string, int) {
	if maxWidth <= 0 {
		return "", 0
	}
	runes := []rune(text)
	var b strings.Builder
	width := 0
	for _, span := range GraphemeSpans(runes) {
		clusterWidth := GraphemeWidth(runes[span.Start:span.End])
		if width+clusterWidth > maxWidth {
			break
		}
		b.WriteString(string(runes[span.Start:span.End]))
		width += clusterWidth
	}
	return b.String(), width
}

func IsWideRune(r rune) bool {
	if r >= 0x1f000 && r <= 0x1fbff {
		return true
	}
	if r >= 0x1f1e6 && r <= 0x1f1ff {
		return true
	}
	return (r >= 0x1100 && r <= 0x115f) ||
		(r >= 0x2600 && r <= 0x27bf) ||
		(r >= 0x2329 && r <= 0x232a) ||
		(r >= 0x2e80 && r <= 0xa4cf) ||
		(r >= 0xac00 && r <= 0xd7a3) ||
		(r >= 0xf900 && r <= 0xfaff) ||
		(r >= 0xfe10 && r <= 0xfe19) ||
		(r >= 0xfe30 && r <= 0xfe6f) ||
		(r >= 0xff00 && r <= 0xff60) ||
		(r >= 0xffe0 && r <= 0xffe6)
}

func GraphemeSpans(runes []rune) []GraphemeSpan {
	if len(runes) == 0 {
		return nil
	}
	spans := make([]GraphemeSpan, 0, len(runes))
	for i := 0; i < len(runes); {
		start := i
		i++
		if IsRegionalIndicator(runes[start]) && i < len(runes) && IsRegionalIndicator(runes[i]) {
			i++
		}
		for i < len(runes) {
			for i < len(runes) && IsGraphemeExtend(runes[i]) {
				i++
			}
			if i < len(runes) && runes[i] == '\u200d' {
				i++
				if i < len(runes) {
					i++
					continue
				}
			}
			break
		}
		spans = append(spans, GraphemeSpan{Start: start, End: i})
	}
	return spans
}

func PreviousGraphemeBoundary(runes []rune, pos int) int {
	pos = max(0, min(pos, len(runes)))
	previous := 0
	for _, span := range GraphemeSpans(runes) {
		if span.End >= pos {
			if span.End == pos {
				return span.Start
			}
			if span.Start < pos {
				return span.Start
			}
			return previous
		}
		previous = span.Start
	}
	if len(runes) == 0 {
		return 0
	}
	return previous
}

func NextGraphemeBoundary(runes []rune, pos int) int {
	pos = max(0, min(pos, len(runes)))
	for _, span := range GraphemeSpans(runes) {
		if span.Start <= pos && pos < span.End {
			return span.End
		}
		if span.Start > pos {
			return span.End
		}
	}
	return len(runes)
}

func RuneByteOffsets(text string) []int {
	offsets := make([]int, 0, len([]rune(text))+1)
	for idx := range text {
		offsets = append(offsets, idx)
	}
	offsets = append(offsets, len(text))
	return offsets
}

func GraphemeWidth(cluster []rune) int {
	if len(cluster) == 0 {
		return 0
	}
	hasWide := false
	hasJoiner := false
	hasKeycap := false
	hasKeycapBase := false
	hasEmojiPresentation := false
	hasNarrowEmojiBase := false
	visible := 0
	for _, r := range cluster {
		if r == 0x20e3 {
			hasKeycap = true
		}
		if r == 0xfe0f {
			hasEmojiPresentation = true
		}
		if isKeycapBaseRune(r) {
			hasKeycapBase = true
		}
		if isNarrowEmojiPresentationBaseRune(r) {
			hasNarrowEmojiBase = true
		}
		switch {
		case r == '\u200d':
			hasJoiner = true
		case r == '\t':
			visible += 3
		case r == '\n' || r == '\r' || r == 0 || unicode.IsControl(r) || IsGraphemeExtend(r):
			continue
		case IsWideRune(r):
			hasWide = true
			visible += 2
		default:
			visible++
		}
	}
	if IsRegionalIndicator(cluster[0]) {
		return 2
	}
	if hasKeycap && hasKeycapBase {
		return 2
	}
	if hasEmojiPresentation && hasNarrowEmojiBase {
		return 2
	}
	if hasJoiner && hasWide {
		return 2
	}
	if hasWide && len(cluster) > 1 {
		return 2
	}
	return visible
}

func isKeycapBaseRune(r rune) bool {
	return (r >= '0' && r <= '9') || r == '#' || r == '*'
}

func isNarrowEmojiPresentationBaseRune(r rune) bool {
	switch r {
	case 0x00a9, 0x00ae, 0x203c, 0x2049, 0x2122, 0x2139:
		return true
	}
	return r >= 0x2194 && r <= 0x21aa
}

func IsGraphemeExtend(r rune) bool {
	return unicode.Is(unicode.Mn, r) ||
		unicode.Is(unicode.Me, r) ||
		(r >= 0xfe00 && r <= 0xfe0f) ||
		(r >= 0xe0100 && r <= 0xe01ef) ||
		(r >= 0x1f3fb && r <= 0x1f3ff) ||
		r == 0x20e3
}

func IsRegionalIndicator(r rune) bool {
	return r >= 0x1f1e6 && r <= 0x1f1ff
}
