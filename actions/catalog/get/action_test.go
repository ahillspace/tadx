package get_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	get "github.com/ahillspace/tadx/actions/catalog/get"
	"github.com/ahillspace/tadx/internal/output"
)

type source struct{ result get.Result }

func (s source) Get(context.Context, get.Input) (get.Result, error) { return s.result, nil }

func TestActionCompactAndFullOutput(t *testing.T) {
	action := get.New(source{result: get.Result{
		Generation: get.Generation{ID: "generation-1", Environment: "production", Site: "marketing", GeneratedAt: "2026-09-01T12:00:00Z", Stale: true},
		Item:       get.Item{LUID: "wb-1", Kind: "workbook", Name: "Finance", ProjectPath: "Ops", Owner: "alice"},
		Warnings:   []string{"catalog generation is older than 12 hours"},
	}})
	for _, test := range []struct {
		name, golden string
		full         bool
	}{{"compact", "testdata/compact.toon", false}, {"full", "testdata/full.toon", true}} {
		t.Run(test.name, func(t *testing.T) {
			value, err := action.Execute(context.Background(), get.Input{Environment: "production", Site: "marketing", SiteResolved: true, LUID: "wb-1"})
			if err != nil {
				t.Fatal(err)
			}
			assertGolden(t, test.golden, value, test.full)
		})
	}
}

func assertGolden(t *testing.T, path string, value any, full bool) {
	t.Helper()
	var actual bytes.Buffer
	if err := output.RenderWithOptions(&actual, value, output.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	expected = bytes.TrimSuffix(expected, []byte("\n"))
	if !bytes.Equal(actual.Bytes(), expected) {
		t.Fatalf("golden mismatch\nexpected:\n%s\nactual:\n%s", expected, actual.Bytes())
	}
}
