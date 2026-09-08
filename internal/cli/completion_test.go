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
			if strings.Contains(output.String(), "Load for this session:") {
				t.Fatal("completion script stdout contains setup instructions")
			}
		})
	}
}

func TestCompletionHelpExplainsSessionAndPersistentSetup(t *testing.T) {
	root := &cobra.Command{Use: "tadx"}
	root.AddCommand(cli.NewCompletion(root))
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"completion", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"source <(tadx completion bash)", "compinit", "completion fish | source", "Out-String | Invoke-Expression", "persistent", "CurrentUserAllHosts", "--no-completion", "-NoCompletion"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("completion help missing %q", want)
		}
	}
}
