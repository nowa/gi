package gicodingagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFrontmatterParsingMatchesPiUtilities(t *testing.T) {
	parsed, err := ParseFrontmatter("---\nname: \"skill-name\"\ndescription: 'A desc'\nfoo-bar: value\n---\n\nBody text")
	if err != nil {
		t.Fatalf("ParseFrontmatter returned error: %v", err)
	}
	if parsed.Frontmatter["name"] != "skill-name" || parsed.Frontmatter["description"] != "A desc" || parsed.Frontmatter["foo-bar"] != "value" {
		t.Fatalf("frontmatter = %#v", parsed.Frontmatter)
	}
	if parsed.Body != "Body text" {
		t.Fatalf("body = %q, want Body text", parsed.Body)
	}

	parsed, err = ParseFrontmatter("---\r\nname: test\r\n---\r\nLine one\r\nLine two")
	if err != nil {
		t.Fatalf("CRLF ParseFrontmatter returned error: %v", err)
	}
	if parsed.Body != "Line one\nLine two" {
		t.Fatalf("CRLF body = %q", parsed.Body)
	}

	parsed, err = ParseFrontmatter("---\ndescription: |\n  Line one\n  Line two\n---\n\nBody")
	if err != nil {
		t.Fatalf("multiline ParseFrontmatter returned error: %v", err)
	}
	if got := parsed.Frontmatter["description"]; got != "Line one\nLine two\n" {
		t.Fatalf("multiline description = %q", got)
	}
	if parsed.Body != "Body" {
		t.Fatalf("multiline body = %q", parsed.Body)
	}

	if _, err := ParseFrontmatter("---\nfoo: [bar\n---\nBody"); err == nil || !strings.Contains(err.Error(), "line 1, column 10") {
		t.Fatalf("invalid YAML error = %v, want line/column", err)
	}

	missing, err := ParseFrontmatter("---\nname: test\nBody without terminator")
	if err != nil {
		t.Fatalf("missing terminator returned error: %v", err)
	}
	if missing.Body != "---\nname: test\nBody without terminator" || len(missing.Frontmatter) != 0 {
		t.Fatalf("missing terminator result = %#v", missing)
	}

	comments, err := ParseFrontmatter("---\n# just a comment\n---\nBody")
	if err != nil {
		t.Fatalf("comment-only returned error: %v", err)
	}
	if len(comments.Frontmatter) != 0 {
		t.Fatalf("comment-only frontmatter = %#v, want empty", comments.Frontmatter)
	}
	if got := StripFrontmatter("---\nkey: value\n---\n\nBody\n"); got != "Body" {
		t.Fatalf("StripFrontmatter = %q, want Body", got)
	}
	if got := StripFrontmatter("\n  No frontmatter body  \n"); got != "\n  No frontmatter body  \n" {
		t.Fatalf("StripFrontmatter without frontmatter = %q", got)
	}
}

