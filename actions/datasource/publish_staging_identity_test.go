package datasource_test

import (
	"context"
	"testing"

	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/identity"
)

type stagingDatasourcePublisher struct {
	resolver *publishPublishResolver
	commits  int
	closes   int
}

func (p *stagingDatasourcePublisher) Prepare(context.Context, datasourceops.PublishRequest) (datasourceops.PreparedPublish, error) {
	// Another publisher replaces the name/project occupant while this upload
	// is staged. Only the authoritative LUID distinguishes the two targets.
	p.resolver.collisions = []datasourceops.Record{{LUID: "ds-replacement", Name: "Sales", ProjectLUID: "project-1"}}
	return p, nil
}

func (p *stagingDatasourcePublisher) Commit(context.Context) (datasourceops.PublishResult, error) {
	p.commits++
	return datasourceops.PublishResult{Status: "succeeded", DatasourceLUID: "ds-replacement", DatasourceName: "Sales", ProjectLUID: "project-1"}, nil
}

func (p *stagingDatasourcePublisher) Close() error { p.closes++; return nil }

func TestPublishRejectsDatasourceReplacementDuringStaging(t *testing.T) {
	for _, mode := range []datasourceops.Mode{datasourceops.ModeCreate, datasourceops.ModeOverwrite, datasourceops.ModeAppend, datasourceops.ModeReplace} {
		t.Run(string(mode), func(t *testing.T) {
			artifact := datasourceops.PublishArtifact{Path: "Sales.hyper", PayloadPath: "Sales.hyper", Filename: "Sales.hyper", Name: "Sales", Size: 10, Fingerprint: "sha256:planned", CompositionStatus: "ordinary"}
			resolver := &publishPublishResolver{project: datasourceops.Project{LUID: "project-1", Path: "Analytics"}}
			if mode != datasourceops.ModeCreate {
				resolver.collisions = []datasourceops.Record{{LUID: "ds-original", Name: "Sales", ProjectLUID: "project-1"}}
			}
			publisher := &stagingDatasourcePublisher{resolver: resolver}
			out, err := datasourceops.NewPublish(&publishPublishArtifactReader{artifact: artifact}, resolver, publisher).Execute(t.Context(), datasourceops.PublishInput{ArtifactPath: artifact.Path, Environment: "dev", Site: "sandbox", Mode: mode, ProjectSelector: identity.Selector{LUID: "project-1"}}, false)
			if err == nil || publisher.commits != 0 || publisher.closes != 1 {
				t.Fatalf("replacement reached publication or stranded prepared ownership: err=%v commits=%d closes=%d out=%#v", err, publisher.commits, publisher.closes, out)
			}
		})
	}
}

func TestPublishAppendAndReplaceRejectReturnedReplacementIdentity(t *testing.T) {
	for _, mode := range []datasourceops.Mode{datasourceops.ModeAppend, datasourceops.ModeReplace} {
		t.Run(string(mode), func(t *testing.T) {
			out := datasourceops.PublishOutput{Plan: datasourceops.PublishPlan{Mode: "execute", PublishMode: mode, DatasourceName: "Sales", Target: datasourceops.PublishTarget{Environment: "dev", Site: "sandbox", ProjectLUID: "project-1", ExistingLUID: "ds-original"}}, Result: &datasourceops.PublishResult{Status: "succeeded", DatasourceLUID: "ds-replacement", DatasourceName: "Sales", ProjectLUID: "project-1", TableauRequestID: "publish-request"}}
			got, err := datasourceops.NewPublish(nil, nil, nil).Complete(out)
			if err == nil || got.Result == nil || got.Result.DatasourceLUID != "ds-replacement" || got.Result.Verification == "confirmed" {
				t.Fatalf("returned replacement was falsely confirmed or discarded: out=%#v err=%v", got, err)
			}
		})
	}
}
