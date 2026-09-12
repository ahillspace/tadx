package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func helpTestTree() (*cobra.Command, *cobra.Command, *cobra.Command) {
	root := &cobra.Command{Use: "tadx", Short: "Manage Tableau content"}
	root.PersistentFlags().String("config", "", "configuration file")
	root.PersistentFlags().Bool("json", false, "render JSON")
	admin := &cobra.Command{Use: "admin", Short: "Manage users and groups"}
	group := &cobra.Command{Use: "group", Short: "Manage groups"}
	member := &cobra.Command{Use: "member", Short: "Manage group members"}
	list := &cobra.Command{Use: "list [query]", Short: "List group members", Run: func(*cobra.Command, []string) { panic("help ran action") }}
	remove := &cobra.Command{Use: "remove <id>", Short: "Remove a group member", Run: list.Run}
	for _, action := range []*cobra.Command{list, remove} {
		action.Flags().String("environment", "", "exact environment")
	}
	list.Flags().Int("limit", 25, "maximum members")
	remove.Flags().StringArray("member-id", nil, "member identities")
	remove.Flags().String("name", "", "member name")
	remove.MarkFlagsOneRequired("member-id", "name")
	remove.MarkFlagsMutuallyExclusive("member-id", "name")
	remove.Flags().String("group-id", "", "group identity")
	_ = remove.MarkFlagRequired("group-id")
	remove.Flags().Bool("preview", false, "show the plan")
	remove.Long = "Remove a group member.\n\nUse an exact identity.\nNo changes occur with --preview."
	remove.Example = "tadx admin group member remove user-id --group-id group-id --preview"
	member.AddCommand(list, remove)
	group.AddCommand(member)
	admin.AddCommand(group)
	root.AddCommand(admin)
	applyShorthand(root)
	rejectGroupingArguments(root)
	return root, admin, remove
}

func renderedHelp(t *testing.T, command *cobra.Command) string {
	t.Helper()
	var output bytes.Buffer
	command.SetOut(&output)
	if err := command.Help(); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestCategoryHelpExplainsAllDescendantActions(t *testing.T) {
	root, admin, _ := helpTestTree()
	installCategoryHelp(root)
	got := renderedHelp(t, admin)
	for _, want := range []string{
		"tadx admin <command> [flags]", "group member list [query]", "group member remove <id>",
		"flags{group member list}", "flags{group member remove}", "--limit <number>", "default: 25",
		"--member-id <string>", "repeatable", "--group-id <string>", "required",
		"at least one of: --member-id, --name", "mutually exclusive: --member-id, --name",
		"alias: --env", "aliases: group=grp, member=mem", "Use an exact identity.",
		"tadx admin group member remove user-id --group-id group-id --preview",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("help missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "exact environment") != 1 || strings.Count(got, "render JSON") != 1 {
		t.Fatalf("identical shared flags should be explained once:\n%s", got)
	}
}

func TestCategoryHelpScopesDifferentAndInheritedFlags(t *testing.T) {
	root, admin, remove := helpTestTree()
	remove.Flags().Lookup("environment").Usage = "explicit write environment"
	local := collectFlags(remove.LocalNonPersistentFlags())
	remove.ResetFlags()
	for _, flag := range local {
		remove.Flags().AddFlag(flag)
	}
	remove.Flags().Bool("json", true, "action-specific JSON")
	installCategoryHelp(root)
	got := renderedHelp(t, admin)
	for _, want := range []string{"exact environment", "explicit write environment", "action-specific JSON", "default: true", "render JSON"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing differently scoped flag %q:\n%s", want, got)
		}
	}
}

func TestCategoryHelpOmitsHiddenTreeAndKeepsRootCompact(t *testing.T) {
	root, admin, remove := helpTestTree()
	remove.Flags().String("private-flag", "", "hidden flag description")
	_ = remove.Flags().MarkHidden("private-flag")
	hidden := &cobra.Command{Use: "hidden-group", Hidden: true}
	hidden.AddCommand(&cobra.Command{Use: "hidden-action", Run: remove.Run})
	admin.AddCommand(hidden)
	installCategoryHelp(root)
	category := renderedHelp(t, admin)
	if strings.Contains(category, "hidden-") || strings.Contains(category, "private-flag") {
		t.Fatalf("hidden metadata leaked:\n%s", category)
	}
	got := renderedHelp(t, root)
	if !strings.Contains(got, "admin") || !strings.Contains(got, "Manage users and groups") || !strings.Contains(got, "tadx admin --help") {
		t.Fatalf("root lacks category index:\n%s", got)
	}
	if strings.Contains(got, "member-id") || strings.Contains(got, "group member remove") {
		t.Fatalf("root expanded descendant details:\n%s", got)
	}
}

func TestCategoryHelpNeverRunsHooksOrShowsCurrentValues(t *testing.T) {
	for _, args := range [][]string{{"admin", "--help"}, {"adm", "grp", "mem", "--help"}, {"help", "admin", "group"}, {"admin", "group", "member", "remove", "--help"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root, _, remove := helpTestTree()
			root.PersistentPreRunE = func(*cobra.Command, []string) error { panic("help ran hook") }
			remove.Args = func(*cobra.Command, []string) error { panic("help ran validation") }
			_ = root.PersistentFlags().Set("config", "private-config-path")
			root.PersistentFlags().Lookup("config").DefValue = "private-default-path"
			_ = remove.Flags().Set("name", "private-current-name")
			installCategoryHelp(root)
			var output bytes.Buffer
			root.SetOut(&output)
			root.SetErr(&output)
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{"private-config-path", "private-default-path", "private-current-name"} {
				if strings.Contains(output.String(), secret) {
					t.Errorf("help exposed %q", secret)
				}
			}
		})
	}
}

