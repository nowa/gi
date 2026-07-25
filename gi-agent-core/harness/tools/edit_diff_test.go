package tools

import (
	"reflect"
	"strings"
	"testing"
)

func TestEditDiffReplacementLineModel(t *testing.T) {
	content := "first\nsecond\nthird"
	if got, want := splitLinesWithEndings(content), []string{"first\n", "second\n", "third"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("split lines = %#v, want %#v", got, want)
	}
	spans := getLineSpans(content)
	if got, want := spans, []lineSpan{{start: 0, end: 6}, {start: 6, end: 13}, {start: 13, end: 18}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("line spans = %#v, want %#v", got, want)
	}
	replacement := editRange{start: 6, end: 13, replacement: "SECOND\n"}
	startLine, endLine, err := getReplacementLineRange(spans, replacement)
	if err != nil || startLine != 1 || endLine != 2 {
		t.Fatalf("replacement line range = (%d, %d, %v)", startLine, endLine, err)
	}
	got, err := applyReplacements(content, []editRange{
		{start: 0, end: 5, replacement: "FIRST"},
		replacement,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != "FIRST\nSECOND\nthird" {
		t.Fatalf("applied replacements = %q", got)
	}
}

func TestApplyReplacementsPreservingUnchangedLinesPiRegressions(t *testing.T) {
	t.Run("replacement equal to nearby normalized line keeps the nearby original", func(t *testing.T) {
		original := "replace me   \nafter   \n"
		base := NormalizeForFuzzyMatch(original)
		start := strings.Index(base, "replace me\n")
		got, err := applyReplacementsPreservingUnchangedLines(original, base, []editRange{{
			start:       start,
			end:         start + len("replace me\n"),
			replacement: "after\n",
		}})
		if err != nil {
			t.Fatal(err)
		}
		if want := "after\nafter   \n"; got != want {
			t.Fatalf("preserved content = %q, want %q", got, want)
		}
	})

	t.Run("multi edit keeps untouched lines byte for byte", func(t *testing.T) {
		original := strings.Join([]string{
			"keep before  ",
			"first target  ",
			"first after",
			"keep middle   ",
			"second target  ",
			"second after",
			"keep after  ",
			"",
		}, "\n")
		base := NormalizeForFuzzyMatch(original)
		replacements := []editRange{
			replacementForText(t, base, "first target\nfirst after", "FIRST\nFIRST2", 0),
			replacementForText(t, base, "second target\nsecond after", "SECOND\nSECOND2", 1),
		}
		got, err := applyReplacementsPreservingUnchangedLines(original, base, replacements)
		if err != nil {
			t.Fatal(err)
		}
		want := strings.Join([]string{
			"keep before  ",
			"FIRST",
			"FIRST2",
			"keep middle   ",
			"SECOND",
			"SECOND2",
			"keep after  ",
			"",
		}, "\n")
		if got != want {
			t.Fatalf("preserved content = %q, want %q", got, want)
		}
	})

	t.Run("rejects a base with a different line count", func(t *testing.T) {
		_, err := applyReplacementsPreservingUnchangedLines(
			"one\ntwo\n",
			"one two\n",
			[]editRange{{start: 0, end: 3, replacement: "ONE"}},
		)
		if err == nil || !strings.Contains(err.Error(), "different line count") {
			t.Fatalf("line-count error = %v", err)
		}
	})
}

func TestApplyEditsToNormalizedContentPreservesUntouchedFuzzyLinesAndPatchContext(t *testing.T) {
	original := strings.Join([]string{
		"keep before  ",
		"first target  ",
		"first after",
		"keep middle   ",
		"second target  ",
		"second after",
		"keep after  ",
		"",
	}, "\n")
	applied, err := ApplyEditsToNormalizedContent(original, []Edit{
		{OldText: "first target\nfirst after", NewText: "FIRST\nFIRST2"},
		{OldText: "second target\nsecond after", NewText: "SECOND\nSECOND2"},
	}, "edit.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"keep before  ",
		"FIRST",
		"FIRST2",
		"keep middle   ",
		"SECOND",
		"SECOND2",
		"keep after  ",
		"",
	}, "\n")
	if applied.NewContent != want {
		t.Fatalf("new content = %q, want %q", applied.NewContent, want)
	}
	for _, contextLine := range []string{" keep before  ", " keep middle   ", " keep after  "} {
		if !strings.Contains(applied.Patch, contextLine) {
			t.Fatalf("patch does not preserve context line %q:\n%s", contextLine, applied.Patch)
		}
	}
}

func replacementForText(t *testing.T, content, oldText, newText string, index int) editRange {
	t.Helper()
	start := strings.Index(content, oldText)
	if start < 0 {
		t.Fatalf("text %q missing from %q", oldText, content)
	}
	return editRange{
		start:       start,
		end:         start + len(oldText),
		index:       index,
		replacement: newText,
	}
}
