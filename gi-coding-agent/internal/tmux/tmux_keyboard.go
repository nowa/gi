package tmux

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

type OptionReader func(context.Context, string) (string, bool)

func CheckKeyboardSetup(ctx context.Context, reader OptionReader) string {
	if reader == nil {
		reader = DefaultOptionReader
	}
	extendedKeys, ok := reader(ctx, "extended-keys")
	if !ok {
		return ""
	}
	extendedKeys = strings.TrimSpace(extendedKeys)
	if extendedKeys != "on" && extendedKeys != "always" {
		return "tmux extended-keys is off. Modified Enter keys may not work. Add `set -g extended-keys on` to ~/.tmux.conf and restart tmux."
	}
	extendedKeysFormat, ok := reader(ctx, "extended-keys-format")
	if ok && strings.TrimSpace(extendedKeysFormat) == "xterm" {
		return "tmux extended-keys-format is xterm. Gi works best with csi-u. Add `set -g extended-keys-format csi-u` to ~/.tmux.conf and restart tmux."
	}
	return ""
}

func DefaultOptionReader(ctx context.Context, option string) (string, bool) {
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(readCtx, "tmux", "show", "-gv", option).Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}
