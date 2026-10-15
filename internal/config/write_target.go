package config

import (
	"fmt"
	"strings"
)

// ResolveWriteEnvironment never uses a read default or artifact provenance.
func (c Config) ResolveWriteEnvironment(alias string) (Environment, error) {
	if alias != "" {
		return c.ResolveEnvironment(alias)
	}
	names := c.EnvironmentAliases()
	if len(names) == 1 {
		return c.ResolveEnvironment(names[0])
	}
	if len(names) == 0 {
		return Environment{}, fmt.Errorf("No environments are configured. Add a Tableau connection before writing")
	}
	return Environment{}, fmt.Errorf("Multiple environments are configured. Choose the target with --env <name>. Configured environments: %s", strings.Join(names, ", "))
}
