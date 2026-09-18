package app

import (
	"context"
	"errors"
	"testing"

	action "github.com/ahillspace/tadx/actions/update"
	updatecli "github.com/ahillspace/tadx/internal/cli/update"
)

type failedUpdateRuntime struct{ updateTestRuntime }

func (*failedUpdateRuntime) InstallationTarget() (string, error) { return "/opt/tadx/bin/tadx", nil }
func (f *failedUpdateRuntime) Install(context.Context, action.Release, []string) error {
	f.installs++
	return errors.New("installation fixture refused replacement")
}

func TestUpdateFailureRetainsKnownContextThroughCLI(t *testing.T) {
	execution := &failedUpdateRuntime{}
	deps := newUpdateCommand(nil, execution)
	deps.Renderer = &updateTestRenderer{}
	cmd := updatecli.New(*deps)
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetArgs([]string{"--target", "codex"})
	err := cmd.ExecuteContext(t.Context())
	retained, ok := errors.AsType[interface {
		error
		OperationOutput() any
	}](err)
	if !ok {
		t.Fatalf("missing known output: %v", err)
	}
	out := retained.OperationOutput().(action.Output)
	if out.InstallationPath != "/opt/tadx/bin/tadx" || out.Version != "1.0.0" || out.LatestVersion != "1.1.0" || out.ReleaseURL == "" || out.Status != "incomplete" || execution.installs != 1 {
		t.Fatalf("out=%+v installs=%d", out, execution.installs)
	}
}
