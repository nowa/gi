//go:build !windows

package gicodingagent

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureLocalBashCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func cancelLocalBashCommand(process *os.Process) error {
	if process == nil {
		return nil
	}
	pgid, err := syscall.Getpgid(process.Pid)
	if err == nil && pgid > 0 {
		if killErr := syscall.Kill(-pgid, syscall.SIGKILL); killErr == nil || errors.Is(killErr, syscall.ESRCH) {
			return nil
		}
	}
	if err := process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}
