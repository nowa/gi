package gillmprovider

import "strings"

// JoinTextContent extracts text blocks in transcript order and joins them with
// separator. Thinking, image, and tool-call blocks are deliberately ignored.
func JoinTextContent(content []ContentPart, separator string) string {
	parts := make([]string, 0, len(content))
	for _, part := range content {
		if part.Type == ContentText {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, separator)
}

// ExtractTextContent is the common newline-joined form.
func ExtractTextContent(content []ContentPart) string {
	return JoinTextContent(content, "\n")
}
