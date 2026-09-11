package inspect_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	definitionget "github.com/ahillspace/tadx/actions/pulse/definition/inspect"
	"github.com/ahillspace/tadx/internal/errs"
	render "github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/readsource"
)

func TestOutputGolden(t *testing.T) {
	output := definitionget.Output{
		Status: "found", Environment: "dev", Site: "sales",
		Definition: definitionget.Definition{LUID: "definition-1", Name: "Revenue", Description: "Recognized revenue", DatasourceLUID: "datasource-1", MeasureField: "Sales", Aggregation: "AGGREGATION_SUM", TimeDimension: "Order Date", Temporality: "TEMPORALITY_OVER_TIME", AllowedDimensions: []string{"Region"}, AllowedGranularities: []string{"GRANULARITY_BY_MONTH"}, Configuration: map[string]any{"version": "1"}},
		RequestID:  "request-1", Help: []string{"tadx pulse definition pull --id definition-1"}, Source: &readsource.Metadata{Mode: readsource.Cache, ObservedAt: "2026-09-04T12:00:00Z", Coverage: readsource.CoverageComplete, GenerationID: "generation-1", GenerationCreated: "2026-09-04T11:00:00Z"},
	}
	assertGolden(t, "compact.toon", output, false)
	assertGolden(t, "full.toon", output, true)
}

func assertGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, value, render.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, buffer.Bytes())
	}
}

type reader struct {
	definition definitionget.Definition
	luid       string
	calls      int
}

func (r *reader) GetDefinition(_ context.Context, luid string) (definitionget.Definition, error) {
	r.calls++
	r.luid = luid
	return r.definition, nil
}

func TestInspectRequiresAndVerifiesExactLUID(t *testing.T) {
	r := &reader{definition: definitionget.Definition{LUID: "definition-1", Name: "Revenue", DatasourceLUID: "datasource-1", MeasureField: "Sales"}}
	output, err := definitionget.New(r).Execute(context.Background(), definitionget.Input{Environment: "dev", Site: "sales", LUID: " definition-1 "})
	if err != nil || r.luid != "definition-1" || output.Definition.LUID != "definition-1" {
		t.Fatalf("luid=%q output=%#v err=%v", r.luid, output, err)
	}
	full := output.FullOutput().(definitionget.FullResult)
	if full.Definition.MeasureField != "Sales" {
		t.Fatalf("full=%#v", full)
	}
}

func TestInspectRejectsMissingOrMismatchedLUID(t *testing.T) {
	for _, test := range []struct {
		name   string
		input  definitionget.Input
		result definitionget.Definition
		kind   errs.Kind
	}{
		{name: "missing", input: definitionget.Input{}, kind: errs.KindUsage},
		{name: "mismatch", input: definitionget.Input{LUID: "definition-1"}, result: definitionget.Definition{LUID: "definition-2"}, kind: errs.KindOperation},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := definitionget.New(&reader{definition: test.result}).Execute(context.Background(), test.input)
			var structured *errs.Error
			if !errors.As(err, &structured) || structured.Kind != test.kind {
				t.Fatalf("error=%#v", err)
			}
		})
	}
}
