package harness

import (
	"testing"
	"unicode/utf8"
)

func TestTruncateCountsUTF8Bytes(t *testing.T) {
	content := "aé🙂\nb"
	result := TruncateHead(content, TruncationOptions{MaxBytes: 100, MaxLines: 10})
	if result.Truncated || result.TotalBytes != len([]byte(content)) || result.OutputBytes != len([]byte(content)) || result.TotalBytes != 9 {
		t.Fatalf("result = %#v", result)
	}
}

func TestTruncateHeadOnUTF8ByteLimitsWithoutPartialLines(t *testing.T) {
	result := TruncateHead("éé\nabc", TruncationOptions{MaxBytes: 4, MaxLines: 10})
	if result.Content != "éé" || !result.Truncated || result.TruncatedBy != TruncatedByBytes || result.OutputBytes != 4 || result.FirstLineExceedsLimit {
		t.Fatalf("result = %#v", result)
	}
}

func TestTruncateHeadReportsFirstLineExceedsByteLimit(t *testing.T) {
	result := TruncateHead("éé\nabc", TruncationOptions{MaxBytes: 3, MaxLines: 10})
	if result.Content != "" || !result.Truncated || result.TruncatedBy != TruncatedByBytes || !result.FirstLineExceedsLimit {
		t.Fatalf("result = %#v", result)
	}
}

func TestTruncateTailUTF8Boundaries(t *testing.T) {
	result := TruncateTail("aé🙂b", TruncationOptions{MaxBytes: 5, MaxLines: 10})
	if result.Content != "🙂b" || !result.Truncated || result.TruncatedBy != TruncatedByBytes || !result.LastLinePartial || result.OutputBytes != 5 {
		t.Fatalf("result = %#v", result)
	}
}

func TestTruncateTailDropsOversizedTrailingCharacter(t *testing.T) {
	result := TruncateTail("abc🙂", TruncationOptions{MaxBytes: 3, MaxLines: 10})
	if result.Content != "" || !result.Truncated || result.TruncatedBy != TruncatedByBytes || !result.LastLinePartial || result.OutputBytes != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestTruncateTailNeverReturnsInvalidUTF8(t *testing.T) {
	inputs := []string{"a🙂", "👩‍💻", "中🙂b", "ééé"}
	for _, input := range inputs {
		for maxBytes := 0; maxBytes <= len([]byte(input))+4; maxBytes++ {
			result := TruncateTail(input, TruncationOptions{MaxBytes: maxBytes, MaxLines: 10})
			if !utf8.ValidString(result.Content) {
				t.Fatalf("invalid utf8 for input=%q maxBytes=%d content=%q", input, maxBytes, result.Content)
			}
			if result.OutputBytes > maxBytes {
				t.Fatalf("output exceeded max bytes for input=%q maxBytes=%d result=%#v", input, maxBytes, result)
			}
		}
	}
}
