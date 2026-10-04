package datasource_test

import (
	"bytes"
	"context"
	"errors"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type inspectReadProvider struct {
	*readServiceProvider
	resolver datasourceops.Resolver
}

func (p *inspectReadProvider) OpenDatasourceRead(_ context.Context, environment, site, _ string) (datasourceops.ReadSession, error) {
	p.opens++
	return datasourceops.ReadSession{ReadTarget: datasourceops.ReadTarget{Environment: environment, Site: site}, Resolver: p.resolver, Upstream: inspectUpstream{}, Inventory: inspectInventory{}}, nil
}

type inspectUpstream struct{}

func (inspectUpstream) ReadDatasourceUpstream(context.Context, string) (datasourceops.UpstreamObservation, error) {
	return datasourceops.UpstreamObservation{}, nil
}

type inspectInventory struct{}

func (inspectInventory) CollectDatasources(context.Context, string, time.Time) (datasourceops.CollectedList, error) {
	return datasourceops.CollectedList{}, nil
}
func (inspectInventory) PublishDatasourceInspect(context.Context, datasourceops.InspectOutput, time.Time) {
}

func inspectWithResolver(ctx context.Context, resolver datasourceops.Resolver, input datasourceops.InspectInput) (datasourceops.InspectOutput, error) {
	service, _ := inspectService(resolver)
	return service.InspectDatasource(ctx, input)
}

func inspectService(resolver datasourceops.Resolver) (*datasourceops.Service, *inspectReadProvider) {
	provider := &inspectReadProvider{readServiceProvider: &readServiceProvider{}, resolver: resolver}
	return datasourceops.New(datasourceops.Ports{Read: provider}), provider
}

type inspectErroringResolver struct{ err error }

func (r inspectErroringResolver) ResolveDatasource(context.Context, identity.Selector) (datasourceops.Record, error) {
	return datasourceops.Record{}, r.err
}

func TestInspectActionClassifiesResolutionFailures(t *testing.T) {
	cases := []struct {
		name    string
		kind    identity.ResolutionErrorKind
		wantID  string
		wantErr errs.Kind
	}{
		{"ambiguous", identity.ResolutionAmbiguous, "datasource.inspect.ambiguous", errs.KindUsage},
		{"not found", identity.ResolutionNotFound, "datasource.inspect.not_found", errs.KindUsage},
		{"invalid selector", identity.ResolutionInvalidSelector, "datasource.inspect.usage", errs.KindUsage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cause := &identity.ResolutionError{Kind: tc.kind, Selector: identity.Selector{Name: "Sales", ProjectPath: "Ops"}}
			input := datasourceops.InspectInput{Environment: "dev", Site: "site", Selector: identity.Selector{Name: "Sales", ProjectPath: "Ops"}}
			_, err := inspectWithResolver(context.Background(), inspectErroringResolver{err: cause}, input)
			var structured *errs.Error
			if !errors.As(err, &structured) || structured.Kind != tc.wantErr || structured.ID != tc.wantID {
				t.Fatalf("error = %#v", structured)
			}
		})
	}
}

func TestInspectActionClassifiesOpaqueResolverErrorAsOperation(t *testing.T) {
	input := datasourceops.InspectInput{Environment: "dev", Site: "site", Selector: identity.Selector{Name: "Sales", ProjectPath: "Ops"}}
	_, err := inspectWithResolver(context.Background(), inspectErroringResolver{err: errors.New("upstream unavailable")}, input)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindOperation || structured.ID != "datasource.inspect.resolve" {
		t.Fatalf("error = %#v", structured)
	}
}

type inspectResolver struct {
	datasource datasourceops.Record
	selector   identity.Selector
}

func (r *inspectResolver) ResolveDatasource(_ context.Context, selector identity.Selector) (datasourceops.Record, error) {
	r.selector = selector
	return r.datasource, nil
}

func TestInspectOutputGolden(t *testing.T) {
	hasExtracts := true
	isCertified := true
	size := int64(42)
	item := datasourceops.Record{
		LUID: "datasource-1", Name: "Sales", ProjectLUID: "project-1", ProjectPath: "Department/Ops",
		Type: "hyper", ContentURL: "sales", UpdatedAt: "2026-09-01T00:00:00Z", Description: "Sales data",
		OwnerLUID: "user-1", CreatedAt: "2026-08-01T00:00:00Z", Size: &size, HasExtracts: &hasExtracts,
		IsCertified: &isCertified, CertificationNote: "Reviewed", Tags: []string{"daily"}, AskDataEnablement: "Enabled", RequestID: "request-1",
	}
	output, err := inspectWithResolver(t.Context(), &inspectResolver{datasource: item}, datasourceops.InspectInput{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "datasource-1"}})
	if err != nil {
		t.Fatal(err)
	}
	// Keep the original inspect projection golden; the Service adds live source
	// and upstream observations, which have separate integration coverage.
	output.Source = nil
	output.Datasource.Upstream = nil
	inspectAssertGolden(t, "compact.toon", output, false)
	inspectAssertGolden(t, "full.toon", output, true)
}

