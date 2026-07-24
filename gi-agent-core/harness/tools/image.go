package tools

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
)

var pngSignature = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}

func DetectSupportedImageMIMEType(content []byte) string {
	switch {
	case len(content) >= 4 && bytes.Equal(content[:3], []byte{0xff, 0xd8, 0xff}):
		if content[3] == 0xf7 {
			return ""
		}
		return "image/jpeg"
	case bytes.HasPrefix(content, pngSignature):
		if isPNG(content) && !isAnimatedPNG(content) {
			return "image/png"
		}
	case len(content) >= 3 && string(content[:3]) == "GIF":
		return "image/gif"
	case len(content) >= 12 && string(content[:4]) == "RIFF" && string(content[8:12]) == "WEBP":
		return "image/webp"
	case len(content) >= 2 && string(content[:2]) == "BM" && isBMP(content):
		return "image/bmp"
	}
	return ""
}

func EncodeBase64(content []byte) string {
	return base64.StdEncoding.EncodeToString(content)
}

func isPNG(content []byte) bool {
	return len(content) >= 16 &&
		binary.BigEndian.Uint32(content[8:12]) == 13 &&
		string(content[12:16]) == "IHDR"
}

func isAnimatedPNG(content []byte) bool {
	offset := len(pngSignature)
	for offset+8 <= len(content) {
		chunkLength := int(binary.BigEndian.Uint32(content[offset : offset+4]))
		chunkType := string(content[offset+4 : offset+8])
		switch chunkType {
		case "acTL":
			return true
		case "IDAT":
			return false
		}
		nextOffset := offset + 8 + chunkLength + 4
		if nextOffset <= offset || nextOffset > len(content) {
			return false
		}
		offset = nextOffset
	}
	return false
}

func isBMP(content []byte) bool {
	if len(content) < 26 {
		return false
	}
	declaredFileSize := binary.LittleEndian.Uint32(content[2:6])
	pixelDataOffset := binary.LittleEndian.Uint32(content[10:14])
	dibHeaderSize := binary.LittleEndian.Uint32(content[14:18])
	if declaredFileSize != 0 && declaredFileSize < 26 {
		return false
	}
	if pixelDataOffset < 14+dibHeaderSize {
		return false
	}
	if declaredFileSize != 0 && pixelDataOffset >= declaredFileSize {
		return false
	}

	var colorPlanes, bitsPerPixel uint16
	switch {
	case dibHeaderSize == 12:
		colorPlanes = binary.LittleEndian.Uint16(content[22:24])
		bitsPerPixel = binary.LittleEndian.Uint16(content[24:26])
	case dibHeaderSize >= 40 && dibHeaderSize <= 124:
		if len(content) < 30 {
			return false
		}
		colorPlanes = binary.LittleEndian.Uint16(content[26:28])
		bitsPerPixel = binary.LittleEndian.Uint16(content[28:30])
	default:
		return false
	}
	if colorPlanes != 1 {
		return false
	}
	switch bitsPerPixel {
	case 1, 4, 8, 16, 24, 32:
		return true
	default:
		return false
	}
}
