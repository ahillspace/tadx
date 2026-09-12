//go:build !windows

package update

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

type processScope struct{ cmd *exec.Cmd }

func newProcessScope(cmd *exec.Cmd) (*processScope, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	scope := &processScope{cmd: cmd}
	cmd.Cancel = func() error {
		err := scope.kill()
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	return scope, nil
}
func (*processScope) started() error { return nil }
func (s *processScope) kill() error {
	if s.cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
}
func (s *processScope) close() { _ = s.kill() }
