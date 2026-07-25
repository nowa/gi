package gicodingagent

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestConvertToPNGPiMatrix(t *testing.T) {
	tinyPNG := testPNGBase64(t, 2, 2)
	result := ConvertToPNG(tinyPNG, "image/png")
	if result == nil || result.Data != tinyPNG || result.MIMEType != "image/png" {
		t.Fatalf("PNG convert result = %#v", result)
	}

	tinyJPEG := testJPEGBase64(t, 2, 2)
	result = ConvertToPNG(tinyJPEG, "image/jpeg")
	if result == nil || result.MIMEType != "image/png" {
		t.Fatalf("JPEG convert result = %#v", result)
	}
	buffer := mustDecodeBase64(t, result.Data)
	if len(buffer) < 4 || buffer[0] != 0x89 || buffer[1] != 0x50 || buffer[2] != 0x4e || buffer[3] != 0x47 {
		t.Fatalf("converted PNG magic = %#v", buffer[:minInt(len(buffer), 4)])
	}

	orientedJPEG := testJPEGBase64WithEXIFOrientation(t, 2, 1, 6)
	result = ConvertToPNG(orientedJPEG, "image/jpeg")
	if result == nil || result.MIMEType != "image/png" {
		t.Fatalf("EXIF JPEG convert result = %#v", result)
	}
	converted, _, err := image.Decode(bytes.NewReader(mustDecodeBase64(t, result.Data)))
	if err != nil {
		t.Fatalf("decode converted EXIF PNG: %v", err)
	}
	if got := converted.Bounds().Size(); got.X != 1 || got.Y != 2 {
		t.Fatalf("EXIF orientation 6 converted size = %dx%d, want 1x2", got.X, got.Y)
	}
}

func TestProcessImageConvertsBMPBeforeOptionalResize(t *testing.T) {
	bmp := createTinyBMP1x1Red24BPP()
	original := append([]byte(nil), bmp...)

	autoResize := false
	withoutResize := ProcessImage(bmp, "image/bmp; charset=binary", ProcessImageOptions{
		AutoResizeImages: &autoResize,
	})
	if !withoutResize.OK ||
		withoutResize.MIMEType != "image/png" ||
		!reflect.DeepEqual(withoutResize.Hints, []string{"[Image converted from image/bmp to image/png.]"}) {
		t.Fatalf("process without resize = %#v", withoutResize)
	}
	assertPNGBase64(t, withoutResize.Data)
	if !bytes.Equal(bmp, original) {
		t.Fatal("ProcessImage mutated caller bytes")
	}

	var resizeInputMIME string
	withResize := ProcessImage(bmp, "image/bmp", ProcessImageOptions{
		ResizeImage: func(data []byte, mimeType string, _ ImageResizeOptions) *ResizedImage {
			resizeInputMIME = mimeType
			if len(data) < 4 || !bytes.Equal(data[:4], []byte{0x89, 0x50, 0x4e, 0x47}) {
				t.Fatalf("resize input is not PNG: %#v", data)
			}
			return &ResizedImage{
				Data:           base64.StdEncoding.EncodeToString(data),
				MIMEType:       "image/png",
				OriginalWidth:  1,
				OriginalHeight: 1,
				Width:          1,
				Height:         1,
			}
		},
	})
	if !withResize.OK ||
		resizeInputMIME != "image/png" ||
		!reflect.DeepEqual(withResize.Hints, []string{"[Image converted from image/bmp to image/png.]"}) {
		t.Fatalf("process with resize = %#v, input MIME = %q", withResize, resizeInputMIME)
	}
	assertPNGBase64(t, withResize.Data)

	invalid := ProcessImage([]byte("not an image"), "application/octet-stream")
	if invalid.OK || !strings.Contains(invalid.Message, "could not be converted") {
		t.Fatalf("invalid process result = %#v", invalid)
	}
}

