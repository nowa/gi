package gicodingagent

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
)

const (
	defaultReadToolLineLimit = 2000
	defaultReadToolByteLimit = 50 * 1024
)

type FileToolOperations struct {
	Access      func(path string) error
	ReadFile    func(path string) ([]byte, error)
	WriteFile   func(path string, content []byte) error
	MkdirAll    func(path string) error
	ResizeImage func(part llm.ContentPart, options ImageResizeOptions) *ResizedImage
}

type Edit struct {
	OldText string
	NewText string
}

type EditToolInput struct {
	Path  string
	Edits []Edit
}

type WriteToolInput struct {
	Path    string
	Content string
}

type FileToolResult struct {
	Text    string
	Content []llm.ContentPart
	Details *FileToolDetails
}

type FileToolDetails struct {
	Truncation *ReadToolTruncation
}

type ReadToolTruncation struct {
	Truncated   bool
	TruncatedBy string
	TotalLines  int
	OutputLines int
}

type EditTool struct {
	cwd string
	ops FileToolOperations
}

type WriteTool struct {
	cwd string
	ops FileToolOperations
}

type ReadTool struct {
	cwd              string
	ops              FileToolOperations
	autoResizeImages bool
}

type ReadToolInput struct {
	Path   string
	Offset int
	Limit  int
}

type ReadToolOptions struct {
	AutoResizeImages *bool
}

func NewEditTool(cwd string, operations ...FileToolOperations) EditTool {
	return EditTool{cwd: cwd, ops: normalizeFileToolOperations(operations...)}
}

func NewWriteTool(cwd string, operations ...FileToolOperations) WriteTool {
	return WriteTool{cwd: cwd, ops: normalizeFileToolOperations(operations...)}
}

func NewReadTool(cwd string, operations ...FileToolOperations) ReadTool {
	return NewReadToolWithOptions(cwd, ReadToolOptions{}, operations...)
}

func NewReadToolWithOptions(cwd string, options ReadToolOptions, operations ...FileToolOperations) ReadTool {
	autoResizeImages := true
	if options.AutoResizeImages != nil {
		autoResizeImages = *options.AutoResizeImages
	}
	return ReadTool{cwd: cwd, ops: normalizeFileToolOperations(operations...), autoResizeImages: autoResizeImages}
}

func (t EditTool) Execute(_ string, input EditToolInput) (FileToolResult, error) {
	if strings.TrimSpace(input.Path) == "" {
		return FileToolResult{}, fmt.Errorf("edit path is required")
	}
	if len(input.Edits) == 0 {
		return FileToolResult{}, fmt.Errorf("edits must contain at least one replacement")
	}
	absolutePath := ResolveToCwd(input.Path, t.cwd)
	var result FileToolResult
	err := WithFileMutationQueue(absolutePath, func() error {
		if err := t.ops.Access(absolutePath); err != nil {
			return fmt.Errorf("could not edit file: %s: %w", input.Path, err)
		}
		content, err := t.ops.ReadFile(absolutePath)
		if err != nil {
			return err
		}
		nextContent, err := applySimpleEdits(string(content), input.Edits)
		if err != nil {
			return err
		}
		if err := t.ops.WriteFile(absolutePath, []byte(nextContent)); err != nil {
			return err
		}
		result = FileToolResult{Text: fmt.Sprintf("Successfully edited %s", input.Path)}
		return nil
	})
	return result, err
}

func (t WriteTool) Execute(_ string, input WriteToolInput) (FileToolResult, error) {
	if strings.TrimSpace(input.Path) == "" {
		return FileToolResult{}, fmt.Errorf("write path is required")
	}
	absolutePath := ResolveToCwd(input.Path, t.cwd)
	err := WithFileMutationQueue(absolutePath, func() error {
		if err := t.ops.MkdirAll(filepath.Dir(absolutePath)); err != nil {
			return err
		}
		return t.ops.WriteFile(absolutePath, []byte(input.Content))
	})
	if err != nil {
		return FileToolResult{}, err
	}
	return FileToolResult{Text: fmt.Sprintf("Successfully wrote %d bytes to %s", len(input.Content), input.Path)}, nil
}

func (t ReadTool) Execute(_ string, input ReadToolInput) (FileToolResult, error) {
	if strings.TrimSpace(input.Path) == "" {
		return FileToolResult{}, fmt.Errorf("read path is required")
	}
	absolutePath, err := ResolveReadPath(input.Path, t.cwd)
	if err != nil {
		return FileToolResult{}, fmt.Errorf("could not read file: %s: %w", input.Path, err)
	}
	content, err := t.ops.ReadFile(absolutePath)
	if err != nil {
		return FileToolResult{}, err
	}
	if mimeType := detectSupportedImageMIMEType(content); mimeType != "" {
		imagePart := llm.Image(base64.StdEncoding.EncodeToString(content), mimeType)
		note := fmt.Sprintf("Read image file [%s]", mimeType)
		if t.autoResizeImages {
			resized := t.ops.ResizeImage(imagePart, ImageResizeOptions{})
			if resized == nil {
				return FileToolResult{
					Text:    note + "\n[Image omitted: could not be resized below the inline image size limit.]",
					Content: []llm.ContentPart{llm.Text(note + "\n[Image omitted: could not be resized below the inline image size limit.]")},
				}, nil
			}
			imagePart = llm.Image(resized.Data, resized.MIMEType)
			note = fmt.Sprintf("Read image file [%s]", resized.MIMEType)
			if dimensionNote := FormatDimensionNote(*resized); dimensionNote != "" {
				note += "\n" + dimensionNote
			}
		}
		return FileToolResult{
			Text: note,
			Content: []llm.ContentPart{
				llm.Text(note),
				imagePart,
			},
		}, nil
	}
	text, details, err := formatReadToolTextContent(string(content), input.Offset, input.Limit)
	if err != nil {
		return FileToolResult{}, err
	}
	return FileToolResult{Text: text, Content: []llm.ContentPart{llm.Text(text)}, Details: details}, nil
}

