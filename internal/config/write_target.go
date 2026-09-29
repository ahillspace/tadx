package config

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// ResolveWriteEnvironment never uses a read default or artifact provenance.
func (c Config) ResolveWriteEnvironment(alias string) (Environment, error) {
	if alias != "" {
		return c.ResolveEnvironment(alias)
	}
	if len(c.Environments) == 1 {
		for name := range c.Environments {
			return c.ResolveEnvironment(name)
		}
	}
	if len(c.Environments) == 0 {
		return Environment{}, fmt.Errorf("No environments are configured. Add a Tableau connection before writing")
	}
	names := slices.Sorted(maps.Keys(c.Environments))
	return Environment{}, fmt.Errorf("Multiple environments are configured. Choose the target with --env <name>. Configured environments: %s", strings.Join(names, ", "))
}
