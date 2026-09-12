package cli_test

import (
	"testing"

	"github.com/ahillspace/tadx/internal/capability"
	"github.com/ahillspace/tadx/internal/cli"
	"github.com/spf13/cobra"
)

func TestBatchMetadataNamesRealRemoteCommandSelectors(t *testing.T) {
	deps := previewDependencies(&previewActionSpy{})
	deps.BatchOptions = capability.BatchOptions()
	root := cli.NewRoot(deps)
	var walk func(*cobra.Command)
	walk = func(command *cobra.Command) {
		id := command.Annotations[cli.CapabilityAnnotation]
		if options, ok := deps.BatchOptions[id]; ok && command.RunE != nil {
			if command.Flags().Lookup("batch-file") == nil {
				t.Errorf("%s missing file batching", id)
			}
			for _, name := range append(append([]string(nil), options.Selectors...), options.NativeSelections...) {
				if command.Flags().Lookup(name) == nil {
					t.Errorf("%s declares nonexistent batch selector --%s", id, name)
				}
			}
		}
		for _, child := range command.Commands() {
			walk(child)
		}
	}
	walk(root)
}
