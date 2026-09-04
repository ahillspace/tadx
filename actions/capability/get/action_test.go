package get_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	capabilityget "github.com/ahillspace/tadx/actions/capability/get"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type source struct {
	item capabilityget.Capability
	ok   bool
}

func (s source) Get(context.Context, string) (capabilityget.Capability, bool) {
	return s.item, s.ok
}

func TestExecuteReturnsExactCapability(t *testing.T) {
	want := capabilityget.Capability{ID: "capability.get", Command: "capability get", Owner: "cli"}
	got, err := capabilityget.New(source{item: want, ok: true}).Execute(context.Background(), capabilityget.Input{ID: want.ID})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.Capability.ID != want.ID || got.Capability.Command != want.Command {
		t.Fatalf("Capability = %#v, want %#v", got.Capability, want)
	}
}

func TestExecuteReportsMutationExecutionState(t *testing.T) {
	item := capabilityget.Capability{ID: "workbook.publish", RemoteMutation: true, ImplementationState: "implemented"}
	action := capabilityget.New(source{item: item, ok: true})
	for _, test := range []struct {
		enabled bool
		want    bool
	}{{false, false}, {true, true}} {
		got, err := action.Execute(context.Background(), capabilityget.Input{ID: item.ID, MutationsEnabled: test.enabled})
		if err != nil {
			t.Fatal(err)
		}
		if got.Capability.ExecutionEnabled != test.want {
			t.Fatalf("enabled = %t, execution_enabled = %t", test.enabled, got.Capability.ExecutionEnabled)
		}
	}
}

func TestExecuteGuardsUnconfiguredSourceWithoutPanic(t *testing.T) {
	for name, action := range map[string]*capabilityget.Action{
		"nil action": nil,
		"nil source": capabilityget.New(nil),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := action.Execute(context.Background(), capabilityget.Input{ID: "capability.get"})
			var structured *errs.Error
			if !errors.As(err, &structured) || structured.Kind != errs.KindRuntime || structured.ID != "capability.get.unconfigured" {
				t.Fatalf("Execute() error = %#v", err)
			}
		})
	}
}

func TestExecuteRejectsMissingID(t *testing.T) {
	_, err := capabilityget.New(source{}).Execute(context.Background(), capabilityget.Input{})
	if !errors.Is(err, capabilityget.ErrIDRequired) {
		t.Fatalf("error = %v, want ErrIDRequired", err)
	}
}

func TestExecuteRejectsUnknownID(t *testing.T) {
	_, err := capabilityget.New(source{}).Execute(context.Background(), capabilityget.Input{ID: "missing"})
	if !errors.Is(err, capabilityget.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestOutputGoldenIncludesExecutionGuidance(t *testing.T) {
	item := capabilityget.Capability{
		ID: "workbook.publish", Domain: "workbook", Verb: "publish", Surface: "tadx content workbook publish",
		Outcome: "Preview and publish one workbook.", OperationType: "deliver", Owner: "cli", Disposition: "ship",
		EvidenceLevel: "docs-only", VerificationReadiness: "ready", ImplementationState: "planned",
		Selectors: []string{"Workbook LUID; explicit target"}, Availability: "Cloud / Server",
		SafetyGuard: "Preview by default; requires --apply", ArtifactEffect: "Read / publish",
		UpstreamOperation: "POST /api/{version}/sites/{site-id}/workbooks", Evidence: "Official REST documentation",
		Validation: "Captured contract test required", RemoteMutation: true, RequiresApply: true,
	}
	result, err := capabilityget.New(source{item: item, ok: true}).Execute(context.Background(), capabilityget.Input{ID: item.ID})
	if err != nil {
		t.Fatal(err)
	}
	var rendered bytes.Buffer
	if err := output.Render(&rendered, result); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/output.toon")
	if err != nil {
		t.Fatal(err)
	}
	wantText := strings.TrimSuffix(string(want), "\n")
	if rendered.String() != wantText {
		t.Fatalf("golden mismatch\nwant:\n%s\ngot:\n%s", want, rendered.String())
	}
}
