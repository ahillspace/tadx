package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/ahillspace/tadx/internal/contentbatch"
	"github.com/spf13/cobra"
)

type batchTestRenderer struct{ value any }

func (r *batchTestRenderer) Render(v any) error { r.value = v; return nil }

func batchTestRoot(renderer Renderer, calls *[]string, enabled bool) *cobra.Command {
	root := &cobra.Command{Use: "tadx", SilenceUsage: true, SilenceErrors: true}
	var id, env, mode string
	var preview bool
	root.PersistentFlags().String("config", "", "config")
	cmd := &cobra.Command{Use: "change", Annotations: map[string]string{CapabilityAnnotation: "test.change"}, Args: func(_ *cobra.Command, args []string) error {
		if len(args) != 0 || id == "" {
			return errors.New("exact id required")
		}
		return nil
	}, RunE: func(_ *cobra.Command, _ []string) error {
		if !enabled && !preview {
			return errors.New("mutation disabled")
		}
		*calls = append(*calls, id+":"+env+":"+mode)
		value := map[string]any{"id": id, "preview": preview}
		if id == "fail" {
			return clierr.WithOutput(value, errors.New("verification unavailable"))
		}
		return renderer.Render(value)
	}}
	cmd.Flags().StringVar(&id, "id", "", "id")
	cmd.Flags().StringVar(&env, "environment", "", "environment")
	cmd.Flags().StringVar(&mode, "mode", "", "mode")
	cmd.Flags().BoolVar(&preview, "preview", false, "preview")
	root.AddCommand(cmd)
	return root
}

func runBatchFixture(t *testing.T, args []string, enabled bool) (any, []string, error) {
	t.Helper()
	renderer := &batchTestRenderer{}
	var calls []string
	factory := func(r Renderer) *cobra.Command { return batchTestRoot(r, &calls, enabled) }
	root := factory(renderer)
	cmd, _, _ := root.Find([]string{"change"})
	attachBatch(root, cmd, "id", renderer, factory)
	root.SetArgs(append([]string{"change"}, args...))
	err := root.ExecuteContext(context.Background())
	return renderer.value, calls, err
}

func batchFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "items.json")
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRepeatedIDsRunInOrderWithPartialEvidence(t *testing.T) {
	value, calls, err := runBatchFixture(t, []string{"--id", "one", "--id", "fail", "--id", "three", "--environment", "dev"}, true)
	if err == nil || !clierr.IsRendered(err) {
		t.Fatalf("error = %v", err)
	}
	if !reflect.DeepEqual(calls, []string{"one:dev:", "fail:dev:", "three:dev:"}) {
		t.Fatalf("calls = %v", calls)
	}
	out := value.(contentbatch.Output)
	if out.Succeeded != 2 || out.Failed != 1 || out.Items[1].Result == nil {
		t.Fatalf("out = %#v", out)
	}
}

func TestBatchFileOverridesItemSettingsAndRetainsContext(t *testing.T) {
	p := batchFile(t, `{"items":[{"id":"one","mode":"Allow"},{"id":"two","mode":"Deny"}]}`)
	value, calls, err := runBatchFixture(t, []string{"--batch-file", p, "--environment", "dev", "--preview"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"one:dev:Allow", "two:dev:Deny"}) {
		t.Fatalf("calls = %v", calls)
	}
	for _, item := range value.(contentbatch.Output).Items {
		if item.Result.(map[string]any)["preview"] != true {
			t.Fatal("preview lost")
		}
	}
}

func TestBatchFileRejectsInvalidSyntaxBeforeAnyItem(t *testing.T) {
	for _, body := range []string{
		`{"items":[{"id":"one"},{"id":"two","unknown":1}]}`,
		`{"items":[{"id":"one"},{"id":"two","preview":false}]}`,
		`{"items":[{"id":"one"},{"id":"two","environment":"other"}]}`,
		`{"items":[{"id":"one","id":"two"}]}`,
		`{"items":[{"id":"one"},{"id":"one"}]}`,
		`{"items":[{"id":"one"},{}]}`,
		`{"items":[{"id":"one"}],"extra":true}`,
		`{"items":[{"id":"one"}]} {}`,
	} {
		t.Run(body, func(t *testing.T) {
			_, calls, err := runBatchFixture(t, []string{"--batch-file", batchFile(t, body)}, true)
			if err == nil || len(calls) != 0 {
				t.Fatalf("error=%v calls=%v", err, calls)
			}
		})
	}
}

func TestBatchPreviewFalseDoesNotBypassPolicy(t *testing.T) {
	_, calls, err := runBatchFixture(t, []string{"--batch-file", batchFile(t, `{"items":[{"id":"one"}]}`), "--preview=false"}, false)
	if err == nil || len(calls) != 0 {
		t.Fatalf("error=%v calls=%v", err, calls)
	}
}

func TestSingleIDKeepsSingleResult(t *testing.T) {
	value, calls, err := runBatchFixture(t, []string{"--id", "one"}, true)
	if err != nil || len(calls) != 1 {
		t.Fatalf("error=%v calls=%v", err, calls)
	}
	if _, ok := value.(map[string]any); !ok {
		t.Fatalf("single shape changed: %T", value)
	}
}

func TestBatchCountsSharedRepeatedFlagsAfterOverrides(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringSlice("capability", nil, "capabilities")
	if err := cmd.Flags().Set("capability", "Read,Write,Delete"); err != nil {
		t.Fatal(err)
	}
	_, count, err := batchArguments(cmd, nil)
	if err != nil || count != 3 {
		t.Fatalf("shared selections: count=%d error=%v", count, err)
	}
	_, count, err = batchArguments(cmd, map[string]json.RawMessage{"capability": json.RawMessage(`["Read"]`)})
	if err != nil || count != 1 {
		t.Fatalf("overridden selections: count=%d error=%v", count, err)
	}
}
