package app

import (
	action "github.com/ahillspace/tadx/actions/update"
	cli "github.com/ahillspace/tadx/internal/cli/update"
	"github.com/ahillspace/tadx/internal/config"
)

func newUpdateCommand(execution action.Runtime) *cli.Dependencies {
	return &cli.Dependencies{Updater: action.New(execution), Use: registryLeafUse("update"), Short: registryShort("update")}
}

// configuredPATVariables lists explicit PAT references so updater children never
// receive them. An unreadable configuration leaves only the conventional names
// withheld, because update must still repair an installation.
func configuredPATVariables(path string) func() []string {
	return func() []string {
		configuration, err := config.Load(path)
		if err != nil {
			return nil
		}
		names := make([]string, 0, 2*len(configuration.Environments))
		for _, environment := range configuration.Environments {
			names = append(names, environment.Auth.PATNameEnv, environment.Auth.PATSecretEnv)
		}
		return names
	}
}
