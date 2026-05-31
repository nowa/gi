package imageresize

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
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
	_ "golang.org/x/image/webp"
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
	img = applyImageEXIFOrientation(img, decoded)
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
	img = applyImageEXIFOrientation(img, decoded)
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

func ImageEXIFOrientation(bytes []byte) int {
	return imageEXIFOrientation(bytes)
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

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func baseImageMIMEType(mimeType string) string {
	if index := strings.Index(mimeType, ";"); index >= 0 {
		mimeType = mimeType[:index]
	}
	return strings.ToLower(strings.TrimSpace(mimeType))
}

func applyImageEXIFOrientation(img image.Image, originalBytes []byte) image.Image {
	orientation := imageEXIFOrientation(originalBytes)
	if orientation <= 1 || orientation > 8 {
		return img
	}
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return img
	}
	dstWidth, dstHeight := width, height
	if orientation >= 5 {
		dstWidth, dstHeight = height, width
	}
	dst := image.NewNRGBA(image.Rect(0, 0, dstWidth, dstHeight))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			dstX, dstY := imageEXIFDestination(x, y, width, height, orientation)
			dst.Set(dstX, dstY, img.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return dst
}

func imageEXIFDestination(x, y, width, height, orientation int) (int, int) {
	switch orientation {
	case 2:
		return width - 1 - x, y
	case 3:
		return width - 1 - x, height - 1 - y
	case 4:
		return x, height - 1 - y
	case 5:
		return y, x
	case 6:
		return height - 1 - y, x
	case 7:
		return height - 1 - y, width - 1 - x
	case 8:
		return y, width - 1 - x
	default:
		return x, y
	}
}

func imageEXIFOrientation(bytes []byte) int {
	tiffOffset := -1
	if len(bytes) >= 2 && bytes[0] == 0xff && bytes[1] == 0xd8 {
		tiffOffset = findJPEGEXIFTIFFOffset(bytes)
	} else if len(bytes) >= 12 &&
		bytes[0] == 'R' && bytes[1] == 'I' && bytes[2] == 'F' && bytes[3] == 'F' &&
		bytes[8] == 'W' && bytes[9] == 'E' && bytes[10] == 'B' && bytes[11] == 'P' {
		tiffOffset = findWebPEXIFTIFFOffset(bytes)
	}
	if tiffOffset < 0 {
		return 1
	}
	return readEXIFOrientationFromTIFF(bytes, tiffOffset)
}

func findJPEGEXIFTIFFOffset(bytes []byte) int {
	offset := 2
	for offset < len(bytes)-1 {
		if bytes[offset] != 0xff {
			return -1
		}
		marker := bytes[offset+1]
		if marker == 0xff {
			offset++
			continue
		}
		if marker == 0xe1 {
			if offset+4 >= len(bytes) {
				return -1
			}
			segmentStart := offset + 4
			if segmentStart+6 > len(bytes) || !hasEXIFHeader(bytes, segmentStart) {
				return -1
			}
			return segmentStart + 6
		}
		if marker == 0xda || marker == 0xd9 {
			return -1
		}
		if offset+4 > len(bytes) {
			return -1
		}
		length := int(bytes[offset+2])<<8 | int(bytes[offset+3])
		if length < 2 {
			return -1
		}
		offset += 2 + length
	}
	return -1
}

func findWebPEXIFTIFFOffset(bytes []byte) int {
	offset := 12
	for offset+8 <= len(bytes) {
		chunkID := string(bytes[offset : offset+4])
		chunkSize := int(bytes[offset+4]) | int(bytes[offset+5])<<8 | int(bytes[offset+6])<<16 | int(bytes[offset+7])<<24
		dataStart := offset + 8
		if chunkSize < 0 || dataStart+chunkSize > len(bytes) {
			return -1
		}
		if chunkID == "EXIF" {
			if chunkSize >= 6 && hasEXIFHeader(bytes, dataStart) {
				return dataStart + 6
			}
			return dataStart
		}
		offset = dataStart + chunkSize + chunkSize%2
	}
	return -1
}

func hasEXIFHeader(bytes []byte, offset int) bool {
	return offset+6 <= len(bytes) &&
		bytes[offset] == 'E' &&
		bytes[offset+1] == 'x' &&
		bytes[offset+2] == 'i' &&
		bytes[offset+3] == 'f' &&
		bytes[offset+4] == 0x00 &&
		bytes[offset+5] == 0x00
}

func readEXIFOrientationFromTIFF(bytes []byte, tiffStart int) int {
	if tiffStart < 0 || tiffStart+8 > len(bytes) {
		return 1
	}
	byteOrder := int(bytes[tiffStart])<<8 | int(bytes[tiffStart+1])
	littleEndian := byteOrder == 0x4949
	read16 := func(pos int) int {
		if pos+2 > len(bytes) {
			return 0
		}
		if littleEndian {
			return int(bytes[pos]) | int(bytes[pos+1])<<8
		}
		return int(bytes[pos])<<8 | int(bytes[pos+1])
	}
	read32 := func(pos int) int {
		if pos+4 > len(bytes) {
			return 0
		}
		if littleEndian {
			return int(bytes[pos]) | int(bytes[pos+1])<<8 | int(bytes[pos+2])<<16 | int(bytes[pos+3])<<24
		}
		return int(bytes[pos])<<24 | int(bytes[pos+1])<<16 | int(bytes[pos+2])<<8 | int(bytes[pos+3])
	}
	ifdStart := tiffStart + read32(tiffStart+4)
	if ifdStart+2 > len(bytes) {
		return 1
	}
	entryCount := read16(ifdStart)
	for i := 0; i < entryCount; i++ {
		entryPos := ifdStart + 2 + i*12
		if entryPos+12 > len(bytes) {
			return 1
		}
		if read16(entryPos) == 0x0112 {
			value := read16(entryPos + 8)
			if value >= 1 && value <= 8 {
				return value
			}
			return 1
		}
	}
	return 1
}
