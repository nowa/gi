package gicodingagent

import (
	"encoding/base64"
	"os"
	"path/filepath"

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
			imagePart := llm.Image(base64.StdEncoding.EncodeToString(content), mimeType)
			if autoResizeImages {
				resized := resizeImage(imagePart, ImageResizeOptions{})
				if resized == nil {
					result.Text += `<file name="` + absolutePath + `">[Image omitted: could not be resized below the inline image size limit.]</file>` + "\n"
					continue
				}
				imagePart = llm.Image(resized.Data, resized.MIMEType)
				if dimensionNote := FormatDimensionNote(*resized); dimensionNote != "" {
					result.Text += `<file name="` + absolutePath + `">` + dimensionNote + `</file>` + "\n"
				} else {
					result.Text += `<file name="` + absolutePath + `"></file>` + "\n"
				}
			} else {
				result.Text += `<file name="` + absolutePath + `"></file>` + "\n"
			}
			result.Images = append(result.Images, imagePart)
			continue
		}
		result.Text += `<file name="` + absolutePath + `">` + "\n" + string(content) + "\n</file>\n"
	}
	return result, nil
}
