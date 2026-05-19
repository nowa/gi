package gitui

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"sync"
	"testing"
)

func TestIsImageLineDetectsKittyAndITermAnywhere(t *testing.T) {
	if !IsImageLine("prefix \x1b_Ga=T,f=100;abc\x1b\\ suffix") {
		t.Fatalf("expected kitty image line")
	}
	if !IsImageLine("prefix \x1b]1337;File=inline=1:abc\x07 suffix") {
		t.Fatalf("expected iterm2 image line")
	}
	if !IsImageLine("\x1b[31mError\x1b[0m: \x1b]1337;File=inline=1:abc\x07") {
		t.Fatalf("expected image line after ANSI styling")
	}
	if !IsImageLine("Text before \x1b_Ga=T,f=100;" + strings.Repeat("A", 10000)) {
		t.Fatalf("expected long kitty image line")
	}
	if IsImageLine("\x1b[31mred\x1b[0m") {
		t.Fatalf("ansi-only line should not be an image line")
	}
}

func TestIsImageLinePiCoverageMatrix(t *testing.T) {
	longITermLine := "Text prefix " +
		"\x1b]1337;File=size=800,600;inline=1:" +
		strings.Repeat("A", 300000) +
		" suffix"
	if len(longITermLine) <= 300000 {
		t.Fatalf("test fixture should exercise Pi's 304k+ long-line regression, got %d chars", len(longITermLine))
	}

	tests := []struct {
		name string
		line string
		want bool
	}{
		{name: "iterm start", line: "\x1b]1337;File=size=100,100;inline=1:base64encodeddata==\x07", want: true},
		{name: "iterm with text before", line: "Some text \x1b]1337;File=size=100,100;inline=1:base64data==\x07 more text", want: true},
		{name: "iterm middle of long line", line: "Text before image...\x1b]1337;File=inline=1:verylongbase64data==...text after", want: true},
		{name: "iterm at end", line: "Regular text ending with \x1b]1337;File=inline=1:base64data==\x07", want: true},
		{name: "iterm minimal", line: "\x1b]1337;File=:\x07", want: true},
		{name: "kitty start", line: "\x1b_Ga=T,f=100,t=f,d=base64data...\x1b\\\x1b_Gm=i=1;\x1b\\", want: true},
		{name: "kitty with text before", line: "Output: \x1b_Ga=T,f=100;data...\x1b\\\x1b_Gm=i=1;\x1b\\", want: true},
		{name: "kitty with padding", line: "  \x1b_Ga=T,f=100...\x1b\\\x1b_Gm=i=1;\x1b\\  ", want: true},
		{name: "very long iterm", line: longITermLine, want: true},
		{name: "unsupported terminal still detects image", line: "Read image file [image/jpeg]\x1b]1337;File=inline=1:base64data==\x07", want: true},
		{name: "ansi before image", line: "\x1b[31mError output \x1b]1337;File=inline=1:image==\x07", want: true},
		{name: "ansi after image", line: "\x1b_Ga=T,f=100:data...\x1b\\\x1b_Gm=i=1;\x1b\\\x1b[0m reset", want: true},
		{name: "mixed protocols", line: "Kitty: \x1b_Ga=T...\x1b\\\x1b_Gm=i=1;\x1b\\ iTerm2: \x1b]1337;File=inline=1:data==\x07", want: true},
		{name: "multiple image segments", line: "Start \x1b]1337;File=img1==\x07 middle \x1b]1337;File=img2==\x07 end", want: true},
		{name: "plain text", line: "This is just a regular text line without any escape sequences", want: false},
		{name: "ansi only", line: "\x1b[31mRed text\x1b[0m and \x1b[32mgreen text\x1b[0m", want: false},
		{name: "cursor movement only", line: "\x1b[1A\x1b[2KLine cleared and moved up", want: false},
		{name: "partial iterm without escape", line: "Some text with ]1337;File but missing ESC at start", want: false},
		{name: "partial kitty without escape", line: "Some text with _G but missing ESC at start", want: false},
		{name: "empty", line: "", want: false},
		{name: "single newline", line: "\n", want: false},
		{name: "multiple newlines", line: "\n\n", want: false},
		{name: "file path keywords", line: "/path/to/File_1337_backup/image.jpg", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsImageLine(tc.line); got != tc.want {
				t.Fatalf("IsImageLine() = %v, want %v for %q", got, tc.want, tc.line)
			}
		})
	}
}

