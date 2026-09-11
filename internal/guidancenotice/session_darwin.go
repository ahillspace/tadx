//go:build darwin

package guidancenotice

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func parentSessionID() string {
	pid := os.Getppid()
	process, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return ""
	}
	start := process.Proc.P_starttime
	return fmt.Sprintf("parent:%d:%d:%d", pid, start.Sec, start.Usec)
}
