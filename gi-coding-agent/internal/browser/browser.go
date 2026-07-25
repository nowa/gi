package browser

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

type commandStarter func(name string, args []string) error

// Open starts the platform default handler for target without involving a
// shell. The child is reaped asynchronously so callers do not retain process
// state after the launcher has started.
func Open(target string) error {
	return open(runtime.GOOS, target, startCommand)
}

func open(
	platform string,
	target string,
	start commandStarter,
) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	if start == nil {
		return errors.New("browser command starter is required")
	}
	name, args := commandForPlatform(platform, target)
	if err := start(name, args); err != nil {
		return fmt.Errorf("start browser command: %w", err)
	}
	return nil
}

func commandForPlatform(
	platform string,
	target string,
) (string, []string) {
	switch platform {
	case "darwin":
		return "open", []string{target}
	case "windows", "win32":
		return "rundll32", []string{
			"url.dll,FileProtocolHandler",
			target,
		}
	default:
		return "xdg-open", []string{target}
	}
}

func startCommand(name string, args []string) error {
	command := exec.Command(name, args...)
	if err := command.Start(); err != nil {
		return err
	}
	go func() {
		_ = command.Wait()
	}()
	return nil
}
