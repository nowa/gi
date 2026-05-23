//go:build !darwin && !linux && !freebsd && !netbsd && !openbsd

package gicodingagent

func isCLIInteractivePlatformDeadTerminalError(error) bool {
	return false
}
