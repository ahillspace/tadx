package capability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/ahillspace/tadx/internal/output"
	"os"
	"testing"
)

type getSource struct {
	item Capability
	ok   bool
}

func (s getSource) Get(context.Context, string) (Capability, bool) {
	return s.item, s.ok
}

type getTestAction struct{ source getSource }

func newGetAction(source getSource) getTestAction { return getTestAction{source: source} }

func (a getTestAction) Execute(ctx context.Context, input GetInput) (GetOutput, error) {
	item, ok := a.source.Get(ctx, input.ID)
	return getFromItem(input, item, ok, false)
}

func TestExecuteReturnsExactCapability(t *testing.T) {
	want := Capability{ID: "capability.get", Command: "capability get", Owner: "cli"}
	got, err := newGetAction(getSource{item: want, ok: true}).Execute(context.Background(), GetInput{ID: want.ID})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.Capability.ID != want.ID || got.Capability.Command != want.Command {
		t.Fatalf("Capability = %#v, want %#v", got.Capability, want)
	}
}

func TestExecuteReportsMutationExecutionState(t *testing.T) {
	item := Capability{ID: "workbook.publish", RemoteMutation: true, ImplementationState: "implemented"}
	for _, test := range []struct {
		enabled bool
		want    bool
	}{{false, false}, {true, true}} {
		got, err := getFromItem(GetInput{ID: item.ID}, item, true, test.enabled)
		if err != nil {
			t.Fatal(err)
		}
		if got.Capability.ExecutionEnabled != test.want {
			t.Fatalf("enabled = %t, execution_enabled = %t", test.enabled, got.Capability.ExecutionEnabled)
		}
	}
}

func TestExecuteRejectsMissingID(t *testing.T) {
	_, err := newGetAction(getSource{}).Execute(context.Background(), GetInput{})
	if !errors.Is(err, ErrIDRequired) {
		t.Fatalf("error = %v, want ErrIDRequired", err)
	}
}

func TestExecuteRejectsUnknownID(t *testing.T) {
	_, err := newGetAction(getSource{}).Execute(context.Background(), GetInput{ID: "missing"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestOutputGoldenIncludesExecutionGuidance(t *testing.T) {
	item := Capability{
		ID: "workbook.publish", Domain: "workbook", Verb: "publish", Surface: "tadx content workbook publish",
		Outcome: "Publish one workbook, or preview the operation.", OperationType: "deliver", Owner: "cli", Disposition: "ship",
		EvidenceLevel: "docs-only", VerificationReadiness: "ready", ImplementationState: "planned",
		Selectors: []string{"Workbook LUID; explicit target"}, Availability: "Cloud / Server",
		SafetyGuard: "Exact target; optional --preview", ArtifactEffect: "Read / publish",
		UpstreamOperation: "POST /api/{version}/sites/{site-id}/workbooks", Evidence: "Official REST documentation",
		Validation: "Captured contract test required", RemoteMutation: true, SupportsPreview: true,
	}
	result, err := newGetAction(getSource{item: item, ok: true}).Execute(context.Background(), GetInput{ID: item.ID})
	if err != nil {
		t.Fatal(err)
	}
	var rendered bytes.Buffer
	if err := output.Render(&rendered, result); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/get-output.toon")
	if err != nil {
		t.Fatal(err)
	}
	wantText := string(want)
	if rendered.String() != wantText {
		t.Fatalf("golden mismatch\nwant:\n%s\ngot:\n%s", want, rendered.String())
	}
}

func TestFullOutputRetainsDelegatedContract(t *testing.T) {
	item := Capability{
		ID: "pulse.metric.values", Domain: "pulse", Resource: "metric", Verb: "values",
		Surface: "Tableau MCP", Outcome: "Read metric values.", OperationType: "inspect", Owner: "tableau-mcp",
		Disposition: "delegated", ImplementationState: "external/delegated", Command: "",
		Selectors: []string{"Metric LUID"}, Availability: "Tableau Cloud", SafetyGuard: "Use MCP", Evidence: "contract",
	}
	result, err := newGetAction(getSource{item: item, ok: true}).Execute(context.Background(), GetInput{ID: item.ID})
	if err != nil {
		t.Fatal(err)
	}
	full, err := json.Marshal(result.FullOutput())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(full, []byte(`"disposition":"delegated"`)) || !bytes.Contains(full, []byte(`"selectors":["Metric LUID"]`)) {
		t.Fatalf("full delegated output = %s", full)
	}
	compact, err := json.Marshal(result.CompactOutput())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(compact, []byte(`"disposition":"delegated"`)) || !bytes.Contains(compact, []byte(`"disposition":"Out of scope"`)) {
		t.Fatalf("compact delegated output = %s", compact)
	}
	if result.Capability.Disposition != "delegated" {
		t.Fatal("compact projection changed the full result")
	}
	direct, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(full, direct) {
		t.Fatalf("full projection changed field order or values: %s != %s", full, direct)
	}
}
