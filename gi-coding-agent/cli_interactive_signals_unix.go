//go:build darwin || linux || freebsd || netbsd || openbsd

package gicodingagent

import (
	"os"
	"os/signal"
	"syscall"
)

func subscribeCLIInteractiveShutdownSignals() (<-chan os.Signal, func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGHUP)
	return ch, func() {
		signal.Stop(ch)
	}
}
