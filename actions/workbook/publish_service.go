package workbook

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/errs"
)

// PublishProvider binds the write target and artifact before opening native ports.
type PublishProvider interface {
	ResolveWorkbookPublishTarget(context.Context, string, string) (environment, site string, err error)
	OpenWorkbookPublishSource(context.Context, PublishInput) (PublishInput, ArtifactReader, string, error)
	OpenWorkbookPublish(context.Context, string, string, string) (PublishSession, error)
}

// PublishLifecycle retains one accepted mutation's durable receipt and completion.
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

func (s *Service) PublishWorkbook(ctx context.Context, input PublishInput, preview bool) (PublishOutput, error) {
	if err := validatePublishInput(input); err != nil {
		return PublishOutput{}, err
	}
	if s == nil || s.ports.Publish == nil {
		return PublishOutput{}, errs.New(errs.KindRuntime, "workbook publish provider is not configured")
	}
	environment, site, err := s.ports.Publish.ResolveWorkbookPublishTarget(ctx, input.Environment, input.Site)
	if err != nil {
		return PublishOutput{}, err
	}
	input.Environment, input.Site, input.TargetResolved = environment, site, true
	input, reader, sourcePath, err := s.ports.Publish.OpenWorkbookPublishSource(ctx, input)
	if err != nil {
		return PublishOutput{}, err
	}
	session, err := s.ports.Publish.OpenWorkbookPublish(ctx, input.Environment, input.Site, sourcePath)
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
	out.Result.ReceiptPath, saveErr = lifecycle.Record(ctx, out.Result.JobID, out.Result.Status, out.Result.WorkbookLUID, out.Result.TableauRequestID, out.Result.Verification)
	err = errors.Join(err, saveErr)
	if err == nil && out.Result.Status == "pending" && session.NoWait != nil && !session.NoWait() && session.Defer != nil {
		session.Defer(ctx, func(ctx context.Context) (any, error) { return completeWorkbook(ctx, lifecycle, action, out) })
	}
	return out, err
}

func completeWorkbook(ctx context.Context, lifecycle PublishLifecycle, action *publisher, out PublishOutput) (PublishOutput, error) {
	observed, err := lifecycle.Wait(ctx, out.Result.JobID)
	if observed.Status != "" {
		out.Result.Status, out.Result.TableauRequestID = observed.Status, observed.RequestID
	}
	if err == nil && observed.ResourceID != "" {
		out.Result.WorkbookLUID, out.Result.WorkbookName, out.Result.ProjectLUID, err = lifecycle.Destination(ctx, observed.ResourceID)
	}
	if err != nil {
		if out.Result.Status == "succeeded" {
			out.Result.Verification = "destination_unavailable"
		}
		err = errs.PublicationMonitorError("workbook.publish", out.Plan.Target.Environment, out.Plan.Target.Site, out.Result.JobID, out.Result.Status, err)
	} else {
		out, err = action.Complete(ctx, out)
	}
	path, saveErr := lifecycle.Record(ctx, out.Result.JobID, out.Result.Status, out.Result.WorkbookLUID, out.Result.TableauRequestID, out.Result.Verification)
	out.Result.ReceiptPath = path
	return out, errors.Join(err, saveErr)
}
