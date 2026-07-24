package gitui

import "testing"

func TestFindWordBackwardPiMatrix(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		cursor int
		want   int
	}{
		{name: "basic final word", text: "hello world", cursor: 11, want: 6},
		{name: "basic preceding word", text: "hello world", cursor: 6, want: 0},
		{name: "dotted final word", text: "foo.bar", cursor: 7, want: 4},
		{name: "dotted punctuation", text: "foo.bar", cursor: 4, want: 3},
		{name: "dotted first word", text: "foo.bar", cursor: 3, want: 0},
		{name: "colon final word", text: "foo:bar", cursor: 7, want: 4},
		{name: "path final word", text: "path/to/file", cursor: 12, want: 8},
		{name: "path slash", text: "path/to/file", cursor: 8, want: 7},
		{name: "path middle word", text: "path/to/file", cursor: 7, want: 5},
		{name: "cjk final ascii word", text: "你好世界 test", cursor: 9, want: 5},
		{name: "cjk dictionary-sized unit", text: "你好世界 test", cursor: 5, want: 2},
		{name: "cjk first unit", text: "你好世界 test", cursor: 2, want: 0},
		{name: "trailing whitespace", text: "  hello  ", cursor: 9, want: 2},
		{name: "leading whitespace", text: "  hello  ", cursor: 2, want: 0},
		{name: "punctuation run", text: "foo...bar", cursor: 6, want: 3},
		{name: "cursor at zero", text: "hello", cursor: 0, want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := FindWordBackward(test.text, test.cursor); got != test.want {
				t.Fatalf("FindWordBackward(%q, %d) = %d, want %d", test.text, test.cursor, got, test.want)
			}
		})
	}
}

func TestFindWordForwardPiMatrix(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		cursor int
		want   int
	}{
		{name: "basic first word", text: "hello world", cursor: 0, want: 5},
		{name: "basic next word", text: "hello world", cursor: 5, want: 11},
		{name: "dotted first word", text: "foo.bar", cursor: 0, want: 3},
		{name: "dotted punctuation", text: "foo.bar", cursor: 3, want: 4},
		{name: "dotted final word", text: "foo.bar", cursor: 4, want: 7},
		{name: "path first word", text: "path/to/file", cursor: 0, want: 4},
		{name: "path slash", text: "path/to/file", cursor: 4, want: 5},
		{name: "path middle word", text: "path/to/file", cursor: 5, want: 7},
		{name: "leading whitespace", text: "  hello  ", cursor: 0, want: 7},
		{name: "trailing whitespace", text: "  hello  ", cursor: 7, want: 9},
		{name: "punctuation run", text: "foo...bar", cursor: 3, want: 6},
		{name: "cursor at end", text: "hello", cursor: 5, want: 5},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := FindWordForward(test.text, test.cursor); got != test.want {
				t.Fatalf("FindWordForward(%q, %d) = %d, want %d", test.text, test.cursor, got, test.want)
			}
		})
	}
}

func TestFindWordNavigationTreatsCustomAtomicSegmentsAsUnits(t *testing.T) {
	const marker = "[paste #1 +5 lines]"
	text := "hello " + marker + " world"
	segments := map[string][]WordSegment{
		text: {
			{Text: "hello", WordLike: true},
			{Text: " "},
			{Text: marker, WordLike: true},
			{Text: " "},
			{Text: "world", WordLike: true},
		},
		string([]rune(text)[:26]): {
			{Text: "hello", WordLike: true},
			{Text: " "},
			{Text: marker, WordLike: true},
			{Text: " "},
		},
		string([]rune(text)[6:]): {
			{Text: marker, WordLike: true},
			{Text: " "},
			{Text: "world", WordLike: true},
		},
	}
	options := WordNavigationOptions{
		Segment: func(input string) []WordSegment {
			return append([]WordSegment(nil), segments[input]...)
		},
		IsAtomicSegment: func(segment string) bool {
			return segment == marker
		},
	}

	if got := FindWordBackward(text, len([]rune(text)), options); got != 26 {
		t.Fatalf("backward over word = %d, want 26", got)
	}
	if got := FindWordBackward(text, 26, options); got != 6 {
		t.Fatalf("backward over atomic marker = %d, want 6", got)
	}
	if got := FindWordForward(text, 6, options); got != 6+len([]rune(marker)) {
		t.Fatalf("forward over atomic marker = %d, want %d", got, 6+len([]rune(marker)))
	}
}
