package gicodingagent

import (
	"errors"
	"io"
	"os"
)

func isCLIInteractiveDeadTerminalError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, os.ErrClosed) ||
		isCLIInteractivePlatformDeadTerminalError(err)
}
