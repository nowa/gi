package gicodingagent

import (
	"encoding/base64"

	"github.com/nowa/gi/gi-coding-agent/internal/imageresize"
	llm "github.com/nowa/gi/gi-llm-provider"
)

type ConvertedImage = imageresize.ConvertedImage
type ImageResizeOptions = imageresize.ImageResizeOptions
type ResizedImage = imageresize.ResizedImage
type ProcessImageOptions = imageresize.ProcessImageOptions
type ProcessImageResult = imageresize.ProcessImageResult

func ConvertImageBytesToPNG(data []byte) ([]byte, bool) {
	return imageresize.ConvertImageBytesToPNG(data)
}

func ConvertToPNG(base64Data, mimeType string) *ConvertedImage {
	return imageresize.ConvertToPNG(base64Data, mimeType)
}

func ResizeImage(part llm.ContentPart, options ...ImageResizeOptions) *ResizedImage {
	return imageresize.ResizeImage(part, options...)
}

func ResizeImageBytes(data []byte, mimeType string, options ...ImageResizeOptions) *ResizedImage {
	return imageresize.ResizeImageBytes(data, mimeType, options...)
}

func ProcessImage(data []byte, mimeType string, options ...ProcessImageOptions) ProcessImageResult {
	return imageresize.ProcessImage(data, mimeType, options...)
}

func FormatDimensionNote(result ResizedImage) string {
	return imageresize.FormatDimensionNote(result)
}

func processImageWithResize(
	data []byte,
	mimeType string,
	autoResizeImages bool,
	resizeImage func(llm.ContentPart, ImageResizeOptions) *ResizedImage,
) ProcessImageResult {
	options := ProcessImageOptions{AutoResizeImages: &autoResizeImages}
	if resizeImage != nil {
		options.ResizeImage = func(data []byte, mimeType string, resizeOptions ImageResizeOptions) *ResizedImage {
			return resizeImage(
				llm.Image(base64.StdEncoding.EncodeToString(data), mimeType),
				resizeOptions,
			)
		}
	}
	return ProcessImage(data, mimeType, options)
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
