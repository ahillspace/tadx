package app

import (
	"context"
	install "github.com/ahillspace/tadx/actions/agent/install"
	uninstall "github.com/ahillspace/tadx/actions/agent/uninstall"
	"github.com/ahillspace/tadx/internal/agent"
	agentcli "github.com/ahillspace/tadx/internal/cli/agent"
)

func newAgentCommands(runtime *runtimeDependencies) *agentcli.Dependencies {
	installer := agent.Installer{Home: runtime.userHomeDir}
	return &agentcli.Dependencies{Installer: install.New(agentInstaller{installer: installer}), Uninstaller: uninstall.New(agentUninstaller{installer: installer}), Use: registryLeafUse("agent.install"), Short: registryShort("agent.install")}
}

type agentUninstaller struct{ installer agent.Installer }

func (a agentUninstaller) Uninstall(ctx context.Context, input uninstall.Input) (uninstall.Result, error) {
	result, err := a.installer.Uninstall(ctx, input.Target, input.Preview, input.Force)
	if err != nil {
		return uninstall.Result{}, err
	}
	skills := make([]uninstall.Skill, len(result.Skills))
	for i, v := range result.Skills {
		skills[i] = uninstall.Skill{Name: v.Name, Status: v.Status, Path: v.Path, SHA256: v.SHA256, Backup: v.Backup}
	}
	return uninstall.Result{Status: result.Status, Skills: skills, Warnings: result.Warnings}, nil
}

type agentInstaller struct{ installer agent.Installer }

func (a agentInstaller) Install(ctx context.Context, input install.Input) (install.Result, error) {
	result, err := a.installer.Install(ctx, input.Target, input.Preview, input.Force)
	if err != nil {
		return install.Result{}, err
	}
	skills := make([]install.Skill, len(result.Skills))
	for index, skill := range result.Skills {
		skills[index] = install.Skill{Name: skill.Name, Status: skill.Status, Path: skill.Path, SHA256: skill.SHA256, Files: skill.Files, Backup: skill.Backup}
	}
	return install.Result{Status: result.Status, Skills: skills, Warnings: result.Warnings}, nil
}
