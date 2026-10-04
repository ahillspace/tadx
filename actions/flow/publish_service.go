package flow

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/errs"
)

// PublishProvider binds one validated local flow to its authenticated target.
type PublishProvider interface {
	OpenFlowPublishSource(context.Context, PublishInput) (PublishInput, ArtifactReader, string, error)
	OpenFlowPublish(context.Context, PublishInput, string) (PublishSession, error)
}

type PublishLifecycle interface {
	Record(context.Context, string, string, string, string, string) (string, error)
}

type PublishSession struct {
	Environment string
	Site        string
	Resolver    PublishResolver
	Preparer    PublishPreparer
	Lifecycle   func() PublishLifecycle
}

func (s *Service) PublishFlow(ctx context.Context, input PublishInput, preview bool) (PublishOutput, error) {
	if err := validatePublishInput(input); err != nil {
		return PublishOutput{}, err
	}
	if s == nil || s.ports.Publish == nil {
		return PublishOutput{}, errs.New(errs.KindRuntime, "flow publish provider is not configured")
	}
	input, reader, sourcePath, err := s.ports.Publish.OpenFlowPublishSource(ctx, input)
	if err != nil {
		return PublishOutput{}, err
	}
	session, err := s.ports.Publish.OpenFlowPublish(ctx, input, sourcePath)
	if err != nil {
		return PublishOutput{}, err
	}
	input.Environment, input.Site, input.TargetResolved = session.Environment, session.Site, true
	out, err := newPublisher(reader, session.Resolver, session.Preparer).executeValidated(ctx, input, preview)
	if session.Lifecycle == nil {
		return out, err
	}
	lifecycle := session.Lifecycle()
	if lifecycle == nil || out.Result == nil || out.Result.Status == "" {
		return out, err
	}
	var saveErr error
	out.Result.ReceiptPath, saveErr = lifecycle.Record(ctx, "", out.Result.Status, out.Result.FlowLUID, out.Result.TableauRequestID, "")
	return out, errors.Join(err, saveErr)
}
