package inspect_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	workbookget "github.com/ahillspace/tadx/actions/workbook/inspect"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
)

type erroringResolver struct{ err error }

func (r erroringResolver) ResolveWorkbook(context.Context, identity.Selector) (workbookget.Workbook, error) {
	return workbookget.Workbook{}, r.err
}

func TestActionClassifiesResolutionFailures(t *testing.T) {
	cases := []struct {
		name   string
		kind   identity.ResolutionErrorKind
		wantID string
	}{
		{"ambiguous", identity.ResolutionAmbiguous, "workbook.inspect.ambiguous"},
		{"not found", identity.ResolutionNotFound, "workbook.inspect.not_found"},
		{"invalid selector", identity.ResolutionInvalidSelector, "workbook.inspect.usage"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cause := &identity.ResolutionError{Kind: tc.kind, Selector: identity.Selector{Name: "Finance", ProjectPath: "Ops"}}
			input := workbookget.Input{Environment: "dev", Site: "site", Selector: identity.Selector{Name: "Finance", ProjectPath: "Ops"}}
			_, err := workbookget.New(erroringResolver{err: cause}).Execute(context.Background(), input)
			var structured *errs.Error
			if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || structured.ID != tc.wantID {
				t.Fatalf("error = %#v", structured)
			}
		})
	}
}

func TestActionClassifiesOpaqueResolverErrorAsOperation(t *testing.T) {
	input := workbookget.Input{Environment: "dev", Site: "site", Selector: identity.Selector{Name: "Finance", ProjectPath: "Ops"}}
	_, err := workbookget.New(erroringResolver{err: errors.New("upstream unavailable")}).Execute(context.Background(), input)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindOperation || structured.ID != "workbook.inspect.resolve" {
		t.Fatalf("error = %#v", structured)
	}
}

type resolver struct {
	workbook workbookget.Workbook
	selector identity.Selector
}

func (r *resolver) ResolveWorkbook(_ context.Context, selector identity.Selector) (workbookget.Workbook, error) {
	r.selector = selector
	return r.workbook, nil
}

func TestOutputGolden(t *testing.T) {
	output := workbookget.Output{
		Status: "found", Environment: "dev", Site: "sandbox", RequestID: "request-1",
		Workbook: workbookget.Workbook{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectPath: "Department/Ops", ContentURL: "Finance", UpdatedAt: "2026-09-01T00:00:00Z", Description: "Finance reporting", OwnerLUID: "user-1", CreatedAt: "2026-08-01T00:00:00Z", Tags: []string{"finance"}},
		Help:     []string{"tadx content workbook pull --id wb-1"},
	}
	assertGolden(t, "compact.toon", output, false)
	assertGolden(t, "full.toon", output, true)
}

func TestActionGetsExactWorkbookAndBoundsTags(t *testing.T) {
	tags := make([]string, 60)
	for index := range tags {
		tags[index] = fmt.Sprintf("tag-%02d", index)
	}
	r := &resolver{workbook: workbookget.Workbook{LUID: "wb-1", Name: "Finance", ProjectPath: "Department/Ops", Tags: tags, RequestID: "request-1"}}
	output, err := workbookget.New(r).Execute(context.Background(), workbookget.Input{Environment: "dev", Selector: identity.Selector{Name: "Finance", ProjectPath: "Department/Ops"}})
	if err != nil {
		t.Fatal(err)
	}
	if r.selector.Name != "Finance" || r.selector.ProjectPath != "Department/Ops" {
		t.Fatalf("selector = %#v", r.selector)
	}
	if output.RequestID != "request-1" || output.CompactOutput().(workbookget.CompactResult).Workbook.ProjectPath != "Department/Ops" {
		t.Fatalf("output = %#v", output)
	}
	full := output.FullOutput().(workbookget.FullResult)
	if len(full.Workbook.Tags) != 50 || full.Workbook.TagsOmitted != 10 {
		t.Fatalf("full = %#v", full)
	}
}

type spyResolver struct {
	called   bool
	workbook workbookget.Workbook
}

func (r *spyResolver) ResolveWorkbook(_ context.Context, _ identity.Selector) (workbookget.Workbook, error) {
	r.called = true
	return r.workbook, nil
}

func TestActionRejectsConflictingSelector(t *testing.T) {
	r := &spyResolver{}
	input := workbookget.Input{Selector: identity.Selector{LUID: "wb-1", Name: "Finance"}}
	_, err := workbookget.New(r).Execute(context.Background(), input)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || structured.ID != "workbook.inspect.usage" {
		t.Fatalf("error = %#v", structured)
	}
	if r.called {
		t.Fatal("resolver called for conflicting selector")
	}
}

func TestActionRejectsWhitespaceOnlySelector(t *testing.T) {
	r := &spyResolver{}
	input := workbookget.Input{Selector: identity.Selector{LUID: "   ", Name: " ", ProjectPath: "\t"}}
	_, err := workbookget.New(r).Execute(context.Background(), input)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || structured.ID != "workbook.inspect.usage" {
		t.Fatalf("error = %#v", structured)
	}
	if r.called {
		t.Fatal("resolver called for whitespace-only selector")
	}
}

func TestActionRejectsMismatchedIdentity(t *testing.T) {
	cases := []struct {
		name     string
		selector identity.Selector
		workbook workbookget.Workbook
	}{
		{"empty LUID", identity.Selector{Name: "Finance", ProjectPath: "Ops"}, workbookget.Workbook{Name: "Finance"}},
		{"empty Name", identity.Selector{Name: "Finance", ProjectPath: "Ops"}, workbookget.Workbook{LUID: "wb-1"}},
		{"LUID differs from request", identity.Selector{LUID: "wb-1"}, workbookget.Workbook{LUID: "other", Name: "Finance"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &spyResolver{workbook: tc.workbook}
			_, err := workbookget.New(r).Execute(context.Background(), workbookget.Input{Selector: tc.selector})
			var structured *errs.Error
			if !errors.As(err, &structured) || structured.Kind != errs.KindOperation || structured.ID != "workbook.inspect.identity_mismatch" {
				t.Fatalf("error = %#v", structured)
			}
		})
	}
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
