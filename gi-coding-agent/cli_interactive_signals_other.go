//go:build !darwin && !linux && !freebsd && !netbsd && !openbsd

package gicodingagent

import "os"

func subscribeCLIInteractiveShutdownSignals() (<-chan os.Signal, func()) {
	return nil, func() {}
}
