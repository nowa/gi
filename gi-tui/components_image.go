package gitui

import (
	"fmt"
	"sync"
)

type ImageOptions struct {
	Alt            string
	MimeType       string
	Filename       string
	MaxWidth       int
	MaxHeight      int
	MaxWidthCells  int
	MaxHeightCells int
	ImageID        uint32
	ImageId        uint32
	Dimensions     *ImageDimensions
}

type ImageTheme struct {
	Fallback      func(string) string
	FallbackColor func(string) string
}

type Image struct {
	mu      sync.Mutex
	Data    []byte
	Options ImageOptions
	Theme   ImageTheme
	imageID uint32
}

func NewImage(data []byte, options ImageOptions, theme ...ImageTheme) *Image {
	img := &Image{Data: append([]byte(nil), data...), Options: options, imageID: imageOptionID(options)}
	if len(theme) > 0 {
		img.Theme = theme[0]
	}
	return img
}

func (i *Image) Invalidate() {}
func (i *Image) Render(width int) []string {
	i.mu.Lock()
	defer i.mu.Unlock()
	caps := GetCapabilities()
	if caps.Images && len(i.Data) > 0 {
		dims := i.imageDimensions()
		maxWidth := i.maxWidthCells(width)
		maxHeight := i.maxHeightCells(maxWidth)
		if caps.Protocol == ImageProtocolKitty {
			if i.imageID == 0 {
				i.imageID = AllocateImageID()
			}
		}
		moveCursor := false
		result := RenderImageWithDimensions(i.Data, dims, ImageRenderOptions{
			ID:             i.imageID,
			MaxWidthCells:  maxWidth,
			MaxHeightCells: maxHeight,
			Alt:            i.Options.Alt,
			Protocol:       caps.Protocol,
			MoveCursor:     &moveCursor,
		})
		if result != nil && IsImageLine(result.Sequence) {
			if result.ImageID > 0 {
				i.imageID = result.ImageID
			}
			lines := make([]string, max(1, result.Rows))
			if caps.Protocol == ImageProtocolITerm {
				rowOffset := max(0, result.Rows-1)
				moveUp := ""
				if rowOffset > 0 {
					moveUp = fmt.Sprintf("\x1b[%dA", rowOffset)
				}
				lines[len(lines)-1] = moveUp + result.Sequence
			} else {
				lines[0] = result.Sequence
			}
			return lines
		}
	}
	text := i.fallbackText()
	if i.Options.MimeType == "" && i.Options.Filename == "" && i.Options.Dimensions == nil {
		text = ImageFallback(text, width)
	}
	if i.Theme.Fallback != nil {
		text = i.Theme.Fallback(text)
	} else if i.Theme.FallbackColor != nil {
		text = i.Theme.FallbackColor(text)
	}
	return []string{text}
}

func (i *Image) ImageID() uint32 {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.imageID
}

func (i *Image) GetImageID() uint32 {
	return i.ImageID()
}

func (i *Image) GetImageId() uint32 {
	return i.ImageID()
}

func (i *Image) imageDimensions() ImageDimensions {
	if i.Options.Dimensions != nil {
		dims := normalizeImageDimensions(*i.Options.Dimensions)
		if dims.Width > 0 && dims.Height > 0 {
			return dims
		}
	}
	if dims, err := GetImageDimensions(i.Data); err == nil {
		return dims
	}
	return ImageDimensions{Width: 800, Height: 600}
}

func (i *Image) maxWidthCells(width int) int {
	maxWidth := i.Options.MaxWidthCells
	if maxWidth <= 0 {
		maxWidth = i.Options.MaxWidth
	}
	if maxWidth <= 0 {
		maxWidth = 60
	}
	return max(1, min(max(1, width-2), maxWidth))
}

func (i *Image) maxHeightCells(maxWidth int) int {
	maxHeight := i.Options.MaxHeightCells
	if maxHeight <= 0 {
		maxHeight = i.Options.MaxHeight
	}
	if maxHeight > 0 {
		return maxHeight
	}
	cell := GetCellDimensions()
	cell = normalizeCellDimensions(cell)
	if cell.Height <= 0 {
		return 1
	}
	return max(1, (maxWidth*cell.Width+cell.Height-1)/cell.Height)
}

func (i *Image) fallbackText() string {
	if i.Options.MimeType != "" || i.Options.Filename != "" || i.Options.Dimensions != nil {
		return ImageFallbackDescription(i.Options.MimeType, i.Options.Dimensions, i.Options.Filename)
	}
	if i.Options.Alt != "" {
		return i.Options.Alt
	}
	return fmt.Sprintf("[image %d bytes]", len(i.Data))
}

func imageOptionID(options ImageOptions) uint32 {
	if options.ImageID > 0 {
		return options.ImageID
	}
	return options.ImageId
}
