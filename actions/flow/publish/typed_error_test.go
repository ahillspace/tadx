package publish_test

import (
	"context"
	"errors"
	"testing"

	flowpublish "github.com/ahillspace/tadx/actions/flow/publish"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

type typedErrorPrepared struct{}

func (typedErrorPrepared) Commit(context.Context) (flowpublish.Result, error) {
	return flowpublish.Result{Status: "succeeded", FlowLUID: "flow-1"}, &errs.Error{
		ID:      "flow.publish.receipt",
		Phase:   errs.PhasePersistence,
		Outcome: errs.OutcomeConfirmed,
	}
}

type typedErrorPublisher struct{}

func (typedErrorPublisher) Prepare(context.Context, flowpublish.PublishRequest) (flowpublish.PreparedPublish, error) {
	return typedErrorPrepared{}, nil
}

func TestPublishPreservesTypedCommitOutcome(t *testing.T) {
	output, err := flowpublish.New(
		artifactReader{artifact: flowpublish.Artifact{Path: "artifact", PayloadPath: "Daily.tfl", Filename: "Daily.tfl", Size: 10, Name: "Daily", Fingerprint: "sha256:x"}},
		&resolver{project: flowpublish.Project{LUID: "p-1", Path: "Ops"}},
		typedErrorPublisher{},
	).Execute(context.Background(), flowpublish.Input{
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