func normalizeFileToolOperations(operations ...FileToolOperations) FileToolOperations {
	ops := FileToolOperations{}
	if len(operations) > 0 {
		ops = operations[0]
	}
	if ops.Access == nil {
		ops.Access = func(path string) error {
			_, err := os.Stat(path)
			return err
		}
	}
	if ops.ReadFile == nil {
		ops.ReadFile = os.ReadFile
	}
	if ops.WriteFile == nil {
		ops.WriteFile = func(path string, content []byte) error {
			return os.WriteFile(path, content, 0o644)
		}
	}
	if ops.MkdirAll == nil {
		ops.MkdirAll = func(path string) error {
			return os.MkdirAll(path, 0o755)
		}
	}
	if ops.ResizeImage == nil {
		ops.ResizeImage = func(part llm.ContentPart, options ImageResizeOptions) *ResizedImage {
			return ResizeImage(part, options)
		}
	}
	return ops
}

func applySimpleEdits(content string, edits []Edit) (string, error) {
	next := content
	for _, edit := range edits {
		if edit.OldText == "" {
			return "", fmt.Errorf("oldText must not be empty")
		}
		if count := strings.Count(next, edit.OldText); count != 1 {
			return "", fmt.Errorf("oldText must match exactly once, got %d matches", count)
		}
		next = strings.Replace(next, edit.OldText, edit.NewText, 1)
	}
	return next, nil
}

func formatReadToolTextContent(content string, offset, limit int) (string, *FileToolDetails, error) {
	lines := strings.Split(content, "\n")
	totalLines := len(lines)
	start := 0
	if offset > 0 {
		start = offset - 1
	}
	if start >= totalLines {
		return "", nil, fmt.Errorf("Offset %d is beyond end of file (%d lines total)", offset, totalLines)
	}
	lineLimit := defaultReadToolLineLimit
	explicitLimit := false
	if limit > 0 {
		lineLimit = limit
		explicitLimit = true
	}

	capacity := lineLimit
	if remaining := totalLines - start; remaining < capacity {
		capacity = remaining
	}
	outputLines := make([]string, 0, capacity)
	outputBytes := 0
	truncatedBy := ""
	nextOffset := 0
	for index := start; index < totalLines; index++ {
		if len(outputLines) >= lineLimit {
			truncatedBy = "lines"
			if explicitLimit {
				truncatedBy = "limit"
			}
			nextOffset = index + 1
			break
		}
		line := lines[index]
		addedBytes := len(line)
		if len(outputLines) > 0 {
			addedBytes++
		}
		if outputBytes+addedBytes > defaultReadToolByteLimit && len(outputLines) > 0 {
			truncatedBy = "bytes"
			nextOffset = index + 1
			break
		}
		outputLines = append(outputLines, line)
		outputBytes += addedBytes
	}

	output := strings.Join(outputLines, "\n")
	if truncatedBy == "" {
		return output, nil, nil
	}

	displayStart := start + 1
	displayEnd := start + len(outputLines)
	remaining := totalLines - displayEnd
	if explicitLimit {
		output += fmt.Sprintf("\n[%d more lines in file. Use offset=%d to continue.]", remaining, nextOffset)
	} else if truncatedBy == "bytes" {
		output += fmt.Sprintf("\n[Showing lines %d-%d of %d (byte limit). Use offset=%d to continue.]", displayStart, displayEnd, totalLines, nextOffset)
	} else {
		output += fmt.Sprintf("\n[Showing lines %d-%d of %d. Use offset=%d to continue.]", displayStart, displayEnd, totalLines, nextOffset)
	}
	return output, &FileToolDetails{Truncation: &ReadToolTruncation{
		Truncated:   true,
		TruncatedBy: truncatedBy,
		TotalLines:  totalLines,
		OutputLines: len(outputLines),
	}}, nil
}

func detectSupportedImageMIMEType(content []byte) string {
	if len(content) >= 8 && bytes.Equal(content[:8], []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}) {
		return "image/png"
	}
	if len(content) >= 3 && content[0] == 0xff && content[1] == 0xd8 && content[2] == 0xff {
		return "image/jpeg"
	}
	if len(content) >= 6 && (string(content[:6]) == "GIF87a" || string(content[:6]) == "GIF89a") {
		return "image/gif"
	}
	if len(content) >= 12 && string(content[:4]) == "RIFF" && string(content[8:12]) == "WEBP" {
		return "image/webp"
	}
	return ""
}

func imageMIMETypeForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return ""
	}
}
