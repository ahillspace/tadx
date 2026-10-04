package agent_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/actions/agent"
	agentcli "github.com/ahillspace/tadx/internal/cli/agent"
)

type partialUninstaller struct {
	output agent.UninstallOutput
	err    error
}

func (u partialUninstaller) Uninstall(context.Context, agent.UninstallInput) (agent.UninstallOutput, error) {
	return u.output, u.err
}
func (partialUninstaller) Install(context.Context, agent.InstallInput) (agent.InstallOutput, error) {
	panic("unexpected install")
}

type unusedRenderer struct{}

func (unusedRenderer) Render(any) error {
	return errors.New("a failed uninstall must not render as success")
}

func TestUninstallFailureCarriesConfirmedPackageStateToTheErrorRenderer(t *testing.T) {
	cause := errors.New("uninstall failed and rollback is incomplete; inspect target packages")
	partial := agent.UninstallOutput{Status: "partial", Target: "codex", Skills: []agent.UninstallSkill{{Name: "tadx", Status: "backed-up", Path: ".codex/skills/tadx", Backup: ".codex/.tadx-skill-staging/tadx-A"}}}
	command := agentcli.New(agentcli.Dependencies{Service: partialUninstaller{output: partial, err: cause}, Renderer: unusedRenderer{}})
	command.SetArgs([]string{"uninstall", "--target", "codex"})
	command.SilenceErrors, command.SilenceUsage = true, true
	err := command.ExecuteContext(t.Context())
	if !errors.Is(err, cause) {
		t.Fatalf("error = %v", err)
	}
	var carrier interface{ OperationOutput() any }
	if !errors.As(err, &carrier) {
		t.Fatalf("error does not carry the partial output: %#v", err)
	}
	output, ok := carrier.OperationOutput().(agent.UninstallOutput)
	if !ok || output.Status != "partial" || len(output.Skills) != 1 || output.Skills[0].Backup != ".codex/.tadx-skill-staging/tadx-A" {
		t.Fatalf("carried output = %#v", carrier.OperationOutput())
	}
}

func TestUninstallFailureWithoutPackageStateReturnsOnlyTheError(t *testing.T) {
	cause := errors.New("target locked")
	command := agentcli.New(agentcli.Dependencies{Service: partialUninstaller{err: cause}, Renderer: unusedRenderer{}})
	command.SetArgs([]string{"uninstall", "--target", "codex"})
	command.SilenceErrors, command.SilenceUsage = true, true
	err := command.ExecuteContext(t.Context())
	var carrier interface{ OperationOutput() any }
	if !errors.Is(err, cause) || errors.As(err, &carrier) {
		t.Fatalf("error = %#v", err)
	}
}
