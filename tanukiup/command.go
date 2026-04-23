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
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	childDone := false
	select {
	case <-done:
		childDone = true
	default:
	}
	if terminateErr != nil && !errors.Is(terminateErr, os.ErrProcessDone) {
		if !childDone {
			<-done
		}
		return terminateErr
	}

	for {
		if childDone && commandGroupDone(cmd) {
			return ctx.Err()
		}
		select {
		case <-done:
			childDone = true
		case <-ticker.C:
		case <-timer.C:
			killErr := killCommand(cmd)
			if !childDone {
				<-done
			}
			if killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
				return killErr
			}
			return ctx.Err()
		}
	}
}
