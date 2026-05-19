package gitui

import (
	"strings"
	"testing"
)

func TestVisibleWidthHandlesANSIAndWideRunes(t *testing.T) {
	if got := VisibleWidth("\x1b[31mhello\x1b[0m"); got != 5 {
		t.Fatalf("visible ansi width = %d, want 5", got)
	}
	if got := VisibleWidth("你好"); got != 4 {
		t.Fatalf("visible CJK width = %d, want 4", got)
	}
	if got := VisibleWidth("🇺🇸"); got != 2 {
		t.Fatalf("regional indicators width = %d, want 2", got)
	}
}

func TestUtilitiesRecognizeC1ControlSequences(t *testing.T) {
	if got := VisibleWidth("\x9b31mred\x9b0m"); got != 3 {
		t.Fatalf("raw C1 CSI visible width = %d, want 3", got)
	}
	if got := VisibleWidth("\u009b31mred\u009b0m"); got != 3 {
		t.Fatalf("UTF-8 C1 CSI visible width = %d, want 3", got)
	}
	if got := VisibleWidth("\x9d133;A\x9chello\x9d133;B\x9c"); got != 5 {
		t.Fatalf("raw C1 OSC visible width = %d, want 5", got)
	}
	if got := VisibleWidth("\u009d133;A\u009chello\u009d133;B\u009c"); got != 5 {
		t.Fatalf("UTF-8 C1 OSC visible width = %d, want 5", got)
	}

	for _, sample := range []string{
		"\x9b31mred",
		"\u009b31mred",
		"\x9d8;;https://example.com\x9clink",
		"\u009d8;;https://example.com\u009clink",
	} {
		code, ok := ExtractAnsiCode(sample, 0)
		if !ok || code.Length <= 0 || code.Code != sample[:code.Length] {
			t.Fatalf("ExtractAnsiCode(%q) = %#v ok=%v", sample, code, ok)
		}
	}

	got := TruncateToWidth("\u009b31mabcdef\u009b0m", 4, "…", true)
	if VisibleWidth(got) != 4 || !strings.Contains(got, "\u009b31m") {
		t.Fatalf("C1 truncation should preserve style and width, got %q width %d", got, VisibleWidth(got))
	}
}

func TestVisibleWidthPiGraphemeClusters(t *testing.T) {
	for _, sample := range []string{"🇨", "🇺🇸", "👍", "👍🏻", "✅", "⚡", "⚡️", "👨", "👨‍💻", "🏳️‍🌈"} {
		if got := VisibleWidth(sample); got != 2 {
			t.Fatalf("visible width %q = %d, want 2", sample, got)
		}
	}
}

func TestVisibleWidthPiEmojiPresentationClusters(t *testing.T) {
	for _, sample := range []string{"1️⃣", "#️⃣", "*️⃣", "©️", "®️", "™️", "↔️"} {
		if got := VisibleWidth(sample); got != 2 {
			t.Fatalf("emoji presentation width %q = %d, want 2", sample, got)
		}
	}
	if got := VisibleWidth("\u20e3"); got != 0 {
		t.Fatalf("standalone keycap combining mark width = %d, want 0", got)
	}
}

func TestTruncateToWidthPreservesAnsiAndPads(t *testing.T) {
	got := TruncateToWidth("\x1b[31mabcdef\x1b[0m", 4, "…", true)
	if VisibleWidth(got) != 4 {
		t.Fatalf("visible width = %d, want 4 (%q)", VisibleWidth(got), got)
	}
	if !strings.Contains(got, "\x1b[31m") {
		t.Fatalf("expected ANSI prefix to be preserved: %q", got)
	}
}

func TestTruncateToWidthPiEdgeCases(t *testing.T) {
	if got := TruncateToWidth("abcdef", 1, "🙂"); got != "" {
		t.Fatalf("wide ellipsis width 1 = %q, want empty", got)
	}
	if got := TruncateToWidth("abcdef", 2, "🙂"); got != "\x1b[0m🙂\x1b[0m" {
		t.Fatalf("wide ellipsis width 2 = %q", got)
	}
	if got := TruncateToWidth("abcdef", 2, "..."); got != "\x1b[0m..\x1b[0m" {
		t.Fatalf("clipped ascii ellipsis width 2 = %q", got)
	}
	if got := TruncateToWidth("a", 2, "🙂"); got != "a" {
		t.Fatalf("fitting text with wide ellipsis = %q", got)
	}
	if got := TruncateToWidth("\x1b[31m"+strings.Repeat("hello", 20), 10, ""); !strings.HasSuffix(got, "\x1b[0m") {
		t.Fatalf("truncated without ellipsis should end with reset: %q", got)
	}
}

