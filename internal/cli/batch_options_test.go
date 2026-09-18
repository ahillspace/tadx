package cli

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/batchspec"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/ahillspace/tadx/internal/contentbatch"
	"github.com/spf13/cobra"
)

type batchOptionCall struct {
	ID, Name, Environment string
	Args, Members         []string
	MembersSet, Preview   bool
}

func runBatchOptions(t *testing.T, options batchspec.Options, args []string) (any, []batchOptionCall, error) {
	t.Helper()
	var calls []batchOptionCall
	factory := func(renderer Renderer) *cobra.Command {
		root := &cobra.Command{Use: "tadx", SilenceErrors: true, SilenceUsage: true}
		var input batchOptionCall
		command := &cobra.Command{Use: "change", Annotations: map[string]string{CapabilityAnnotation: "test.change"}}
		command.Flags().StringVar(&input.ID, "id", "", "id")
		command.Flags().StringVar(&input.Name, "name", "", "name")
		command.Flags().StringVar(&input.Environment, "environment", "", "environment")
		command.Flags().StringArrayVar(&input.Members, "member-id", []string{"default"}, "desired members")
		command.Flags().BoolVar(&input.Preview, "preview", false, "preview")
		command.Args = func(cmd *cobra.Command, args []string) error {
			if options.Positional {
				if err := cobra.ExactArgs(1)(cmd, args); err != nil {
					return err
				}
			} else if len(args) != 0 {
				return errors.New("unexpected positionals")
			}
			if input.ID != "" && input.Name != "" {
				return errors.New("id and name are exclusive")
			}
			input.Args = append([]string(nil), args...)
			input.MembersSet = cmd.Flags().Changed("member-id")
			return nil
		}
		command.RunE = func(_ *cobra.Command, _ []string) error {
			calls = append(calls, input)
			if input.ID == "fail" {
				return clierr.WithOutput(input, errors.New("item failed after a confirmed result"))
			}
			return renderer.Render(input)
		}
		root.AddCommand(command)
		return root
	}
	renderer := &batchTestRenderer{}
	root := factory(renderer)
	command, _, _ := root.Find([]string{"change"})
	attachBatchWithOptions(root, command, options, renderer, factory)
	root.SetArgs(append([]string{"change"}, args...))
	err := root.ExecuteContext(context.Background())
	return renderer.value, calls, err
}

func TestBatchAlternativeSelectorsPreserveOrderAndPropertyLists(t *testing.T) {
	options := batchspec.Options{Selectors: []string{"id", "name"}}
	_, calls, err := runBatchOptions(t, options, []string{"--name", "alpha", "--name", "beta", "--environment", "shared", "--member-id", "a", "--member-id", "b", "--preview"})
	if err != nil || len(calls) != 2 {
		t.Fatalf("calls=%v err=%v", calls, err)
	}
	for index, name := range []string{"alpha", "beta"} {
		call := calls[index]
		if call.Name != name || call.Environment != "shared" || !call.Preview || !reflect.DeepEqual(call.Members, []string{"a", "b"}) {
			t.Fatalf("call=%#v", call)
		}
	}
}

func TestBatchFileDuplicateDiagnosticNamesRowsAndNormalizedSelectors(t *testing.T) {
	file := batchFile(t, `{"items":[{"id":" one "},{"id":"one"}]}`)
	_, calls, err := runBatchOptions(t, batchspec.Options{Selectors: []string{"id"}}, []string{"--batch-file", file})
	if err == nil || len(calls) != 0 || !strings.Contains(err.Error(), "batch item 2 duplicates batch item 1") || !strings.Contains(err.Error(), `normalized selectors "id=one"`) {
		t.Fatalf("calls=%v err=%v", calls, err)
	}
}

