package definition_test

import (
	"context"
	"errors"
	"time"

	pulsedefinition "github.com/ahillspace/tadx/actions/pulse/definition"
	"github.com/ahillspace/tadx/internal/readsource"
)

// These helpers follow composition's input-validation boundary before invoking operations.
func list(ctx context.Context, reader pulsedefinition.ListReader, input pulsedefinition.ListInput) (pulsedefinition.ListOutput, error) {
	if err := pulsedefinition.ListValidateInput(&input); err != nil {
		return pulsedefinition.ListOutput{}, err
	}
	if err := pulsedefinition.ListValidateContinuation(input); err != nil {
		return pulsedefinition.ListOutput{}, err
	}
	return pulsedefinition.List(ctx, reader, input)
}

func inspect(ctx context.Context, reader pulsedefinition.InspectReader, input pulsedefinition.InspectInput) (pulsedefinition.InspectOutput, error) {
	return pulsedefinition.New(workflowDefinitionProvider{target: pulsedefinition.ReadTarget{Environment: input.Environment, Site: input.Site}, port: workflowDefinitionPort{InspectReader: reader}}).InspectPulseDefinition(ctx, input)
}

type workflowDefinitionPort struct{ pulsedefinition.InspectReader }

func (workflowDefinitionPort) ListDefinitions(context.Context, pulsedefinition.ListPageRequest) (pulsedefinition.ListPage, error) {
	return pulsedefinition.ListPage{}, errors.New("unexpected definition list")
}
func (workflowDefinitionPort) Source() *readsource.Metadata { return nil }
func (workflowDefinitionPort) Publish()                     {}

type workflowDefinitionProvider struct {
	target pulsedefinition.ReadTarget
	port   workflowDefinitionPort
}

func (p workflowDefinitionProvider) CacheTarget(string) (pulsedefinition.ReadTarget, error) {
	return p.target, nil
}
func (p workflowDefinitionProvider) CachedList(pulsedefinition.ReadTarget) pulsedefinition.CachedListPort {
	return p.port
}
func (p workflowDefinitionProvider) CachedInspect(pulsedefinition.ReadTarget) pulsedefinition.CachedInspectPort {
	return p.port
}
func (p workflowDefinitionProvider) Open(context.Context, string, string, string) (pulsedefinition.ReadSession, error) {
	return pulsedefinition.ReadSession{ReadTarget: p.target, List: p.port, Inspect: p.port}, nil
}
func (workflowDefinitionProvider) CacheSetupError(_, _ string, err error) error { return err }
func (workflowDefinitionProvider) Now() time.Time                               { return time.Now() }

func delete(ctx context.Context, reader pulsedefinition.DeleteReader, deleter pulsedefinition.Deleter, input pulsedefinition.DeleteInput) (pulsedefinition.DeleteOutput, error) {
	if err := pulsedefinition.DeleteValidateInput(input); err != nil {
		return pulsedefinition.DeleteOutput{}, err
	}
	return pulsedefinition.Delete(ctx, reader, deleter, input)
}

func pull(ctx context.Context, reader pulsedefinition.PullReader, writer pulsedefinition.PullWriter, input pulsedefinition.PullInput) (pulsedefinition.PullOutput, error) {
	if err := pulsedefinition.PullValidateInput(input); err != nil {
		return pulsedefinition.PullOutput{}, err
	}
	return pulsedefinition.Pull(ctx, reader, writer, input)
}

func create(ctx context.Context, validator pulsedefinition.CreateFieldValidator, finder pulsedefinition.CreateCollisionFinder, creator pulsedefinition.CreateCreator, input pulsedefinition.CreateInput, preview bool) (pulsedefinition.CreateOutput, error) {
	if err := pulsedefinition.CreateValidateInput(&input); err != nil {
		return pulsedefinition.CreateOutput{}, err
	}
	return pulsedefinition.Create(ctx, validator, finder, creator, input, preview)
}

func preparePublish(input pulsedefinition.PublishInput, bundle pulsedefinition.PublishBundle) (pulsedefinition.PublishPlan, error) {
	if err := pulsedefinition.PublishValidateInput(&input); err != nil {
		return pulsedefinition.PublishPlan{}, err
	}
	return pulsedefinition.PublishPrepareBundle(input, bundle)
}

func publish(ctx context.Context, deps *publishDependencies, input pulsedefinition.PublishInput) (pulsedefinition.PublishOutput, error) {
	plan, err := preparePublish(input, deps.bundle)
	if err != nil {
		return pulsedefinition.PublishOutput{}, err
	}
	return pulsedefinition.Publish(ctx, deps, deps, input, plan)
}
