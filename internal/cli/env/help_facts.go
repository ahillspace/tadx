package env

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

// ApplyHelpFacts attaches this command family's presentation-only contracts.
func ApplyHelpFacts(command *cobra.Command, path string) {
	if path == "env" {
		helpmeta.Summary(command, "Tableau site profiles and defaults")
	}
	switch path {
	case "env add", "env update":
		helpmeta.Value(command, "cache-max-concurrency", "1..256")
		if path == "env add" {
			helpmeta.FlagAnnotation(command, "cache-max-concurrency", "tadx.help.default", []string{"32"})
		} else {
			helpmeta.FlagAnnotation(command, "cache-max-concurrency", "tadx.help.omission", []string{"unchanged"})
		}
		helpmeta.Value(command, "url", "https://server")
		helpmeta.Value(command, "api-version", "major.minor")
	}
	switch path {
	case "env list":
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"20"})
	}
	switch path {
	case "env add":
		helpmeta.Required(command, "url")
	case "env update":
		for _, name := range []string{"site", "api-version", "pat-name-env", "pat-secret-env", "default-workspace", "cache-max-concurrency"} {
			helpmeta.Group(command, "exclusive", name, "clear-"+name)
		}
	}
	if command.Name() == "update" {
		for _, name := range []string{"url", "site", "api-version", "pat-name-env", "pat-secret-env", "default-workspace"} {
			helpmeta.FlagAnnotation(command, name, "tadx.help.omission", []string{"unchanged"})
		}
	}
}
