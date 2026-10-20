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
	"github.com/ahillspace/tadx/internal/value"
	"os"
	"path/filepath"
	"reflect"
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
			_, err := workbookops.Inspect(t.Context(), inspectErroringResolver{err: cause}, input)
			var structured *errs.Error
			if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || structured.ID != tc.wantID {
				t.Fatalf("error = %#v", structured)
			}
		})
	}
}

func TestInspectActionClassifiesOpaqueResolverErrorAsOperation(t *testing.T) {
	input := workbookops.InspectInput{Environment: "dev", Site: "site", Selector: identity.Selector{Name: "Finance", ProjectPath: "Ops"}}
	_, err := workbookops.Inspect(t.Context(), inspectErroringResolver{err: errors.New("upstream unavailable")}, input)
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

func TestInspectActionRetainsRESTWithoutMetadata(t *testing.T) {
	r := &inspectResolver{workbook: workbookops.Record{LUID: "wb-1", Name: "Finance", RequestID: "rest-request"}}
	out, err := workbookops.Inspect(t.Context(), r, workbookops.InspectInput{Selector: identity.Selector{LUID: "wb-1"}})
	if err != nil || out.Status != "found" || out.Workbook.LUID != "wb-1" || out.RequestID != "rest-request" {
		t.Fatalf("confirmed REST inspection depends on metadata: output=%#v err=%v", out, err)
	}
}

func TestInspectMetadataPreservesConfirmedDetailsAndRetryAdvice(t *testing.T) {
	base := workbookops.InspectOutput{Status: "found", Environment: "dev", Site: "site", Workbook: workbookops.Record{LUID: "wb-1", Name: "Finance"}, RequestID: "rest-request"}
	providerErr := &errs.Error{ID: "metadata.unavailable", Retryable: new(true), CorrectiveAction: "Retry after the service recovers.", TableauRequestID: "metadata-request"}
	details := value.MetadataWorkbookDatasources{
		Identity:    value.MetadataIdentity{LUID: "wb-1", MetadataID: "metadata-workbook"},
		Datasources: []value.MetadataWorkbookDatasource{{Identity: value.MetadataIdentity{MetadataID: "embedded-1"}, Kind: "embedded", Complete: true}},
		Warnings:    []string{"One datasource continuation failed."},
	}
	out, err := workbookops.ApplyInspectMetadata(base, details, providerErr)
	structured, ok := errors.AsType[*errs.Error](err)
	if !ok || structured.ID != "workbook.inspect.metadata" || !errors.Is(err, providerErr) || structured.Retryable == nil || !*structured.Retryable || structured.CorrectiveAction != providerErr.CorrectiveAction || structured.TableauRequestID != "metadata-request" || structured.Environment != "dev" || structured.Site != "site" {
		t.Fatalf("metadata retry advice = %#v, error=%v", structured, err)
	}
	if out.Status != "partial" || out.DatasourceStatus != "incomplete" || !reflect.DeepEqual(out.Workbook, base.Workbook) || out.RequestID != "rest-request" || len(out.Datasources) != 1 || out.Datasources[0].Identity.MetadataID != "embedded-1" || len(out.MetadataWarnings) != 1 {
		t.Fatalf("partial metadata discarded confirmed evidence: %#v", out)
	}
	out, err = workbookops.ApplyInspectMetadata(base, value.MetadataWorkbookDatasources{}, providerErr)
	structured, ok = errors.AsType[*errs.Error](err)
	if !ok || structured.ID != "workbook.inspect.metadata" || structured.Retryable == nil || !*structured.Retryable || out.Workbook.LUID != "wb-1" || out.DatasourceStatus != "incomplete" {
		t.Fatalf("provider failure without metadata: output=%#v error=%#v", out, structured)
	}
}

func TestInspectMetadataValidatesIdentityAndCoverage(t *testing.T) {
	for _, tc := range []struct {
		name       string
		luid       string
		metadataID string
		complete   bool
		wantStatus string
		wantErr    bool
	}{
		{name: "complete", luid: "wb-1", metadataID: "metadata-workbook", complete: true, wantStatus: "found"},
		{name: "incomplete", luid: "wb-1", metadataID: "metadata-workbook", wantStatus: "partial"},
		{name: "mismatched workbook", luid: "wb-other", metadataID: "metadata-other", complete: true, wantStatus: "partial", wantErr: true},
		{name: "missing metadata identity", luid: "wb-1", complete: true, wantStatus: "partial", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := workbookops.InspectOutput{Status: "found", Workbook: workbookops.Record{LUID: "wb-1", Name: "Finance"}}
			details := value.MetadataWorkbookDatasources{Identity: value.MetadataIdentity{LUID: tc.luid, MetadataID: tc.metadataID}, Complete: tc.complete, Datasources: []value.MetadataWorkbookDatasource{{Identity: value.MetadataIdentity{MetadataID: "embedded-1"}}}}
			out, err := workbookops.ApplyInspectMetadata(base, details, nil)
			if (err != nil) != tc.wantErr || out.Status != tc.wantStatus || out.Workbook.LUID != "wb-1" {
				t.Fatalf("output=%#v error=%v", out, err)
			}
			if tc.wantErr {
				structured, ok := errors.AsType[*errs.Error](err)
				if !ok || structured.ID != "workbook.inspect.metadata_identity" || len(out.Datasources) != 0 {
					t.Fatalf("unconfirmed metadata accepted: output=%#v error=%v", out, err)
				}
			} else if len(out.Datasources) != 1 || (out.DatasourceStatus == "complete") != tc.complete {
				t.Fatalf("coverage projection=%#v", out)
			}
		})
	}
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
	output, err := workbookops.Inspect(t.Context(), r, workbookops.InspectInput{Environment: "dev", Selector: identity.Selector{Name: "Finance", ProjectPath: "Department/Ops"}})
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
	_, err := workbookops.Inspect(t.Context(), r, input)
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
	_, err := workbookops.Inspect(t.Context(), r, input)
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
			_, err := workbookops.Inspect(t.Context(), r, workbookops.InspectInput{Selector: tc.selector})
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
