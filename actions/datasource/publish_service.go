package datasource

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/errs"
)

// PublishProvider binds one validated artifact to its authenticated destination.
type PublishProvider interface {
	OpenDatasourcePublishSource(context.Context, PublishInput) (PublishInput, ArtifactReader, string, error)
	OpenDatasourcePublish(context.Context, PublishInput, string) (PublishSession, error)
}

type PublishLifecycle interface {
	Record(context.Context, string, string, string, string, string) (string, error)
	Wait(context.Context, string) (PublishObservation, error)
	Destination(context.Context, string) (id, name, project string, err error)
}

type PublishObservation struct {
	Status, RequestID, ResourceID string
}

type PublishSession struct {
	Environment string
	Site        string
	Resolver    PublishResolver
	Preparer    PublishPreparer
	Lifecycle   func() PublishLifecycle
	NoWait      func() bool
	Defer       func(context.Context, func(context.Context) (any, error))
}

func (s *Service) PublishDatasource(ctx context.Context, input PublishInput, preview bool) (PublishOutput, error) {
	if err := ValidatePublishInput(input); err != nil {
		return PublishOutput{}, err
	}
	if s == nil || s.ports.Publish == nil {
		return PublishOutput{}, errs.New(errs.KindRuntime, "datasource publish provider is not configured")
	}
	input, reader, sourcePath, err := s.ports.Publish.OpenDatasourcePublishSource(ctx, input)
	if err != nil {
		return PublishOutput{}, err
	}
	session, err := s.ports.Publish.OpenDatasourcePublish(ctx, input, sourcePath)
	if err != nil {
		return PublishOutput{}, err
	}
	input.Environment, input.Site, input.TargetResolved = session.Environment, session.Site, true
	action := newPublisher(reader, session.Resolver, session.Preparer)
	out, err := action.executeValidated(ctx, input, preview)
	if session.Lifecycle == nil {
		return out, err
	}
	lifecycle := session.Lifecycle()
	if lifecycle == nil || out.Result == nil || out.Result.Status == "" {
		return out, err
	}
	var saveErr error
	out.Result.ReceiptPath, saveErr = lifecycle.Record(ctx, out.Result.JobID, out.Result.Status, out.Result.DatasourceLUID, out.Result.TableauRequestID, out.Result.Verification)
	err = errors.Join(err, saveErr)
	if err == nil && out.Result.Status == "pending" && session.NoWait != nil && !session.NoWait() && session.Defer != nil {
		session.Defer(ctx, func(ctx context.Context) (any, error) { return completeDatasource(ctx, lifecycle, action, out) })
	}
	return out, err
}

func completeDatasource(ctx context.Context, lifecycle PublishLifecycle, action *publisher, out PublishOutput) (PublishOutput, error) {
	observed, err := lifecycle.Wait(ctx, out.Result.JobID)
	if observed.Status != "" {
		out.Result.Status, out.Result.TableauRequestID = observed.Status, observed.RequestID
	}
	if err == nil && observed.ResourceID != "" {
		out.Result.DatasourceLUID, out.Result.DatasourceName, out.Result.ProjectLUID, err = lifecycle.Destination(ctx, observed.ResourceID)
	}
	if err != nil {
		if out.Result.Status == "succeeded" {
			out.Result.Verification = "destination_unavailable"
		}
		err = errs.PublicationMonitorError("datasource.publish", out.Plan.Target.Environment, out.Plan.Target.Site, out.Result.JobID, out.Result.Status, err)
	} else {
		out, err = action.Complete(ctx, out)
	}
	path, saveErr := lifecycle.Record(ctx, out.Result.JobID, out.Result.Status, out.Result.DatasourceLUID, out.Result.TableauRequestID, out.Result.Verification)
	out.Result.ReceiptPath = path
	return out, errors.Join(err, saveErr)
}
