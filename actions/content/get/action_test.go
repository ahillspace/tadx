package get_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	get "github.com/ahillspace/tadx/actions/content/get"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type resolver struct{ result get.Result }

func (r resolver) Get(context.Context, get.Input) (get.Result, error) { return r.result, nil }

type stubResolver struct{ called bool }

func (r *stubResolver) Get(context.Context, get.Input) (get.Result, error) {
	r.called = true
	return get.Result{}, nil
}

func TestActionRejectsWhitespaceOnlyLUIDBeforeResolver(t *testing.T) {
	stub := &stubResolver{}
	_, err := get.New(stub).Execute(context.Background(), get.Input{Kind: "workbook", LUID: " "})
	if err == nil {
		t.Fatal("Execute() error = nil")
	}
	var structured *errs.Error
	if !errors.As(err, &structured) {
		t.Fatalf("Execute() error = %v, want *errs.Error", err)
	}
	if structured.Kind != errs.KindUsage {
		t.Errorf("Kind = %q, want %q", structured.Kind, errs.KindUsage)
	}
	if stub.called {
		t.Error("resolver.Get was called for a whitespace-only selector")
	}
}

func TestActionCompactAndFullOutput(t *testing.T) {
	action := get.New(resolver{result: get.Result{Item: get.Item{LUID: "wb-1", Kind: "workbook", Name: "Finance", ProjectPath: "Ops", Owner: "alice", ModifiedAt: "2026-09-01T10:00:00Z", URL: "https://tableau.example/views/Finance"}, Warnings: []string{"resource-specific fields are bounded"}}})
	for _, test := range []struct {
		name, golden string
		full         bool
	}{{"compact", "testdata/compact.toon", false}, {"full", "testdata/full.toon", true}} {
		t.Run(test.name, func(t *testing.T) {
			value, err := action.Execute(context.Background(), get.Input{Kind: "workbook", LUID: "wb-1"})
			if err != nil {
				t.Fatal(err)
			}
			assertGolden(t, test.golden, value, test.full)
		})
	}
}

func TestActionRejectsMixedLUIDAndNameBeforeResolution(t *testing.T) {
	_, err := get.New(resolver{}).Execute(context.Background(), get.Input{Kind: "workbook", LUID: "wb-1", Name: "Finance"})
	if err == nil {
		t.Fatal("Execute() error = nil")
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
