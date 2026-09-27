package definition_test

import (
	"context"

	pulsedefinition "github.com/ahillspace/tadx/actions/pulse/definition"
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
	if err := pulsedefinition.InspectValidateInput(input); err != nil {
		return pulsedefinition.InspectOutput{}, err
	}
	return pulsedefinition.Inspect(ctx, reader, input)
}

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
