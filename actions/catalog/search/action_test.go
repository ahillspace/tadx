package search_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	search "github.com/ahillspace/tadx/actions/catalog/search"
	"github.com/ahillspace/tadx/internal/output"
)

type source struct{ result search.Result }

func (s source) Search(context.Context, search.Input) (search.Result, error) { return s.result, nil }

func TestActionReturnsNormalizedBoundedSearchEnvelope(t *testing.T) {
	action := search.New(source{result: search.Result{
		Page:         search.Page{Returned: 1, Total: 2, Limit: 1, NextCursor: "1"},
		GenerationID: "generation-1", Environment: "production", Site: "marketing", GeneratedAt: "2026-08-30T00:00:00Z", Stale: false,
		Items: []search.Item{{LUID: "wb-1", Kind: "workbook", Name: "Finance", ProjectPath: "Ops"}},
	}})
	output, err := action.Execute(context.Background(), search.Input{Environment: "production", Text: "Finance", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if output.Page.Total != 2 || output.Generation.Environment != "production" || output.Generation.Site != "marketing" || len(output.Items) != 1 || len(output.Help) != 1 || !strings.Contains(output.Help[0], "<luid>") {
		t.Fatalf("output = %#v", output)
	}
}

func TestActionGoldenOutputShape(t *testing.T) {
	value := search.Output{
		Page:       search.Page{Returned: 1, Total: 2, Limit: 1, NextCursor: "1"},
		Generation: search.Generation{ID: "generation-1", Environment: "production", Site: "marketing", GeneratedAt: "2026-08-30T00:00:00Z"},
		Items:      []search.Item{{LUID: "wb-1", Kind: "workbook", Name: "Finance", ProjectPath: "Ops"}},
		Help:       []string{"tadx capability get <luid>"},
	}
	var actual bytes.Buffer
	if err := output.Render(&actual, value); err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile("testdata/output.toon")
	if err != nil {
		t.Fatal(err)
	}
	expected = bytes.TrimSuffix(expected, []byte("\n"))
	if !bytes.Equal(actual.Bytes(), expected) {
		t.Fatalf("golden mismatch\nexpected:\n%s\nactual:\n%s", expected, actual.Bytes())
	}
}
