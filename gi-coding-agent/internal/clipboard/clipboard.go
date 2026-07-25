package clipboard

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	DefaultCopyTimeout    = 5 * time.Second
	DefaultReadTimeout    = 3 * time.Second
	DefaultMaxTextBytes   = 16 * 1024 * 1024
	maxOSC52EncodedLength = 100000
)

var ErrClipboardTextTooLarge = errors.New(
	"clipboard text exceeds the configured size limit",
)

type ClipboardCopyOptions struct {
	Env        map[string]string
	Platform   string
	Operations ClipboardCopyOperations
}

type ClipboardTextReadOptions struct {
	Env        map[string]string
	Platform   string
	Timeout    time.Duration
	MaxBytes   int
	Operations ClipboardTextReadOperations
}

type ClipboardTextCommandOptions struct {
	Input   string
	Timeout time.Duration
}

type ClipboardTextReadOperations struct {
	GetText    func(context.Context) (string, error)
	RunCommand func(
		context.Context,
		string,
		[]string,
		int,
	) (string, error)
}

type ClipboardCopyOperations struct {
	SetText        func(text string) error
	ExecCommand    func(command string, options ClipboardTextCommandOptions) error
	SpawnWithInput func(command string, args []string, input string) error
	WriteStdout    func(text string) error
	IsWayland      func(env map[string]string) bool
}

type clipboardTextReadCommand struct {
	name string
	args []string
}

// ReadClipboardText returns the first authoritative clipboard result from the
// native adapter or platform tools. Clipboard access is best-effort: command,
// permission, timeout, and size-limit failures are represented by ok=false.
func ReadClipboardText(
	ctx context.Context,
	options ClipboardTextReadOptions,
) (text string, ok bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	env := options.Env
	if env == nil {
		env = currentEnvMap()
	}
	platform := options.Platform
	if platform == "" {
		platform = runtime.GOOS
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = DefaultReadTimeout
	}
	maxBytes := options.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxTextBytes
	}
	readCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ops := withDefaultClipboardTextReadOperations(
		options.Operations,
	)
	hasDisplay := platform != "linux" ||
		env["DISPLAY"] != "" ||
		env["WAYLAND_DISPLAY"] != ""
	if ops.GetText != nil &&
		env["TERMUX_VERSION"] == "" &&
		hasDisplay {
		text, err := ops.GetText(readCtx)
		if err == nil {
			if len(text) > maxBytes || text == "" {
				return "", false
			}
			return text, true
		}
	}

	for _, command := range clipboardTextReadCommands(
		platform,
		env,
	) {
		text, err := ops.RunCommand(
			readCtx,
			command.name,
			command.args,
			maxBytes,
		)
		if err == nil {
			if len(text) > maxBytes || text == "" {
				return "", false
			}
			return text, true
		}
		if readCtx.Err() != nil {
			return "", false
		}
	}
	return "", false
}

func CopyToClipboard(text string, options ClipboardCopyOptions) error {
	env := options.Env
	if env == nil {
		env = currentEnvMap()
	}
	platform := options.Platform
	if platform == "" {
		platform = runtime.GOOS
	}
	ops := withDefaultClipboardCopyOperations(options.Operations)

	copied := false
	if ops.SetText != nil && platform != "linux" {
		if err := ops.SetText(text); err == nil {
			copied = true
		}
	}

	remote := isRemoteClipboardSession(env)
	if copied && !remote {
		return nil
	}

	commandOptions := ClipboardTextCommandOptions{Input: text, Timeout: DefaultCopyTimeout}
	if !copied {
		switch platform {
		case "darwin":
			if err := ops.ExecCommand("pbcopy", commandOptions); err == nil {
				copied = true
			}
		case "win32", "windows":
			if err := ops.ExecCommand("clip", commandOptions); err == nil {
				copied = true
			}
		default:
			if env["TERMUX_VERSION"] != "" {
				if err := ops.ExecCommand("termux-clipboard-set", commandOptions); err == nil {
					copied = true
				}
			}
			if !copied {
				hasWaylandDisplay := env["WAYLAND_DISPLAY"] != ""
				hasX11Display := env["DISPLAY"] != ""
				isWayland := ops.IsWayland(env)
				if isWayland && hasWaylandDisplay {
					if err := ops.ExecCommand("which wl-copy", ClipboardTextCommandOptions{Timeout: DefaultCopyTimeout}); err == nil {
						if err := ops.SpawnWithInput("wl-copy", nil, text); err == nil {
							copied = true
						}
					}
					if !copied && hasX11Display {
						copied = copyToX11Clipboard(ops, commandOptions)
					}
				} else if hasX11Display {
					copied = copyToX11Clipboard(ops, commandOptions)
				}
			}
		}
	}

	if remote || !copied {
		if emitOSC52Clipboard(text, ops) {
			copied = true
		}
	}
	if !copied {
		return errors.New("Failed to copy to clipboard")
	}
	return nil
}

