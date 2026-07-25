package clipboard

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestReadClipboardTextPrefersNativeAdapter(t *testing.T) {
	commandCalls := 0
	text, ok := ReadClipboardText(
		context.Background(),
		ClipboardTextReadOptions{
			Platform: "darwin",
			Operations: ClipboardTextReadOperations{
				GetText: func(context.Context) (
					string,
					error,
				) {
					return "native clipboard", nil
				},
				RunCommand: func(
					context.Context,
					string,
					[]string,
					int,
				) (string, error) {
					commandCalls++
					return "", nil
				},
			},
		},
	)
	if !ok ||
		text != "native clipboard" ||
		commandCalls != 0 {
		t.Fatalf(
			"clipboard = %q, %v, command calls = %d",
			text,
			ok,
			commandCalls,
		)
	}
}

func TestReadClipboardTextFallsBackAcrossPlatformCommands(
	t *testing.T,
) {
	var calls []string
	text, ok := ReadClipboardText(
		context.Background(),
		ClipboardTextReadOptions{
			Platform: "linux",
			Env: map[string]string{
				"WAYLAND_DISPLAY": "wayland-1",
				"DISPLAY":         ":0",
			},
			Operations: ClipboardTextReadOperations{
				RunCommand: func(
					_ context.Context,
					name string,
					args []string,
					maxBytes int,
				) (string, error) {
					calls = append(
						calls,
						name+" "+strings.Join(args, " "),
					)
					if maxBytes != DefaultMaxTextBytes {
						t.Errorf(
							"max bytes = %d",
							maxBytes,
						)
					}
					if name == "xclip" {
						return "x11 clipboard", nil
					}
					return "", errors.New("unavailable")
				},
			},
		},
	)
	if !ok || text != "x11 clipboard" {
		t.Fatalf("clipboard = %q, %v", text, ok)
	}
	want := []string{
		"wl-paste --no-newline",
		"xclip -selection clipboard -out",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("commands = %#v, want %#v", calls, want)
	}
}

func TestReadClipboardTextUsesWSLPowerShellAndBoundsNativeText(
	t *testing.T,
) {
	var command string
	text, ok := ReadClipboardText(
		context.Background(),
		ClipboardTextReadOptions{
			Platform: "linux",
			Env: map[string]string{
				"WSL_DISTRO_NAME": "Ubuntu",
			},
			Operations: ClipboardTextReadOperations{
				GetText: func(context.Context) (
					string,
					error,
				) {
					t.Fatal(
						"headless Linux should not use native clipboard",
					)
					return "", nil
				},
				RunCommand: func(
					_ context.Context,
					name string,
					args []string,
					_ int,
				) (string, error) {
					command = name + " " +
						strings.Join(args, " ")
					return "wsl clipboard", nil
				},
			},
		},
	)
	if !ok ||
		text != "wsl clipboard" ||
		!strings.HasPrefix(command, "powershell.exe ") {
		t.Fatalf(
			"clipboard = %q, %v, command = %q",
			text,
			ok,
			command,
		)
	}

	text, ok = ReadClipboardText(
		context.Background(),
		ClipboardTextReadOptions{
			Platform: "darwin",
			MaxBytes: 3,
			Operations: ClipboardTextReadOperations{
				GetText: func(context.Context) (
					string,
					error,
				) {
					return "four", nil
				},
			},
		},
	)
	if ok || text != "" {
		t.Fatalf("oversized native clipboard = %q, %v", text, ok)
	}

	text, ok = ReadClipboardText(
		context.Background(),
		ClipboardTextReadOptions{
			Platform: "darwin",
			MaxBytes: 3,
			Operations: ClipboardTextReadOperations{
				RunCommand: func(
					context.Context,
					string,
					[]string,
					int,
				) (string, error) {
					return "four", nil
				},
			},
		},
	)
	if ok || text != "" {
		t.Fatalf("oversized command clipboard = %q, %v", text, ok)
	}
}

func TestClipboardTextReadCommandsAreArgumentSafe(t *testing.T) {
	commands := clipboardTextReadCommands(
		"windows",
		map[string]string{},
	)
	if len(commands) != 1 ||
		commands[0].name != "powershell.exe" ||
		!reflect.DeepEqual(
			commands[0].args,
			[]string{
				"-NoProfile",
				"-NonInteractive",
				"-Command",
				"Get-Clipboard -Raw",
			},
		) {
		t.Fatalf("Windows commands = %#v", commands)
	}
}

func TestRunClipboardTextReadCommandBoundsOutputAndCancellation(
	t *testing.T,
) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GI_CLIPBOARD_TEXT_HELPER", "large")
	_, err = runClipboardTextReadCommand(
		context.Background(),
		executable,
		[]string{
			"-test.run=^TestClipboardTextReadCommandHelper$",
		},
		16,
	)
	if !errors.Is(err, ErrClipboardTextTooLarge) {
		t.Fatalf("large output error = %v", err)
	}

	t.Setenv("GI_CLIPBOARD_TEXT_HELPER", "block")
	ctx, cancel := context.WithTimeout(
		context.Background(),
		100*time.Millisecond,
	)
	defer cancel()
	started := time.Now()
	_, err = runClipboardTextReadCommand(
		ctx,
		executable,
		[]string{
			"-test.run=^TestClipboardTextReadCommandHelper$",
		},
		16,
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancelled read error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("cancelled read took %v", elapsed)
	}
}

func TestClipboardTextReadCommandHelper(t *testing.T) {
	switch os.Getenv("GI_CLIPBOARD_TEXT_HELPER") {
	case "large":
		_, _ = fmt.Fprint(os.Stdout, strings.Repeat("x", 1024))
	case "block":
		time.Sleep(10 * time.Second)
	}
}
