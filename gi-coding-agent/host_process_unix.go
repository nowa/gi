//go:build !windows

package gicodingagent

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureHostProcessCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func terminateHostProcess(process *os.Process) error {
	return signalHostProcess(process, syscall.SIGTERM)
}

func killHostProcess(process *os.Process) error {
	return signalHostProcess(process, syscall.SIGKILL)
}

func signalHostProcess(process *os.Process, signal syscall.Signal) error {
	if process == nil {
		return nil
	}
	pgid, err := syscall.Getpgid(process.Pid)
	if err == nil && pgid > 0 {
		if killErr := syscall.Kill(-pgid, signal); killErr == nil || errors.Is(killErr, syscall.ESRCH) {
			return nil
		}
	}
	if err := process.Signal(signal); err != nil && !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}
