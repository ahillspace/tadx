package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestApplyShorthandPreservesCanonicalFlagState(t *testing.T) {
	var environment, name string
	var preview, force bool
	var artifacts []string
	root := &cobra.Command{Use: "tadx"}
	content := &cobra.Command{Use: "content"}
	publish := &cobra.Command{Use: "publish", RunE: func(cmd *cobra.Command, _ []string) error {
		for _, canonical := range []string{"environment", "name", "preview", "force", "artifact"} {
			if !cmd.Flags().Changed(canonical) {
				t.Errorf("canonical flag %q was not marked changed", canonical)
			}
		}
		return nil
	}}
	publish.Flags().StringVar(&environment, "environment", "", "environment")
	publish.Flags().StringVar(&name, "name", "", "name")
	publish.Flags().BoolVar(&preview, "preview", true, "preview")
	publish.Flags().BoolVar(&force, "force", false, "force")
	publish.Flags().StringArrayVar(&artifacts, "artifact", nil, "artifact")
	content.AddCommand(publish)
	root.AddCommand(content)
	applyShorthand(root)
	root.SetArgs([]string{"con", "pub", "-e", "prod", "--nm", "Sales", "--pv=false", "--frc", "--art", "one", "--art", "two"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if environment != "prod" || name != "Sales" || preview || !force {
		t.Fatalf("parsed environment=%q name=%q preview=%t force=%t", environment, name, preview, force)
	}
	if strings.Join(artifacts, ",") != "one,two" {
		t.Fatalf("artifacts = %v", artifacts)
	}
}

func TestApplyShorthandMakesAliasesDiscoverableInHelp(t *testing.T) {
	root := &cobra.Command{Use: "tadx"}
	child := &cobra.Command{Use: "workspace", Short: "Manage workspaces"}
	leaf := &cobra.Command{Use: "status", Short: "Show status", Run: func(*cobra.Command, []string) {}}
	var workspace string
	leaf.Flags().StringVar(&workspace, "workspace", "", "logical workspace")
	child.AddCommand(leaf)
	root.AddCommand(child)
	applyShorthand(root)

	var output bytes.Buffer
	root.SetOut(&output)
	if err := root.Help(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "alias: ws") {
		t.Fatalf("root help does not surface child alias:\n%s", output.String())
	}
	output.Reset()
	leaf.SetOut(&output)
	if err := leaf.Help(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Aliases:", "status, st", "-w, --workspace", "alias: --ws"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("leaf help missing %q:\n%s", want, output.String())
		}
	}
}

func TestShorthandVocabularyHasNoAmbiguousTargets(t *testing.T) {
	assertUniqueValues(t, "command", commandAliases)
	assertUniqueValues(t, "flag", flagLongAliases)
}

func TestValidateShorthandTreeChecksPersistentFlagsAndCanonicalCollisions(t *testing.T) {
	t.Run("missing persistent shorthand", func(t *testing.T) {
		root := &cobra.Command{Use: "tadx"}
		root.PersistentFlags().String("future-setting", "", "future setting")
		err := validateShorthandTree(root)
		if err == nil || !strings.Contains(err.Error(), "--future-setting has no shorthand") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("long alias steals canonical flag", func(t *testing.T) {
		root := &cobra.Command{Use: "tadx"}
		root.Flags().String("environment", "", "environment")
		root.Flags().String("env", "", "different canonical flag")
		err := validateShorthandTree(root)
		if err == nil || !strings.Contains(err.Error(), "--env for --environment collides") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("reserved long alias cannot be a standalone canonical flag", func(t *testing.T) {
		root := &cobra.Command{Use: "tadx"}
		root.Flags().String("env", "", "canonical flag using a reserved alias")
		err := validateShorthandTree(root)
		if err == nil || !strings.Contains(err.Error(), "--env for --environment collides") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("child canonical flag steals inherited alias", func(t *testing.T) {
		root := &cobra.Command{Use: "tadx"}
		root.PersistentFlags().String("environment", "", "environment")
		child := &cobra.Command{Use: "x"}
		child.Flags().String("env", "", "different canonical flag")
		root.AddCommand(child)
		err := validateShorthandTree(root)
		if err == nil || !strings.Contains(err.Error(), "tadx x: flag alias --env for --environment collides") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("missing command alias", func(t *testing.T) {
		root := &cobra.Command{Use: "tadx"}
		root.AddCommand(&cobra.Command{Use: "future-command"})
		err := validateShorthandTree(root)
		if err == nil || !strings.Contains(err.Error(), "command \"future-command\" has no alias") {
			t.Fatalf("error = %v", err)
		}
	})
}

func assertUniqueValues(t *testing.T, kind string, values map[string]string) {
	t.Helper()
	seen := map[string]string{}
	for canonical, alias := range values {
		if prior := seen[alias]; prior != "" {
			t.Errorf("%s alias %q is shared by %q and %q", kind, alias, prior, canonical)
		}
		seen[alias] = canonical
	}
}
