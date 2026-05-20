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
