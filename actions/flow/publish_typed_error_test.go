package flow

import (
	"context"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

type publishTypedErrorPrepared struct{}

func (publishTypedErrorPrepared) Commit(context.Context) (PublishResult, error) {
	return PublishResult{Status: "succeeded", FlowLUID: "flow-1"}, &errs.Error{
		ID:      "flow.publish.receipt",
		Phase:   errs.PhasePersistence,
		Outcome: errs.OutcomeConfirmed,
	}
}

type publishTypedErrorPublisher struct{}

func (publishTypedErrorPublisher) Prepare(context.Context, PublishRequest) (PreparedPublish, error) {
	return publishTypedErrorPrepared{}, nil
}

func TestPublishPublishPreservesTypedCommitOutcome(t *testing.T) {
	output, err := newPublisher(
		publishArtifactReader{artifact: PublishArtifact{Path: "artifact", PayloadPath: "Daily.tfl", Filename: "Daily.tfl", Size: 10, Name: "Daily", Fingerprint: "sha256:x"}},
		&publishResolver{project: Project{LUID: "p-1", Path: "Ops"}},
		publishTypedErrorPublisher{},
	).Execute(context.Background(), PublishInput{
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
