//go:build !windows

package managedpolicy

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/capability"
)

func Install(context.Context, InstallOptions, []capability.Definition) (InstallResult, error) {
	return InstallResult{Phase: "validation"}, errors.New("policy install is supported only on Windows; deploy a protected policy using the platform administrator instructions")
}

func RunInstallHelper([]string, []capability.Definition) (bool, int) { return false, 0 }
func sameInstallPath(a, b string) bool                               { return a != "" && a == b }
func systemPolicyLocation() (string, bool, error)                    { path, err := SystemPath(); return path, false, err }
