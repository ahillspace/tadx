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
// receive them. It reads the configuration path when a child starts, after the
// root flags select it. An unreadable configuration leaves only the conventional
// names withheld, because update must still repair an installation.
func (r *runtimeDependencies) configuredPATVariables() []string {
	configuration, err := config.Load(r.configPath)
	if err != nil {
		return nil
	}
	aliases := configuration.EnvironmentAliases()
	names := make([]string, 0, 2*len(aliases))
	for _, alias := range aliases {
		environment, err := configuration.EnvironmentForRepair(alias)
		if err != nil {
			continue
		}
		for _, name := range []string{environment.Auth.PATNameEnv, environment.Auth.PATSecretEnv} {
			if config.ValidVariableReference(name) {
				names = append(names, name)
			}
		}
	}
	return names
}
