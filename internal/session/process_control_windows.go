//go:build windows

package session

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

func configureCommandForPlatform(cmd *exec.Cmd) {
	_ = cmd
}

func interruptSessionProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	if err := cmd.Process.Signal(os.Interrupt); err == nil || errors.Is(err, os.ErrProcessDone) {
		return nil
	}

	if err := cmd.Process.Kill(); err == nil || errors.Is(err, os.ErrProcessDone) {
		return nil
	}

	return fmt.Errorf("failed to interrupt session pid=%d", cmd.Process.Pid)
}

func killSessionProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	if err := cmd.Process.Kill(); err == nil || errors.Is(err, os.ErrProcessDone) {
		return nil
	}

	return fmt.Errorf("failed to kill session pid=%d", cmd.Process.Pid)
}