func TestLeafHelpRemainsFocusedAndShowsRelationships(t *testing.T) {
	root, _, remove := helpTestTree()
	remove.Flags().String("source", "", "source")
	remove.Flags().String("target", "", "target")
	remove.MarkFlagsRequiredTogether("source", "target")
	installCategoryHelp(root)
	got := renderedHelp(t, remove)
	for _, want := range []string{"Usage:", "Aliases:", "remove, rm", "Flags:", "required together: --source, --target", "--preview"} {
		if !strings.Contains(got, want) {
			t.Errorf("leaf help missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "maximum members") {
		t.Fatalf("leaf help includes sibling flags:\n%s", got)
	}
}

func TestCategoryHelpUsesStructuredValuesAndDeclaredDefaults(t *testing.T) {
	root, admin, remove := helpTestTree()
	remove.Flags().String("mode", "Allow", "permission mode")
	_ = remove.Flags().SetAnnotation("mode", "tadx.help.choices", []string{"Allow", "Deny"})
	_ = remove.Flags().SetAnnotation("member-id", "tadx.help.value", []string{"LUID"})
	_ = remove.Flags().SetAnnotation("mode", "tadx.help.required", []string{"true"})
	remove.Flags().StringSlice("scope", []string{"users", "groups"}, "inventory scopes")
	installCategoryHelp(root)
	before := renderedHelp(t, admin)
	_ = remove.Flags().Set("mode", "Deny")
	_ = remove.Flags().Set("member-id", "private-member-value")
	_ = remove.Flags().Set("scope", "private-scope-value")
	_ = root.PersistentFlags().Set("json", "true")
	after := renderedHelp(t, admin)
	if before != after {
		t.Fatalf("help changed after flag values changed:\n%s", after)
	}
	for _, command := range []*cobra.Command{admin, remove} {
		got := renderedHelp(t, command)
		for _, want := range []string{"--mode <Allow|Deny> (required; default: \"Allow\")", "--member-id <LUID> (repeatable)", "--scope <string> (repeatable; comma-separated values accepted; default: [users,groups])", "--json[=true|false] (default: false)"} {
			if !strings.Contains(got, want) {
				t.Errorf("missing declared help metadata %q:\n%s", want, got)
			}
		}
	}
}

func TestInstallingCategoryHelpPreservesRootAction(t *testing.T) {
	root, _, _ := helpTestTree()
	runs := 0
	root.Run = func(*cobra.Command, []string) { runs++ }
	root.RunE = nil
	installCategoryHelp(root)
	root.SetArgs(nil)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Fatalf("root overview ran %d times", runs)
	}
}

func TestCategoryHelpDeduplicatesScopedNotesAndActionSummaries(t *testing.T) {
	root, admin, remove := helpTestTree()
	list, _, err := root.Find([]string{"admin", "group", "member", "list"})
	if err != nil {
		t.Fatal(err)
	}
	shared := "Batch JSON uses one items array.\nEach item contains flags for this action."
	list.Long = "List group members.\n\n" + shared
	remove.Long = "Remove a group member.\n\n" + shared + "\n\nOnly removal changes membership."
	installCategoryHelp(root)
	got := renderedHelp(t, admin)
	for _, single := range []string{"List group members", "Remove a group member", "Batch JSON uses one items array.", "Each item contains flags for this action.", "Only removal changes membership."} {
		if strings.Count(got, single) != 1 {
			t.Errorf("expected %q once:\n%s", single, got)
		}
	}
	if !strings.Contains(got, "notes{all actions}:") || !strings.Contains(got, "notes{group member remove}:") {
		t.Fatalf("notes lack exact action scopes:\n%s", got)
	}
	if strings.Contains(got, "(alias: rm)") || !strings.Contains(got, "remove=rm") {
		t.Fatalf("action alias was omitted or repeated in its short description:\n%s", got)
	}
}

func TestCategoryHelpDescribesRepeatedSelectorsAndEffectiveDefaults(t *testing.T) {
	root, admin, remove := helpTestTree()
	flag := remove.Flags().Lookup("group-id")
	flag.Value = &repeatedSelector{Value: flag.Value}
	remove.Flags().Int("limit", 0, "maximum items")
	_ = remove.Flags().SetAnnotation("limit", "tadx.help.default", []string{"25"})
	installCategoryHelp(root)
	for _, command := range []*cobra.Command{admin, remove} {
		got := renderedHelp(t, command)
		if !strings.Contains(got, "--group-id <string> (repeatable; required)") || !strings.Contains(got, "--limit <number> (default: 25)") {
			t.Fatalf("repeat/default metadata missing:\n%s", got)
		}
	}
}

func TestCategoryHelpRemovesAliasMarkersFromLongSummaries(t *testing.T) {
	for _, summary := range []string{
		"Remove a group member. (alias: rm)",
		"Remove a group member (alias:rm)",
		"Remove a group member. (aliases: rm, delete)",
	} {
		t.Run(summary, func(t *testing.T) {
			root, admin, remove := helpTestTree()
			admin.Long = admin.Short + "\n\nCategory guidance."
			remove.Short = summary
			remove.Long = summary + "\n\nAction guidance."
			installCategoryHelp(root)
			got := renderedHelp(t, admin)
			for _, text := range []string{"Manage users and groups", "Remove a group member", "Category guidance.", "Action guidance."} {
				if strings.Count(got, text) != 1 {
					t.Errorf("expected %q once:\n%s", text, got)
				}
			}
			for _, alias := range []string{"(alias: rm)", "(alias:rm)", "(aliases: rm, delete)", "(alias: adm)"} {
				if strings.Contains(got, alias) {
					t.Errorf("summary alias remained in notes: %q\n%s", alias, got)
				}
			}
			remove.Long = summary
			got = renderedHelp(t, admin)
			if strings.Contains(got, "notes{group member remove}:") {
				t.Fatalf("summary-only Long created an empty or alias-only note:\n%s", got)
			}
		})
	}
}
