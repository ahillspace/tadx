package content

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	flowops "github.com/ahillspace/tadx/actions/flow"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/spf13/cobra"
)

type collidingPublicationPlans struct{ writes atomic.Int32 }

func (*collidingPublicationPlans) AuthorizeBatchPreparation(context.Context, string) error {
	return nil
}
func (*collidingPublicationPlans) ResolveBatchResource(context.Context, string, string, identity.Selector) (identity.LUID, error) {
	return "project", nil
}

type collidingWorkbookPlans struct{ *collidingPublicationPlans }

func (p collidingWorkbookPlans) Execute(_ context.Context, _ workbookops.PublishInput, preview bool) (workbookops.PublishOutput, error) {
	if !preview {
		p.writes.Add(1)
	}
	return workbookops.PublishOutput{Plan: workbookops.PublishPlan{WorkbookName: "Destination", Target: workbookops.PublishTarget{Environment: "production", Site: "site", ProjectLUID: "project"}}}, nil
}

func (p *collidingPublicationPlans) PublishDatasource(_ context.Context, _ datasourceops.PublishInput, preview bool) (datasourceops.PublishOutput, error) {
	if !preview {
		p.writes.Add(1)
	}
	return datasourceops.PublishOutput{Plan: datasourceops.PublishPlan{DatasourceName: "Destination", Target: datasourceops.PublishTarget{Environment: "production", Site: "site", ProjectLUID: "project"}}}, nil
}

func (p *collidingPublicationPlans) PublishFlow(_ context.Context, _ flowops.PublishInput, preview bool) (flowops.PublishOutput, error) {
	if !preview {
		p.writes.Add(1)
	}
	return flowops.PublishOutput{Plan: flowops.PublishPlan{FlowName: "Destination", Target: flowops.PublishTarget{Environment: "production", Site: "site", ProjectLUID: "project"}}}, nil
}

func TestNativePublicationSelectionRejectsDestinationCollisionsBeforeWriting(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow"} {
		t.Run(kind, func(t *testing.T) {
			provider := &collidingPublicationPlans{}
			deps := Dependencies{Publisher: collidingWorkbookPlans{provider}, FlowPublisher: provider, BatchTargets: provider, Renderer: &batchRenderer{}}
			var command *cobra.Command
			switch kind {
			case "workbook":
				command = newPublish(deps)
			case "datasource":
				command = newDatasourcePublish(datasourceLifecycleDependencies{publisher: provider, batchTargets: provider, renderer: deps.Renderer})
			default:
				command = newFlowPublish(deps)
			}
			command.SilenceErrors, command.SilenceUsage = true, true
			command.SetArgs([]string{"--artifact", "artifacts/" + kind + "/first", "--artifact", "artifacts/" + kind + "/second", "--environment", "production", "--project-id", "project", "--overwrite"})
			if err := command.ExecuteContext(t.Context()); err == nil || !strings.Contains(err.Error(), "same publication destination") || provider.writes.Load() != 0 {
				t.Fatalf("writes=%d error=%v", provider.writes.Load(), err)
			}
		})
	}
}
