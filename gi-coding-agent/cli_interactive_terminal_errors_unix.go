//go:build darwin || linux || freebsd || netbsd || openbsd

package gicodingagent

import (
	"errors"
	"syscall"
)

func isCLIInteractivePlatformDeadTerminalError(err error) bool {
	return errors.Is(err, syscall.EIO) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ENOTCONN)
}
