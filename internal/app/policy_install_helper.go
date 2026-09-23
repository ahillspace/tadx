package app

import (
	"github.com/ahillspace/tadx/internal/capability"
	"github.com/ahillspace/tadx/internal/managedpolicy"
)

// RunPolicyInstallHelper wires the scoped elevated entry point without opening
// ordinary runtime configuration, saved credentials, or publication workers.
func RunPolicyInstallHelper(args []string) (bool, int) {
	return managedpolicy.RunInstallHelper(args, capability.All())
}
