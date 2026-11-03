package content

import (
	"context"
	"fmt"

	"github.com/ahillspace/tadx/actions/batchinput"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	flowops "github.com/ahillspace/tadx/actions/flow"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/spf13/cobra"
)

func publicationPreparation(command *cobra.Command, resolver batchinput.Resolver, environment, operation string, count int) (context.Context, bool, error) {
	ctx := command.Context()
	if ctx == nil {
		return nil, false, nil
	}
	if !batchinput.Active(ctx) && count <= 1 && !batchinput.HasPublicationExpectations(ctx) {
		return ctx, false, nil
	}
	ctx = batchinput.EnsurePublicationPreparation(ctx)
	command.SetContext(ctx)
	if !batchinput.Active(ctx) {
		ctx, _ = batchinput.Collect(ctx, operation)
	}
	if err := batchinput.Authorize(ctx, resolver, environment); err != nil {
		return ctx, false, err
	}
	return ctx, true, nil
}

func recordPublication(ctx context.Context, kind string, destination batchinput.PublishDestination, seen map[batchinput.Target]bool) error {
	if destination.Environment == "" || destination.ProjectLUID == "" || destination.Name == "" {
		return fmt.Errorf("%s publication plan omitted its authoritative destination", kind)
	}
	for _, target := range batchinput.PublicationTargets(kind, destination) {
		if seen[target] {
			return fmt.Errorf("multiple %s sources select the same publication destination %q", kind, destination.Name)
		}
		seen[target] = true
		batchinput.Record(ctx, target)
	}
	return batchinput.RecordPublication(ctx, destination)
}

func prepareWorkbookPublications(command *cobra.Command, deps Dependencies, input workbookops.PublishInput, artifacts []string) (map[string]workbookops.PublishInput, error) {
	local := !batchinput.Active(command.Context())
	ctx, active, err := publicationPreparation(command, deps.BatchTargets, input.Environment, "workbook.publish", len(artifacts))
	if err != nil || !active {
		return nil, err
	}
	prepared := make(map[string]workbookops.PublishInput, len(artifacts))
	seen := make(map[batchinput.Target]bool)
	for _, source := range artifacts {
		item := input
		item.ArtifactPath = source
		item.ExpectedDestination, err = batchinput.NextPublicationExpectation(ctx)
		if err != nil {
			return nil, err
		}
		item = item.ExpectDestination(item.ExpectedDestination)
		out, err := deps.Publisher.Execute(ctx, item, true)
		if err != nil {
			return nil, err
		}
		destination := out.Plan.Destination()
		if err := recordPublication(ctx, "workbook", destination, seen); err != nil {
			return nil, err
		}
		prepared[source] = out.Plan.PrepareInput(item)
	}
	if local {
		return prepared, batchinput.CheckPublicationPreparation(ctx)
	}
	return prepared, nil
}

func prepareDatasourcePublications(command *cobra.Command, deps datasourceLifecycleDependencies, input datasourceops.PublishInput, artifacts []string) (map[string]datasourceops.PublishInput, error) {
	local := !batchinput.Active(command.Context())
	ctx, active, err := publicationPreparation(command, deps.batchTargets, input.Environment, "datasource.publish", len(artifacts))
	if err != nil || !active {
		return nil, err
	}
	prepared := make(map[string]datasourceops.PublishInput, len(artifacts))
	seen := make(map[batchinput.Target]bool)
	for _, source := range artifacts {
		item := input
		item.ArtifactPath = source
		item.ExpectedDestination, err = batchinput.NextPublicationExpectation(ctx)
		if err != nil {
			return nil, err
		}
		item = item.ExpectDestination(item.ExpectedDestination)
		out, err := deps.publisher.PublishDatasource(ctx, item, true)
		if err != nil {
			return nil, err
		}
		destination := out.Plan.Destination()
		if err := recordPublication(ctx, "datasource", destination, seen); err != nil {
			return nil, err
		}
		prepared[source] = out.Plan.PrepareInput(item)
	}
	if local {
		return prepared, batchinput.CheckPublicationPreparation(ctx)
	}
	return prepared, nil
}

func prepareFlowPublications(command *cobra.Command, deps Dependencies, input flowops.PublishInput, artifacts []string) (map[string]flowops.PublishInput, error) {
	local := !batchinput.Active(command.Context())
	ctx, active, err := publicationPreparation(command, deps.BatchTargets, input.Environment, "flow.publish", len(artifacts))
	if err != nil || !active {
		return nil, err
	}
	prepared := make(map[string]flowops.PublishInput, len(artifacts))
	seen := make(map[batchinput.Target]bool)
	for _, source := range artifacts {
		item := input
		item.ArtifactPath = source
		item.ExpectedDestination, err = batchinput.NextPublicationExpectation(ctx)
		if err != nil {
			return nil, err
		}
		item = item.ExpectDestination(item.ExpectedDestination)
		out, err := deps.FlowPublisher.PublishFlow(ctx, item, true)
		if err != nil {
			return nil, err
		}
		destination := out.Plan.Destination()
		if err := recordPublication(ctx, "flow", destination, seen); err != nil {
			return nil, err
		}
		prepared[source] = out.Plan.PrepareInput(item)
	}
	if local {
		return prepared, batchinput.CheckPublicationPreparation(ctx)
	}
	return prepared, nil
}
