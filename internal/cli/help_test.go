package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/spf13/cobra"
)

func helpTestTree() (*cobra.Command, *cobra.Command, *cobra.Command) {
	root := &cobra.Command{Use: "tadx"}
	root.PersistentFlags().String("config", "", "configuration")
	root.PersistentFlags().Bool("json", false, "JSON")
	admin := &cobra.Command{Use: "admin", Short: "Manage users and groups"}
	group := &cobra.Command{Use: "group", Short: "Manage groups"}
	list := &cobra.Command{Use: "list [query]", Short: "List groups", Run: func(*cobra.Command, []string) { panic("help executed action") }}
	remove := &cobra.Command{Use: "remove <id>", Short: "Remove group", Run: list.Run}
	for _, action := range []*cobra.Command{list, remove} {
		action.Flags().String("environment", "", "environment")
	}
	list.Flags().Int("limit", 25, "limit")
	remove.Flags().StringArray("member-id", nil, "members")
	remove.Flags().String("name", "", "name")
	remove.MarkFlagsOneRequired("member-id", "name")
	remove.MarkFlagsMutuallyExclusive("member-id", "name")
	remove.Flags().String("group-id", "", "group")
	_ = remove.MarkFlagRequired("group-id")
	remove.Flags().Bool("preview", false, "preview")
	group.AddCommand(list, remove)
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

func TestCategoryHelpNavigatesWithoutExpandingReferences(t *testing.T) {
	root, admin, remove := helpTestTree()
	installCategoryHelp(root)
	got := renderedHelp(t, admin)
	for _, want := range []string{"Usage: tadx admin <resource> <verb>", "group: Manage groups", "list, remove"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "--member-id") {
		t.Fatal("category expanded syntax")
	}
	got = renderedHelp(t, root)
	if !strings.Contains(got, "admin: Manage users and groups") || !strings.Contains(got, "group") || strings.Contains(got, "member-id") {
		t.Fatal(got)
	}
	ref := renderedHelp(t, remove.Parent())
	focused := renderedHelp(t, remove)
	if ref == focused {
		t.Fatal("verb repeats the complete resource reference")
	}
	for _, want := range []string{"--group-id <string>", "--member-id <string>...", "--limit <number> (default: 25)", "at least one of: --member-id, --name", "mutually exclusive: --member-id, --name"} {
		if !strings.Contains(ref, want) {
			t.Errorf("complete reference missing %q: %s", want, ref)
		}
	}
	for _, want := range []string{"Usage: tadx admin group remove <id> [flags]", "--group-id <string>", "--member-id <string>...", "at least one of: --member-id, --name", "mutually exclusive: --member-id, --name"} {
		if !strings.Contains(focused, want) {
			t.Errorf("focused reference missing %q: %s", want, focused)
		}
	}
	if strings.Contains(focused, "--limit") || strings.Contains(focused, "\n  list:") {
		t.Fatalf("focused reference contains sibling action details:\n%s", focused)
	}
}

func TestHelpPathValidationPrecedesTargetFlagParsing(t *testing.T) {
	root, _, _ := helpTestTree()
	installCategoryHelp(root)
	root.SetArgs([]string{"help", "admin", "group", "missing", "--group-id", "group-id"})
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	err := root.Execute()
	if err == nil {
		t.Fatal("invalid help path unexpectedly succeeded")
	}
	var structured *errs.Error
	if !errors.As(err, &structured) {
		t.Fatalf("error is not structured: %T %v", err, err)
	}
	if structured.Kind != errs.KindUsage || !strings.Contains(structured.Summary, "admin group missing") {
		t.Fatalf("unexpected help path error: %#v", structured)
	}
	if !strings.Contains(structured.CorrectiveAction, "tadx admin group -h") {
		t.Fatalf("missing supported route: %#v", structured)
	}
}

func TestValidateHelpArgsRejectsUnknownDirectRouteButKeepsConfigHelpFallback(t *testing.T) {
	root, _, _ := helpTestTree()
	installCategoryHelp(root)
	if err := ValidateHelpArgs(root, []string{"admin", "group", "missing", "--group-id", "group-id", "--help"}); err == nil {
		t.Fatal("unknown direct help route unexpectedly accepted")
	}
	if err := ValidateHelpArgs(root, []string{"config", "--help"}); err != nil {
		t.Fatalf("config --help should retain root help fallback: %v", err)
	}
	if err := ValidateHelpArgs(root, []string{"admin", "group", "remove", "--help"}); err != nil {
		t.Fatalf("known direct help route rejected: %v", err)
	}
}

func TestCategoryHelpNeverRunsHooksOrShowsCurrentValues(t *testing.T) {
	for _, args := range [][]string{{"admin", "--help"}, {"admin", "-h"}, {"adm", "grp", "--help"}, {"help", "admin", "group"}, {"admin", "group", "remove", "--help"}, {"adm", "grp", "rm", "-h"}} {
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
			if strings.Contains(output.String(), "private-") {
				t.Fatal("help leaked values")
			}
		})
	}
}
func TestResourceHelpUsesDeclaredValuesAndOmitsHiddenFlags(t *testing.T) {
	root, admin, remove := helpTestTree()
	remove.Flags().String("mode", "Allow", "mode")
	_ = remove.Flags().SetAnnotation("mode", "tadx.help.choices", []string{"Allow", "Deny"})
	remove.Flags().String("private-flag", "", "hidden")
	_ = remove.Flags().MarkHidden("private-flag")
	admin.AddCommand(&cobra.Command{Use: "hidden", Hidden: true, Run: remove.Run})
	installCategoryHelp(root)
	before := renderedHelp(t, remove)
	_ = remove.Flags().Set("mode", "Deny")
	if got := renderedHelp(t, remove); got != before {
		t.Fatal("parsed value changed reference")
	}
	if !strings.Contains(before, `--mode <Allow|Deny> (default: "Allow")`) {
		t.Fatal(before)
	}
	if strings.Contains(before, "private-flag") || strings.Contains(renderedHelp(t, admin), "hidden") {
		t.Fatal("hidden metadata exposed")
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
		t.Fatalf("root ran %d times", runs)
	}
}
func TestBatchHelpSeparatesScalarRepetitionFromListValues(t *testing.T) {
	root, _, remove := helpTestTree()
	remove.Flags().String("batch-file", "", "batch")
	remove.Annotations = map[string]string{"tadx.batch.file": "true", "tadx.batch.selectors": "group-id"}
	installCategoryHelp(root)
	got := renderedHelp(t, remove)
	for _, want := range []string{"--batch-file <path>", "arrays for lists", "Env/control flags outside rows", "repeat one of --group-id", "1-100 sequential items"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
}

func TestCompletionHelpTreatsUnregisteredUtilityAsExecutable(t *testing.T) {
	root := &cobra.Command{Use: "tadx"}
	root.PersistentFlags().Bool("json", false, "JSON")
	completion := NewCompletion(root)
	root.AddCommand(completion)
	installCategoryHelp(root)
	got := renderedHelp(t, completion)
	for _, want := range []string{"Usage: tadx completion <bash|zsh|fish|powershell>", "Writes a shell script to stdout (not JSON)", "Out-String | Invoke-Expression"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "--json") || strings.Contains(got, "--full") {
		t.Fatal("completion help advertises ineffective output flags")
	}
	root.SetArgs([]string{"completion", "powershell", "-h"})
	var out bytes.Buffer
	root.SetOut(&out)
	completion.SetOut(&out)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if out.String() != got {
		t.Fatal("completion argument changed reference")
	}
}

func TestSharedReferenceNotesCoverAcceptedSetupMembershipAndCompletionGuidance(t *testing.T) {
	tests := []struct {
		path string
		want []string
	}{
		{path: "auth check", want: []string{"env add/update", "never put PAT values in config"}},
		{path: "admin user inspect", want: []string{"--id", "--name", "--username", "alias"}},
		{path: "admin group inspect", want: []string{"provider-reported", "Embedded Analytics", "Cloud+"}},
		{path: "admin group update", want: []string{"--set-members replaces all direct members", "--member-id requires --set-members", "clears membership"}},
		{path: "completion", want: []string{"stdout", "bash -n", "scriptblock"}},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			root := &cobra.Command{Use: "tadx"}
			parent := root
			for _, part := range strings.Fields(test.path) {
				child := &cobra.Command{Use: part}
				parent.AddCommand(child)
				parent = child
			}
			text := strings.Join(referenceActionNotes(parent), "\n")
			for _, want := range test.want {
				if !strings.Contains(text, want) {
					t.Errorf("notes missing %q: %s", want, text)
				}
			}
		})
	}
}

func TestContentPublicationHelpExplainsNoWaitAndFlowSync(t *testing.T) {
	for _, test := range []struct {
		resource string
		want     string
		avoid    string
	}{
		{resource: "workbook", want: "--no-wait: one single/batch status command", avoid: "flow publication is synchronous"},
		{resource: "flow", want: "Flow publication can complete synchronously", avoid: "flow publication is synchronous;"},
	} {
		t.Run(test.resource, func(t *testing.T) {
			resource := &cobra.Command{Use: test.resource}
			publish := &cobra.Command{Use: "publish"}
			resource.AddCommand(publish)
			var output bytes.Buffer
			writeCompactContentNotes(&output, resource, []*cobra.Command{publish})
			if !strings.Contains(output.String(), test.want) || strings.Contains(output.String(), test.avoid) {
				t.Fatalf("publication notes = %s", output.String())
			}
		})
	}
}