func TestTruncateToWidthPiLargeMalformedAndContiguousPrefix(t *testing.T) {
	large := strings.Repeat("🙂界", 100_000)
	truncated := TruncateToWidth(large, 40, "…")
	if VisibleWidth(truncated) > 40 {
		t.Fatalf("large unicode truncation width = %d, want <= 40", VisibleWidth(truncated))
	}
	if !strings.HasSuffix(truncated, "…\x1b[0m") {
		t.Fatalf("large unicode truncation should end with ellipsis reset: %q", truncated)
	}

	malformed := "abc\x1bnot-ansi " + strings.Repeat("🙂", 1000)
	if got := TruncateToWidth(malformed, 20, "…"); VisibleWidth(got) > 20 {
		t.Fatalf("malformed ANSI truncation width = %d, want <= 20: %q", VisibleWidth(got), got)
	}

	got := TruncateToWidth("🙂\t界 \x1b_abc\x07", 7, "…", true)
	want := "🙂\t\x1b[0m…\x1b[0m "
	if got != want {
		t.Fatalf("contiguous prefix truncation = %q, want %q", got, want)
	}
}

func TestNormalizeTerminalOutput(t *testing.T) {
	if got := NormalizeTerminalOutput("ำ"); got != "ํา" {
		t.Fatalf("thai AM normalized = %q", got)
	}
	if got := NormalizeTerminalOutput("ຳ"); got != "ໍາ" {
		t.Fatalf("lao AM normalized = %q", got)
	}
	for _, tc := range []struct {
		input string
		want  int
	}{
		{"ำ", 1},
		{"ຳ", 1},
		{"กำ", 2},
		{"ກຳ", 2},
		{"\t\x1b[31m界\x1b[0m", 5},
	} {
		if got := VisibleWidth(tc.input); got != tc.want {
			t.Fatalf("visible width %q = %d, want %d", tc.input, got, tc.want)
		}
	}
	if VisibleWidth(NormalizeTerminalOutput("ำabc")) != VisibleWidth("ำabc") {
		t.Fatalf("normalized Thai width changed")
	}
}

func TestWrapTextWithANSI(t *testing.T) {
	got := WrapTextWithANSI("alpha beta gamma", 8)
	want := []string{"alpha", "beta", "gamma"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("wrap = %#v, want %#v", got, want)
	}
}

func TestWrapTextWithANSIPartialFlagBeforeOverflow(t *testing.T) {
	got := WrapTextWithANSI("      - 🇨", 9)
	if len(got) != 2 {
		t.Fatalf("wrap = %#v, want 2 lines", got)
	}
	if VisibleWidth(got[0]) != 7 || VisibleWidth(got[1]) != 2 {
		t.Fatalf("wrapped widths = %d,%d for %#v", VisibleWidth(got[0]), VisibleWidth(got[1]), got)
	}

	keycap := WrapTextWithANSI("      - 1\ufe0f\u20e3", 9)
	if len(keycap) != 2 {
		t.Fatalf("keycap wrap = %#v, want 2 lines", keycap)
	}
	if VisibleWidth(keycap[0]) != 7 || VisibleWidth(keycap[1]) != 2 {
		t.Fatalf("keycap wrapped widths = %d,%d for %#v", VisibleWidth(keycap[0]), VisibleWidth(keycap[1]), keycap)
	}
}

func TestWrapTextWithANSIColonUnderlineDoesNotReplayDim(t *testing.T) {
	got := WrapTextWithANSI("\x1b[4:2malpha beta", 6)
	if len(got) < 2 {
		t.Fatalf("wrap = %#v, want multiple lines", got)
	}
	if strings.Contains(strings.Join(got, "\n"), "\x1b[2m") {
		t.Fatalf("underline substyle should not replay dim: %#v", got)
	}
	if !strings.HasPrefix(got[1], "\x1b[4m") {
		t.Fatalf("wrapped line should replay underline style: %#v", got)
	}
}

func TestWrapTextWithANSIColonExtendedColorReplaysStyle(t *testing.T) {
	got := WrapTextWithANSI("\x1b[38:2::12:34:56malpha beta", 6)
	if len(got) < 2 {
		t.Fatalf("wrap = %#v, want multiple lines", got)
	}
	if !strings.HasPrefix(got[1], "\x1b[38;2;12;34;56m") {
		t.Fatalf("wrapped line should replay RGB foreground style: %#v", got)
	}
}

