//go:build windows

package gicodingagent

import (
	"errors"
	"os"
	"os/exec"
)

func configureLocalBashCommand(cmd *exec.Cmd) {}

func cancelLocalBashCommand(process *os.Process) error {
	if process == nil {
		return nil
	}
	if err := process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}
