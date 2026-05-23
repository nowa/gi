package gicodingagent

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

type TmuxOptionReader func(context.Context, string) (string, bool)

func CheckTmuxKeyboardSetup(ctx context.Context, reader TmuxOptionReader) string {
	if reader == nil {
		reader = DefaultTmuxOptionReader
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

func DefaultTmuxOptionReader(ctx context.Context, option string) (string, bool) {
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(readCtx, "tmux", "show", "-gv", option).Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}
