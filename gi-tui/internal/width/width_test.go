package width

import "testing"

func TestVisibleWidthPlainGraphemeCases(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want int
	}{
		{name: "ascii", text: "hello", want: 5},
		{name: "tab", text: "a\tb", want: 5},
		{name: "cjk", text: "你好", want: 4},
		{name: "emoji presentation", text: "✅", want: 2},
		{name: "regional flag", text: "🇺🇸", want: 2},
		{name: "zwj emoji", text: "👨‍💻", want: 2},
		{name: "combining mark", text: "e\u0301", want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := VisibleWidthPlain(tc.text); got != tc.want {
				t.Fatalf("VisibleWidthPlain(%q) = %d, want %d", tc.text, got, tc.want)
			}
		})
	}
}

func TestGraphemeBoundaries(t *testing.T) {
	runes := []rune("a👨‍💻b")
	if got := NextGraphemeBoundary(runes, 1); got != 4 {
		t.Fatalf("NextGraphemeBoundary() = %d, want 4", got)
	}
	if got := PreviousGraphemeBoundary(runes, 4); got != 1 {
		t.Fatalf("PreviousGraphemeBoundary() = %d, want 1", got)
	}
}

func TestTruncateFragmentToWidth(t *testing.T) {
	got, width := TruncateFragmentToWidth("ab你好", 5)
	if got != "ab你" || width != 4 {
		t.Fatalf("TruncateFragmentToWidth() = %q, %d; want %q, %d", got, width, "ab你", 4)
	}
}