func TestImageProcessingCallersConvertBMPWhenAutoResizeDisabled(t *testing.T) {
	dir := t.TempDir()
	imagePath := filepath.Join(dir, "image.bmp")
	if err := os.WriteFile(imagePath, createTinyBMP1x1Red24BPP(), 0o600); err != nil {
		t.Fatal(err)
	}
	autoResize := false

	processed, err := ProcessFileArguments([]string{imagePath}, ProcessFileArgumentsOptions{
		AutoResizeImages: &autoResize,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(processed.Images) != 1 ||
		processed.Images[0].MIMEType != "image/png" ||
		!strings.Contains(processed.Text, "[Image converted from image/bmp to image/png.]") {
		t.Fatalf("file arguments = %#v", processed)
	}
	assertPNGBase64(t, processed.Images[0].Data)

	tool := NewReadToolWithOptions(dir, ReadToolOptions{AutoResizeImages: &autoResize})
	result, err := tool.Execute("read-bmp", ReadToolInput{Path: imagePath})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 2 ||
		result.Content[1].MIMEType != "image/png" ||
		!strings.Contains(result.Content[0].Text, "[Image converted from image/bmp to image/png.]") {
		t.Fatalf("read BMP result = %#v", result)
	}
	assertPNGBase64(t, result.Content[1].Data)
}

func TestResizeImagePiMatrix(t *testing.T) {
	tinyPNG := testPNGBase64(t, 2, 2)
	result := ResizeImage(llm.Image(tinyPNG, "image/png"), ImageResizeOptions{MaxWidth: 100, MaxHeight: 100, MaxBytes: 1024 * 1024})
	if result == nil {
		t.Fatal("ResizeImage tiny PNG returned nil")
	}
	if result.WasResized || result.Data != tinyPNG || result.OriginalWidth != 2 || result.OriginalHeight != 2 || result.Width != 2 || result.Height != 2 {
		t.Fatalf("tiny resize result = %#v", result)
	}

	mediumPNG := testPNGBase64(t, 100, 100)
	result = ResizeImage(llm.Image(mediumPNG, "image/png"), ImageResizeOptions{MaxWidth: 50, MaxHeight: 50, MaxBytes: 1024 * 1024})
	if result == nil {
		t.Fatal("ResizeImage medium PNG returned nil")
	}
	if !result.WasResized || result.OriginalWidth != 100 || result.OriginalHeight != 100 || result.Width > 50 || result.Height > 50 {
		t.Fatalf("dimension resize result = %#v", result)
	}

	largePNG := testNoisyPNGBase64(t, 200, 200)
	originalBuffer := mustDecodeBase64(t, largePNG)
	result = ResizeImage(llm.Image(largePNG, "image/png"), ImageResizeOptions{MaxWidth: 2000, MaxHeight: 2000, MaxBytes: int(float64(len(largePNG)) * 0.9)})
	if result == nil {
		t.Fatal("ResizeImage byte-limited PNG returned nil")
	}
	resultBuffer := mustDecodeBase64(t, result.Data)
	if len(resultBuffer) >= len(originalBuffer) || len(result.Data) >= len(largePNG) {
		t.Fatalf("byte resize sizes: result=%d/%d original=%d/%d", len(resultBuffer), len(result.Data), len(originalBuffer), len(largePNG))
	}

	if result := ResizeImage(llm.Image(largePNG, "image/png"), ImageResizeOptions{MaxWidth: 2000, MaxHeight: 2000, MaxBytes: 1}); result != nil {
		t.Fatalf("ResizeImage impossible maxBytes = %#v, want nil", result)
	}

	tinyJPEG := testJPEGBase64(t, 2, 2)
	result = ResizeImage(llm.Image(tinyJPEG, "image/jpeg"), ImageResizeOptions{MaxWidth: 100, MaxHeight: 100, MaxBytes: 1024 * 1024})
	if result == nil || result.WasResized || result.OriginalWidth != 2 || result.OriginalHeight != 2 {
		t.Fatalf("JPEG resize result = %#v", result)
	}
}

func assertPNGBase64(t *testing.T, data string) {
	t.Helper()
	decoded := mustDecodeBase64(t, data)
	if len(decoded) < 4 || !bytes.Equal(decoded[:4], []byte{0x89, 0x50, 0x4e, 0x47}) {
		t.Fatalf("PNG magic = %#v", decoded[:minInt(len(decoded), 4)])
	}
}

func TestFormatDimensionNotePiMatrix(t *testing.T) {
	note := FormatDimensionNote(ResizedImage{
		MIMEType:       "image/png",
		OriginalWidth:  100,
		OriginalHeight: 100,
		Width:          100,
		Height:         100,
		WasResized:     false,
	})
	if note != "" {
		t.Fatalf("non-resized note = %q, want empty", note)
	}

	note = FormatDimensionNote(ResizedImage{
		MIMEType:       "image/png",
		OriginalWidth:  2000,
		OriginalHeight: 1000,
		Width:          1000,
		Height:         500,
		WasResized:     true,
	})
	for _, want := range []string{"original 2000x1000", "displayed at 1000x500", "2.00"} {
		if !strings.Contains(note, want) {
			t.Fatalf("note = %q, missing %q", note, want)
		}
	}
}

func TestImageEXIFOrientationParsesWebPPiStyle(t *testing.T) {
	webp := testWebPWithEXIFOrientation(8)
	if got := imageEXIFOrientation(webp); got != 8 {
		t.Fatalf("WebP EXIF orientation = %d, want 8", got)
	}
}

func TestImageResizeCallersOmitUnsafeImages(t *testing.T) {
	dir := t.TempDir()
	imagePath := filepath.Join(dir, "test.png")
	if err := os.WriteFile(imagePath, mustDecodeBase64(t, tinyPNGBase64), 0o600); err != nil {
		t.Fatal(err)
	}
	resizeFailure := func(llm.ContentPart, ImageResizeOptions) *ResizedImage { return nil }

	tool := NewReadTool(dir, FileToolOperations{ResizeImage: resizeFailure})
	result, err := tool.Execute("test-read-image", ReadToolInput{Path: imagePath})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 || result.Content[0].Type != llm.ContentText || !strings.Contains(result.Content[0].Text, "Image omitted") {
		t.Fatalf("read unsafe image result = %#v", result.Content)
	}

	processed, err := ProcessFileArguments([]string{imagePath}, ProcessFileArgumentsOptions{ResizeImage: resizeFailure})
	if err != nil {
		t.Fatal(err)
	}
	if len(processed.Images) != 0 || !strings.Contains(processed.Text, "Image omitted") {
		t.Fatalf("processed unsafe image result = %#v", processed)
	}
}

func testPNGBase64(t *testing.T, width, height int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 0xff, A: 0xff})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes())
}

