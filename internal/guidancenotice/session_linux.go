//go:build linux

package guidancenotice

import (
	"fmt"
	"os"
	"strings"
)

func parentSessionID() string {
	pid := os.Getppid()
	contents, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return ""
	}
	end := strings.LastIndexByte(string(contents), ')')
	if end < 0 {
		return ""
	}
	fields := strings.Fields(string(contents)[end+1:])
	if len(fields) < 20 {
		return ""
	}
	bootID, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil || strings.TrimSpace(string(bootID)) == "" {
		return ""
	}
	return fmt.Sprintf("parent:%d:%s:%s", pid, fields[19], strings.TrimSpace(string(bootID)))
}
