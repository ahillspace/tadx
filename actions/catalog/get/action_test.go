package get_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	get "github.com/ahillspace/tadx/actions/catalog/get"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type source struct{ result get.Result }

func (s source) Get(context.Context, get.Input) (get.Result, error) { return s.result, nil }

type failingSource struct{ err error }

func (f failingSource) Get(context.Context, get.Input) (get.Result, error) {
	return get.Result{}, f.err
}

type stubSource struct{ called bool }

func (s *stubSource) Get(context.Context, get.Input) (get.Result, error) {
	s.called = true
	return get.Result{}, nil
}

type notFoundErr struct{}

func (notFoundErr) Error() string               { return "catalog record was not found" }
func (notFoundErr) CatalogRecordNotFound() bool { return true }

type scopeUnavailableErr struct{}

func (scopeUnavailableErr) Error() string                 { return "catalog scope unavailable" }
func (scopeUnavailableErr) CatalogScopeUnavailable() bool { return true }

func assertActionError(t *testing.T, err error, wantID string, wantKind errs.Kind, wantSummary string) {
	t.Helper()
	if err == nil {
		t.Fatal("Execute() error = nil")
	}
	var structured *errs.Error
	if !errors.As(err, &structured) {
		t.Fatalf("Execute() error = %v, want *errs.Error", err)
	}
	if structured.ID != wantID {
		t.Errorf("ID = %q, want %q", structured.ID, wantID)
	}
	if structured.Kind != wantKind {
		t.Errorf("Kind = %q, want %q", structured.Kind, wantKind)
	}
	if structured.Summary != wantSummary {
		t.Errorf("Summary = %q, want %q", structured.Summary, wantSummary)
	}
}

func TestActionMapsNotFoundToUsage(t *testing.T) {
	action := get.New(failingSource{err: notFoundErr{}})
	_, err := action.Execute(context.Background(), get.Input{Environment: "production", Site: "marketing", SiteResolved: true, Kind: "workbook", Name: "Typo"})
	assertActionError(t, err, "catalog.get.not_found", errs.KindUsage, "No catalog record matched the selector.")
}

func TestActionMapsScopeUnavailableToUsage(t *testing.T) {
	action := get.New(failingSource{err: scopeUnavailableErr{}})
	_, err := action.Execute(context.Background(), get.Input{Environment: "production", Site: "marketing", SiteResolved: true, Kind: "workbook", Name: "Finance"})
	assertActionError(t, err, "catalog.get.scope_unavailable", errs.KindUsage, "The requested content kind is not in the current catalog generation; refresh that scope.")
}

func TestActionRejectsWhitespaceOnlyLUIDBeforeStore(t *testing.T) {
	stub := &stubSource{}
	action := get.New(stub)
	_, err := action.Execute(context.Background(), get.Input{Environment: "production", Site: "marketing", SiteResolved: true, LUID: " "})
	assertActionError(t, err, "catalog.get.usage", errs.KindUsage, "Catalog get requires a resolved site and a LUID or exact kind and name.")
	if stub.called {
		t.Error("source.Get was called for a whitespace-only selector")
	}
}

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