func TestDetectCapabilitiesFromEnvironment(t *testing.T) {
	t.Setenv("TERM", "")
	t.Setenv("TERM_PROGRAM", "ghostty")
	t.Setenv("TMUX", "")
	t.Setenv("CMUX_WORKSPACE_ID", "")
	t.Setenv("KITTY_WINDOW_ID", "")
	t.Setenv("GHOSTTY_RESOURCES_DIR", "")
	t.Setenv("WEZTERM_PANE", "")
	t.Setenv("ITERM_SESSION_ID", "")
	caps := DetectCapabilities()
	if !caps.Images || caps.Protocol != ImageProtocolKitty || !caps.Hyperlinks {
		t.Fatalf("ghostty caps = %#v", caps)
	}

	t.Setenv("TERM_PROGRAM", "iterm.app")
	caps = DetectCapabilities()
	if caps.Protocol != ImageProtocolITerm {
		t.Fatalf("iterm caps = %#v", caps)
	}

	t.Setenv("TERM", "tmux-256color")
	t.Setenv("TERM_PROGRAM", "ghostty")
	t.Setenv("TMUX", "/tmp/tmux")
	caps = DetectCapabilities()
	if caps.Images || caps.Hyperlinks {
		t.Fatalf("tmux caps should disable images/hyperlinks: %#v", caps)
	}

	t.Setenv("TMUX", "")
	t.Setenv("TERM", "screen-256color")
	t.Setenv("TERM_PROGRAM", "")
	caps = DetectCapabilities()
	if caps.Images || caps.Hyperlinks {
		t.Fatalf("screen caps should disable images/hyperlinks: %#v", caps)
	}

	t.Setenv("TERM", "")
	t.Setenv("KITTY_WINDOW_ID", "1")
	caps = DetectCapabilities()
	if !caps.Images || caps.Protocol != ImageProtocolKitty || !caps.Hyperlinks {
		t.Fatalf("kitty caps = %#v", caps)
	}

	t.Setenv("KITTY_WINDOW_ID", "")
	t.Setenv("WEZTERM_PANE", "0")
	caps = DetectCapabilities()
	if !caps.Images || caps.Protocol != ImageProtocolKitty || !caps.Hyperlinks {
		t.Fatalf("wezterm caps = %#v", caps)
	}

	t.Setenv("WEZTERM_PANE", "")
	t.Setenv("TERM_PROGRAM", "vscode")
	caps = DetectCapabilities()
	if caps.Images || caps.Protocol != ImageProtocolNone || !caps.Hyperlinks {
		t.Fatalf("vscode caps should enable hyperlinks without images: %#v", caps)
	}

	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("TERM", "")
	t.Setenv("COLORTERM", "")
	caps = DetectCapabilities()
	if caps.Images || caps.Protocol != ImageProtocolNone || caps.Hyperlinks || caps.TrueColor {
		t.Fatalf("unknown terminal caps = %#v, want no images/hyperlinks/truecolor", caps)
	}

	t.Setenv("TERM_PROGRAM", "iterm.app")
	t.Setenv("TERM", "tmux-256color")
	caps = DetectCapabilities()
	if caps.Images || caps.Protocol != ImageProtocolNone || caps.Hyperlinks {
		t.Fatalf("TERM=tmux should override outer terminal support: %#v", caps)
	}

	t.Setenv("TERM_PROGRAM", "ghostty")
	t.Setenv("TERM", "")
	t.Setenv("CMUX_WORKSPACE_ID", "workspace")
	caps = DetectCapabilities()
	if !caps.Images || caps.Protocol != ImageProtocolKitty || !caps.Hyperlinks {
		t.Fatalf("cmux should not disable Ghostty images: %#v", caps)
	}
}

