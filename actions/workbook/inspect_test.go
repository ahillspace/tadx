package workbook_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
	"os"
	"path/filepath"
	"testing"
)

type inspectErroringResolver struct{ err error }

func (r inspectErroringResolver) ResolveWorkbook(context.Context, identity.Selector) (workbookops.Record, error) {
	return workbookops.Record{}, r.err
}

func TestInspectActionClassifiesResolutionFailures(t *testing.T) {
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
			input := workbookops.InspectInput{Environment: "dev", Site: "site", Selector: identity.Selector{Name: "Finance", ProjectPath: "Ops"}}
			_, err := workbookops.Inspect(context.Background(), inspectErroringResolver{err: cause}, input)
			var structured *errs.Error
			if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || structured.ID != tc.wantID {
				t.Fatalf("error = %#v", structured)
			}
		})
	}
}

func TestInspectActionClassifiesOpaqueResolverErrorAsOperation(t *testing.T) {
	input := workbookops.InspectInput{Environment: "dev", Site: "site", Selector: identity.Selector{Name: "Finance", ProjectPath: "Ops"}}
	_, err := workbookops.Inspect(context.Background(), inspectErroringResolver{err: errors.New("upstream unavailable")}, input)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindOperation || structured.ID != "workbook.inspect.resolve" {
		t.Fatalf("error = %#v", structured)
	}
}

type inspectResolver struct {
	workbook workbookops.Record
	selector identity.Selector
}

func (r *inspectResolver) ResolveWorkbook(_ context.Context, selector identity.Selector) (workbookops.Record, error) {
	r.selector = selector
	return r.workbook, nil
}

func TestInspectOutputGolden(t *testing.T) {
	output := workbookops.InspectOutput{
		Status: "found", Environment: "dev", Site: "sandbox", RequestID: "request-1",
		Workbook: workbookops.Record{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectPath: "Department/Ops", ContentURL: "Finance", UpdatedAt: "2026-09-01T00:00:00Z", Description: "Finance reporting", OwnerLUID: "user-1", CreatedAt: "2026-08-01T00:00:00Z", Tags: []string{"finance"}},
		Help:     []string{"tadx content workbook pull --id wb-1"},
	}
	inspectAssertGolden(t, "compact.toon", output, false)
	inspectAssertGolden(t, "full.toon", output, true)
}

func TestInspectActionGetsExactWorkbookAndBoundsTags(t *testing.T) {
	tags := make([]string, 60)
	for index := range tags {
		tags[index] = fmt.Sprintf("tag-%02d", index)
	}
	r := &inspectResolver{workbook: workbookops.Record{LUID: "wb-1", Name: "Finance", ProjectPath: "Department/Ops", Tags: tags, RequestID: "request-1"}}
	output, err := workbookops.Inspect(context.Background(), r, workbookops.InspectInput{Environment: "dev", Selector: identity.Selector{Name: "Finance", ProjectPath: "Department/Ops"}})
	if err != nil {
		t.Fatal(err)
	}
	if r.selector.Name != "Finance" || r.selector.ProjectPath != "Department/Ops" {
		t.Fatalf("selector = %#v", r.selector)
	}
	if output.RequestID != "request-1" || output.CompactOutput().(workbookops.InspectCompactResult).Workbook.ProjectPath != "Department/Ops" {
		t.Fatalf("output = %#v", output)
	}
	full := output.FullOutput().(workbookops.InspectFullResult)
	if len(full.Workbook.Tags) != 50 || full.Workbook.TagsOmitted != 10 {
		t.Fatalf("full = %#v", full)
	}
}

type inspectSpyResolver struct {
	called   bool
	workbook workbookops.Record
}

func (r *inspectSpyResolver) ResolveWorkbook(_ context.Context, _ identity.Selector) (workbookops.Record, error) {
	r.called = true
	return r.workbook, nil
}

func TestInspectActionRejectsConflictingSelector(t *testing.T) {
	r := &inspectSpyResolver{}
	input := workbookops.InspectInput{Selector: identity.Selector{LUID: "wb-1", Name: "Finance"}}
	_, err := workbookops.Inspect(context.Background(), r, input)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || structured.ID != "workbook.inspect.usage" {
		t.Fatalf("error = %#v", structured)
	}
	if r.called {
		t.Fatal("resolver called for conflicting selector")
	}
}

func TestInspectActionRejectsWhitespaceOnlySelector(t *testing.T) {
	r := &inspectSpyResolver{}
	input := workbookops.InspectInput{Selector: identity.Selector{LUID: "   ", Name: " ", ProjectPath: "\t"}}
	_, err := workbookops.Inspect(context.Background(), r, input)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || structured.ID != "workbook.inspect.usage" {
		t.Fatalf("error = %#v", structured)
	}
	if r.called {
		t.Fatal("resolver called for whitespace-only selector")
	}
}

func TestInspectActionRejectsMismatchedIdentity(t *testing.T) {
	cases := []struct {
		name     string
		selector identity.Selector
		workbook workbookops.Record
	}{
		{"empty LUID", identity.Selector{Name: "Finance", ProjectPath: "Ops"}, workbookops.Record{Name: "Finance"}},
		{"empty Name", identity.Selector{Name: "Finance", ProjectPath: "Ops"}, workbookops.Record{LUID: "wb-1"}},
		{"LUID differs from request", identity.Selector{LUID: "wb-1"}, workbookops.Record{LUID: "other", Name: "Finance"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &inspectSpyResolver{workbook: tc.workbook}
			_, err := workbookops.Inspect(context.Background(), r, workbookops.InspectInput{Selector: tc.selector})
			var structured *errs.Error
			if !errors.As(err, &structured) || structured.Kind != errs.KindOperation || structured.ID != "workbook.inspect.identity_mismatch" {
				t.Fatalf("error = %#v", structured)
			}
		})
	}
}

func inspectAssertGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, value, render.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata/inspect", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, buffer.Bytes())
	}
}
