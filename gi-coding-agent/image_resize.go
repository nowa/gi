package gicodingagent

import (
	"github.com/nowa/gi/gi-coding-agent/internal/imageresize"
	llm "github.com/nowa/gi/gi-llm-provider"
)

type ConvertedImage = imageresize.ConvertedImage
type ImageResizeOptions = imageresize.ImageResizeOptions
type ResizedImage = imageresize.ResizedImage

func ConvertToPNG(base64Data, mimeType string) *ConvertedImage {
	return imageresize.ConvertToPNG(base64Data, mimeType)
}

func ResizeImage(part llm.ContentPart, options ...ImageResizeOptions) *ResizedImage {
	return imageresize.ResizeImage(part, options...)
}

func FormatDimensionNote(result ResizedImage) string {
	return imageresize.FormatDimensionNote(result)
}

func imageEXIFOrientation(bytes []byte) int {
	return imageresize.ImageEXIFOrientation(bytes)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
