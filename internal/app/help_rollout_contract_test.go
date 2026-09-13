package app

import (
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/capability"
)

// Exercise real composition-root help for every executable capability. The
// fixture rejects network/credential access and checks local state stays intact.
func TestEveryExecutableMirrorsItsBoundedOperationalReference(t *testing.T) {
	dir, options := contentHelpPilotSetup(t)
	for _, definition := range capability.Executable() {
		t.Run(definition.ID, func(t *testing.T) {
			path := append([]string(nil), definition.CommandPath...)
			if definition.ID == "session.overview" {
				path = nil
			}
			owner := append([]string(nil), path...)
			if len(path) == 3 || (len(path) == 2 && path[0] != "catalog") {
				owner = path[:len(path)-1]
			}
			want := contentHelpPilotRun(t, dir, options, append(append([]string(nil), owner...), "-h")...)
			if len(want) > 4000 {
				t.Errorf("reference %v grew to %d bytes (budget 4000)", owner, len(want))
			}
			for _, args := range [][]string{
				append(append([]string(nil), path...), "-h"),
				append(append([]string(nil), path...), "--help"),
				append([]string{"help"}, path...),
				append(append([]string(nil), path...), "--full", "--json", "--help"),
			} {
				if got := contentHelpPilotRun(t, dir, options, args...); got != want {
					t.Errorf("%v differs from owning reference %v", args, owner)
				}
			}
			if strings.Contains(want, "flags{") || strings.Contains(want, "(alias:") {
				t.Error("reference restores rejected verbose help decoration")
			}
		})
	}
}

func TestHelpNavigationDoesNotExpandResourceManuals(t *testing.T) {
	dir, options := contentHelpPilotSetup(t)
	for _, category := range []string{"admin", "catalog", "content", "pulse"} {
		out := contentHelpPilotRun(t, dir, options, category, "-h")
		if len(out) > 1200 || strings.Contains(out, "--batch-file") || strings.Contains(out, "Shared:") {
			t.Errorf("%s navigation expanded into a manual (%d bytes)", category, len(out))
		}
	}
}
