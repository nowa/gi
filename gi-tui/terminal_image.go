package gitui

import (
	"bytes"
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

type ImageProtocol string

const (
	ImageProtocolNone  ImageProtocol = "none"
	ImageProtocolKitty ImageProtocol = "kitty"
	ImageProtocolITerm ImageProtocol = "iterm2"
)

type CellDimensions struct {
	Width    int
	Height   int
	WidthPx  int
	HeightPx int
}

type ImageDimensions struct {
	Width    int
	Height   int
	WidthPx  int
	HeightPx int
}

type TerminalCapabilities struct {
	Images     bool
	Protocol   ImageProtocol
	TrueColor  bool
	Hyperlinks bool
}

type ImageRenderOptions struct {
	ID                    uint32
	ImageID               uint32
	ImageId               uint32
	Width                 int
	Height                int
	MaxWidthCells         int
	MaxHeightCells        int
	HeightAuto            bool
	Alt                   string
	Protocol              ImageProtocol
	DisableCursorMovement bool
	MoveCursor            *bool
	Inline                *bool
	PreserveAspectRatio   *bool
}

type ImageRenderResult struct {
	Sequence string
	Rows     int
	ImageID  uint32
	ImageId  uint32
}

var (
	imageIDCounter        uint32
	terminalImageStateMu  sync.RWMutex
	capabilities          TerminalCapabilities
	capabilitiesSet       bool
	defaultCellDimensions = CellDimensions{Width: 9, Height: 18}
	cellDimensions        = defaultCellDimensions
)

func AllocateImageID() uint32 {
	var buf [4]byte
	if _, err := cryptorand.Read(buf[:]); err == nil {
		return binary.LittleEndian.Uint32(buf[:])%0xfffffffe + 1
	}
	return atomic.AddUint32(&imageIDCounter, 1)
}

func GetCapabilities() TerminalCapabilities {
	terminalImageStateMu.Lock()
	defer terminalImageStateMu.Unlock()
	if capabilitiesSet {
		return capabilities
	}
	capabilities = DetectCapabilities()
	capabilitiesSet = true
	return capabilities
}
func SetCapabilities(c TerminalCapabilities) {
	terminalImageStateMu.Lock()
	defer terminalImageStateMu.Unlock()
	capabilities = c
	capabilitiesSet = true
}
func ResetCapabilitiesCache() {
	terminalImageStateMu.Lock()
	defer terminalImageStateMu.Unlock()
	capabilities = TerminalCapabilities{}
	capabilitiesSet = false
}

func DetectCapabilities() TerminalCapabilities {
	termProgram := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	term := strings.ToLower(os.Getenv("TERM"))
	colorTerm := strings.ToLower(os.Getenv("COLORTERM"))
	trueColor := colorTerm == "truecolor" || colorTerm == "24bit"
	if os.Getenv("TMUX") != "" || strings.HasPrefix(term, "tmux") || strings.HasPrefix(term, "screen") {
		return TerminalCapabilities{Protocol: ImageProtocolNone, TrueColor: trueColor, Hyperlinks: false}
	}
	if os.Getenv("KITTY_WINDOW_ID") != "" || termProgram == "kitty" {
		return TerminalCapabilities{Images: true, Protocol: ImageProtocolKitty, TrueColor: true, Hyperlinks: true}
	}
	if termProgram == "ghostty" || strings.Contains(term, "ghostty") || os.Getenv("GHOSTTY_RESOURCES_DIR") != "" {
		return TerminalCapabilities{Images: true, Protocol: ImageProtocolKitty, TrueColor: true, Hyperlinks: true}
	}
	if os.Getenv("WEZTERM_PANE") != "" || termProgram == "wezterm" {
		return TerminalCapabilities{Images: true, Protocol: ImageProtocolKitty, TrueColor: true, Hyperlinks: true}
	}
	if os.Getenv("ITERM_SESSION_ID") != "" || termProgram == "iterm.app" {
		return TerminalCapabilities{Images: true, Protocol: ImageProtocolITerm, TrueColor: true, Hyperlinks: true}
	}
	if termProgram == "vscode" || termProgram == "alacritty" {
		return TerminalCapabilities{Protocol: ImageProtocolNone, TrueColor: true, Hyperlinks: true}
	}
	return TerminalCapabilities{Protocol: ImageProtocolNone, TrueColor: trueColor, Hyperlinks: false}
}

func GetCellDimensions() CellDimensions {
	terminalImageStateMu.RLock()
	defer terminalImageStateMu.RUnlock()
	return cellDimensions
}

func SetCellDimensions(d CellDimensions) {
	d = normalizeCellDimensions(d)
	if d.Width > 0 && d.Height > 0 {
		terminalImageStateMu.Lock()
		defer terminalImageStateMu.Unlock()
		cellDimensions = d
	}
}

func normalizeCellDimensions(d CellDimensions) CellDimensions {
	if d.Width <= 0 {
		d.Width = d.WidthPx
	}
	if d.Height <= 0 {
		d.Height = d.HeightPx
	}
	return d
}

func normalizeImageDimensions(d ImageDimensions) ImageDimensions {
	if d.Width <= 0 {
		d.Width = d.WidthPx
	}
	if d.Height <= 0 {
		d.Height = d.HeightPx
	}
	return d
}

type ImageCellSize struct {
	Columns int
	Rows    int
}

func CalculateImageCellSize(imageDimensions ImageDimensions, maxWidthCells int, args ...any) ImageCellSize {
	dims, maxHeight := imageCellSizeArgs(args)
	imageDimensions = normalizeImageDimensions(imageDimensions)
	maxWidth := max(1, maxWidthCells)
	imageWidth := max(1, imageDimensions.Width)
	imageHeight := max(1, imageDimensions.Height)
	widthScale := float64(maxWidth*dims.Width) / float64(imageWidth)
	heightScale := widthScale
	if maxHeight > 0 {
		heightScale = float64(maxHeight*dims.Height) / float64(imageHeight)
	}
	scale := math.Min(widthScale, heightScale)
	columns := int(math.Ceil(float64(imageWidth) * scale / float64(dims.Width)))
	rows := int(math.Ceil(float64(imageHeight) * scale / float64(dims.Height)))
	if maxHeight > 0 {
		rows = min(rows, maxHeight)
	}
	return ImageCellSize{Columns: max(1, min(maxWidth, columns)), Rows: max(1, rows)}
}

func imageCellSizeArgs(args []any) (CellDimensions, int) {
	dims := normalizeCellDimensions(GetCellDimensions())
	maxHeight := 0
	for _, arg := range args {
		switch value := arg.(type) {
		case int:
			if value > 0 {
				maxHeight = max(1, value)
			}
		case CellDimensions:
			normalized := normalizeCellDimensions(value)
			if normalized.Width > 0 && normalized.Height > 0 {
				dims = normalized
			}
		case *CellDimensions:
			if value != nil {
				normalized := normalizeCellDimensions(*value)
				if normalized.Width > 0 && normalized.Height > 0 {
					dims = normalized
				}
			}
		}
	}
	return dims, maxHeight
}

func CalculateImageRows(imageDimensions ImageDimensions, targetWidthCells int, cellDimensions ...CellDimensions) int {
	if len(cellDimensions) > 0 {
		return CalculateImageCellSize(imageDimensions, targetWidthCells, cellDimensions[0]).Rows
	}
	return CalculateImageCellSize(imageDimensions, targetWidthCells).Rows
}

func EncodeKitty(data []byte, options ImageRenderOptions) string {
	base64Data := base64.StdEncoding.EncodeToString(data)
	params := []string{"a=T", "f=100", "q=2"}
	if imageRenderDisableCursorMovement(options) {
		params = append(params, "C=1")
	}
	if width := imageRenderColumns(options); width > 0 {
		params = append(params, fmt.Sprintf("c=%d", width))
	}
	if height := imageRenderRows(options); height > 0 {
		params = append(params, fmt.Sprintf("r=%d", height))
	}
	if id := imageRenderID(options); id > 0 {
		params = append(params, fmt.Sprintf("i=%d", id))
	}
	const chunkSize = 4096
	if len(base64Data) <= chunkSize {
		return fmt.Sprintf("\x1b_G%s;%s\x1b\\", strings.Join(params, ","), base64Data)
	}
	var out strings.Builder
	for offset := 0; offset < len(base64Data); offset += chunkSize {
		end := min(offset+chunkSize, len(base64Data))
		chunk := base64Data[offset:end]
		if offset == 0 {
			out.WriteString(fmt.Sprintf("\x1b_G%s,m=1;%s\x1b\\", strings.Join(params, ","), chunk))
		} else if end == len(base64Data) {
			out.WriteString(fmt.Sprintf("\x1b_Gm=0;%s\x1b\\", chunk))
		} else {
			out.WriteString(fmt.Sprintf("\x1b_Gm=1;%s\x1b\\", chunk))
		}
	}
	return out.String()
}

func EncodeITerm2(data []byte, options ImageRenderOptions) string {
	inline := true
	if options.Inline != nil {
		inline = *options.Inline
	}
	params := []string{fmt.Sprintf("inline=%d", boolInt(inline))}
	if width := imageRenderColumns(options); width > 0 {
		params = append(params, fmt.Sprintf("width=%d", width))
	}
	if options.HeightAuto {
		params = append(params, "height=auto")
	} else if height := imageRenderRows(options); height > 0 {
		params = append(params, fmt.Sprintf("height=%d", height))
	}
	if options.Alt != "" {
		name := base64.StdEncoding.EncodeToString([]byte(options.Alt))
		params = append(params, "name="+name)
	}
	if options.PreserveAspectRatio != nil && !*options.PreserveAspectRatio {
		params = append(params, "preserveAspectRatio=0")
	}
	return fmt.Sprintf("\x1b]1337;File=%s:%s\x07", strings.Join(params, ";"), base64.StdEncoding.EncodeToString(data))
}

func RenderImage(data []byte, options ImageRenderOptions) string {
	protocol := options.Protocol
	if protocol == "" {
		protocol = GetCapabilities().Protocol
	}
	switch protocol {
	case ImageProtocolKitty:
		return EncodeKitty(data, options)
	case ImageProtocolITerm:
		if options.Height == 0 {
			options.HeightAuto = true
		}
		return EncodeITerm2(data, options)
	default:
		return ""
	}
}

func RenderImageWithDimensions(data []byte, imageDimensions ImageDimensions, options ImageRenderOptions) *ImageRenderResult {
	caps := GetCapabilities()
	if !caps.Images {
		return nil
	}
	protocol := options.Protocol
	if protocol == "" {
		protocol = caps.Protocol
	}
	if protocol == ImageProtocolNone {
		return nil
	}

	maxWidth := options.MaxWidthCells
	if maxWidth <= 0 {
		maxWidth = options.Width
	}
	if maxWidth <= 0 {
		maxWidth = 80
	}
	maxHeight := options.MaxHeightCells
	if maxHeight <= 0 {
		maxHeight = options.Height
	}
	size := CalculateImageCellSize(imageDimensions, maxWidth)
	if maxHeight > 0 {
		size = CalculateImageCellSize(imageDimensions, maxWidth, maxHeight)
	}

	renderOptions := options
	renderOptions.Protocol = protocol
	renderOptions.Width = size.Columns
	renderOptions.Height = size.Rows

	switch protocol {
	case ImageProtocolKitty:
		id := imageRenderID(renderOptions)
		return &ImageRenderResult{
			Sequence: EncodeKitty(data, renderOptions),
			Rows:     size.Rows,
			ImageID:  id,
			ImageId:  id,
		}
	case ImageProtocolITerm:
		renderOptions.Height = 0
		renderOptions.HeightAuto = true
		return &ImageRenderResult{Sequence: EncodeITerm2(data, renderOptions), Rows: size.Rows}
	default:
		return nil
	}
}

func imageRenderID(options ImageRenderOptions) uint32 {
	if options.ID > 0 {
		return options.ID
	}
	if options.ImageID > 0 {
		return options.ImageID
	}
	return options.ImageId
}

func imageRenderColumns(options ImageRenderOptions) int {
	if options.Width > 0 {
		return options.Width
	}
	return options.MaxWidthCells
}

func imageRenderRows(options ImageRenderOptions) int {
	if options.Height > 0 {
		return options.Height
	}
	return options.MaxHeightCells
}

func imageRenderDisableCursorMovement(options ImageRenderOptions) bool {
	if options.MoveCursor != nil {
		return !*options.MoveCursor
	}
	return options.DisableCursorMovement
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func IsImageLine(line string) bool {
	return strings.Contains(line, "\x1b_G") || strings.Contains(line, "\x1b]1337;File=")
}

func ImageFallback(alt string, args ...any) string {
	if len(args) > 0 {
		if width, ok := args[0].(int); ok {
			return imageAltFallback(alt, width)
		}
		var dimensions *ImageDimensions
		var filename string
		switch value := args[0].(type) {
		case ImageDimensions:
			dimensions = &value
		case *ImageDimensions:
			dimensions = value
		case string:
			filename = value
		}
		if len(args) > 1 {
			if value, ok := args[1].(string); ok {
				filename = value
			}
		}
		return ImageFallbackDescription(alt, dimensions, filename)
	}
	return ImageFallbackDescription(alt, nil, "")
}

func imageAltFallback(alt string, width int) string {
	if alt == "" {
		alt = "image"
	}
	return TruncateToWidth("["+alt+"]", width, "", true)
}

func ImageFallbackDescription(mimeType string, dimensions *ImageDimensions, filename string) string {
	var parts []string
	if filename != "" {
		parts = append(parts, filename)
	}
	parts = append(parts, "["+mimeType+"]")
	if dimensions != nil {
		dims := normalizeImageDimensions(*dimensions)
		parts = append(parts, fmt.Sprintf("%dx%d", dims.Width, dims.Height))
	}
	return "[Image: " + strings.Join(parts, " ") + "]"
}

func Hyperlink(text, url string) string {
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

func DeleteKittyImage(id uint32) string {
	return fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", id)
}

func DeleteAllKittyImages() string { return "\x1b_Ga=d,d=A,q=2\x1b\\" }

func GetImageDimensions(data []byte) (ImageDimensions, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		if dims, webpErr := GetWebpDimensions(data); webpErr == nil {
			return dims, nil
		}
		return ImageDimensions{}, err
	}
	return ImageDimensions{Width: config.Width, Height: config.Height}, nil
}

func GetPngDimensions(data []byte) (ImageDimensions, error) {
	return getImageDimensionsForFormat(data, "png")
}
func GetJpegDimensions(data []byte) (ImageDimensions, error) {
	return getImageDimensionsForFormat(data, "jpeg")
}
func GetGifDimensions(data []byte) (ImageDimensions, error) {
	return getImageDimensionsForFormat(data, "gif")
}

func getImageDimensionsForFormat(data []byte, wantFormat string) (ImageDimensions, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return ImageDimensions{}, err
	}
	if format != wantFormat {
		return ImageDimensions{}, fmt.Errorf("expected %s image, got %s", wantFormat, format)
	}
	return ImageDimensions{Width: config.Width, Height: config.Height}, nil
}

func GetWebpDimensions(data []byte) (ImageDimensions, error) {
	if len(data) < 20 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return ImageDimensions{}, fmt.Errorf("invalid webp image")
	}
	chunk := string(data[12:16])
	payload := 20
	switch chunk {
	case "VP8 ":
		if len(data) < payload+10 {
			return ImageDimensions{}, fmt.Errorf("truncated vp8 webp image")
		}
		width := int(binary.LittleEndian.Uint16(data[payload+6:payload+8]) & 0x3fff)
		height := int(binary.LittleEndian.Uint16(data[payload+8:payload+10]) & 0x3fff)
		if width <= 0 || height <= 0 {
			return ImageDimensions{}, fmt.Errorf("invalid vp8 webp dimensions")
		}
		return ImageDimensions{Width: width, Height: height}, nil
	case "VP8L":
		if len(data) < payload+5 {
			return ImageDimensions{}, fmt.Errorf("truncated vp8l webp image")
		}
		if data[payload] != 0x2f {
			return ImageDimensions{}, fmt.Errorf("invalid vp8l webp signature")
		}
		bits := binary.LittleEndian.Uint32(data[payload+1 : payload+5])
		width := int(bits&0x3fff) + 1
		height := int((bits>>14)&0x3fff) + 1
		return ImageDimensions{Width: width, Height: height}, nil
	case "VP8X":
		if len(data) < payload+10 {
			return ImageDimensions{}, fmt.Errorf("truncated vp8x webp image")
		}
		width := int(data[payload+4]) | int(data[payload+5])<<8 | int(data[payload+6])<<16
		height := int(data[payload+7]) | int(data[payload+8])<<8 | int(data[payload+9])<<16
		return ImageDimensions{Width: width + 1, Height: height + 1}, nil
	default:
		return ImageDimensions{}, fmt.Errorf("unsupported webp chunk %q", chunk)
	}
}
