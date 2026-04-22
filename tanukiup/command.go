package tanukiup

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"time"
)

const processShutdownGracePeriod = 5 * time.Second

func newCommand(name string, arg ...string) *exec.Cmd {
	cmd := exec.Command(name, arg...)
	configureCommand(cmd)
	return cmd
}

func runCommand(ctx context.Context, cmd *exec.Cmd) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
	}

	terminateErr := terminateCommand(cmd)
	timer := time.NewTimer(processShutdownGracePeriod)
	defer timer.Stop()

	select {
	case err := <-done:
		// The direct child may have exited while descendants in its process group
		// are still alive. Make a final best-effort sweep of the group.
		_ = killCommand(cmd)
		if err != nil {
			return err
		}
		if terminateErr != nil && !errors.Is(terminateErr, os.ErrProcessDone) {
			return terminateErr
		}
		return ctx.Err()
	case <-timer.C:
		killErr := killCommand(cmd)
		err := <-done
		if err != nil {
			return err
		}
		if killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
			return killErr
		}
		return ctx.Err()
	}
}