func TestWrapTextWithANSIClosesTransientStylesAtLineBreaks(t *testing.T) {
	underlineOn := "\x1b[4m"
	underlineOff := "\x1b[24m"
	got := WrapTextWithANSI("prefix "+underlineOn+"UNDERLINED_CONTENT_THAT_WRAPS"+underlineOff+" suffix", 18)
	if len(got) < 3 {
		t.Fatalf("wrap = %#v, want multiple lines", got)
	}
	if got[0] != "prefix" {
		t.Fatalf("unstyled prefix line = %q, want prefix", got[0])
	}
	for i, line := range got[:len(got)-1] {
		if strings.Contains(line, underlineOn) && !strings.Contains(line, underlineOff) {
			t.Fatalf("underlined wrapped line %d should close underline: %#v", i, got)
		}
		if strings.HasSuffix(line, "\x1b[0m") {
			t.Fatalf("wrapped line %d should not use full reset: %#v", i, got)
		}
	}
}

func TestWrapTextWithANSIPiUnderlineAndBackgroundEdgeCases(t *testing.T) {
	t.Run("no whitespace before underline reset", func(t *testing.T) {
		underlineOn := "\x1b[4m"
		underlineOff := "\x1b[24m"
		got := WrapTextWithANSI(underlineOn+"underlined text here "+underlineOff+"more", 18)
		if len(got) == 0 {
			t.Fatalf("wrap returned no lines")
		}
		if strings.Contains(got[0], " "+underlineOff) {
			t.Fatalf("first wrapped line should not keep whitespace before underline reset: %#v", got)
		}
	})

	t.Run("background color survives wrapped lines", func(t *testing.T) {
		bgBlue := "\x1b[44m"
		reset := "\x1b[0m"
		got := WrapTextWithANSI(bgBlue+"hello world this is blue background text"+reset, 15)
		if len(got) < 2 {
			t.Fatalf("wrap = %#v, want multiple lines", got)
		}
		for i, line := range got {
			if !strings.Contains(line, bgBlue) {
				t.Fatalf("wrapped line %d should carry background color: %#v", i, got)
			}
		}
		for i, line := range got[:len(got)-1] {
			if strings.HasSuffix(line, reset) {
				t.Fatalf("middle wrapped line %d should not end with full reset: %#v", i, got)
			}
		}
	})

	t.Run("underline reset preserves active background", func(t *testing.T) {
		underlineOn := "\x1b[4m"
		underlineOff := "\x1b[24m"
		reset := "\x1b[0m"
		got := WrapTextWithANSI("\x1b[41mprefix "+underlineOn+"UNDERLINED_CONTENT_THAT_WRAPS"+underlineOff+" suffix"+reset, 20)
		if len(got) < 2 {
			t.Fatalf("wrap = %#v, want multiple lines", got)
		}
		for i, line := range got {
			if !strings.Contains(line, "[41m") && !strings.Contains(line, ";41m") && !strings.Contains(line, "[41;") {
				t.Fatalf("wrapped line %d should carry red background: %#v", i, got)
			}
		}
		for i, line := range got[:len(got)-1] {
			if (strings.Contains(line, "[4m") || strings.Contains(line, "[4;") || strings.Contains(line, ";4m")) &&
				!strings.Contains(line, underlineOff) {
				t.Fatalf("underlined wrapped line %d should close underline only: %#v", i, got)
			}
			if strings.HasSuffix(line, reset) {
				t.Fatalf("underlined middle line %d should not end with full reset: %#v", i, got)
			}
		}
	})

	t.Run("truncates pure whitespace to width", func(t *testing.T) {
		got := WrapTextWithANSI("  ", 1)
		if len(got) == 0 {
			t.Fatalf("wrap returned no lines")
		}
		if VisibleWidth(got[0]) > 1 {
			t.Fatalf("pure whitespace line width = %d, want <= 1: %#v", VisibleWidth(got[0]), got)
		}
	})
}

func TestWrapTextWithANSIOSC8HyperlinksCloseAndReopen(t *testing.T) {
	url := "https://example.com"
	open := "\x1b]8;;" + url + "\x1b\\"
	close := "\x1b]8;;\x1b\\"
	lines := WrapTextWithANSI(open+"0123456789"+close, 6)
	if len(lines) != 2 {
		t.Fatalf("hyperlink wrap lines = %#v, want 2 lines", lines)
	}
	for i, line := range lines {
		if !strings.HasPrefix(line, open) {
			t.Fatalf("line %d should reopen OSC 8 hyperlink: %#v", i, lines)
		}
	}
	if !strings.HasSuffix(lines[0], close) {
		t.Fatalf("first hyperlink line should close before line break: %#v", lines)
	}

	belOpen := "\x1b]8;;" + url + "\x07"
	belClose := "\x1b]8;;\x07"
	belLines := WrapTextWithANSI(belOpen+strings.Repeat("a", 10)+belClose, 6)
	if len(belLines) != 2 {
		t.Fatalf("BEL hyperlink wrap lines = %#v, want 2 lines", belLines)
	}
	if !strings.HasPrefix(belLines[1], belOpen) || !strings.HasSuffix(belLines[0], belClose) {
		t.Fatalf("BEL hyperlink terminator should be preserved: %#v", belLines)
	}

	unwrapped := WrapTextWithANSI("before "+open+"link"+close+" after", 80)
	if len(unwrapped) != 1 {
		t.Fatalf("wide hyperlink line should not wrap: %#v", unwrapped)
	}
	if got := strings.Count(unwrapped[0], open); got != 1 {
		t.Fatalf("wide hyperlink line should contain one OSC 8 open, got %d in %q", got, unwrapped[0])
	}
	if got := strings.Count(unwrapped[0], close); got != 1 {
		t.Fatalf("wide hyperlink line should contain one OSC 8 close, got %d in %q", got, unwrapped[0])
	}
}