func withDefaultClipboardTextReadOperations(
	ops ClipboardTextReadOperations,
) ClipboardTextReadOperations {
	if ops.RunCommand == nil {
		ops.RunCommand = runClipboardTextReadCommand
	}
	return ops
}

func clipboardTextReadCommands(
	platform string,
	env map[string]string,
) []clipboardTextReadCommand {
	switch platform {
	case "darwin":
		return []clipboardTextReadCommand{{
			name: "pbpaste",
		}}
	case "windows", "win32":
		return []clipboardTextReadCommand{{
			name: "powershell.exe",
			args: []string{
				"-NoProfile",
				"-NonInteractive",
				"-Command",
				"Get-Clipboard -Raw",
			},
		}}
	}

	if env["TERMUX_VERSION"] != "" {
		return []clipboardTextReadCommand{{
			name: "termux-clipboard-get",
		}}
	}
	var commands []clipboardTextReadCommand
	if env["WSL_DISTRO_NAME"] != "" ||
		env["WSL_INTEROP"] != "" {
		commands = append(commands, clipboardTextReadCommand{
			name: "powershell.exe",
			args: []string{
				"-NoProfile",
				"-NonInteractive",
				"-Command",
				"Get-Clipboard -Raw",
			},
		})
	}
	if env["WAYLAND_DISPLAY"] != "" {
		commands = append(commands, clipboardTextReadCommand{
			name: "wl-paste",
			args: []string{"--no-newline"},
		})
	}
	if env["DISPLAY"] != "" {
		commands = append(
			commands,
			clipboardTextReadCommand{
				name: "xclip",
				args: []string{
					"-selection",
					"clipboard",
					"-out",
				},
			},
			clipboardTextReadCommand{
				name: "xsel",
				args: []string{
					"--clipboard",
					"--output",
				},
			},
		)
	}
	return commands
}

func runClipboardTextReadCommand(
	ctx context.Context,
	name string,
	args []string,
	maxBytes int,
) (string, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxTextBytes
	}
	command := exec.CommandContext(ctx, name, args...)
	command.Stderr = io.Discard
	stdout, err := command.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := command.Start(); err != nil {
		return "", err
	}
	type readResult struct {
		data []byte
		err  error
	}
	read := make(chan readResult, 1)
	go func() {
		data, err := io.ReadAll(
			io.LimitReader(stdout, int64(maxBytes)+1),
		)
		read <- readResult{data: data, err: err}
	}()

	var result readResult
	select {
	case result = <-read:
	case <-ctx.Done():
		_ = stdout.Close()
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		_ = command.Wait()
		return "", ctx.Err()
	}
	if result.err != nil {
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		_ = command.Wait()
		return "", result.err
	}
	if len(result.data) > maxBytes {
		_ = stdout.Close()
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		_ = command.Wait()
		return "", ErrClipboardTextTooLarge
	}
	if err := command.Wait(); err != nil {
		return "", err
	}
	return string(result.data), nil
}

func withDefaultClipboardCopyOperations(ops ClipboardCopyOperations) ClipboardCopyOperations {
	if ops.ExecCommand == nil {
		ops.ExecCommand = runClipboardTextCommand
	}
	if ops.SpawnWithInput == nil {
		ops.SpawnWithInput = spawnClipboardTextCommand
	}
	if ops.WriteStdout == nil {
		ops.WriteStdout = func(text string) error {
			_, err := io.WriteString(os.Stdout, text)
			return err
		}
	}
	if ops.IsWayland == nil {
		ops.IsWayland = IsWaylandSession
	}
	return ops
}

func copyToX11Clipboard(ops ClipboardCopyOperations, options ClipboardTextCommandOptions) bool {
	if err := ops.ExecCommand("xclip -selection clipboard", options); err == nil {
		return true
	}
	return ops.ExecCommand("xsel --clipboard --input", options) == nil
}

func isRemoteClipboardSession(env map[string]string) bool {
	return env["SSH_CONNECTION"] != "" || env["SSH_CLIENT"] != "" || env["MOSH_CONNECTION"] != ""
}

func emitOSC52Clipboard(text string, ops ClipboardCopyOperations) bool {
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	if len(encoded) > maxOSC52EncodedLength {
		return false
	}
	return ops.WriteStdout("\x1b]52;c;"+encoded+"\x07") == nil
}

func runClipboardTextCommand(command string, options ClipboardTextCommandOptions) error {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = DefaultCopyTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", command)
	}
	if options.Input != "" {
		cmd.Stdin = strings.NewReader(options.Input)
	}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func spawnClipboardTextCommand(command string, args []string, input string) error {
	cmd := exec.Command(command, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if _, err := io.WriteString(stdin, input); err != nil {
		_ = stdin.Close()
		return err
	}
	if err := stdin.Close(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
