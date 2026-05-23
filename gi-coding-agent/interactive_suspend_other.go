//go:build !darwin && !linux && !freebsd && !netbsd && !openbsd

package gicodingagent

import "runtime"

func defaultInteractiveSuspendOperations() InteractiveSuspendOperations {
	return InteractiveSuspendOperations{Platform: runtime.GOOS}
}
