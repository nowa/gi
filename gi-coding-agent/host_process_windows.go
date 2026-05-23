//go:build windows

package gicodingagent

import (
	"errors"
	"os"
	"os/exec"
)

func configureHostProcessCommand(cmd *exec.Cmd) {}

func terminateHostProcess(process *os.Process) error {
	return killHostProcess(process)
}

func killHostProcess(process *os.Process) error {
	if process == nil {
		return nil
	}
	if err := process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}
