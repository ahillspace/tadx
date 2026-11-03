package flow_test

import (
	"context"
	"testing"

	flowops "github.com/ahillspace/tadx/actions/flow"
	"github.com/ahillspace/tadx/internal/identity"
)

type stagingFlowPublisher struct {
	resolver *publishResolver
	commits  int
	closes   int
}

func (p *stagingFlowPublisher) Prepare(context.Context, flowops.PublishRequest) (flowops.PreparedPublish, error) {
	p.resolver.collisions = []flowops.Record{{LUID: "flow-replacement", Name: "Daily", ProjectLUID: "project-1"}}
	return p, nil
}

func (p *stagingFlowPublisher) Commit(context.Context) (flowops.PublishResult, error) {
	p.commits++
	return flowops.PublishResult{Status: "succeeded", FlowLUID: "flow-replacement", FlowName: "Daily", ProjectLUID: "project-1"}, nil
}

func (p *stagingFlowPublisher) Close() error { p.closes++; return nil }

func TestPublishRejectsFlowReplacementDuringStaging(t *testing.T) {
	resolver := &publishResolver{project: flowops.Project{LUID: "project-1", Path: "Operations"}, collisions: []flowops.Record{{LUID: "flow-original", Name: "Daily", ProjectLUID: "project-1"}}}
	publisher := &stagingFlowPublisher{resolver: resolver}
	artifact := flowops.PublishArtifact{Path: "Daily.tfl", PayloadPath: "Daily.tfl", Filename: "Daily.tfl", Name: "Daily", Size: 10, Fingerprint: "sha256:planned"}
	out, err := flowops.NewPublish(publishArtifactReader{artifact: artifact}, resolver, publisher).Execute(t.Context(), flowops.PublishInput{ArtifactPath: artifact.Path, Environment: "dev", Site: "sandbox", Overwrite: true, ProjectSelector: identity.Selector{LUID: "project-1"}}, false)
	if err == nil || publisher.commits != 0 || publisher.closes != 1 {
		t.Fatalf("replacement reached publication or stranded prepared ownership: err=%v commits=%d closes=%d out=%#v", err, publisher.commits, publisher.closes, out)
	}
}
