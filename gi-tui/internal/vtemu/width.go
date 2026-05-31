package vtemu

import terminalwidth "github.com/nowa/gi/gi-tui/internal/width"

func visibleWidthPlain(clean string) int {
	return terminalwidth.VisibleWidthPlain(clean)
}

func truncateFragmentToWidth(text string, maxWidth int) (string, int) {
	return terminalwidth.TruncateFragmentToWidth(text, maxWidth)
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

func runeByteOffsets(text string) []int {
	return terminalwidth.RuneByteOffsets(text)
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