func TestTerminalImageGlobalStateConcurrentAccess(t *testing.T) {
	t.Cleanup(func() {
		ResetCapabilitiesCache()
		SetCellDimensions(defaultCellDimensions)
	})

	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				if (worker+i)%2 == 0 {
					SetCapabilities(TerminalCapabilities{Images: true, Protocol: ImageProtocolKitty, TrueColor: true, Hyperlinks: true})
				} else {
					SetCapabilities(TerminalCapabilities{Protocol: ImageProtocolNone})
				}
				_ = GetCapabilities()
				SetCellDimensions(CellDimensions{Width: 8 + worker, Height: 16 + i%5})
				_ = GetCellDimensions()
				_ = CalculateImageCellSize(ImageDimensions{Width: 320, Height: 120}, 40, 10)
				_ = RenderImageWithDimensions([]byte("data"), ImageDimensions{Width: 10, Height: 10}, ImageRenderOptions{MaxWidthCells: 2})
				if i%25 == 0 {
					ResetCapabilitiesCache()
				}
			}
		}(worker)
	}
	wg.Wait()
}

func TestEncodeKittyDeleteAndHyperlink(t *testing.T) {
	for range 8 {
		if id := AllocateImageID(); id == 0 || id == ^uint32(0) {
			t.Fatalf("allocated kitty image id = %d, want Pi-style range [1, 0xfffffffe]", id)
		}
	}

	encoded := EncodeKitty([]byte("hello"), ImageRenderOptions{ID: 7, Width: 2, Height: 1, DisableCursorMovement: true})
	if !strings.Contains(encoded, "i=7") || !strings.Contains(encoded, "C=1") || !strings.Contains(encoded, "c=2") || !strings.Contains(encoded, "r=1") {
		t.Fatalf("kitty encoding missing params: %q", encoded)
	}
	noID := EncodeKitty([]byte("hello"), ImageRenderOptions{Width: 2, Height: 2, DisableCursorMovement: true})
	if !strings.HasPrefix(noID, "\x1b_Ga=T,f=100,q=2,C=1,c=2,r=2;") || strings.Contains(noID, "i=") {
		t.Fatalf("kitty encoding without explicit id should match pi params and omit i=: %q", noID)
	}
	if DeleteKittyImage(7) != "\x1b_Ga=d,d=I,i=7,q=2\x1b\\" {
		t.Fatalf("delete kitty image sequence changed")
	}
	if DeleteAllKittyImages() != "\x1b_Ga=d,d=A,q=2\x1b\\" {
		t.Fatalf("delete all kitty images sequence changed")
	}
	if got := Hyperlink("docs", "https://example.com"); !strings.Contains(got, "https://example.com") {
		t.Fatalf("hyperlink missing url: %q", got)
	}
}

func TestRenderImageKittyCursorMovementOption(t *testing.T) {
	SetCapabilities(TerminalCapabilities{Images: true, Protocol: ImageProtocolKitty, TrueColor: true, Hyperlinks: true})
	SetCellDimensions(CellDimensions{Width: 10, Height: 10})
	defer func() {
		ResetCapabilitiesCache()
		SetCellDimensions(defaultCellDimensions)
	}()

	sequence := RenderImage([]byte("hello"), ImageRenderOptions{Width: 2, Height: 2})
	if strings.Contains(sequence, ",C=1,") || strings.Contains(sequence, ",C=1;") {
		t.Fatalf("RenderImage should preserve Kitty terminal-side cursor movement by default: %q", sequence)
	}

	sequence = RenderImage([]byte("hello"), ImageRenderOptions{Width: 2, Height: 2, DisableCursorMovement: true})
	if !strings.Contains(sequence, ",C=1,") {
		t.Fatalf("RenderImage should support disabling Kitty terminal-side cursor movement: %q", sequence)
	}
}

