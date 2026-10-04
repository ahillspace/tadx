package lineage_test

import (
	"context"
	"errors"
	"testing"

	lineageops "github.com/ahillspace/tadx/actions/lineage"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
)

type serviceWorkspace struct {
	called int
	err    error
}

func (w *serviceWorkspace) ResolveLineageWorkspace(_ context.Context, selector, environment string) (lineageops.Workspace, error) {
	w.called++
	if selector != "local" || environment != "requested" {
		return lineageops.Workspace{}, errors.New("unexpected workspace selection")
	}
	return lineageops.Workspace{Root: "resolved-root", Name: "local"}, w.err
}

type serviceProvider struct {
	called  int
	session lineageops.ReadSession
	err     error
}

type previewCapture struct{ called int }

func (r *previewCapture) CaptureLineage(context.Context, lineageops.CaptureRequest) (lineageops.Graph, error) {
	r.called++
	return lineageops.Graph{}, nil
}

type previewSpy struct{ called int }

func (p *previewSpy) PreviewLineage(_ context.Context, input lineageops.Input, resource lineageops.Resource) (value.AcquisitionPlan, error) {
	p.called++
	if input.Workspace != "resolved-root" || input.Environment != "canonical" || resource.LUID != "wb-1" {
		return value.AcquisitionPlan{}, errors.New("preview did not receive resolved identity")
	}
	return value.AcquisitionPlan{Status: "preview", Operation: "lineage.pull"}, nil
}

func (p *serviceProvider) OpenLineage(_ context.Context, environment string) (lineageops.ReadSession, error) {
	p.called++
	if environment != "requested" {
		return lineageops.ReadSession{}, errors.New("unexpected source environment")
	}
	return p.session, p.err
}

func TestServiceValidatesBeforeWorkspaceAndProvider(t *testing.T) {
	workspace, provider := &serviceWorkspace{}, &serviceProvider{}
	_, err := lineageops.New(workspace, provider).PullLineage(t.Context(), lineageops.Input{Workspace: "local", Kind: "other", Environment: "requested", Selector: identity.Selector{LUID: "wb-1"}})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "lineage.pull.usage" || workspace.called != 0 || provider.called != 0 {
		t.Fatalf("error=%v workspace=%d provider=%d", err, workspace.called, provider.called)
	}
}

func TestServiceWorkspaceFailureDoesNotOpenProvider(t *testing.T) {
	workspace, provider := &serviceWorkspace{err: errors.New("workspace unavailable")}, &serviceProvider{}
	_, err := lineageops.New(workspace, provider).PullLineage(t.Context(), lineageops.Input{Workspace: "local", Kind: "workbook", Environment: "requested", Site: "selected", Selector: identity.Selector{LUID: "wb-1"}})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "lineage.pull.workspace" || structured.Environment != "requested" || structured.Site != "selected" || structured.Phase != errs.PhaseSetup || structured.Outcome != errs.OutcomeNotAttempted || provider.called != 0 {
		t.Fatalf("error=%#v provider=%d", structured, provider.called)
	}
}

func TestServiceSetupFailureReportsResolvedSource(t *testing.T) {
	workspace := &serviceWorkspace{}
	provider := &serviceProvider{session: lineageops.ReadSession{Environment: "canonical", Site: ""}, err: errors.New("source unavailable")}
	_, err := lineageops.New(workspace, provider).PullLineage(t.Context(), lineageops.Input{Workspace: "local", Kind: "workbook", Environment: "requested", Site: "stale", Selector: identity.Selector{LUID: "wb-1"}})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "lineage.pull.setup" || structured.Environment != "canonical" || structured.Site != "" || structured.Phase != errs.PhaseSetup || structured.Outcome != errs.OutcomeNotAttempted || provider.called != 1 {
		t.Fatalf("error=%#v provider=%d", structured, provider.called)
	}
}

func TestServiceBindsCanonicalSourceAndResolvedWorkspace(t *testing.T) {
	workspace, artifact := &serviceWorkspace{}, &writer{}
	provider := &serviceProvider{session: lineageops.ReadSession{
		Environment: "canonical", Site: "canonical-site", ServerOrigin: "https://tableau.example.com", SiteLUID: "site-1",
		Resolver: resolver{resource: lineageops.Resource{Kind: "workbook", LUID: "wb-1", Name: "Book"}},
		Reader:   reader{graph: lineageops.Graph{Complete: true}}, Writer: artifact,
	}}
	output, err := lineageops.New(workspace, provider).PullLineage(t.Context(), lineageops.Input{Workspace: "local", Kind: "workbook", Environment: "requested", Site: "stale", Selector: identity.Selector{LUID: "wb-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if workspace.called != 1 || provider.called != 1 || artifact.input.Workspace != "resolved-root" || artifact.input.Environment != "canonical" || artifact.input.Site != "canonical-site" || artifact.input.ServerOrigin != "https://tableau.example.com" || artifact.input.SiteLUID != "site-1" || output.Provenance.Environment != "canonical" || output.Provenance.Site != "canonical-site" {
		t.Fatalf("output=%#v artifact=%#v workspace=%d provider=%d", output, artifact.input, workspace.called, provider.called)
	}
}

func TestServiceRequiresDeclaredPreviewPortAfterResolutionWithoutCapture(t *testing.T) {
	workspace, capture, artifact := &serviceWorkspace{}, &previewCapture{}, &writer{}
	provider := &serviceProvider{session: lineageops.ReadSession{
		Environment: "canonical", Resolver: resolver{resource: lineageops.Resource{Kind: "workbook", LUID: "wb-1", Name: "Book"}},
		Reader: capture, Writer: artifact,
	}}
	_, err := lineageops.New(workspace, provider).PullLineage(t.Context(), lineageops.Input{Preview: true, Workspace: "local", Kind: "workbook", Environment: "requested", Selector: identity.Selector{LUID: "wb-1"}})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "lineage.pull.preview" || workspace.called != 1 || provider.called != 1 || capture.called != 0 || artifact.input.Workspace != "" {
		t.Fatalf("error=%#v workspace=%d provider=%d capture=%d artifact=%#v", structured, workspace.called, provider.called, capture.called, artifact.input)
	}
}

func TestServiceUsesDeclaredPreviewPortWithoutCaptureOrWrite(t *testing.T) {
	workspace, capture, artifact, preview := &serviceWorkspace{}, &previewCapture{}, &writer{}, &previewSpy{}
	provider := &serviceProvider{session: lineageops.ReadSession{
		Environment: "canonical", Resolver: resolver{resource: lineageops.Resource{Kind: "workbook", LUID: "wb-1", Name: "Book"}},
		Reader: capture, Writer: artifact, Previewer: preview,
	}}
	output, err := lineageops.New(workspace, provider).PullLineage(t.Context(), lineageops.Input{Preview: true, Workspace: "local", Kind: "workbook", Environment: "requested", Selector: identity.Selector{LUID: "wb-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if output.Preview == nil || output.Preview.Status != "preview" || preview.called != 1 || capture.called != 0 || artifact.input.Workspace != "" {
		t.Fatalf("output=%#v preview=%d capture=%d artifact=%#v", output, preview.called, capture.called, artifact.input)
	}
}
