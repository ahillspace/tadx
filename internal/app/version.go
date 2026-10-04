package app

import (
	versionaction "github.com/ahillspace/tadx/actions/version"
	versioncli "github.com/ahillspace/tadx/internal/cli/version"
	versioncore "github.com/ahillspace/tadx/internal/version"
)

func newVersionCommand(runtime *runtimeDependencies) *versioncli.Dependencies {
	return &versioncli.Dependencies{Getter: versionaction.New(versioncore.Current(), versioncore.Checker{Client: runtime.httpClient}), Use: registryLeafUse("version.get"), Short: registryShort("version.get")}
}
