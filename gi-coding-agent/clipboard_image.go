package gicodingagent

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	defaultClipboardListTimeout       = time.Second
	defaultClipboardReadTimeout       = 3 * time.Second
	defaultClipboardPowerShellTimeout = 5 * time.Second
	defaultClipboardMaxBufferBytes    = 50 * 1024 * 1024
)

var supportedClipboardImageMIMETypes = []string{"image/png", "image/jpeg", "image/webp", "image/gif"}

type ClipboardImage struct {
	Bytes    []byte
	MIMEType string
}

type ClipboardImageOptions struct {
	Env        map[string]string
	Platform   string
	Operations ClipboardImageOperations
}

type ClipboardImageCommandOptions struct {
	Timeout        time.Duration
	MaxBufferBytes int
	Env            map[string]string
}

type ClipboardImageCommandResult struct {
	Stdout []byte
	OK     bool
}

type ClipboardImageOperations struct {
	RunCommand           func(command string, args []string, options ClipboardImageCommandOptions) ClipboardImageCommandResult
	NativeHasImage       func() bool
	NativeGetImageBinary func() ([]byte, error)
	TempFilePath         func() (string, error)
	ReadFile             func(path string) ([]byte, error)
	RemoveFile           func(path string) error
	ProcVersion          func() (string, error)
}

func ReadClipboardImage(options ClipboardImageOptions) *ClipboardImage {
	env := options.Env
	if env == nil {
		env = currentEnvMap()
	}
	platform := options.Platform
	if platform == "" {
		platform = runtime.GOOS
	}
	ops := withDefaultClipboardImageOperations(options.Operations)

	if env["TERMUX_VERSION"] != "" {
		return nil
	}

	var imageData *ClipboardImage
	if platform == "linux" {
		wsl := isWSLClipboardSession(env, ops)
		wayland := IsWaylandSession(env)
		if wayland || wsl {
			imageData = readClipboardImageViaWlPaste(ops)
			if imageData == nil {
				imageData = readClipboardImageViaXclip(ops)
			}
		}
		if imageData == nil && wsl {
			imageData = readClipboardImageViaPowerShell(ops)
		}
		if imageData == nil && !wayland {
			imageData = readClipboardImageViaNativeClipboard(ops)
		}
	} else {
		imageData = readClipboardImageViaNativeClipboard(ops)
	}

	if imageData == nil {
		return nil
	}
	if isSupportedClipboardImageMIMEType(imageData.MIMEType) {
		return imageData
	}
	pngBytes, ok := convertClipboardImageToPNG(imageData.Bytes, imageData.MIMEType)
	if !ok {
		return nil
	}
	return &ClipboardImage{Bytes: pngBytes, MIMEType: "image/png"}
}

func IsWaylandSession(env map[string]string) bool {
	return env["WAYLAND_DISPLAY"] != "" || env["XDG_SESSION_TYPE"] == "wayland"
}

func ExtensionForImageMIMEType(mimeType string) string {
	switch baseImageMIMEType(mimeType) {
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpg"
	case "image/webp":
		return "webp"
	case "image/gif":
		return "gif"
	default:
		return ""
	}
}

func withDefaultClipboardImageOperations(ops ClipboardImageOperations) ClipboardImageOperations {
	if ops.RunCommand == nil {
		ops.RunCommand = runClipboardImageCommand
	}
	if ops.TempFilePath == nil {
		ops.TempFilePath = func() (string, error) {
			file, err := os.CreateTemp("", "gi-wsl-clip-*.png")
			if err != nil {
				return "", err
			}
			path := file.Name()
			_ = file.Close()
			return path, nil
		}
	}
	if ops.ReadFile == nil {
		ops.ReadFile = os.ReadFile
	}
	if ops.RemoveFile == nil {
		ops.RemoveFile = os.Remove
	}
	if ops.ProcVersion == nil {
		ops.ProcVersion = func() (string, error) {
			data, err := os.ReadFile("/proc/version")
			return string(data), err
		}
	}
	return ops
}

func runClipboardImageCommand(command string, args []string, options ClipboardImageCommandOptions) ClipboardImageCommandResult {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultClipboardReadTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	if options.Env != nil {
		cmd.Env = envMapToList(options.Env)
	}
	stdout, err := cmd.Output()
	if err != nil {
		return ClipboardImageCommandResult{}
	}
	if options.MaxBufferBytes > 0 && len(stdout) > options.MaxBufferBytes {
		return ClipboardImageCommandResult{}
	}
	return ClipboardImageCommandResult{Stdout: stdout, OK: true}
}

