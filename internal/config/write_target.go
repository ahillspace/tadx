package config

import (
	"fmt"
	"sort"
	"strings"
)

// ResolveWriteEnvironment never uses a read default or artifact provenance.
func (c Config) ResolveWriteEnvironment(alias string) (Environment, error) {
	if alias == "" {
		if len(c.Environments) == 1 {
			for name := range c.Environments {
				alias = name
			}
		} else {
			names := make([]string, 0, len(c.Environments))
			for name := range c.Environments {
				names = append(names, name)
			}
			sort.Strings(names)
			if len(names) == 0 {
				return Environment{}, fmt.Errorf("No environments are configured. Add a Tableau connection before writing")
			}
			return Environment{}, fmt.Errorf("Multiple environments are configured. Choose the target with --env <name>. Configured environments: %s", strings.Join(names, ", "))
		}
	}
	return c.ResolveEnvironment(alias)
}