func TestImageRenderOptionsPiAliases(t *testing.T) {
	moveCursorFalse := false
	moveCursorTrue := true

	encoded := EncodeKitty([]byte("hello"), ImageRenderOptions{
		ImageID:        9,
		MaxWidthCells:  2,
		MaxHeightCells: 1,
		MoveCursor:     &moveCursorFalse,
	})
	for _, want := range []string{"i=9", "C=1", "c=2", "r=1"} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("kitty alias encoding missing %q in %q", want, encoded)
		}
	}

	encoded = EncodeKitty([]byte("hello"), ImageRenderOptions{ImageId: 10, MoveCursor: &moveCursorTrue})
	if !strings.Contains(encoded, "i=10") || strings.Contains(encoded, "C=1") {
		t.Fatalf("kitty ImageId/moveCursor aliases not honored: %q", encoded)
	}

	SetCapabilities(TerminalCapabilities{Images: true, Protocol: ImageProtocolKitty, TrueColor: true, Hyperlinks: true})
	SetCellDimensions(CellDimensions{Width: 10, Height: 10})
	defer func() {
		ResetCapabilitiesCache()
		SetCellDimensions(defaultCellDimensions)
	}()

	result := RenderImageWithDimensions([]byte("hello"), ImageDimensions{WidthPx: 20, HeightPx: 10}, ImageRenderOptions{
		ImageID:        11,
		MaxWidthCells:  2,
		MaxHeightCells: 1,
		MoveCursor:     &moveCursorFalse,
	})
	if result == nil || result.Rows != 1 || result.ImageID != 11 || result.ImageId != 11 {
		t.Fatalf("RenderImageWithDimensions result = %#v, want one-row kitty image id 11", result)
	}
	for _, want := range []string{"i=11", "C=1", "c=2", "r=1"} {
		if !strings.Contains(result.Sequence, want) {
			t.Fatalf("RenderImageWithDimensions missing %q in %q", want, result.Sequence)
		}
	}
}

func TestRenderImageReturnsEmptyWithoutImageProtocol(t *testing.T) {
	SetCapabilities(TerminalCapabilities{Images: false, Protocol: ImageProtocolNone})
	defer ResetCapabilitiesCache()

	if got := RenderImage([]byte("hello"), ImageRenderOptions{Alt: "chart", Width: 10}); got != "" {
		t.Fatalf("RenderImage without image protocol = %q, want empty string like Pi null result", got)
	}

	component := NewImage([]byte("not an image"), ImageOptions{Alt: "chart"})
	lines := component.Render(10)
	if len(lines) != 1 || strings.TrimRight(lines[0], " ") != "[chart]" {
		t.Fatalf("Image component should still own fallback rendering, got %#v", lines)
	}
}

func TestHyperlinkPiOSC8ExactRendering(t *testing.T) {
	if got := Hyperlink("click me", "https://example.com"); got != "\x1b]8;;https://example.com\x1b\\click me\x1b]8;;\x1b\\" {
		t.Fatalf("hyperlink = %q", got)
	}
	styled := "\x1b[4m\x1b[34mclick me\x1b[0m"
	if got := Hyperlink(styled, "https://example.com"); !strings.Contains(got, styled) || !strings.HasPrefix(got, "\x1b]8;;https://example.com\x1b\\") || !strings.HasSuffix(got, "\x1b]8;;\x1b\\") {
		t.Fatalf("styled hyperlink should preserve ANSI text inside OSC 8 wrapper: %q", got)
	}
	if got := Hyperlink("", "https://example.com"); got != "\x1b]8;;https://example.com\x1b\\\x1b]8;;\x1b\\" {
		t.Fatalf("empty-text hyperlink = %q", got)
	}
	if got := Hyperlink("empty href", ""); got != "\x1b]8;;\x1b\\empty href\x1b]8;;\x1b\\" {
		t.Fatalf("empty-url hyperlink should still emit OSC 8 wrapper like Pi: %q", got)
	}
	if got := Hyperlink("README.md", "file:///home/user/README.md"); !strings.Contains(got, "file:///home/user/README.md") || !strings.Contains(got, "README.md") {
		t.Fatalf("file URI hyperlink missing content: %q", got)
	}
}

