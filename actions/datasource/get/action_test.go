package get_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	datasourceget "github.com/ahillspace/tadx/actions/datasource/get"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
)

type erroringResolver struct{ err error }

func (r erroringResolver) ResolveDatasource(context.Context, identity.Selector) (datasourceget.Datasource, error) {
	return datasourceget.Datasource{}, r.err
}

func TestActionClassifiesResolutionFailures(t *testing.T) {
	cases := []struct {
		name    string
		kind    identity.ResolutionErrorKind
		wantID  string
		wantErr errs.Kind
	}{
		{"ambiguous", identity.ResolutionAmbiguous, "datasource.get.ambiguous", errs.KindUsage},
		{"not found", identity.ResolutionNotFound, "datasource.get.not_found", errs.KindUsage},
		{"invalid selector", identity.ResolutionInvalidSelector, "datasource.get.usage", errs.KindUsage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cause := &identity.ResolutionError{Kind: tc.kind, Selector: identity.Selector{Name: "Sales", ProjectPath: "Ops"}}
			input := datasourceget.Input{Environment: "dev", Site: "site", Selector: identity.Selector{Name: "Sales", ProjectPath: "Ops"}}
			_, err := datasourceget.New(erroringResolver{err: cause}).Execute(context.Background(), input)
			var structured *errs.Error
			if !errors.As(err, &structured) || structured.Kind != tc.wantErr || structured.ID != tc.wantID {
				t.Fatalf("error = %#v", structured)
			}
		})
	}
}

func TestActionClassifiesOpaqueResolverErrorAsOperation(t *testing.T) {
	input := datasourceget.Input{Environment: "dev", Site: "site", Selector: identity.Selector{Name: "Sales", ProjectPath: "Ops"}}
	_, err := datasourceget.New(erroringResolver{err: errors.New("upstream unavailable")}).Execute(context.Background(), input)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindOperation || structured.ID != "datasource.get.resolve" {
		t.Fatalf("error = %#v", structured)
	}
}

type resolver struct {
	datasource datasourceget.Datasource
	selector   identity.Selector
}

func (r *resolver) ResolveDatasource(_ context.Context, selector identity.Selector) (datasourceget.Datasource, error) {
	r.selector = selector
	return r.datasource, nil
}

func TestOutputGolden(t *testing.T) {
	hasExtracts := true
	isCertified := true
	size := int64(42)
	output := datasourceget.Output{
		Status: "found", Environment: "dev", Site: "sandbox", RequestID: "request-1",
		Datasource: datasourceget.Datasource{
			LUID: "datasource-1", Name: "Sales", ProjectLUID: "project-1", ProjectPath: "Department/Ops",
			Type: "hyper", ContentURL: "sales", UpdatedAt: "2026-09-01T00:00:00Z", Description: "Sales data",
			OwnerLUID: "user-1", CreatedAt: "2026-08-01T00:00:00Z", Size: &size, HasExtracts: &hasExtracts,
			IsCertified: &isCertified, CertificationNote: "Reviewed", Tags: []string{"daily"}, AskDataEnablement: "Enabled",
		},
		Help: []string{"tadx content datasource list"},
	}
	assertGolden(t, "compact.toon", output, false)
	assertGolden(t, "full.toon", output, true)
}

type spyResolver struct {
	called     bool
	datasource datasourceget.Datasource
}

func (r *spyResolver) ResolveDatasource(_ context.Context, _ identity.Selector) (datasourceget.Datasource, error) {
	r.called = true
	return r.datasource, nil
}

func TestActionRejectsConflictingSelector(t *testing.T) {
	r := &spyResolver{}
	input := datasourceget.Input{Selector: identity.Selector{LUID: "ds-1", Name: "Sales"}}
	_, err := datasourceget.New(r).Execute(context.Background(), input)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || structured.ID != "datasource.get.usage" {
		t.Fatalf("error = %#v", structured)
	}
	if r.called {
		t.Fatal("resolver called for conflicting selector")
	}
}

func TestActionRejectsWhitespaceOnlySelector(t *testing.T) {
	r := &spyResolver{}
	input := datasourceget.Input{Selector: identity.Selector{LUID: "   ", Name: " ", ProjectPath: "\t"}}
	_, err := datasourceget.New(r).Execute(context.Background(), input)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || structured.ID != "datasource.get.usage" {
		t.Fatalf("error = %#v", structured)
	}
	if r.called {
		t.Fatal("resolver called for whitespace-only selector")
	}
}

func TestActionRejectsMismatchedIdentity(t *testing.T) {
	cases := []struct {
		name       string
		selector   identity.Selector
		datasource datasourceget.Datasource
	}{
		{"empty LUID", identity.Selector{Name: "Sales", ProjectPath: "Ops"}, datasourceget.Datasource{Name: "Sales"}},
		{"empty Name", identity.Selector{Name: "Sales", ProjectPath: "Ops"}, datasourceget.Datasource{LUID: "ds-1"}},
		{"LUID differs from request", identity.Selector{LUID: "ds-1"}, datasourceget.Datasource{LUID: "other", Name: "Sales"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &spyResolver{datasource: tc.datasource}
			_, err := datasourceget.New(r).Execute(context.Background(), datasourceget.Input{Selector: tc.selector})
			var structured *errs.Error
			if !errors.As(err, &structured) || structured.Kind != errs.KindOperation || structured.ID != "datasource.get.identity_mismatch" {
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

func TestActionGetsExactDatasourceByNameAndCanonicalProjectPath(t *testing.T) {
	r := &resolver{datasource: datasourceget.Datasource{LUID: "ds-1", Name: "Sales", ProjectPath: "Department/Ops", RequestID: "request-1"}}
	input := datasourceget.Input{Environment: "dev", Site: "site"}
	input.SetSelector("", "Sales", "Department/Ops")
	output, err := datasourceget.New(r).Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if r.selector.Name != "Sales" || r.selector.ProjectPath != "Department/Ops" || output.RequestID != "request-1" {
		t.Fatalf("selector = %#v, output = %#v", r.selector, output)
	}
}

func TestActionRequiresAuthoritativeOrExactDatasourceSelector(t *testing.T) {
	for _, selector := range []identity.Selector{{}, {Name: "Sales"}, {ProjectPath: "Department/Ops"}} {
		_, err := datasourceget.New(&resolver{}).Execute(context.Background(), datasourceget.Input{Selector: selector})
		if err == nil {
			t.Fatalf("selector %#v succeeded", selector)
		}
	}
}

func TestFullDatasourceGetBoundsTags(t *testing.T) {
	tags := make([]string, 60)
	output := datasourceget.Output{Datasource: datasourceget.Datasource{LUID: "ds-1", Tags: tags}}
	full := output.FullOutput().(datasourceget.FullResult)
	if len(full.Datasource.Tags) != 50 || full.Datasource.TagsOmitted != 10 {
		t.Fatalf("full = %#v", full)
	}
}