func TestBatchRejectsMultipleVaryingDimensionsBeforeDispatch(t *testing.T) {
	options := batchspec.Options{Selectors: []string{"id", "name"}}
	_, calls, err := runBatchOptions(t, options, []string{"--id", "a", "--id", "b", "--name", "x", "--name", "y"})
	if err == nil || len(calls) != 0 || !strings.Contains(err.Error(), "one selector dimension") {
		t.Fatalf("calls=%v err=%v", calls, err)
	}
	command := &cobra.Command{}
	command.Flags().StringArray("artifact", []string{"one", "two"}, "native targets")
	command.Flags().StringArray("id", []string{"x", "y"}, "native targets")
	if _, _, err := varyingBatchSelector(command, batchspec.Options{Selectors: []string{"artifact", "id"}}); err == nil {
		t.Fatal("native target dimensions must not bypass ambiguity checks")
	}
}

func TestBatchEmptyArrayOverridesSharedAndDefaultListsOnlyForItsRow(t *testing.T) {
	file := batchFile(t, `{"items":[{"id":"clear","member-id":[]},{"id":"inherit"},{"id":"replace","member-id":["fresh"]}]}`)
	for _, shared := range [][]string{nil, {"--member-id", "shared"}} {
		args := append([]string{"--batch-file", file, "--preview"}, shared...)
		_, calls, err := runBatchOptions(t, batchspec.Options{Selectors: []string{"id"}}, args)
		if err != nil || len(calls) != 3 {
			t.Fatalf("calls=%v err=%v", calls, err)
		}
		if len(calls[0].Members) != 0 || !calls[0].MembersSet || !reflect.DeepEqual(calls[2].Members, []string{"fresh"}) {
			t.Fatalf("empty/replace calls=%#v", calls)
		}
		want := "default"
		if len(shared) != 0 {
			want = "shared"
		}
		if !reflect.DeepEqual(calls[1].Members, []string{want}) {
			t.Fatalf("inherit=%#v", calls[1])
		}
	}
}

func TestBatchPositionalRowsPreserveLiteralArgumentsAndValidateEveryRow(t *testing.T) {
	options := batchspec.Options{Positional: true}
	file := batchFile(t, `{"items":[{"args":["two words"]},{"args":["--literal"]}]}`)
	_, calls, err := runBatchOptions(t, options, []string{"--batch-file", file})
	if err != nil || len(calls) != 2 || calls[0].Args[0] != "two words" || calls[1].Args[0] != "--literal" {
		t.Fatalf("calls=%v err=%v", calls, err)
	}
	for _, body := range []string{
		`{"items":[{"args":["valid"]},{"args":[]}]}`,
		`{"items":[{"args":["valid"]},{"args":["one","two"]}]}`,
		`{"items":[{"args":["valid"]},{"args":[false]}]}`,
		`{"items":[{"args":["valid"]},{"args":null}]}`,
	} {
		_, calls, err := runBatchOptions(t, options, []string{"--batch-file", batchFile(t, body)})
		if err == nil || len(calls) != 0 {
			t.Fatalf("body=%s calls=%v err=%v", body, calls, err)
		}
	}
	_, calls, err = runBatchOptions(t, options, []string{"first", "second"})
	if err != nil || len(calls) != 2 || calls[1].Args[0] != "second" {
		t.Fatalf("calls=%v err=%v", calls, err)
	}
}

func TestBatchPartialFailurePreservesConfirmedResults(t *testing.T) {
	file := batchFile(t, `{"items":[{"id":"alpha"},{"id":"fail"},{"id":"beta"}]}`)
	value, calls, err := runBatchOptions(t, batchspec.Options{Selectors: []string{"id"}}, []string{"--batch-file", file})
	if err == nil || len(calls) != 3 {
		t.Fatalf("calls=%v err=%v", calls, err)
	}
	batch := value.(contentbatch.Output)
	if batch.Succeeded != 2 || batch.Failed != 1 || batch.Items[1].Result == nil || calls[0].ID != "alpha" || calls[2].ID != "beta" {
		t.Fatalf("batch=%#v calls=%#v", batch, calls)
	}
}