func TestEncodeITerm2Options(t *testing.T) {
	preserve := false
	inline := false
	encoded := EncodeITerm2([]byte("hello"), ImageRenderOptions{
		Width:               4,
		HeightAuto:          true,
		Alt:                 "chart.png",
		Inline:              &inline,
		PreserveAspectRatio: &preserve,
	})
	for _, want := range []string{"inline=0", "width=4", "height=auto", "name=Y2hhcnQucG5n", "preserveAspectRatio=0"} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("iterm2 encoding missing %q in %q", want, encoded)
		}
	}
	if !strings.HasSuffix(encoded, ":"+"aGVsbG8="+"\x07") {
		t.Fatalf("iterm2 encoding missing payload: %q", encoded)
	}
}

func TestDefaultCellDimensionsMatchPi(t *testing.T) {
	SetCellDimensions(defaultCellDimensions)
	if got := GetCellDimensions(); got.Width != 9 || got.Height != 18 {
		t.Fatalf("default cell dimensions = %#v, want 9x18", got)
	}
}

func TestImageDimensionsAndCellSizing(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	pngData := buf.Bytes()
	dims, err := GetPngDimensions(pngData)
	if err != nil {
		t.Fatal(err)
	}
	if dims.Width != 2 || dims.Height != 3 {
		t.Fatalf("dims = %#v", dims)
	}
	SetCellDimensions(CellDimensions{Width: 10, Height: 10})
	defer SetCellDimensions(defaultCellDimensions)
	size := CalculateImageCellSize(ImageDimensions{Width: 100, Height: 50}, 10, 3)
	if size.Columns != 6 || size.Rows != 3 {
		t.Fatalf("cell size = %#v", size)
	}
	SetCellDimensions(CellDimensions{WidthPx: 10, HeightPx: 10})
	aliasSize := CalculateImageCellSize(ImageDimensions{WidthPx: 100, HeightPx: 50}, 10, 3)
	if aliasSize != size {
		t.Fatalf("Pi-style WidthPx/HeightPx sizing = %#v, want %#v", aliasSize, size)
	}
	if got := GetCellDimensions(); got.Width != 10 || got.Height != 10 {
		t.Fatalf("Pi-style cell dimension aliases should normalize to Width/Height, got %#v", got)
	}

	custom := CalculateImageCellSize(
		ImageDimensions{WidthPx: 90, HeightPx: 90},
		10,
		CellDimensions{WidthPx: 9, HeightPx: 9},
	)
	if custom.Columns != 10 || custom.Rows != 10 {
		t.Fatalf("custom cell dimensions size = %#v, want 10x10", custom)
	}

	customLimited := CalculateImageCellSize(
		ImageDimensions{WidthPx: 90, HeightPx: 90},
		10,
		3,
		CellDimensions{WidthPx: 9, HeightPx: 9},
	)
	if customLimited.Columns != 3 || customLimited.Rows != 3 {
		t.Fatalf("custom cell dimensions with max height = %#v, want 3x3", customLimited)
	}

	if got := CalculateImageRows(
		ImageDimensions{WidthPx: 90, HeightPx: 90},
		10,
		CellDimensions{WidthPx: 9, HeightPx: 9},
	); got != 10 {
		t.Fatalf("CalculateImageRows with custom cell dimensions = %d, want 10", got)
	}
}