func readClipboardImageViaWlPaste(ops ClipboardImageOperations) *ClipboardImage {
	list := ops.RunCommand("wl-paste", []string{"--list-types"}, ClipboardImageCommandOptions{
		Timeout:        defaultClipboardListTimeout,
		MaxBufferBytes: defaultClipboardMaxBufferBytes,
	})
	if !list.OK {
		return nil
	}
	selectedType := selectPreferredClipboardImageMIMEType(splitClipboardMIMETypes(string(list.Stdout)))
	if selectedType == "" {
		return nil
	}
	data := ops.RunCommand("wl-paste", []string{"--type", selectedType, "--no-newline"}, ClipboardImageCommandOptions{
		Timeout:        defaultClipboardReadTimeout,
		MaxBufferBytes: defaultClipboardMaxBufferBytes,
	})
	if !data.OK || len(data.Stdout) == 0 {
		return nil
	}
	return &ClipboardImage{Bytes: append([]byte(nil), data.Stdout...), MIMEType: baseImageMIMEType(selectedType)}
}

func readClipboardImageViaXclip(ops ClipboardImageOperations) *ClipboardImage {
	targets := ops.RunCommand("xclip", []string{"-selection", "clipboard", "-t", "TARGETS", "-o"}, ClipboardImageCommandOptions{
		Timeout:        defaultClipboardListTimeout,
		MaxBufferBytes: defaultClipboardMaxBufferBytes,
	})
	var candidateTypes []string
	if targets.OK {
		candidateTypes = splitClipboardMIMETypes(string(targets.Stdout))
	}

	var tryTypes []string
	if preferred := selectPreferredClipboardImageMIMEType(candidateTypes); preferred != "" {
		tryTypes = append(tryTypes, preferred)
	}
	tryTypes = append(tryTypes, supportedClipboardImageMIMETypes...)

	seen := map[string]bool{}
	for _, mimeType := range tryTypes {
		if seen[mimeType] {
			continue
		}
		seen[mimeType] = true
		data := ops.RunCommand("xclip", []string{"-selection", "clipboard", "-t", mimeType, "-o"}, ClipboardImageCommandOptions{
			Timeout:        defaultClipboardReadTimeout,
			MaxBufferBytes: defaultClipboardMaxBufferBytes,
		})
		if data.OK && len(data.Stdout) > 0 {
			return &ClipboardImage{Bytes: append([]byte(nil), data.Stdout...), MIMEType: baseImageMIMEType(mimeType)}
		}
	}
	return nil
}

func readClipboardImageViaPowerShell(ops ClipboardImageOperations) *ClipboardImage {
	tmpFile, err := ops.TempFilePath()
	if err != nil || tmpFile == "" {
		return nil
	}
	defer func() { _ = ops.RemoveFile(tmpFile) }()

	winPathResult := ops.RunCommand("wslpath", []string{"-w", tmpFile}, ClipboardImageCommandOptions{
		Timeout:        defaultClipboardListTimeout,
		MaxBufferBytes: defaultClipboardMaxBufferBytes,
	})
	if !winPathResult.OK {
		return nil
	}
	winPath := strings.TrimSpace(string(winPathResult.Stdout))
	if winPath == "" {
		return nil
	}

	psQuotedWinPath := strings.ReplaceAll(winPath, "'", "''")
	psScript := strings.Join([]string{
		"Add-Type -AssemblyName System.Windows.Forms",
		"Add-Type -AssemblyName System.Drawing",
		"$path = '" + psQuotedWinPath + "'",
		"$img = [System.Windows.Forms.Clipboard]::GetImage()",
		"if ($img) { $img.Save($path, [System.Drawing.Imaging.ImageFormat]::Png); Write-Output 'ok' } else { Write-Output 'empty' }",
	}, "; ")

	result := ops.RunCommand("powershell.exe", []string{"-NoProfile", "-Command", psScript}, ClipboardImageCommandOptions{
		Timeout:        defaultClipboardPowerShellTimeout,
		MaxBufferBytes: defaultClipboardMaxBufferBytes,
	})
	if !result.OK || strings.TrimSpace(string(result.Stdout)) != "ok" {
		return nil
	}

	bytes, err := ops.ReadFile(tmpFile)
	if err != nil || len(bytes) == 0 {
		return nil
	}
	return &ClipboardImage{Bytes: append([]byte(nil), bytes...), MIMEType: "image/png"}
}

func readClipboardImageViaNativeClipboard(ops ClipboardImageOperations) *ClipboardImage {
	if ops.NativeHasImage == nil || ops.NativeGetImageBinary == nil || !ops.NativeHasImage() {
		return nil
	}
	bytes, err := ops.NativeGetImageBinary()
	if err != nil || len(bytes) == 0 {
		return nil
	}
	return &ClipboardImage{Bytes: append([]byte(nil), bytes...), MIMEType: "image/png"}
}

