//go:build !unix

package tanukiup

import (
	"os"
	"os/exec"
)

func configureCommand(cmd *exec.Cmd) {}

func terminateCommand(cmd *exec.Cmd) error {
	return killCommand(cmd)
}

func killCommand(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return os.ErrProcessDone
	}
	return cmd.Process.Kill()
}