func TestImageFormatDimensionHelpersMatchPi(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 7, 5))
	img.Set(0, 0, color.RGBA{R: 255, G: 128, A: 255})

	var jpegBuf bytes.Buffer
	if err := jpeg.Encode(&jpegBuf, img, nil); err != nil {
		t.Fatal(err)
	}
	jpegDims, err := GetJpegDimensions(jpegBuf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if jpegDims.Width != 7 || jpegDims.Height != 5 {
		t.Fatalf("jpeg dimensions = %#v, want 7x5", jpegDims)
	}
	if generic, err := GetImageDimensions(jpegBuf.Bytes()); err != nil || generic != jpegDims {
		t.Fatalf("generic jpeg dimensions = %#v, %v; want %#v", generic, err, jpegDims)
	}

	var gifBuf bytes.Buffer
	if err := gif.Encode(&gifBuf, img, nil); err != nil {
		t.Fatal(err)
	}
	gifDims, err := GetGifDimensions(gifBuf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if gifDims.Width != 7 || gifDims.Height != 5 {
		t.Fatalf("gif dimensions = %#v, want 7x5", gifDims)
	}
	if generic, err := GetImageDimensions(gifBuf.Bytes()); err != nil || generic != gifDims {
		t.Fatalf("generic gif dimensions = %#v, %v; want %#v", generic, err, gifDims)
	}

	if _, err := GetJpegDimensions(gifBuf.Bytes()); err == nil {
		t.Fatalf("specific JPEG dimension helper should reject GIF data")
	}
	if _, err := GetGifDimensions(jpegBuf.Bytes()); err == nil {
		t.Fatalf("specific GIF dimension helper should reject JPEG data")
	}
}

func TestWebpDimensions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		data   []byte
		width  int
		height int
	}{
		{name: "vp8", data: testWebPVP8(320, 240), width: 320, height: 240},
		{name: "vp8l", data: testWebPVP8L(123, 45), width: 123, height: 45},
		{name: "vp8x", data: testWebPVP8X(4096, 2048), width: 4096, height: 2048},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dims, err := GetWebpDimensions(tc.data)
			if err != nil {
				t.Fatal(err)
			}
			if dims.Width != tc.width || dims.Height != tc.height {
				t.Fatalf("webp dims = %#v, want %dx%d", dims, tc.width, tc.height)
			}
			generic, err := GetImageDimensions(tc.data)
			if err != nil {
				t.Fatal(err)
			}
			if generic != dims {
				t.Fatalf("generic dims = %#v, want %#v", generic, dims)
			}
		})
	}
	if _, err := GetWebpDimensions([]byte("not webp")); err == nil {
		t.Fatalf("invalid webp should return an error")
	}
	if _, err := GetPngDimensions(testWebPVP8(10, 10)); err == nil {
		t.Fatalf("specific PNG dimension helper should reject WebP data")
	}
}

func TestImageComponentRendersITerm2PlacementOnLastRow(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	SetCapabilities(TerminalCapabilities{Images: true, Protocol: ImageProtocolITerm, TrueColor: true, Hyperlinks: true})
	SetCellDimensions(CellDimensions{Width: 10, Height: 10})
	defer func() {
		ResetCapabilitiesCache()
		SetCellDimensions(defaultCellDimensions)
	}()

	component := NewImage(buf.Bytes(), ImageOptions{MaxWidth: 2, Alt: "chart"})
	lines := component.Render(6)
	if len(lines) != 2 {
		t.Fatalf("line count = %d, want 2", len(lines))
	}
	if lines[0] != "" {
		t.Fatalf("first iTerm2 padding line = %q, want empty", lines[0])
	}
	if !strings.HasPrefix(lines[1], "\x1b[1A\x1b]1337;File=") || !strings.Contains(lines[1], "inline=1;width=2;height=auto") {
		t.Fatalf("iTerm2 image placement line = %q", lines[1])
	}
}

func TestImageComponentRendersKittySequenceAndPaddingRows(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 100))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	SetCapabilities(TerminalCapabilities{Images: true, Protocol: ImageProtocolKitty, TrueColor: true, Hyperlinks: true})
	SetCellDimensions(CellDimensions{Width: 10, Height: 20})
	defer func() {
		ResetCapabilitiesCache()
		SetCellDimensions(defaultCellDimensions)
	}()

	component := NewImage(buf.Bytes(), ImageOptions{MaxWidth: 10, Alt: "chart"})
	lines := component.Render(12)
	if len(lines) != 5 {
		t.Fatalf("line count = %d, want 5", len(lines))
	}
	if component.ImageID() == 0 {
		t.Fatalf("image id should be allocated")
	}
	if !strings.HasPrefix(lines[0], "\x1b_G") || !strings.Contains(lines[0], "C=1") || !strings.Contains(lines[0], "c=1") || !strings.Contains(lines[0], "r=5") {
		t.Fatalf("kitty image line missing placement params: %q", lines[0])
	}
	for idx, line := range lines[1:] {
		if line != "" {
			t.Fatalf("padding line %d = %q, want empty", idx+1, line)
		}
	}
}