type inspectSpyResolver struct {
	called     bool
	datasource datasourceops.Record
}

func (r *inspectSpyResolver) ResolveDatasource(_ context.Context, _ identity.Selector) (datasourceops.Record, error) {
	r.called = true
	return r.datasource, nil
}

func TestInspectActionRejectsConflictingSelector(t *testing.T) {
	r := &inspectSpyResolver{}
	input := datasourceops.InspectInput{Selector: identity.Selector{LUID: "ds-1", Name: "Sales"}}
	service, provider := inspectService(r)
	_, err := service.InspectDatasource(t.Context(), input)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || structured.ID != "datasource.inspect.usage" {
		t.Fatalf("error = %#v", structured)
	}
	if provider.opens != 0 || r.called {
		t.Fatal("provider opened or resolver called for conflicting selector")
	}
}

func TestInspectActionRejectsWhitespaceOnlySelector(t *testing.T) {
	r := &inspectSpyResolver{}
	input := datasourceops.InspectInput{Selector: identity.Selector{LUID: "   ", Name: " ", ProjectPath: "\t"}}
	service, provider := inspectService(r)
	_, err := service.InspectDatasource(t.Context(), input)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || structured.ID != "datasource.inspect.usage" {
		t.Fatalf("error = %#v", structured)
	}
	if provider.opens != 0 || r.called {
		t.Fatal("provider opened or resolver called for whitespace-only selector")
	}
}

func TestInspectActionRejectsMismatchedIdentity(t *testing.T) {
	cases := []struct {
		name       string
		selector   identity.Selector
		datasource datasourceops.Record
	}{
		{"empty LUID", identity.Selector{Name: "Sales", ProjectPath: "Ops"}, datasourceops.Record{Name: "Sales"}},
		{"empty Name", identity.Selector{Name: "Sales", ProjectPath: "Ops"}, datasourceops.Record{LUID: "ds-1"}},
		{"LUID differs from request", identity.Selector{LUID: "ds-1"}, datasourceops.Record{LUID: "other", Name: "Sales"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &inspectSpyResolver{datasource: tc.datasource}
			_, err := inspectWithResolver(context.Background(), r, datasourceops.InspectInput{Selector: tc.selector})
			var structured *errs.Error
			if !errors.As(err, &structured) || structured.Kind != errs.KindOperation || structured.ID != "datasource.inspect.identity_mismatch" {
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

func TestInspectActionGetsExactDatasourceByNameAndCanonicalProjectPath(t *testing.T) {
	r := &inspectResolver{datasource: datasourceops.Record{LUID: "ds-1", Name: "Sales", ProjectPath: "Department/Ops", RequestID: "request-1"}}
	input := datasourceops.InspectInput{Environment: "dev", Site: "site"}
	input.SetSelector("", "Sales", "Department/Ops")
	output, err := inspectWithResolver(context.Background(), r, input)
	if err != nil {
		t.Fatal(err)
	}
	if r.selector.Name != "Sales" || r.selector.ProjectPath != "Department/Ops" || output.RequestID != "request-1" {
		t.Fatalf("selector = %#v, output = %#v", r.selector, output)
	}
}

func TestInspectActionRequiresAuthoritativeOrExactDatasourceSelector(t *testing.T) {
	for _, selector := range []identity.Selector{{}, {Name: "Sales"}, {ProjectPath: "Department/Ops"}} {
		_, err := inspectWithResolver(context.Background(), &inspectResolver{}, datasourceops.InspectInput{Selector: selector})
		if err == nil {
			t.Fatalf("selector %#v succeeded", selector)
		}
	}
}

func TestInspectFullDatasourceInspectBoundsTags(t *testing.T) {
	tags := make([]string, 60)
	output := datasourceops.InspectOutput{Datasource: datasourceops.InspectDatasource{LUID: "ds-1", Tags: tags}}
	full := output.FullOutput().(datasourceops.InspectFullResult)
	if len(full.Datasource.Tags) != 50 || full.Datasource.TagsOmitted != 10 {
		t.Fatalf("full = %#v", full)
	}
}
