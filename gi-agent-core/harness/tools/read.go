package tools

import (
	"context"
	"fmt"
	"strings"

	core "github.com/nowa/gi/gi-agent-core"
	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	llm "github.com/nowa/gi/gi-llm-provider"
)

type ReadToolDetails struct {
	Truncation *agentharness.TruncationResult `json:"truncation,omitempty"`
}

type ReadImageProcessorResult struct {
	OK       bool
	Data     string
	MIMEType string
	Hints    []string
	Message  string
}

type ReadImageProcessor func(ctx context.Context, content []byte, mimeType string, autoResizeImages bool) (ReadImageProcessorResult, error)

type ReadToolOptions struct {
	AutoResizeImages *bool
	ImageProcessor   ReadImageProcessor
}

func CreateReadTool(options ...ReadToolOptions) agentharness.AgentHarnessTool {
	opts := ReadToolOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	autoResizeImages := true
	if opts.AutoResizeImages != nil {
		autoResizeImages = *opts.AutoResizeImages
	}
	return agentharness.AgentHarnessTool{
		Name:        "read",
		Label:       "read",
		Description: fmt.Sprintf("Read the contents of a file. Supports text files and images (jpg, png, gif, webp, bmp). Images are sent as attachments. For text files, output is truncated to %d lines or %dKB (whichever is hit first). Use offset/limit for large files. When you need the full file, continue with offset until complete.", agentharness.DefaultMaxLines, agentharness.DefaultMaxBytes/1024),
		Parameters: llm.Object(map[string]llm.Schema{
			"path": {
				Type:        "string",
				Description: "Path to the file to read (relative or absolute)",
			},
			"offset": {
				Type:        "number",
				Description: "Line number to start reading from (1-indexed)",
			},
			"limit": {
				Type:        "number",
				Description: "Maximum number of lines to read",
			},
		}, "path"),
		Execute: func(ctx context.Context, _ string, params map[string]any, _ core.AgentToolUpdateCallback, contextValue any) (core.AgentToolResult, error) {
			provider, _, err := executionContext(contextValue)
			if err != nil {
				return core.AgentToolResult{}, err
			}
			path, err := requiredString(params, "path")
			if err != nil {
				return core.AgentToolResult{}, err
			}
			offset, err := optionalNonNegativeInt(params, "offset")
			if err != nil {
				return core.AgentToolResult{}, err
			}
			limit, err := optionalNonNegativeInt(params, "limit")
			if err != nil {
				return core.AgentToolResult{}, err
			}
			env := provider.ExecutionEnvironment()
			absolutePath, err := ResolveReadToolPath(ctx, env, path)
			if err != nil {
				return core.AgentToolResult{}, err
			}
			content, err := env.ReadBinaryFile(ctx, absolutePath)
			if err != nil {
				return core.AgentToolResult{}, err
			}
			if mimeType := DetectSupportedImageMIMEType(content); mimeType != "" {
				return readImage(ctx, content, mimeType, autoResizeImages, opts.ImageProcessor)
			}
			text, details, err := formatReadText(string(content), path, offset, limit)
			if err != nil {
				return core.AgentToolResult{}, err
			}
			result := core.AgentToolResult{Content: []llm.ContentPart{llm.Text(text)}}
			if details != nil {
				result.Details = details
			}
			return result, nil
		},
	}
}

func readImage(ctx context.Context, content []byte, mimeType string, autoResizeImages bool, processor ReadImageProcessor) (core.AgentToolResult, error) {
	if processor != nil {
		processed, err := processor(ctx, append([]byte(nil), content...), mimeType, autoResizeImages)
		if err != nil {
			return core.AgentToolResult{}, err
		}
		if !processed.OK {
			return core.AgentToolResult{
				Content: []llm.ContentPart{llm.Text(fmt.Sprintf("Read image file [%s]\n%s", mimeType, processed.Message))},
			}, nil
		}
		note := fmt.Sprintf("Read image file [%s]", processed.MIMEType)
		if len(processed.Hints) > 0 {
			note += "\n" + strings.Join(processed.Hints, "\n")
		}
		return core.AgentToolResult{
			Content: []llm.ContentPart{
				llm.Text(note),
				llm.Image(processed.Data, processed.MIMEType),
			},
		}, nil
	}
	if mimeType == "image/bmp" {
		return core.AgentToolResult{
			Content: []llm.ContentPart{llm.Text("Read image file [image/bmp]\n[Image omitted: configure an image processor to convert BMP images.]")},
		}, nil
	}
	return core.AgentToolResult{
		Content: []llm.ContentPart{
			llm.Text(fmt.Sprintf("Read image file [%s]", mimeType)),
			llm.Image(EncodeBase64(content), mimeType),
		},
	}, nil
}

func formatReadText(content, path string, offset, limit int) (string, *ReadToolDetails, error) {
	allLines := strings.Split(content, "\n")
	startLine := 0
	if offset > 0 {
		startLine = offset - 1
	}
	if startLine >= len(allLines) {
		return "", nil, fmt.Errorf("Offset %d is beyond end of file (%d lines total)", offset, len(allLines))
	}
	startLineDisplay := startLine + 1
	selectedContent := strings.Join(allLines[startLine:], "\n")
	userLimitedLines := -1
	if limit > 0 {
		endLine := min(startLine+limit, len(allLines))
		selectedContent = strings.Join(allLines[startLine:endLine], "\n")
		userLimitedLines = endLine - startLine
	}

	truncation := agentharness.TruncateHead(selectedContent, agentharness.TruncationOptions{})
	switch {
	case truncation.FirstLineExceedsLimit:
		firstLineSize := formatSize(len([]byte(allLines[startLine])))
		text := fmt.Sprintf("[Line %d is %s, exceeds %s limit. Use bash: sed -n '%dp' %s | head -c %d]",
			startLineDisplay,
			firstLineSize,
			formatSize(agentharness.DefaultMaxBytes),
			startLineDisplay,
			path,
			agentharness.DefaultMaxBytes,
		)
		return text, &ReadToolDetails{Truncation: &truncation}, nil
	case truncation.Truncated:
		endLineDisplay := startLineDisplay + truncation.OutputLines - 1
		nextOffset := endLineDisplay + 1
		text := truncation.Content
		if truncation.TruncatedBy == agentharness.TruncatedByLines {
			text += fmt.Sprintf("\n\n[Showing lines %d-%d of %d. Use offset=%d to continue.]",
				startLineDisplay, endLineDisplay, len(allLines), nextOffset)
		} else {
			text += fmt.Sprintf("\n\n[Showing lines %d-%d of %d (%s limit). Use offset=%d to continue.]",
				startLineDisplay, endLineDisplay, len(allLines), formatSize(agentharness.DefaultMaxBytes), nextOffset)
		}
		return text, &ReadToolDetails{Truncation: &truncation}, nil
	case userLimitedLines >= 0 && startLine+userLimitedLines < len(allLines):
		remaining := len(allLines) - (startLine + userLimitedLines)
		nextOffset := startLine + userLimitedLines + 1
		return fmt.Sprintf("%s\n\n[%d more lines in file. Use offset=%d to continue.]", truncation.Content, remaining, nextOffset), nil, nil
	default:
		return truncation.Content, nil, nil
	}
}

func formatSize(size int) string {
	switch {
	case size < 1024:
		return fmt.Sprintf("%dB", size)
	case size < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(size)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(size)/(1024*1024))
	}
}
