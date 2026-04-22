//go:build unix

package tanukiup

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func terminateCommand(cmd *exec.Cmd) error {
	return signalCommandGroup(cmd, syscall.SIGTERM)
}

func killCommand(cmd *exec.Cmd) error {
	return signalCommandGroup(cmd, syscall.SIGKILL)
}

func signalCommandGroup(cmd *exec.Cmd, sig syscall.Signal) error {
	if cmd.Process == nil {
		return os.ErrProcessDone
	}
	if err := syscall.Kill(-cmd.Process.Pid, sig); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	return nil
}
