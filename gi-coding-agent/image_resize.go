package gicodingagent

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"math"

	llm "github.com/nowa/gi/gi-llm-provider"
)

const defaultImageMaxBytes = int(4.5 * 1024 * 1024)

type ConvertedImage struct {
	Data     string
	MIMEType string
}

type ImageResizeOptions struct {
	MaxWidth    int
	MaxHeight   int
	MaxBytes    int
	JPEGQuality int
}

type ResizedImage struct {
	Data           string
	MIMEType       string
	OriginalWidth  int
	OriginalHeight int
	Width          int
	Height         int
	WasResized     bool
}

func ConvertToPNG(base64Data, mimeType string) *ConvertedImage {
	if baseImageMIMEType(mimeType) == "image/png" {
		return &ConvertedImage{Data: base64Data, MIMEType: "image/png"}
	}
	decoded, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(decoded))
	if err != nil {
		return nil
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		return nil
	}
	return &ConvertedImage{Data: base64.StdEncoding.EncodeToString(buffer.Bytes()), MIMEType: "image/png"}
}

func ResizeImage(part llm.ContentPart, options ...ImageResizeOptions) *ResizedImage {
	opts := normalizeImageResizeOptions(options...)
	decoded, err := base64.StdEncoding.DecodeString(part.Data)
	if err != nil {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(decoded))
	if err != nil {
		return nil
	}
	bounds := img.Bounds()
	originalWidth := bounds.Dx()
	originalHeight := bounds.Dy()
	if originalWidth <= 0 || originalHeight <= 0 {
		return nil
	}
	if originalWidth <= opts.MaxWidth && originalHeight <= opts.MaxHeight && len(part.Data) < opts.MaxBytes {
		return &ResizedImage{
			Data:           part.Data,
			MIMEType:       firstNonEmptyString(part.MIMEType, "image/png"),
			OriginalWidth:  originalWidth,
			OriginalHeight: originalHeight,
			Width:          originalWidth,
			Height:         originalHeight,
			WasResized:     false,
		}
	}

	targetWidth, targetHeight := fitImageDimensions(originalWidth, originalHeight, opts.MaxWidth, opts.MaxHeight)
	if len(part.Data) >= opts.MaxBytes && opts.MaxBytes > 0 {
		sizeScale := math.Sqrt(float64(opts.MaxBytes) / float64(len(part.Data)))
		if sizeScale > 0 && sizeScale < 1 {
			targetWidth = maxInt(1, int(math.Floor(float64(targetWidth)*sizeScale)))
			targetHeight = maxInt(1, int(math.Floor(float64(targetHeight)*sizeScale)))
		}
	}

	for {
		resized := resizeNearestNeighbor(img, targetWidth, targetHeight)
		encoded, ok := encodePNGBase64(resized)
		if ok && len(encoded) < opts.MaxBytes {
			return &ResizedImage{
				Data:           encoded,
				MIMEType:       "image/png",
				OriginalWidth:  originalWidth,
				OriginalHeight: originalHeight,
				Width:          targetWidth,
				Height:         targetHeight,
				WasResized:     true,
			}
		}
		if targetWidth == 1 && targetHeight == 1 {
			break
		}
		nextWidth := targetWidth
		nextHeight := targetHeight
		if nextWidth > 1 {
			nextWidth = maxInt(1, int(math.Floor(float64(nextWidth)*0.75)))
		}
		if nextHeight > 1 {
			nextHeight = maxInt(1, int(math.Floor(float64(nextHeight)*0.75)))
		}
		if nextWidth == targetWidth && nextHeight == targetHeight {
			break
		}
		targetWidth = nextWidth
		targetHeight = nextHeight
	}
	return nil
}

func FormatDimensionNote(result ResizedImage) string {
	if !result.WasResized {
		return ""
	}
	scale := float64(result.OriginalWidth) / float64(result.Width)
	return fmt.Sprintf("[Image: original %dx%d, displayed at %dx%d. Multiply coordinates by %.2f to map to original image.]", result.OriginalWidth, result.OriginalHeight, result.Width, result.Height, scale)
}

func normalizeImageResizeOptions(options ...ImageResizeOptions) ImageResizeOptions {
	opts := ImageResizeOptions{MaxWidth: 2000, MaxHeight: 2000, MaxBytes: defaultImageMaxBytes, JPEGQuality: 80}
	if len(options) > 0 {
		if options[0].MaxWidth > 0 {
			opts.MaxWidth = options[0].MaxWidth
		}
		if options[0].MaxHeight > 0 {
			opts.MaxHeight = options[0].MaxHeight
		}
		if options[0].MaxBytes > 0 {
			opts.MaxBytes = options[0].MaxBytes
		}
		if options[0].JPEGQuality > 0 {
			opts.JPEGQuality = options[0].JPEGQuality
		}
	}
	return opts
}

func fitImageDimensions(width, height, maxWidth, maxHeight int) (int, int) {
	targetWidth := width
	targetHeight := height
	if targetWidth > maxWidth {
		targetHeight = int(math.Round(float64(targetHeight) * float64(maxWidth) / float64(targetWidth)))
		targetWidth = maxWidth
	}
	if targetHeight > maxHeight {
		targetWidth = int(math.Round(float64(targetWidth) * float64(maxHeight) / float64(targetHeight)))
		targetHeight = maxHeight
	}
	return maxInt(1, targetWidth), maxInt(1, targetHeight)
}

func resizeNearestNeighbor(src image.Image, width, height int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	bounds := src.Bounds()
	sourceWidth := bounds.Dx()
	sourceHeight := bounds.Dy()
	for y := 0; y < height; y++ {
		sourceY := bounds.Min.Y + y*sourceHeight/height
		for x := 0; x < width; x++ {
			sourceX := bounds.Min.X + x*sourceWidth/width
			dst.Set(x, y, color.NRGBAModel.Convert(src.At(sourceX, sourceY)))
		}
	}
	return dst
}

func encodePNGBase64(img image.Image) (string, bool) {
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		return "", false
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes()), true
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
