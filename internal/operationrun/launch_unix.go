//go:build unix

package operationrun

import (
	"fmt"
	"os/exec"
	"syscall"
)

func launch(executable string, args []string, workingDirectory string) error {
	command := exec.Command(executable, args...)
	command.Dir = workingDirectory
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start detached operation worker: %w", err)
	}
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("release detached operation worker: %w", err)
	}
	return nil
}
