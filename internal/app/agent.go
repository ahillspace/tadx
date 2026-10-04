package app

import (
	agentaction "github.com/ahillspace/tadx/actions/agent"
	"github.com/ahillspace/tadx/internal/agent"
	agentcli "github.com/ahillspace/tadx/internal/cli/agent"
)

func newAgentCommands(runtime *runtimeDependencies) *agentcli.Dependencies {
	installer := agent.Installer{Home: runtime.userHomeDir}
	return &agentcli.Dependencies{Service: agentaction.New(installer), Use: registryLeafUse("agent.install"), Short: registryShort("agent.install")}
}
