package gicodingagent

import (
	"context"
	"strings"
	"testing"
)

func TestCheckTmuxKeyboardSetupPiStyle(t *testing.T) {
	t.Run("warns when extended keys are disabled", func(t *testing.T) {
		warning := CheckTmuxKeyboardSetup(context.Background(), func(_ context.Context, option string) (string, bool) {
			if option == "extended-keys" {
				return "off", true
			}
			return "", false
		})
		if !strings.Contains(warning, "tmux extended-keys is off") {
			t.Fatalf("warning = %q", warning)
		}
	})

	t.Run("warns when extended keys format is xterm", func(t *testing.T) {
		warning := CheckTmuxKeyboardSetup(context.Background(), func(_ context.Context, option string) (string, bool) {
			switch option {
			case "extended-keys":
				return "on", true
			case "extended-keys-format":
				return "xterm", true
			default:
				return "", false
			}
		})
		if !strings.Contains(warning, "tmux extended-keys-format is xterm") {
			t.Fatalf("warning = %q", warning)
		}
	})

	t.Run("does not warn when tmux cannot be queried", func(t *testing.T) {
		warning := CheckTmuxKeyboardSetup(context.Background(), func(context.Context, string) (string, bool) {
			return "", false
		})
		if warning != "" {
			t.Fatalf("warning = %q", warning)
		}
	})

	t.Run("accepts csi-u setup", func(t *testing.T) {
		warning := CheckTmuxKeyboardSetup(context.Background(), func(_ context.Context, option string) (string, bool) {
			switch option {
			case "extended-keys":
				return "always", true
			case "extended-keys-format":
				return "csi-u", true
			default:
				return "", false
			}
		})
		if warning != "" {
			t.Fatalf("warning = %q", warning)
		}
	})
}
