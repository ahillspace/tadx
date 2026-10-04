// Package content contains thin Cobra plumbing for content lifecycle commands.
package content

import (
	"context"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	flowops "github.com/ahillspace/tadx/actions/flow"
	projectops "github.com/ahillspace/tadx/actions/project"
	workbookops "github.com/ahillspace/tadx/actions/workbook"

	"github.com/spf13/cobra"
)

// Puller enters the workbook pull Service.
type Puller interface {
	PullWorkbook(context.Context, workbookops.PullInput) (workbookops.PullOutput, error)
}

// Publisher executes workbook.publish or returns a preview.
type Publisher interface {
	PublishWorkbook(context.Context, workbookops.PublishInput, bool) (workbookops.PublishOutput, error)
}

type WorkbookMover interface {
	MoveWorkbook(context.Context, workbookops.MoveInput, bool) (workbookops.MoveOutput, error)
}
type WorkbookUpdater interface {
	UpdateWorkbook(context.Context, workbookops.UpdateInput, bool) (workbookops.UpdateOutput, error)
}
type DatasourceMover interface {
	MoveDatasource(context.Context, datasourceops.MoveInput, bool) (datasourceops.MoveOutput, error)
}
type DatasourceUpdater interface {
	UpdateDatasource(context.Context, datasourceops.UpdateInput, bool) (datasourceops.UpdateOutput, error)
}
type FlowUpdater interface {
	UpdateFlow(context.Context, flowops.UpdateInput, bool) (flowops.UpdateOutput, error)
}
type ProjectMover interface {
	MoveProject(context.Context, projectops.MoveInput, bool) (projectops.MoveOutput, error)
}

// Renderer writes one structured result.
type Renderer interface{ Render(any) error }

// Dependencies contains content command wiring.
type Dependencies struct {
	Puller              Puller
	Publisher           Publisher
	WorkbookLister      WorkbookLister
	WorkbookInspector   WorkbookInspector
	WorkbookDeleter     WorkbookDeleter
	WorkbookMover       WorkbookMover
	WorkbookUpdater     WorkbookUpdater
	DatasourceLister    DatasourceLister
	DatasourceInspector DatasourceInspector
	DatasourceSchema    DatasourceSchemaGetter
	DatasourcePuller    DatasourcePuller
	DatasourcePublisher DatasourcePublisher
	DatasourceDeleter   DatasourceDeleter
	DatasourceMover     DatasourceMover
	DatasourceUpdater   DatasourceUpdater
	ProjectLister       ProjectLister
	ProjectInspector    ProjectInspector
	ProjectCreator      ProjectCreator
	ProjectUpdater      ProjectUpdater
	ProjectDeleter      ProjectDeleter
	ProjectMover        ProjectMover
	FlowLister          FlowLister
	FlowInspector       FlowInspector
	FlowPuller          FlowPuller
	FlowPublisher       FlowPublisher
	FlowMover           FlowMover
	FlowDeleter         FlowDeleter
	FlowUpdater         FlowUpdater
	Renderer            Renderer
	MutationsEnabled    bool
	PullUse             string
	PullShort           string
	PublishUse          string
	PublishShort        string
}

// New creates the content workbook command tree.
func New(deps Dependencies) *cobra.Command {
	content := &cobra.Command{
		Use:   "content",
		Short: "Operate Tableau content lifecycle",
		Long: "Operate workbook, datasource, flow, and project lifecycle with TADX.\n\n" +
			"TADX supports content discovery and schema inspection; it does not query datasource values or render views.",
	}
	workbook := &cobra.Command{Use: "workbook", Short: "Operate Tableau workbooks"}
	workbook.AddCommand(newPull(deps), newPublish(deps))
	if deps.WorkbookLister != nil && deps.WorkbookInspector != nil {
		workbook.AddCommand(newWorkbookList(deps.WorkbookLister, deps.Renderer), newWorkbookInspect(deps.WorkbookInspector, deps.Renderer))
	}
	if deps.WorkbookDeleter != nil {
		workbook.AddCommand(newWorkbookDelete(deps.WorkbookDeleter, deps.Renderer, deps.MutationsEnabled))
	}
	if deps.WorkbookMover != nil {
		workbook.AddCommand(newWorkbookMove(deps))
	}
	if deps.WorkbookUpdater != nil {
		workbook.AddCommand(newWorkbookUpdate(deps))
	}
	content.AddCommand(workbook)
	if deps.DatasourceLister != nil && deps.DatasourceInspector != nil {
		datasource := newDatasourceInventory(deps.DatasourceLister, deps.DatasourceInspector, deps.Renderer)
		if deps.DatasourceSchema != nil {
			datasource.AddCommand(newDatasourceSchema(deps.DatasourceSchema, deps.Renderer))
		}
		if deps.DatasourcePuller != nil && deps.DatasourcePublisher != nil && deps.DatasourceDeleter != nil {
			addDatasourceLifecycle(datasource, datasourceLifecycleDependencies{puller: deps.DatasourcePuller, publisher: deps.DatasourcePublisher, deleter: deps.DatasourceDeleter, renderer: deps.Renderer, mutationsEnabled: deps.MutationsEnabled})
		}
		if deps.DatasourceMover != nil {
			datasource.AddCommand(newDatasourceMove(deps))
		}
		if deps.DatasourceUpdater != nil {
			datasource.AddCommand(newDatasourceUpdate(deps))
		}
		content.AddCommand(datasource)
	}
	if deps.ProjectLister != nil && deps.ProjectInspector != nil {
		content.AddCommand(newProject(deps))
	}
	if deps.FlowLister != nil && deps.FlowInspector != nil && deps.FlowPuller != nil && deps.FlowPublisher != nil && deps.FlowMover != nil && deps.FlowDeleter != nil {
		content.AddCommand(newFlow(deps))
	}
	return content
}
