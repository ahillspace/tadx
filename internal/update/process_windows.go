//go:build windows

package update

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os/exec"
	"syscall"
	"unsafe"
)

// Start suspended so no installer subprocess can escape before job assignment.
type processScope struct {
	cmd *exec.Cmd
	job windows.Handle
}

func newProcessScope(cmd *exec.Cmd) (*processScope, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_SUSPENDED}
	scope := &processScope{cmd: cmd, job: job}
	cmd.Cancel = func() error { return windows.TerminateJobObject(job, 1) }
	return scope, nil
}
func (s *processScope) started() error {
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(s.cmd.Process.Pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(process)
	if err = windows.AssignProcessToJobObject(s.job, process); err != nil {
		return err
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != uint32(s.cmd.Process.Pid) {
			continue
		}
		thread, openErr := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if openErr != nil {
			return openErr
		}
		_, resumeErr := windows.ResumeThread(thread)
		windows.CloseHandle(thread)
		return resumeErr
	}
	return fmt.Errorf("cannot find suspended installer thread: %w", err)
}
func (s *processScope) close() { _ = windows.CloseHandle(s.job) }
