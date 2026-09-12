package app

import (
	action "github.com/ahillspace/tadx/actions/update"
	cli "github.com/ahillspace/tadx/internal/cli/update"
	updater "github.com/ahillspace/tadx/internal/update"
)

func newUpdateCommand(runtime *runtimeDependencies, overrides ...action.Runtime) *cli.Dependencies {
	var execution action.Runtime = updater.Runtime{}
	if len(overrides) > 0 {
		execution = overrides[0]
	}
	return &cli.Dependencies{Updater: action.New(execution), Use: registryLeafUse("update"), Short: registryShort("update")}
}
