package gicodingagent

import (
	"encoding/base64"
	"os"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type ProcessFileArgumentsOptions struct {
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

	var result ProcessedFileArguments
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return ProcessedFileArguments{}, err
		}
		if mimeType := imageMIMETypeForPath(path); mimeType != "" {
			imagePart := llm.Image(base64.StdEncoding.EncodeToString(content), mimeType)
			if autoResizeImages {
				resized := resizeImage(imagePart, ImageResizeOptions{})
				if resized == nil {
					result.Text += `<file name="` + path + `">[Image omitted: could not be resized below the inline image size limit.]</file>` + "\n"
					continue
				}
				imagePart = llm.Image(resized.Data, resized.MIMEType)
				if dimensionNote := FormatDimensionNote(*resized); dimensionNote != "" {
					result.Text += `<file name="` + path + `">` + dimensionNote + `</file>` + "\n"
				} else {
					result.Text += `<file name="` + path + `"></file>` + "\n"
				}
			} else {
				result.Text += `<file name="` + path + `"></file>` + "\n"
			}
			result.Images = append(result.Images, imagePart)
			continue
		}
		if result.Text != "" && !strings.HasSuffix(result.Text, "\n") {
			result.Text += "\n"
		}
		result.Text += string(content)
	}
	return result, nil
}