func TestWrapTextWithANSIPiOSCVisibleWidthMarkers(t *testing.T) {
	bel := "\x1b]133;A\x07hello\x1b]133;B\x07"
	if got := VisibleWidth(bel); got != 5 {
		t.Fatalf("BEL OSC 133 width = %d, want 5", got)
	}
	st := "\x1b]133;A\x1b\\hello\x1b]133;B\x1b\\"
	if got := VisibleWidth(st); got != 5 {
		t.Fatalf("ST OSC 133 width = %d, want 5", got)
	}
}

func TestWrapTextWithANSICarriesActiveStateAcrossLiteralNewlines(t *testing.T) {
	red := "\x1b[31m"
	reset := "\x1b[0m"
	lines := WrapTextWithANSI(red+"hello\nworld"+reset, 80)
	if len(lines) != 2 {
		t.Fatalf("styled newline lines = %#v, want 2", lines)
	}
	if !strings.HasPrefix(lines[1], red) {
		t.Fatalf("second literal newline line should replay active SGR style: %#v", lines)
	}

	url := "https://example.com"
	open := "\x1b]8;;" + url + "\x07"
	close := "\x1b]8;;\x07"
	lines = WrapTextWithANSI(open+"hello\nworld"+close, 80)
	if len(lines) != 2 {
		t.Fatalf("hyperlink newline lines = %#v, want 2", lines)
	}
	if !strings.HasPrefix(lines[1], open) || strings.Contains(lines[1], "\x1b]8;;"+url+"\x1b\\") {
		t.Fatalf("second literal newline line should replay BEL OSC 8 hyperlink: %#v", lines)
	}
}

func TestExtractAnsiCodePiSequences(t *testing.T) {
	for _, sample := range []string{
		"\x1b[31mred",
		"\x1b]8;;https://example.com\x07link",
		"\x1b]8;;https://example.com\x1b\\link",
		"\x1b_pi:c\x07",
	} {
		code, ok := ExtractAnsiCode(sample, 0)
		if !ok || code.Length <= 0 || code.Code != sample[:code.Length] {
			t.Fatalf("ExtractAnsiCode(%q) = %#v ok=%v", sample, code, ok)
		}
	}
	if _, ok := ExtractAnsiCode("plain", 0); ok {
		t.Fatal("plain text should not be reported as ANSI")
	}
}

func TestSliceByColumnPreservesAnsiAndWideBoundaries(t *testing.T) {
	line := "\x1b[31mab你cd\x1b[0m"
	got := SliceWithWidth(line, 2, 3, true)
	if got.Width != 3 || stripANSI(got.Text) != "你c" {
		t.Fatalf("strict slice = %#v, want width 3 text 你c", got)
	}
	if !strings.HasPrefix(got.Text, "\x1b[31m") {
		t.Fatalf("slice should replay pending ANSI style: %q", got.Text)
	}
	if got := SliceWithWidth(line, 2, 1, true); got.Text != "" || got.Width != 0 {
		t.Fatalf("strict boundary should exclude wide grapheme: %#v", got)
	}
	if got := SliceWithWidth(line, 2, 1, false); stripANSI(got.Text) != "你" || got.Width != 2 {
		t.Fatalf("non-strict boundary should include wide grapheme: %#v", got)
	}
}

func TestExtractSegmentsPreservesStyledAfterSegment(t *testing.T) {
	segments := ExtractSegments("\x1b[31mabcdef\x1b[0m", 2, 4, 2, true)
	if stripANSI(segments.Before) != "ab" || segments.BeforeWidth != 2 {
		t.Fatalf("before segment = %#v", segments)
	}
	if stripANSI(segments.After) != "ef" || segments.AfterWidth != 2 {
		t.Fatalf("after segment = %#v", segments)
	}
	if !strings.HasPrefix(segments.After, "\x1b[31m") {
		t.Fatalf("after segment should inherit active style: %q", segments.After)
	}
}
