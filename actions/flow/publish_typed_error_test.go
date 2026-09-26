package flow_test

import (
	"context"
	"errors"
	"testing"

	flowpublish "github.com/ahillspace/tadx/actions/flow"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

type publishTypedErrorPrepared struct{}

func (publishTypedErrorPrepared) Commit(context.Context) (flowpublish.PublishResult, error) {
	return flowpublish.PublishResult{Status: "succeeded", FlowLUID: "flow-1"}, &errs.Error{
		ID:      "flow.publish.receipt",
		Phase:   errs.PhasePersistence,
		Outcome: errs.OutcomeConfirmed,
	}
}

type publishTypedErrorPublisher struct{}

func (publishTypedErrorPublisher) Prepare(context.Context, flowpublish.PublishRequest) (flowpublish.PreparedPublish, error) {
	return publishTypedErrorPrepared{}, nil
}

func TestPublishPublishPreservesTypedCommitOutcome(t *testing.T) {
	output, err := flowpublish.NewPublish(
		publishArtifactReader{artifact: flowpublish.PublishArtifact{Path: "artifact", PayloadPath: "Daily.tfl", Filename: "Daily.tfl", Size: 10, Name: "Daily", Fingerprint: "sha256:x"}},
		&publishResolver{project: flowpublish.Project{LUID: "p-1", Path: "Ops"}},
		publishTypedErrorPublisher{},
	).Execute(context.Background(), flowpublish.PublishInput{
		Environment:     "dev",
		Site:            "site",
		ArtifactPath:    "artifact",
		ProjectSelector: identity.Selector{LUID: "p-1"},
	}, false)
	if err == nil {
		t.Fatal("expected typed commit error")
	}
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Phase != errs.PhasePersistence || structured.Outcome != errs.OutcomeConfirmed {
		t.Fatalf("typed error = %#v", err)
	}
	if output.Result == nil || output.Result.Status != "succeeded" || output.Result.FlowLUID != "flow-1" {
		t.Fatalf("output result = %#v", output.Result)
	}
}