func TestBatchResultRetainsNormalizedSelectorsInsteadOfOrdinals(t *testing.T) {
	file := batchFile(t, `{"items":[{"id":" one ","member-id":["Allow"]},{"id":"two","member-id":["Deny"]}]}`)
	value, calls, err := runBatchOptions(t, batchspec.Options{Selectors: []string{"id"}}, []string{"--batch-file", file})
	if err != nil || len(calls) != 2 {
		t.Fatalf("calls=%v err=%v", calls, err)
	}
	out, ok := value.(contentbatch.Output)
	if !ok || len(out.Items) != 2 {
		t.Fatalf("output=%#v", value)
	}
	if got := out.Items[0].Selector; got != "id=one" {
		t.Fatalf("first selector = %q, want normalized selector", got)
	}
	if got := out.Items[1].Selector; got != "id=two" {
		t.Fatalf("second selector = %q, want normalized selector", got)
	}
	if strings.Contains(out.Items[0].Selector, "member-id=") || strings.Contains(out.Items[1].Selector, "member-id=") {
		t.Fatal("batch result selector leaked unrelated row flags")
	}
}

func TestBatchEnvironmentOverridesRejectedBeforeDispatch(t *testing.T) {
	file := batchFile(t, `{"items":[{"id":"alpha"},{"id":"beta","environment":"other"}]}`)
	_, calls, err := runBatchOptions(t, batchspec.Options{Selectors: []string{"id"}}, []string{"--environment", "shared", "--batch-file", file})
	if err == nil || len(calls) != 0 || !strings.Contains(err.Error(), "--environment must be selected on the command") {
		t.Fatalf("calls=%v err=%v", calls, err)
	}
}

func TestBatchPropertyValuesDoNotConsumeActionSelectionBudget(t *testing.T) {
	command := &cobra.Command{}
	command.Flags().StringArray("member-id", nil, "desired members")
	members := make([]string, 150)
	for i := range members {
		members[i] = "member"
	}
	encoded, err := json.Marshal(members)
	if err != nil {
		t.Fatal(err)
	}
	_, count, err := batchRowArguments(command, map[string]json.RawMessage{"member-id": encoded}, nil, batchspec.Options{})
	if err != nil || count != 1 {
		t.Fatalf("property count=%d err=%v", count, err)
	}
	command.Flags().StringArray("capability", nil, "native rule actions")
	_, _, err = batchRowArguments(command, map[string]json.RawMessage{"capability": encoded}, nil, batchspec.Options{NativeSelections: []string{"capability"}})
	if err == nil {
		t.Fatal("native rule actions escaped the 100-selection bound")
	}
}

func TestBatchPermissionArraysRequireNativeListFlags(t *testing.T) {
	command := &cobra.Command{}
	command.Flags().String("principal-id", "", "principal")
	command.Flags().StringSlice("capability", nil, "permission rules")
	options := batchspec.Options{Selectors: []string{"principal-id"}, NativeSelections: []string{"capability"}}
	row, count, err := batchRowArguments(command, map[string]json.RawMessage{
		"principal-id": json.RawMessage(`"user-1"`),
		"capability":   json.RawMessage(`["Read","Write"]`),
	}, nil, options)
	if err != nil || count != 2 || !reflect.DeepEqual(row.argv, []string{"--capability=Read", "--capability=Write", "--principal-id=user-1"}) {
		t.Fatalf("native list row=%#v count=%d err=%v", row, count, err)
	}
	_, _, err = batchRowArguments(command, map[string]json.RawMessage{
		"principal-id": json.RawMessage(`["user-1","user-2"]`),
	}, nil, options)
	if err == nil || !strings.Contains(err.Error(), "--principal-id is not a list; use separate batch items for scalar selectors") {
		t.Fatalf("scalar array error=%v", err)
	}
}
