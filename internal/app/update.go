package app

import (
	action "github.com/ahillspace/tadx/actions/update"
	cli "github.com/ahillspace/tadx/internal/cli/update"
)

func newUpdateCommand(execution action.Runtime) *cli.Dependencies {
	return &cli.Dependencies{Updater: action.New(execution), Use: registryLeafUse("update"), Short: registryShort("update")}
}
