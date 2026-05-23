package gicodingagent

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestEditToolFuzzyMatchingPiMatrix(t *testing.T) {
	dir := t.TempDir()
	tool := NewEditTool(dir)

	cases := []struct {
		name       string
		initial    string
		oldText    string
		newText    string
		wantSubstr string
	}{
		{
			name:       "trailing-ws",
			initial:    "line one   \nline two  \nline three\n",
			oldText:    "line one\nline two\n",
			newText:    "replaced\n",
			wantSubstr: "replaced\nline three\n",
		},
		{
			name:       "chinese-punctuation",
			initial:    "你好，世界\n你好（世界）\n",
			oldText:    "你好,世界\n你好(世界)\n",
			newText:    "你好，pi\n你好(pi)\n",
			wantSubstr: "你好，pi\n你好(pi)\n",
		},
		{
			name:       "unicode-compatibility",
			initial:    "ＡＢＣ１２３\ncafe\u0301\n",
			oldText:    "ABC123\ncafé\n",
			newText:    "XYZ789\ncoffee\n",
			wantSubstr: "XYZ789\ncoffee\n",
		},
		{
			name:       "smart-single-quotes",
			initial:    "console.log(\u2018hello\u2019);\n",
			oldText:    "console.log('hello');",
			newText:    "console.log('world');",
			wantSubstr: "world",
		},
		{
			name:       "smart-double-quotes",
			initial:    "const msg = \u201cHello World\u201d;\n",
			oldText:    `const msg = "Hello World";`,
			newText:    `const msg = "Goodbye";`,
			wantSubstr: "Goodbye",
		},
		{
			name:       "unicode-dashes",
			initial:    "range: 1\u20135\nbreak\u2014here\n",
			oldText:    "range: 1-5\nbreak-here",
			newText:    "range: 10-50\nbreak--here",
			wantSubstr: "10-50",
		},
		{
			name:       "nbsp",
			initial:    "hello\u00a0world\n",
			oldText:    "hello world",
			newText:    "hello universe",
			wantSubstr: "hello universe",
		},
		{
			name:       "special-unicode-space",
			initial:    "hello\u2003world\n",
			oldText:    "hello world",
			newText:    "hello universe",
			wantSubstr: "hello universe",
		},
		{
			name:       "low-smart-quotes",
			initial:    "console.log(\u201ahello\u201b);\nconst msg = \u201eHello\u201f;\n",
			oldText:    "console.log('hello');\nconst msg = \"Hello\";",
			newText:    "console.log('world');\nconst msg = \"World\";",
			wantSubstr: "World",
		},
		{
			name:       "horizontal-bar",
			initial:    "range: 1\u20155\n",
			oldText:    "range: 1-5",
			newText:    "range: 10-50",
			wantSubstr: "range: 10-50",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testFile := filepath.Join(dir, tc.name+".txt")
			writeEditToolFile(t, testFile, tc.initial)
			result, err := tool.Execute("test-fuzzy-"+tc.name, EditToolInput{
				Path:  testFile,
				Edits: []Edit{{OldText: tc.oldText, NewText: tc.newText}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(readToolText(result), "Successfully replaced") {
				t.Fatalf("output = %q", readToolText(result))
			}
			if content := readEditToolFile(t, testFile); !strings.Contains(content, tc.wantSubstr) {
				t.Fatalf("content = %q, missing %q", content, tc.wantSubstr)
			}
		})
	}

	exactFile := filepath.Join(dir, "exact-preferred.txt")
	writeEditToolFile(t, exactFile, "const x = 'exact';\nconst y = 'other';\n")
	if _, err := tool.Execute("test-fuzzy-exact", EditToolInput{
		Path:  exactFile,
		Edits: []Edit{{OldText: "const x = 'exact';", NewText: "const x = 'changed';"}},
	}); err != nil {
		t.Fatal(err)
	}
	if content := readEditToolFile(t, exactFile); content != "const x = 'changed';\nconst y = 'other';\n" {
		t.Fatalf("exact preferred content = %q", content)
	}

	noMatchFile := filepath.Join(dir, "no-match.txt")
	writeEditToolFile(t, noMatchFile, "completely different content\n")
	if _, err := tool.Execute("test-fuzzy-no-match", EditToolInput{
		Path:  noMatchFile,
		Edits: []Edit{{OldText: "this does not exist", NewText: "replacement"}},
	}); err == nil || !strings.Contains(err.Error(), "Could not find the exact text") {
		t.Fatalf("no match err = %v", err)
	}

	dupsFile := filepath.Join(dir, "fuzzy-dups.txt")
	writeEditToolFile(t, dupsFile, "hello world   \nhello world\n")
	if _, err := tool.Execute("test-fuzzy-dups", EditToolInput{
		Path:  dupsFile,
		Edits: []Edit{{OldText: "hello world", NewText: "replaced"}},
	}); err == nil || !strings.Contains(err.Error(), "Found 2 occurrences") {
		t.Fatalf("fuzzy dup err = %v", err)
	}

	exactAndFuzzyDupsFile := filepath.Join(dir, "exact-and-fuzzy-dups.txt")
	writeEditToolFile(t, exactAndFuzzyDupsFile, "console.log('hello');\nconsole.log(\u2018hello\u2019);\n")
	if _, err := tool.Execute("test-fuzzy-exact-dups", EditToolInput{
		Path:  exactAndFuzzyDupsFile,
		Edits: []Edit{{OldText: "console.log('hello');", NewText: "console.log('world');"}},
	}); err == nil || !strings.Contains(err.Error(), "Found 2 occurrences") {
		t.Fatalf("exact fuzzy dup err = %v", err)
	}

	multiFile := filepath.Join(dir, "fuzzy-multi.txt")
	writeEditToolFile(t, multiFile, "console.log(\u2018hello\u2019);\nhello\u00a0world\n")
	if _, err := tool.Execute("test-fuzzy-multi", EditToolInput{
		Path: multiFile,
		Edits: []Edit{
			{OldText: "console.log('hello');\n", NewText: "console.log('world');\n"},
			{OldText: "hello world\n", NewText: "hello universe\n"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if content := readEditToolFile(t, multiFile); content != "console.log('world');\nhello universe\n" {
		t.Fatalf("fuzzy multi content = %q", content)
	}
}

func TestEditToolCRLFPiMatrix(t *testing.T) {
	dir := t.TempDir()
	tool := NewEditTool(dir)

	crlfFile := filepath.Join(dir, "crlf-test.txt")
	writeEditToolFile(t, crlfFile, "line one\r\nline two\r\nline three\r\n")
	if _, err := tool.Execute("test-crlf-1", EditToolInput{
		Path:  crlfFile,
		Edits: []Edit{{OldText: "line two\n", NewText: "replaced line\n"}},
	}); err != nil {
		t.Fatal(err)
	}

	preserveFile := filepath.Join(dir, "crlf-preserve.txt")
	writeEditToolFile(t, preserveFile, "first\r\nsecond\r\nthird\r\n")
	if _, err := tool.Execute("test-crlf-2", EditToolInput{
		Path:  preserveFile,
		Edits: []Edit{{OldText: "second\n", NewText: "REPLACED\n"}},
	}); err != nil {
		t.Fatal(err)
	}
	if content := readEditToolFile(t, preserveFile); content != "first\r\nREPLACED\r\nthird\r\n" {
		t.Fatalf("CRLF content = %q", content)
	}

	lfFile := filepath.Join(dir, "lf-preserve.txt")
	writeEditToolFile(t, lfFile, "first\nsecond\nthird\n")
	if _, err := tool.Execute("test-lf-1", EditToolInput{
		Path:  lfFile,
		Edits: []Edit{{OldText: "second\n", NewText: "REPLACED\n"}},
	}); err != nil {
		t.Fatal(err)
	}
	if content := readEditToolFile(t, lfFile); content != "first\nREPLACED\nthird\n" {
		t.Fatalf("LF content = %q", content)
	}

	mixedFile := filepath.Join(dir, "mixed-endings.txt")
	writeEditToolFile(t, mixedFile, "hello\r\nworld\r\n---\r\nhello\nworld\n")
	if _, err := tool.Execute("test-crlf-dup", EditToolInput{
		Path:  mixedFile,
		Edits: []Edit{{OldText: "hello\nworld\n", NewText: "replaced\n"}},
	}); err == nil || !strings.Contains(err.Error(), "Found 2 occurrences") {
		t.Fatalf("mixed duplicate err = %v", err)
	}

	bomFile := filepath.Join(dir, "bom-test.txt")
	writeEditToolFile(t, bomFile, "\ufefffirst\r\nsecond\r\nthird\r\n")
	if _, err := tool.Execute("test-bom", EditToolInput{
		Path:  bomFile,
		Edits: []Edit{{OldText: "second\n", NewText: "REPLACED\n"}},
	}); err != nil {
		t.Fatal(err)
	}
	if content := readEditToolFile(t, bomFile); content != "\ufefffirst\r\nREPLACED\r\nthird\r\n" {
		t.Fatalf("BOM content = %q", content)
	}

	bomFirstLineFile := filepath.Join(dir, "bom-first-line.txt")
	writeEditToolFile(t, bomFirstLineFile, "\ufefffirst\r\nsecond\r\n")
	if _, err := tool.Execute("test-bom-first-line", EditToolInput{
		Path:  bomFirstLineFile,
		Edits: []Edit{{OldText: "first\n", NewText: "FIRST\n"}},
	}); err != nil {
		t.Fatal(err)
	}
	if content := readEditToolFile(t, bomFirstLineFile); content != "\ufeffFIRST\r\nsecond\r\n" {
		t.Fatalf("BOM first line content = %q", content)
	}

	multiFile := filepath.Join(dir, "bom-crlf-multi.txt")
	writeEditToolFile(t, multiFile, "\ufefffirst\r\nsecond\r\nthird\r\nfourth\r\n")
	if _, err := tool.Execute("test-crlf-multi", EditToolInput{
		Path: multiFile,
		Edits: []Edit{
			{OldText: "second\n", NewText: "SECOND\n"},
			{OldText: "fourth\n", NewText: "FOURTH\n"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if content := readEditToolFile(t, multiFile); content != "\ufefffirst\r\nSECOND\r\nthird\r\nFOURTH\r\n" {
		t.Fatalf("multi CRLF content = %q", content)
	}

}
