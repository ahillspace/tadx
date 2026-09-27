package app

import (
	install "github.com/ahillspace/tadx/actions/agent/install"
	uninstall "github.com/ahillspace/tadx/actions/agent/uninstall"
	"github.com/ahillspace/tadx/internal/agent"
	agentcli "github.com/ahillspace/tadx/internal/cli/agent"
)

func newAgentCommands(runtime *runtimeDependencies) *agentcli.Dependencies {
	installer := agent.Installer{Home: runtime.userHomeDir}
	return &agentcli.Dependencies{Installer: install.New(installer), Uninstaller: uninstall.New(installer), Use: registryLeafUse("agent.install"), Short: registryShort("agent.install")}
}
