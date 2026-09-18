//go:build windows

package operationrun

import (
	"fmt"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func launch(executable string, args []string, workingDirectory string) error {
	command := exec.Command(executable, args...)
	command.Dir = workingDirectory
	command.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS | windows.CREATE_NO_WINDOW,
		HideWindow:    true,
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start detached operation worker: %w", err)
	}
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("release detached operation worker: %w", err)
	}
	return nil
}