func isWSLClipboardSession(env map[string]string, ops ClipboardImageOperations) bool {
	if env["WSL_DISTRO_NAME"] != "" || env["WSLENV"] != "" {
		return true
	}
	release, err := ops.ProcVersion()
	if err != nil {
		return false
	}
	release = strings.ToLower(release)
	return strings.Contains(release, "microsoft") || strings.Contains(release, "wsl")
}

func selectPreferredClipboardImageMIMEType(mimeTypes []string) string {
	type normalizedMIME struct {
		raw  string
		base string
	}
	normalized := make([]normalizedMIME, 0, len(mimeTypes))
	for _, mimeType := range mimeTypes {
		raw := strings.TrimSpace(mimeType)
		if raw == "" {
			continue
		}
		normalized = append(normalized, normalizedMIME{raw: raw, base: baseImageMIMEType(raw)})
	}
	for _, preferred := range supportedClipboardImageMIMETypes {
		for _, candidate := range normalized {
			if candidate.base == preferred {
				return candidate.raw
			}
		}
	}
	for _, candidate := range normalized {
		if strings.HasPrefix(candidate.base, "image/") {
			return candidate.raw
		}
	}
	return ""
}

func splitClipboardMIMETypes(value string) []string {
	lines := strings.FieldsFunc(value, func(r rune) bool {
		return r == '\n' || r == '\r'
	})
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func isSupportedClipboardImageMIMEType(mimeType string) bool {
	return ExtensionForImageMIMEType(mimeType) != ""
}

func baseImageMIMEType(mimeType string) string {
	if index := strings.Index(mimeType, ";"); index >= 0 {
		mimeType = mimeType[:index]
	}
	return strings.ToLower(strings.TrimSpace(mimeType))
}

func convertClipboardImageToPNG(data []byte, mimeType string) ([]byte, bool) {
	if baseImageMIMEType(mimeType) != "image/bmp" {
		return nil, false
	}
	img, ok := decodeClipboardBMP(data)
	if !ok {
		return nil, false
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		return nil, false
	}
	return buffer.Bytes(), true
}

func decodeClipboardBMP(data []byte) (image.Image, bool) {
	if len(data) < 54 || string(data[:2]) != "BM" {
		return nil, false
	}
	pixelOffset := int(binary.LittleEndian.Uint32(data[10:14]))
	dibSize := int(binary.LittleEndian.Uint32(data[14:18]))
	if dibSize < 40 || pixelOffset < 14+dibSize || pixelOffset > len(data) {
		return nil, false
	}
	width := int(int32(binary.LittleEndian.Uint32(data[18:22])))
	heightRaw := int32(binary.LittleEndian.Uint32(data[22:26]))
	if width <= 0 || heightRaw == 0 || heightRaw == -2147483648 {
		return nil, false
	}
	topDown := heightRaw < 0
	height := int(heightRaw)
	if topDown {
		height = -height
	}
	planes := binary.LittleEndian.Uint16(data[26:28])
	bitsPerPixel := int(binary.LittleEndian.Uint16(data[28:30]))
	compression := binary.LittleEndian.Uint32(data[30:34])
	if planes != 1 || compression != 0 || (bitsPerPixel != 24 && bitsPerPixel != 32) {
		return nil, false
	}
	rowStride := ((width*bitsPerPixel + 31) / 32) * 4
	required := pixelOffset + rowStride*height
	if rowStride <= 0 || required < pixelOffset || required > len(data) {
		return nil, false
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	bytesPerPixel := bitsPerPixel / 8
	for y := 0; y < height; y++ {
		sourceY := y
		if !topDown {
			sourceY = height - 1 - y
		}
		rowOffset := pixelOffset + sourceY*rowStride
		for x := 0; x < width; x++ {
			offset := rowOffset + x*bytesPerPixel
			alpha := uint8(0xff)
			if bitsPerPixel == 32 {
				alpha = data[offset+3]
			}
			img.SetRGBA(x, y, color.RGBA{
				R: data[offset+2],
				G: data[offset+1],
				B: data[offset],
				A: alpha,
			})
		}
	}
	return img, true
}

func currentEnvMap() map[string]string {
	result := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			result[key] = value
		}
	}
	return result
}

func envMapToList(env map[string]string) []string {
	result := make([]string, 0, len(env))
	for key, value := range env {
		result = append(result, key+"="+value)
	}
	return result
}
