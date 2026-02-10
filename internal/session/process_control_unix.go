//go:build !windows

package session

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func configureCommandForPlatform(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func interruptSessionProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	pid := cmd.Process.Pid
	if pid <= 0 {
		return nil
	}

	if err := syscall.Kill(-pid, syscall.SIGINT); err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}

	if err := cmd.Process.Signal(os.Interrupt); err == nil || errors.Is(err, os.ErrProcessDone) {
		return nil
	}

	return fmt.Errorf("failed to interrupt session pid=%d", pid)
}

func killSessionProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	pid := cmd.Process.Pid
	if pid <= 0 {
		return nil
	}

	if err := syscall.Kill(-pid, syscall.SIGKILL); err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}

	if err := cmd.Process.Kill(); err == nil || errors.Is(err, os.ErrProcessDone) {
		return nil
	}

	return fmt.Errorf("failed to kill session pid=%d", pid)
}
