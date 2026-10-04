package definition

import (
	"context"
	"errors"
	"time"

	"github.com/ahillspace/tadx/internal/readsource"
)

// These helpers follow composition's input-validation boundary before invoking operations.
func list(ctx context.Context, reader ListReader, input ListInput) (ListOutput, error) {
	if err := ListValidateInput(&input); err != nil {
		return ListOutput{}, err
	}
	if err := ListValidateContinuation(input); err != nil {
		return ListOutput{}, err
	}
	return List(ctx, reader, input)
}

func inspect(ctx context.Context, reader InspectReader, input InspectInput) (InspectOutput, error) {
	return New(Ports{Read: workflowDefinitionProvider{target: ReadTarget{Environment: input.Environment, Site: input.Site}, port: workflowDefinitionPort{InspectReader: reader}}}).InspectPulseDefinition(ctx, input)
}

type workflowDefinitionPort struct{ InspectReader }

func (workflowDefinitionPort) ListDefinitions(context.Context, ListPageRequest) (ListPage, error) {
	return ListPage{}, errors.New("unexpected definition list")
}
func (workflowDefinitionPort) Source() *readsource.Metadata { return nil }
func (workflowDefinitionPort) Publish()                     {}

type workflowDefinitionProvider struct {
	target ReadTarget
	port   workflowDefinitionPort
}

func (p workflowDefinitionProvider) CacheTarget(string) (ReadTarget, error) {
	return p.target, nil
}
func (p workflowDefinitionProvider) CachedList(ReadTarget) CachedListPort {
	return p.port
}
func (p workflowDefinitionProvider) CachedInspect(ReadTarget) CachedInspectPort {
	return p.port
}
func (p workflowDefinitionProvider) Open(context.Context, string, string, string) (ReadSession, error) {
	return ReadSession{ReadTarget: p.target, List: p.port, Inspect: p.port}, nil
}
func (workflowDefinitionProvider) CacheSetupError(_, _ string, err error) error { return err }
func (workflowDefinitionProvider) Now() time.Time                               { return time.Now() }

func deleteWorkflow(ctx context.Context, reader DeleteReader, deleter Deleter, input DeleteInput) (DeleteOutput, error) {
	if err := deleteValidateInput(input); err != nil {
		return DeleteOutput{}, err
	}
	return runDelete(ctx, reader, deleter, input)
}

func pull(ctx context.Context, reader PullReader, writer PullWriter, input PullInput) (PullOutput, error) {
	if err := pullValidateInput(input); err != nil {
		return PullOutput{}, err
	}
	return runPull(ctx, reader, writer, input)
}

func create(ctx context.Context, validator CreateFieldValidator, finder CreateCollisionFinder, creator CreateCreator, input CreateInput, preview bool) (CreateOutput, error) {
	if err := createValidateInput(&input); err != nil {
		return CreateOutput{}, err
	}
	return runCreate(ctx, validator, finder, creator, input, preview)
}

func preparePublish(input PublishInput, bundle PublishBundle) (PublishPlan, error) {
	if err := publishValidateInput(&input); err != nil {
		return PublishPlan{}, err
	}
	return preparePublishBundle(input, bundle)
}

func publish(ctx context.Context, deps *publishDependencies, input PublishInput) (PublishOutput, error) {
	plan, err := preparePublish(input, deps.bundle)
	if err != nil {
		return PublishOutput{}, err
	}
	return runPublish(ctx, deps, deps, input, plan)
}
