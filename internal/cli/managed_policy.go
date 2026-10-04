package cli

import (
	"github.com/spf13/cobra"
)

// BindManagedPolicy installs enforcement after publication dispatch wrapping so denied work cannot launch a worker.
// Workers call Run again and load the protected policy independently.
func BindManagedPolicy(root *cobra.Command, checkCapability func(string) error, checkRemoteMutation func(string) error) {
	var visit func(*cobra.Command)
	visit = func(command *cobra.Command) {
		if command.Annotations["tadx.grouping"] != "true" && (command.RunE != nil || command.Run != nil) {
			originalE, original := command.RunE, command.Run
			command.Run = nil
			command.RunE = func(c *cobra.Command, args []string) error {
				id := c.Annotations[CapabilityAnnotation]
				if c == root {
					if version, _ := c.Flags().GetBool("version"); version {
						id = "version.get"
					}
				}
				if err := checkCapability(id); err != nil {
					return err
				}
				if checkRemoteMutation != nil {
					preview, _ := c.Flags().GetBool("preview")
					if !preview {
						if err := checkRemoteMutation(id); err != nil {
							return err
						}
					}
				}
				if originalE != nil {
					return originalE(c, args)
				}
				original(c, args)
				return nil
			}
		}
		for _, child := range command.Commands() {
			visit(child)
		}
	}
	visit(root)
}
