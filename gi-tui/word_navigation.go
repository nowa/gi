package gitui

import "unicode"

// WordSegment is one logical unit produced by a WordSegmenter. Text from
// consecutive segments must reconstruct the input text.
type WordSegment struct {
	Text     string
	WordLike bool
}

// WordSegmenter splits text into logical word, whitespace, and punctuation
// units. Cursor positions and segment lengths are measured in runes.
type WordSegmenter func(text string) []WordSegment

// WordNavigationOptions customizes word boundaries. IsAtomicSegment can mark a
// segment, such as an editor paste marker, as an indivisible cursor unit.
type WordNavigationOptions struct {
	Segment         WordSegmenter
	IsAtomicSegment func(segment string) bool
}

// FindWordBackward returns the rune offset reached by moving one logical word
// backward. It is pure and safe to use independently of an Editor or Input.
func FindWordBackward(text string, cursor int, options ...WordNavigationOptions) int {
	runes := []rune(text)
	cursor = max(0, min(cursor, len(runes)))
	if cursor == 0 {
		return 0
	}

	opts := wordNavigationOptions(options)
	segments := opts.Segment(string(runes[:cursor]))
	newCursor := cursor

	for len(segments) > 0 {
		last := segments[len(segments)-1]
		if isAtomicWordSegment(last.Text, opts) || !wordSegmentIsWhitespace(last.Text) {
			break
		}
		newCursor -= len([]rune(last.Text))
		segments = segments[:len(segments)-1]
	}
	if len(segments) == 0 {
		return max(0, newCursor)
	}

	last := segments[len(segments)-1]
	switch {
	case isAtomicWordSegment(last.Text, opts):
		newCursor -= len([]rune(last.Text))
	case last.WordLike:
		segmentRunes := []rune(last.Text)
		lastPunctuationEnd := -1
		for index, char := range segmentRunes {
			if IsPunctuationChar(char) {
				lastPunctuationEnd = index + 1
			}
		}
		if lastPunctuationEnd < 0 {
			newCursor -= len(segmentRunes)
		} else {
			newCursor -= len(segmentRunes) - lastPunctuationEnd
		}
	default:
		for len(segments) > 0 {
			last = segments[len(segments)-1]
			if isAtomicWordSegment(last.Text, opts) || last.WordLike || wordSegmentIsWhitespace(last.Text) {
				break
			}
			newCursor -= len([]rune(last.Text))
			segments = segments[:len(segments)-1]
		}
	}
	return max(0, newCursor)
}

// FindWordForward returns the rune offset reached by moving one logical word
// forward. It is pure and safe to use independently of an Editor or Input.
func FindWordForward(text string, cursor int, options ...WordNavigationOptions) int {
	runes := []rune(text)
	cursor = max(0, min(cursor, len(runes)))
	if cursor == len(runes) {
		return cursor
	}

	opts := wordNavigationOptions(options)
	segments := opts.Segment(string(runes[cursor:]))
	newCursor := cursor
	index := 0

	for index < len(segments) {
		segment := segments[index]
		if isAtomicWordSegment(segment.Text, opts) || !wordSegmentIsWhitespace(segment.Text) {
			break
		}
		newCursor += len([]rune(segment.Text))
		index++
	}
	if index >= len(segments) {
		return min(newCursor, len(runes))
	}

	segment := segments[index]
	switch {
	case isAtomicWordSegment(segment.Text, opts):
		newCursor += len([]rune(segment.Text))
	case segment.WordLike:
		segmentRunes := []rune(segment.Text)
		punctuationStart := len(segmentRunes)
		for position, char := range segmentRunes {
			if IsPunctuationChar(char) {
				punctuationStart = position
				break
			}
		}
		newCursor += punctuationStart
	default:
		for index < len(segments) {
			segment = segments[index]
			if isAtomicWordSegment(segment.Text, opts) || segment.WordLike || wordSegmentIsWhitespace(segment.Text) {
				break
			}
			newCursor += len([]rune(segment.Text))
			index++
		}
	}
	return min(newCursor, len(runes))
}

func wordNavigationOptions(options []WordNavigationOptions) WordNavigationOptions {
	opts := WordNavigationOptions{Segment: defaultWordSegments}
	if len(options) == 0 {
		return opts
	}
	opts = options[0]
	if opts.Segment == nil {
		opts.Segment = defaultWordSegments
	}
	return opts
}

func isAtomicWordSegment(segment string, options WordNavigationOptions) bool {
	return options.IsAtomicSegment != nil && options.IsAtomicSegment(segment)
}

func wordSegmentIsWhitespace(segment string) bool {
	if segment == "" {
		return false
	}
	for _, char := range segment {
		if !IsWhitespaceChar(char) {
			return false
		}
	}
	return true
}

func defaultWordSegments(text string) []WordSegment {
	runes := []rune(text)
	segments := make([]WordSegment, 0, len(runes))
	for start := 0; start < len(runes); {
		end := start + 1
		switch {
		case unicode.IsSpace(runes[start]):
			for end < len(runes) && unicode.IsSpace(runes[end]) {
				end++
			}
			segments = append(segments, WordSegment{Text: string(runes[start:end])})
		case unicode.In(runes[start], unicode.Han):
			// Intl.Segmenter uses a language dictionary for Han text. Go's
			// standard library intentionally has no locale dictionary, so keep
			// adjacent Han characters in small, useful cursor units.
			for end < len(runes) && end-start < 2 && unicode.In(runes[end], unicode.Han) {
				end++
			}
			segments = append(segments, WordSegment{Text: string(runes[start:end]), WordLike: true})
		case isDefaultWordRune(runes[start]):
			for end < len(runes) && !unicode.In(runes[end], unicode.Han) && isDefaultWordRune(runes[end]) {
				end++
			}
			segments = append(segments, WordSegment{Text: string(runes[start:end]), WordLike: true})
		default:
			for end < len(runes) &&
				!unicode.IsSpace(runes[end]) &&
				!unicode.In(runes[end], unicode.Han) &&
				!isDefaultWordRune(runes[end]) {
				end++
			}
			segments = append(segments, WordSegment{Text: string(runes[start:end])})
		}
		start = end
	}
	return segments
}

func isDefaultWordRune(char rune) bool {
	return char == '_' ||
		unicode.IsLetter(char) ||
		unicode.IsDigit(char) ||
		unicode.IsNumber(char) ||
		unicode.IsMark(char)
}
