package definition

import (
	"context"

	"github.com/ahillspace/tadx/internal/errs"
)

type PublishBundleReader interface {
	LoadBundle(context.Context, string, string, PublishInput) (PublishInput, PublishBundle, error)
}

type PublishSession struct {
	Environment, Site, SiteLUID string
	Validator                   PublishValidator
	Writer                      PublishWriter
}

type PublishProvider interface {
	ResolveBundleWorkspace(context.Context, string, string) (PullWorkspace, error)
	OpenDefinitionPublish(context.Context, string, string) (PublishSession, error)
	BundleReader() PublishBundleReader
}

func (s *Service) PublishPulseDefinition(ctx context.Context, input PublishInput) (PublishOutput, error) {
	if err := publishValidateInput(&input); err != nil {
		return PublishOutput{}, err
	}
	if s == nil || s.ports.Publish == nil {
		return PublishOutput{}, &errs.Error{ID: "pulse.definition.publish.unconfigured", Kind: errs.KindRuntime, Operation: "pulse.definition.publish", Summary: "Pulse definition publish is not configured.", Retryable: errs.Bool(false)}
	}
	workspace, err := s.ports.Publish.ResolveBundleWorkspace(ctx, input.Workspace, input.Environment)
	if err != nil {
		return PublishOutput{}, err
	}
	input, bundle, err := s.ports.Publish.BundleReader().LoadBundle(ctx, workspace.Root, workspace.Name, input)
	if err != nil {
		return PublishOutput{}, err
	}
	plan, err := preparePublishBundle(input, bundle)
	if err != nil {
		return PublishOutput{}, err
	}
	session, err := s.ports.Publish.OpenDefinitionPublish(ctx, input.Environment, input.Site)
	if err != nil {
		return PublishOutput{}, err
	}
	input.Environment, input.Site, input.SiteLUID = session.Environment, session.Site, session.SiteLUID
	return runPublish(ctx, session.Validator, session.Writer, input, plan)
}
