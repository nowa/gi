package gicodingagent

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

type clipboardCommandCall struct {
	command string
	args    []string
	options ClipboardImageCommandOptions
}

func TestReadClipboardImageWaylandUsesWlPaste(t *testing.T) {
	var calls []clipboardCommandCall
	ops := ClipboardImageOperations{
		RunCommand: func(command string, args []string, options ClipboardImageCommandOptions) ClipboardImageCommandResult {
			calls = append(calls, clipboardCommandCall{command: command, args: append([]string(nil), args...), options: options})
			if command == "wl-paste" && len(args) > 0 && args[0] == "--list-types" {
				return ClipboardImageCommandResult{Stdout: []byte("text/plain\nimage/png\n"), OK: true}
			}
			if command == "wl-paste" && len(args) > 0 && args[0] == "--type" {
				return ClipboardImageCommandResult{Stdout: []byte{1, 2, 3}, OK: true}
			}
			t.Fatalf("unexpected command: %s %s", command, strings.Join(args, " "))
			return ClipboardImageCommandResult{}
		},
		NativeHasImage: func() bool {
			t.Fatal("native clipboard should not be called on Wayland")
			return false
		},
		ProcVersion: func() (string, error) { return "", errors.New("no proc version") },
	}

	result := ReadClipboardImage(ClipboardImageOptions{
		Platform:   "linux",
		Env:        map[string]string{"WAYLAND_DISPLAY": "1"},
		Operations: ops,
	})
	if result == nil {
		t.Fatal("ReadClipboardImage returned nil")
	}
	if result.MIMEType != "image/png" || !reflect.DeepEqual(result.Bytes, []byte{1, 2, 3}) {
		t.Fatalf("image = %#v", result)
	}
	if len(calls) != 2 || calls[0].command != "wl-paste" || calls[1].command != "wl-paste" {
		t.Fatalf("wl-paste calls = %#v", calls)
	}
}

func TestReadClipboardImageWaylandFallsBackToXclip(t *testing.T) {
	ops := ClipboardImageOperations{
		RunCommand: func(command string, args []string, _ ClipboardImageCommandOptions) ClipboardImageCommandResult {
			if command == "wl-paste" {
				return ClipboardImageCommandResult{}
			}
			if command == "xclip" && clipboardArgsContain(args, "TARGETS") {
				return ClipboardImageCommandResult{Stdout: []byte("image/png\n"), OK: true}
			}
			if command == "xclip" && clipboardArgsContain(args, "image/png") {
				return ClipboardImageCommandResult{Stdout: []byte{9, 8}, OK: true}
			}
			return ClipboardImageCommandResult{OK: true}
		},
		NativeHasImage: func() bool {
			t.Fatal("native clipboard should not be called on Wayland")
			return false
		},
		ProcVersion: func() (string, error) { return "", errors.New("no proc version") },
	}

	result := ReadClipboardImage(ClipboardImageOptions{
		Platform:   "linux",
		Env:        map[string]string{"XDG_SESSION_TYPE": "wayland"},
		Operations: ops,
	})
	if result == nil {
		t.Fatal("ReadClipboardImage returned nil")
	}
	if result.MIMEType != "image/png" || !reflect.DeepEqual(result.Bytes, []byte{9, 8}) {
		t.Fatalf("image = %#v", result)
	}
}

func TestReadClipboardImageWSLPassesPowerShellPathDirectly(t *testing.T) {
	tempPath := "/tmp/gi-wsl-clip.png"
	files := map[string][]byte{}
	ops := ClipboardImageOperations{
		RunCommand: func(command string, args []string, options ClipboardImageCommandOptions) ClipboardImageCommandResult {
			if command == "wl-paste" || command == "xclip" {
				return ClipboardImageCommandResult{OK: true}
			}
			if command == "wslpath" {
				if len(args) != 2 || args[0] != "-w" || args[1] != tempPath {
					t.Fatalf("wslpath args = %#v", args)
				}
				return ClipboardImageCommandResult{Stdout: []byte("C:\\Users\\O'Hare\\clip.png\n"), OK: true}
			}
			if command == "powershell.exe" {
				if options.Env != nil {
					if _, ok := options.Env["PI_WSL_CLIPBOARD_IMAGE_PATH"]; ok {
						t.Fatal("PowerShell command should not receive PI_WSL_CLIPBOARD_IMAGE_PATH")
					}
				}
				if len(args) != 3 || !strings.Contains(args[2], "$path = 'C:\\Users\\O''Hare\\clip.png'") {
					t.Fatalf("powershell args = %#v", args)
				}
				files[tempPath] = []byte{4, 5, 6}
				return ClipboardImageCommandResult{Stdout: []byte("ok\n"), OK: true}
			}
			t.Fatalf("unexpected command: %s %s", command, strings.Join(args, " "))
			return ClipboardImageCommandResult{}
		},
		NativeHasImage: func() bool {
			t.Fatal("native clipboard should not be called before PowerShell on WSL")
			return false
		},
		TempFilePath: func() (string, error) { return tempPath, nil },
		ReadFile: func(path string) ([]byte, error) {
			data, ok := files[path]
			if !ok {
				return nil, errors.New("missing file")
			}
			return data, nil
		},
		RemoveFile: func(path string) error {
			delete(files, path)
			return nil
		},
	}

	result := ReadClipboardImage(ClipboardImageOptions{
		Platform:   "linux",
		Env:        map[string]string{"WSL_DISTRO_NAME": "Ubuntu"},
		Operations: ops,
	})
	if result == nil {
		t.Fatal("ReadClipboardImage returned nil")
	}
	if result.MIMEType != "image/png" || !reflect.DeepEqual(result.Bytes, []byte{4, 5, 6}) {
		t.Fatalf("image = %#v", result)
	}
}