func testNoisyPNGBase64(t *testing.T, width, height int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: byte((x*17 + y*31) % 256),
				G: byte((x*47 + y*13) % 256),
				B: byte((x*7 + y*61) % 256),
				A: 0xff,
			})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes())
}

func testJPEGBase64(t *testing.T, width, height int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, color.RGBA{B: 0xff, A: 0xff})
		}
	}
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, img, nil); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes())
}

func testJPEGBase64WithEXIFOrientation(t *testing.T, width, height, orientation int) string {
	t.Helper()
	jpegBytes := mustDecodeBase64(t, testJPEGBase64(t, width, height))
	if len(jpegBytes) < 2 || jpegBytes[0] != 0xff || jpegBytes[1] != 0xd8 {
		t.Fatalf("test JPEG missing SOI marker")
	}
	exif := testEXIFPayload(orientation)
	segmentLength := len(exif) + 2
	segment := []byte{0xff, 0xe1, byte(segmentLength >> 8), byte(segmentLength)}
	segment = append(segment, exif...)
	oriented := append([]byte{}, jpegBytes[:2]...)
	oriented = append(oriented, segment...)
	oriented = append(oriented, jpegBytes[2:]...)
	return base64.StdEncoding.EncodeToString(oriented)
}

func testWebPWithEXIFOrientation(orientation int) []byte {
	exif := testEXIFPayload(orientation)
	chunkSize := len(exif)
	riffSize := 4 + 8 + chunkSize
	webp := []byte{
		'R', 'I', 'F', 'F',
		byte(riffSize), byte(riffSize >> 8), byte(riffSize >> 16), byte(riffSize >> 24),
		'W', 'E', 'B', 'P',
		'E', 'X', 'I', 'F',
		byte(chunkSize), byte(chunkSize >> 8), byte(chunkSize >> 16), byte(chunkSize >> 24),
	}
	webp = append(webp, exif...)
	if chunkSize%2 == 1 {
		webp = append(webp, 0)
	}
	return webp
}

func testEXIFPayload(orientation int) []byte {
	return []byte{
		'E', 'x', 'i', 'f', 0x00, 0x00,
		'I', 'I', 0x2a, 0x00,
		0x08, 0x00, 0x00, 0x00,
		0x01, 0x00,
		0x12, 0x01,
		0x03, 0x00,
		0x01, 0x00, 0x00, 0x00,
		byte(orientation), 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
}
