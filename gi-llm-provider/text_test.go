package gillmprovider

import "testing"

func TestContentText(t *testing.T) {
	content := []ContentPart{
		Thinking("reasoning"),
		Text("first"),
		ToolCall("1", "read", nil),
		Text("second"),
	}

	t.Run("extracts assistant text blocks", func(t *testing.T) {
		if got := ExtractTextContent(content); got != "first\nsecond" {
			t.Fatalf("text = %q", got)
		}
	})

	t.Run("supports custom separators", func(t *testing.T) {
		if got := JoinTextContent(content, ""); got != "firstsecond" {
			t.Fatalf("text = %q", got)
		}
	})

	t.Run("passes string content through", func(t *testing.T) {
		if got := ExtractTextContent([]ContentPart{Text("hello")}); got != "hello" {
			t.Fatalf("text = %q", got)
		}
	})

	t.Run("extracts text from tool-result content", func(t *testing.T) {
		toolResult := []ContentPart{
			Text("first"),
			Image("...", "image/png"),
			Text("second"),
		}
		if got := JoinTextContent(toolResult, ""); got != "firstsecond" {
			t.Fatalf("text = %q", got)
		}
	})
}
