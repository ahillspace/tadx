package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/cli"
	"github.com/spf13/cobra"
)

func TestCompletionGeneratesSupportedShells(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			root := &cobra.Command{Use: "tadx"}
			root.AddCommand(cli.NewCompletion(root))
			var output bytes.Buffer
			root.SetOut(&output)
			root.SetArgs([]string{"completion", shell})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.ToLower(output.String()), "tadx") {
				t.Fatalf("completion output is empty: %q", output.String())
			}
		})
	}
}
