package definition

import (
	"context"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

type pullServiceProvider struct {
	workspaceCalls int
	openCalls      int
	reader         *pullServicePort
}

func (p *pullServiceProvider) ResolveDefinitionWorkspace(context.Context, string, string, string) (PullWorkspace, error) {
	p.workspaceCalls++
	return PullWorkspace{Root: "workspace", Name: "logical"}, nil
}

func (p *pullServiceProvider) OpenDefinitionPull(context.Context, string, string) (PullSession, error) {
	p.openCalls++
	return PullSession{Environment: "canonical", Site: "site", SiteLUID: "site-1", ServerOrigin: "https://example.invalid", Reader: p.reader, Writer: p.reader}, nil
}

type pullServicePort struct {
	input PullInput
	reads int
}

func (p *pullServicePort) GetDefinition(_ context.Context, luid string) (PullDefinition, error) {
	p.reads++
	return PullDefinition{LUID: luid, Name: "Sales", DatasourceLUID: "datasource-1", Configuration: []byte(`{"name":"Sales"}`), MetricsComplete: true, Metrics: []PullMetric{{LUID: "metric-1", DefinitionLUID: luid}}}, nil
}

func (p *pullServicePort) WriteDefinition(context.Context, PullArtifact) (PullArtifactResult, error) {
	panic("preview must not write")
}

func (p *pullServicePort) PreviewDefinition(_ context.Context, input PullInput, _ PullDefinition) (value.AcquisitionPlan, error) {
	p.input = input
	return value.AcquisitionPlan{Status: "preview"}, nil
}

func TestPullServiceValidatesBeforeProviderAndBindsCanonicalTarget(t *testing.T) {
	port := &pullServicePort{}
	provider := &pullServiceProvider{reader: port}
	service := New(Ports{Pull: provider})
	if _, err := service.PullPulseDefinition(t.Context(), PullInput{}); err == nil {
		t.Fatal("expected exact selector error")
	}
	if provider.workspaceCalls != 0 || provider.openCalls != 0 {
		t.Fatalf("provider acquired on invalid input: workspace=%d open=%d", provider.workspaceCalls, provider.openCalls)
	}
	output, err := service.PullPulseDefinition(t.Context(), PullInput{LUID: "definition-1", Workspace: "logical", Environment: "alias", Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	if output.Status != "preview" || port.reads != 1 || provider.workspaceCalls != 1 || provider.openCalls != 1 {
		t.Fatalf("output=%#v reads=%d workspace=%d open=%d", output, port.reads, provider.workspaceCalls, provider.openCalls)
	}
	if port.input.Workspace != "workspace" || port.input.WorkspaceName != "logical" || port.input.Environment != "canonical" || port.input.Site != "site" || port.input.SiteLUID != "site-1" || port.input.ServerOrigin != "https://example.invalid" {
		t.Fatalf("unbound pull input: %#v", port.input)
	}
}