func TestImagePiFallbackDescription(t *testing.T) {
	dims := ImageDimensions{Width: 10, Height: 20}
	if got := ImageFallbackDescription("image/png", &dims, "chart.png"); got != "[Image: chart.png [image/png] 10x20]" {
		t.Fatalf("fallback description = %q", got)
	}
	if got := ImageFallback("image/png", dims, "chart.png"); got != "[Image: chart.png [image/png] 10x20]" {
		t.Fatalf("Pi-style ImageFallback with value dims = %q", got)
	}
	if got := ImageFallback("image/png", &ImageDimensions{WidthPx: 10, HeightPx: 20}, "chart.png"); got != "[Image: chart.png [image/png] 10x20]" {
		t.Fatalf("Pi-style ImageFallback with WidthPx/HeightPx = %q", got)
	}
	if got := ImageFallback("image/png", "chart.png"); got != "[Image: chart.png [image/png]]" {
		t.Fatalf("Pi-style ImageFallback filename-only form = %q", got)
	}
	if got := ImageFallback("chart", 10); got != "[chart]   " {
		t.Fatalf("legacy alt-width ImageFallback = %q", got)
	}
	zeroDims := ImageDimensions{}
	if got := ImageFallbackDescription("image/png", &zeroDims, ""); got != "[Image: [image/png] 0x0]" {
		t.Fatalf("zero-dimension fallback description = %q", got)
	}

	SetCapabilities(TerminalCapabilities{Images: false, Protocol: ImageProtocolNone})
	defer ResetCapabilitiesCache()

	component := NewImage([]byte("not an image"), ImageOptions{
		MimeType:   "image/png",
		Filename:   "chart.png",
		Dimensions: &dims,
	}, ImageTheme{Fallback: func(text string) string { return text }})
	lines := component.Render(20)
	if len(lines) != 1 || lines[0] != "[Image: chart.png [image/png] 10x20]" {
		t.Fatalf("fallback lines = %#v", lines)
	}

	colored := NewImage([]byte("not an image"), ImageOptions{
		MimeType:   "image/png",
		Filename:   "chart.png",
		Dimensions: &dims,
	}, ImageTheme{FallbackColor: func(text string) string { return "\x1b[33m" + text + "\x1b[0m" }})
	lines = colored.Render(40)
	if len(lines) != 1 || lines[0] != "\x1b[33m[Image: chart.png [image/png] 10x20]\x1b[0m" {
		t.Fatalf("Pi-style fallbackColor lines = %#v", lines)
	}
}

func TestImagePiOptionsReuseKittyIDAndAvoidITermID(t *testing.T) {
	dims := ImageDimensions{Width: 20, Height: 20}
	SetCellDimensions(CellDimensions{Width: 10, Height: 10})
	defer func() {
		ResetCapabilitiesCache()
		SetCellDimensions(defaultCellDimensions)
	}()

	SetCapabilities(TerminalCapabilities{Images: true, Protocol: ImageProtocolKitty, TrueColor: true, Hyperlinks: true})
	kitty := NewImage([]byte("raw"), ImageOptions{Dimensions: &dims, MaxWidthCells: 2, ImageID: 42})
	kittyLines := kitty.Render(4)
	if kitty.ImageID() != 42 || !strings.Contains(kittyLines[0], "i=42") {
		t.Fatalf("kitty image id not reused: id=%d lines=%#v", kitty.ImageID(), kittyLines)
	}
	kittyAlias := NewImage([]byte("raw"), ImageOptions{Dimensions: &dims, MaxWidthCells: 2, ImageId: 43})
	kittyAliasLines := kittyAlias.Render(4)
	if kittyAlias.ImageID() != 43 || !strings.Contains(kittyAliasLines[0], "i=43") {
		t.Fatalf("kitty ImageId alias not reused: id=%d lines=%#v", kittyAlias.ImageID(), kittyAliasLines)
	}

	SetCapabilities(TerminalCapabilities{Images: true, Protocol: ImageProtocolITerm, TrueColor: true, Hyperlinks: true})
	iterm := NewImage([]byte("raw"), ImageOptions{Dimensions: &dims, MaxWidthCells: 2})
	itermLines := iterm.Render(4)
	if iterm.ImageID() != 0 {
		t.Fatalf("iTerm2 should not allocate kitty image id, got %d", iterm.ImageID())
	}
	if len(itermLines) != 2 || !strings.HasPrefix(itermLines[1], "\x1b[1A\x1b]1337;File=") {
		t.Fatalf("iterm lines = %#v", itermLines)
	}
}

