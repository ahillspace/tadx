//go:build windows

package guidancenotice

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func parentSessionID() string {
	pid := os.Getppid()
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(process)
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(process, &created, &exited, &kernel, &user); err != nil {
		return ""
	}
	return fmt.Sprintf("parent:%d:%d:%d", pid, created.HighDateTime, created.LowDateTime)
}
