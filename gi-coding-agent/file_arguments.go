package gicodingagent

import (
	"os"
	"path/filepath"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type ProcessFileArgumentsOptions struct {
	CWD              string
	AutoResizeImages *bool
	ResizeImage      func(part llm.ContentPart, options ImageResizeOptions) *ResizedImage
}

type ProcessedFileArguments struct {
	Text   string
	Images []llm.ContentPart
}

func ProcessFileArguments(paths []string, options ...ProcessFileArgumentsOptions) (ProcessedFileArguments, error) {
	opts := ProcessFileArgumentsOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	autoResizeImages := true
	if opts.AutoResizeImages != nil {
		autoResizeImages = *opts.AutoResizeImages
	}
	resizeImage := opts.ResizeImage
	if resizeImage == nil {
		resizeImage = func(part llm.ContentPart, options ImageResizeOptions) *ResizedImage {
			return ResizeImage(part, options)
		}
	}
	cwd := opts.CWD
	if cwd == "" {
		if current, err := os.Getwd(); err == nil && current != "" {
			cwd = current
		} else {
			cwd = "."
		}
	}

	var result ProcessedFileArguments
	for _, path := range paths {
		absolutePath, err := ResolveReadPath(path, cwd)
		if err != nil {
			return ProcessedFileArguments{}, err
		}
		if resolved, err := filepath.Abs(absolutePath); err == nil {
			absolutePath = filepath.Clean(resolved)
		}
		content, err := os.ReadFile(absolutePath)
		if err != nil {
			return ProcessedFileArguments{}, err
		}
		if len(content) == 0 {
			continue
		}
		if mimeType := detectSupportedImageMIMEType(content); mimeType != "" {
			processed := processImageWithResize(content, mimeType, autoResizeImages, resizeImage)
			if !processed.OK {
				result.Text += `<file name="` + absolutePath + `">` + processed.Message + `</file>` + "\n"
				continue
			}
			result.Text += `<file name="` + absolutePath + `">` + strings.Join(processed.Hints, "\n") + `</file>` + "\n"
			result.Images = append(result.Images, llm.Image(processed.Data, processed.MIMEType))
			continue
		}
		result.Text += `<file name="` + absolutePath + `">` + "\n" + string(content) + "\n</file>\n"
	}
	return result, nil
}