func TestImagePiManualDemoLayoutFallback(t *testing.T) {
	SetCapabilities(TerminalCapabilities{Images: false, Protocol: ImageProtocolNone})
	defer ResetCapabilitiesCache()

	dims := ImageDimensions{WidthPx: 2, HeightPx: 3}
	terminal := NewVirtualTerminal(80, 8)
	ui := NewTUI(terminal)
	ui.AddChild(NewText("Image Rendering Test", 1, 1))
	ui.AddChild(NewSpacer(1))
	ui.AddChild(NewImage([]byte("raw"), ImageOptions{
		MimeType:      "image/png",
		MaxWidthCells: 60,
		Dimensions:    &dims,
	}, ImageTheme{FallbackColor: func(text string) string { return "\x1b[33m" + text + "\x1b[0m" }}))
	ui.AddChild(NewSpacer(1))
	ui.AddChild(NewText("Press Ctrl+C to exit", 1, 0))
	ui.Start()
	defer ui.Stop()

	plain := strings.Join(terminal.GetViewport(), "\n")
	for _, want := range []string{
		" Image Rendering Test",
		"[Image: [image/png] 2x3]",
		" Press Ctrl+C to exit",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("manual image demo layout missing %q in %q", want, plain)
		}
	}
	if !strings.Contains(terminal.Output(), "\x1b[33m[Image: [image/png] 2x3]\x1b[0m") {
		t.Fatalf("manual image demo fallback color missing in terminal output: %q", terminal.Output())
	}
}

func testWebPChunk(fourCC string, payload []byte) []byte {
	var data []byte
	data = append(data, "RIFF"...)
	var riffSize [4]byte
	binary.LittleEndian.PutUint32(riffSize[:], uint32(4+8+len(payload)))
	data = append(data, riffSize[:]...)
	data = append(data, "WEBP"...)
	data = append(data, fourCC...)
	var chunkSize [4]byte
	binary.LittleEndian.PutUint32(chunkSize[:], uint32(len(payload)))
	data = append(data, chunkSize[:]...)
	data = append(data, payload...)
	return data
}

func testWebPVP8(width, height int) []byte {
	payload := make([]byte, 10)
	copy(payload[3:6], []byte{0x9d, 0x01, 0x2a})
	binary.LittleEndian.PutUint16(payload[6:8], uint16(width))
	binary.LittleEndian.PutUint16(payload[8:10], uint16(height))
	return testWebPChunk("VP8 ", payload)
}

func testWebPVP8L(width, height int) []byte {
	payload := make([]byte, 5)
	payload[0] = 0x2f
	bits := uint32(width-1) | uint32(height-1)<<14
	binary.LittleEndian.PutUint32(payload[1:5], bits)
	return testWebPChunk("VP8L", payload)
}

func testWebPVP8X(width, height int) []byte {
	payload := make([]byte, 10)
	w := width - 1
	h := height - 1
	payload[4] = byte(w)
	payload[5] = byte(w >> 8)
	payload[6] = byte(w >> 16)
	payload[7] = byte(h)
	payload[8] = byte(h >> 8)
	payload[9] = byte(h >> 16)
	return testWebPChunk("VP8X", payload)
}