func TestReadClipboardImageNonWaylandUsesNativeClipboard(t *testing.T) {
	ops := ClipboardImageOperations{
		RunCommand: func(command string, args []string, _ ClipboardImageCommandOptions) ClipboardImageCommandResult {
			t.Fatalf("command should not be called for non-Wayland session: %s %s", command, strings.Join(args, " "))
			return ClipboardImageCommandResult{}
		},
		NativeHasImage:       func() bool { return true },
		NativeGetImageBinary: func() ([]byte, error) { return []byte{7}, nil },
		ProcVersion:          func() (string, error) { return "", errors.New("no proc version") },
	}

	result := ReadClipboardImage(ClipboardImageOptions{
		Platform:   "linux",
		Env:        map[string]string{},
		Operations: ops,
	})
	if result == nil {
		t.Fatal("ReadClipboardImage returned nil")
	}
	if result.MIMEType != "image/png" || !reflect.DeepEqual(result.Bytes, []byte{7}) {
		t.Fatalf("image = %#v", result)
	}
}

func TestReadClipboardImageNonWaylandReturnsNilWithoutNativeImage(t *testing.T) {
	ops := ClipboardImageOperations{
		RunCommand: func(command string, args []string, _ ClipboardImageCommandOptions) ClipboardImageCommandResult {
			t.Fatalf("command should not be called for non-Wayland session: %s %s", command, strings.Join(args, " "))
			return ClipboardImageCommandResult{}
		},
		NativeHasImage: func() bool { return false },
		ProcVersion:    func() (string, error) { return "", errors.New("no proc version") },
	}

	result := ReadClipboardImage(ClipboardImageOptions{
		Platform:   "linux",
		Env:        map[string]string{},
		Operations: ops,
	})
	if result != nil {
		t.Fatalf("ReadClipboardImage returned %#v, want nil", result)
	}
}

func TestReadClipboardImageConvertsBMPToPNGOnWayland(t *testing.T) {
	ops := ClipboardImageOperations{
		RunCommand: func(command string, args []string, _ ClipboardImageCommandOptions) ClipboardImageCommandResult {
			if command == "wl-paste" && clipboardArgsContain(args, "--list-types") {
				return ClipboardImageCommandResult{Stdout: []byte("image/bmp\n"), OK: true}
			}
			if command == "wl-paste" && clipboardArgsContain(args, "image/bmp") {
				return ClipboardImageCommandResult{Stdout: createTinyBMP1x1Red24BPP(), OK: true}
			}
			return ClipboardImageCommandResult{}
		},
		ProcVersion: func() (string, error) { return "", errors.New("no proc version") },
	}

	result := ReadClipboardImage(ClipboardImageOptions{
		Platform:   "linux",
		Env:        map[string]string{"WAYLAND_DISPLAY": "wayland-0"},
		Operations: ops,
	})
	if result == nil {
		t.Fatal("ReadClipboardImage returned nil")
	}
	if result.MIMEType != "image/png" {
		t.Fatalf("MIMEType = %q, want image/png", result.MIMEType)
	}
	if len(result.Bytes) < 4 || result.Bytes[0] != 0x89 || result.Bytes[1] != 0x50 || result.Bytes[2] != 0x4e || result.Bytes[3] != 0x47 {
		t.Fatalf("PNG magic = %#v", result.Bytes[:minInt(len(result.Bytes), 4)])
	}
}

func createTinyBMP1x1Red24BPP() []byte {
	buffer := make([]byte, 58)
	buffer[0] = 'B'
	buffer[1] = 'M'
	writeUint32LE(buffer[2:], uint32(len(buffer)))
	writeUint32LE(buffer[10:], 54)
	writeUint32LE(buffer[14:], 40)
	writeUint32LE(buffer[18:], 1)
	writeUint32LE(buffer[22:], 1)
	writeUint16LE(buffer[26:], 1)
	writeUint16LE(buffer[28:], 24)
	writeUint32LE(buffer[34:], 4)
	buffer[54] = 0x00
	buffer[55] = 0x00
	buffer[56] = 0xff
	buffer[57] = 0x00
	return buffer
}

func writeUint16LE(buffer []byte, value uint16) {
	buffer[0] = byte(value)
	buffer[1] = byte(value >> 8)
}

func writeUint32LE(buffer []byte, value uint32) {
	buffer[0] = byte(value)
	buffer[1] = byte(value >> 8)
	buffer[2] = byte(value >> 16)
	buffer[3] = byte(value >> 24)
}

func clipboardArgsContain(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
