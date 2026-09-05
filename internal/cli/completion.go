package cli

import (
	"errors"

	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

// NewCompletion creates the unregistered local shell-completion generator.
func NewCompletion(root *cobra.Command) *cobra.Command {
	command := &cobra.Command{Use: "completion <shell>", Short: "Generate shell completion for Bash, Zsh, Fish, or PowerShell.", ValidArgs: []string{"bash", "zsh", "fish", "powershell"}, Args: func(command *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(1)(command, args); err != nil {
			return clierr.Usage("completion", err)
		}
		for _, value := range command.ValidArgs {
			if args[0] == value {
				return nil
			}
		}
		return clierr.Usage("completion", errors.New("shell must be bash, zsh, fish, or powershell"))
	}, Annotations: map[string]string{"tadx.grouping": "true"}}
	command.RunE = func(command *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return root.GenBashCompletion(command.OutOrStdout())
		case "zsh":
			return root.GenZshCompletion(command.OutOrStdout())
		case "fish":
			return root.GenFishCompletion(command.OutOrStdout(), true)
		case "powershell":
			return root.GenPowerShellCompletion(command.OutOrStdout())
		}
		return nil
	}
	return command
}