func TestPathUtilitiesMatchPiReadPathBehavior(t *testing.T) {
	if got := ExpandPath("file\u00a0name.txt"); got != "file name.txt" {
		t.Fatalf("ExpandPath NBSP = %q", got)
	}
	if got := ExpandPath("~"); strings.Contains(got, "~") {
		t.Fatalf("ExpandPath home did not expand: %q", got)
	}
	if got := ResolveToCwd("relative/file.txt", "/some/cwd"); got != filepath.Clean("/some/cwd/relative/file.txt") {
		t.Fatalf("ResolveToCwd relative = %q", got)
	}
	if got := ResolveToCwd("/absolute/path/file.txt", "/some/cwd"); got != filepath.Clean("/absolute/path/file.txt") {
		t.Fatalf("ResolveToCwd absolute = %q", got)
	}

	temp := t.TempDir()
	existing := filepath.Join(temp, "test-file.txt")
	if err := os.WriteFile(existing, []byte("content"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := ResolveReadPath("test-file.txt", temp)
	if err != nil || got != existing {
		t.Fatalf("ResolveReadPath existing = %q err=%v", got, err)
	}

	nfdName := "filee\u0301.txt"
	if err := os.WriteFile(filepath.Join(temp, nfdName), []byte("content"), 0o600); err != nil {
		t.Fatalf("write nfd fixture: %v", err)
	}
	got, err = ResolveReadPath("file\u00e9.txt", temp)
	if err != nil || comparableUserPathText(filepath.Base(got)) != comparableUserPathText(nfdName) {
		t.Fatalf("ResolveReadPath NFC/NFD = %q err=%v", got, err)
	}

	curlyName := "Capture d\u2019cran.txt"
	if err := os.WriteFile(filepath.Join(temp, curlyName), []byte("content"), 0o600); err != nil {
		t.Fatalf("write curly fixture: %v", err)
	}
	got, err = ResolveReadPath("Capture d'cran.txt", temp)
	if err != nil || filepath.Base(got) != curlyName {
		t.Fatalf("ResolveReadPath curly quote = %q err=%v", got, err)
	}

	screenshotName := "Screenshot 2024-01-01 at 10.00.00\u202fAM.png"
	if err := os.WriteFile(filepath.Join(temp, screenshotName), []byte("content"), 0o600); err != nil {
		t.Fatalf("write screenshot fixture: %v", err)
	}
	got, err = ResolveReadPath("Screenshot 2024-01-01 at 10.00.00 AM.png", temp)
	if err != nil || filepath.Base(got) != screenshotName {
		t.Fatalf("ResolveReadPath screenshot AM = %q err=%v", got, err)
	}

	lowerTemp := t.TempDir()
	lowerScreenshotName := "Screenshot 2024-01-01 at 10.00.00\u202fam.png"
	if err := os.WriteFile(filepath.Join(lowerTemp, lowerScreenshotName), []byte("content"), 0o600); err != nil {
		t.Fatalf("write lowercase screenshot fixture: %v", err)
	}
	got, err = ResolveReadPath("Screenshot 2024-01-01 at 10.00.00 am.png", lowerTemp)
	if err != nil || filepath.Base(got) != lowerScreenshotName {
		t.Fatalf("ResolveReadPath screenshot am = %q err=%v", got, err)
	}
}

func TestStripAnsiMatchesPiCompatibilityInputs(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
	}{
		{"plain", "plain"},
		{"a\x1b[31mred\x1b[0mz", "aredz"},
		{"a\x1b]8;;https://example.com\x07link\x1b]8;;\x07z", "alinkz"},
		{"a\x1b]unterminated", "anterminated"},
		{"a\x1b]funterminated", "aunterminated"},
		{"a\x1bPabc\x1b\\z", "aabc\x1b\\z"},
		{"a\x1b_abc\u009cz", "a\x1b_abc\u009cz"},
		{"a\u0090abc\u009cz", "a\u0090abc\u009cz"},
		{"a\u009dabc\u009cz", "a\u009dabc\u009cz"},
		{"a\u009b31mred", "ared"},
		{"a\x1b(0x", "ax"},
		{"a\x1b*0x", "a\x1b*0x"},
		{"a\x1b+c", "a\x1b+c"},
		{"a\x1b/0x", "a\x1b/0x"},
		{"a\x1bcok", "aok"},
		{"a\x1b\\ok", "a\x1b\\ok"},
		{"a\x1b[31mred\x1b[0m\x1b]8;;https://example.com\x07link\x1b]8;;\x07z", "aredlinkz"},
	} {
		if got := StripAnsi(tc.input); got != tc.want {
			t.Fatalf("StripAnsi(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}

	for code := 'g'; code <= 'm'; code++ {
		if got := StripAnsi("\x1b" + string(code) + "ok"); got != "ok" {
			t.Fatalf("StripAnsi ESC %c = %q, want ok", code, got)
		}
	}
	for code := 'r'; code <= 't'; code++ {
		if got := StripAnsi("\x1b" + string(code) + "ok"); got != "ok" {
			t.Fatalf("StripAnsi ESC %c = %q, want ok", code, got)
		}
	}
}
