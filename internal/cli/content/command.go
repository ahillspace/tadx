// Package content contains thin Cobra plumbing for content lifecycle commands.
package content

import (
	"context"
	"errors"
	"path"
	"strings"

	datasourcemove "github.com/ahillspace/tadx/actions/datasource/move"
	datasourceupdate "github.com/ahillspace/tadx/actions/datasource/update"
	flowupdate "github.com/ahillspace/tadx/actions/flow/update"
	projectmove "github.com/ahillspace/tadx/actions/project/move"
	workbookmove "github.com/ahillspace/tadx/actions/workbook/move"
	workbookpublish "github.com/ahillspace/tadx/actions/workbook/publish"
	workbookpull "github.com/ahillspace/tadx/actions/workbook/pull"
	workbookupdate "github.com/ahillspace/tadx/actions/workbook/update"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/ahillspace/tadx/internal/cli/progress"
	"github.com/ahillspace/tadx/internal/contentbatch"
	"github.com/ahillspace/tadx/internal/pathspec"
	"github.com/spf13/cobra"
)

// Puller executes workbook.pull.
type Puller interface {
	Execute(context.Context, workbookpull.Input) (workbookpull.Output, error)
}

// Publisher executes workbook.publish or returns a preview.
type Publisher interface {
	Execute(context.Context, workbookpublish.Input, bool) (workbookpublish.Output, error)
}

type WorkbookMover interface {
	MoveWorkbook(context.Context, workbookmove.Input, bool) (workbookmove.Output, error)
}
type WorkbookUpdater interface {
	UpdateWorkbook(context.Context, workbookupdate.Input, bool) (workbookupdate.Output, error)
}
type DatasourceMover interface {
	MoveDatasource(context.Context, datasourcemove.Input, bool) (datasourcemove.Output, error)
}
type DatasourceUpdater interface {
	UpdateDatasource(context.Context, datasourceupdate.Input, bool) (datasourceupdate.Output, error)
}
type FlowUpdater interface {
	UpdateFlow(context.Context, flowupdate.Input, bool) (flowupdate.Output, error)
}
type ProjectMover interface {
	MoveProject(context.Context, projectmove.Input, bool) (projectmove.Output, error)
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
	LineagePuller       LineagePuller
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

func newPull(deps Dependencies) *cobra.Command {
	use := deps.PullUse
	if use == "" {
		use = "pull"
	}
	short := deps.PullShort
	if short == "" {
		short = "Pull workbook artifacts sequentially."
	}
	var input workbookpull.Input
	var ids []string
	var name, project string
	var includeExtract bool
	command := &cobra.Command{
		Use: use, Short: short, Annotations: map[string]string{"tadx.capability": "workbook.pull"},
		Args: func(command *cobra.Command, args []string) error {
			if err := cobra.NoArgs(command, args); err != nil {
				return clierr.Usage("workbook.pull", err)
			}
			if len(ids) == 0 && name == "" {
				return clierr.Usage("workbook.pull", errors.New("one of --id or --name is required"))
			}
			if len(ids) > 0 {
				if err := contentbatch.Validate(ids); err != nil {
					return clierr.Usage("workbook.pull", err)
				}
			}
			if len(ids) > 1 {
				if err := batchPullArgs("workbook.pull", &ids, &name, &project, func(id, name, project string) { input.LUID, input.Name, input.ProjectPath = id, name, project })(command, args); err != nil {
					return err
				}
			} else {
				input.Name, input.ProjectPath = name, project
				if len(ids) == 1 {
					input.LUID = ids[0]
				}
			}
			if command.Flags().Changed("include-extract") {
				value := includeExtract
				input.IncludeExtract = &value
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			return runContentSelection(command.Context(), "workbook.pull", ids, deps.Renderer, func(ctx context.Context, id string) (workbookpull.Output, error) {
				item := input
				if id != "" {
					item.LUID = id
				}
				return deps.Puller.Execute(ctx, item)
			})
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to configured read environment")
	command.Flags().StringVar(&input.Workspace, "workspace", "", "logical workspace name; uses deterministic defaults when omitted")
	command.Flags().StringArrayVar(&ids, "id", nil, "authoritative workbook LUID; repeat for up to 100 items, processed sequentially")
	command.Flags().StringVar(&name, "name", "", "exact workbook name")
	command.Flags().StringVar(&project, "project", "", "exact slash-delimited project path")
	command.Flags().BoolVar(&includeExtract, "include-extract", true, "include workbook extracts")
	command.Flags().BoolVar(&input.IncludePDS, "include-pds", false, "acquire direct published datasource dependencies as sibling artifacts without recursion")
	command.Flags().BoolVar(&input.Overwrite, "overwrite", false, "replace a dirty local artifact")
	command.Flags().BoolVar(&input.Preview, "preview", false, "resolve acquisition scope and local conflicts without writing artifacts")
	command.Flags().Bool("no-wait", false, "start the local download in the background and return one check-status command without polling")
	command.MarkFlagsMutuallyExclusive("preview", "no-wait")
	return command
}

func newPublish(deps Dependencies) *cobra.Command {
	use := deps.PublishUse
	if use == "" {
		use = "publish"
	}
	short := deps.PublishShort
	if short == "" {
		short = "Publish workbook artifacts sequentially."
	}
	var input workbookpublish.Input
	var artifacts []string
	var projectID, projectPath string
	var preview bool
	command := &cobra.Command{
		Use: use, Short: short, Annotations: map[string]string{"tadx.capability": "workbook.publish"},
		Args: func(command *cobra.Command, args []string) error {
			if err := cobra.NoArgs(command, args); err != nil {
				return clierr.Usage("workbook.publish", err)
			}
			var err error
			artifacts, err = publishSelections(artifacts, "workbook", input.Name, input.File, input.ArtifactID, input.ArtifactName)
			if err != nil {
				return clierr.Usage("workbook.publish", err)
			}
			input.ArtifactPath = artifacts[0]
			// The command root resolves the write environment independently of
			// artifact provenance. The destination project remains explicit.
			if input.Environment != "" && projectID == "" && projectPath == "" {
				return clierr.Usage("workbook.publish", errors.New("choose a destination with --project-id or --project"))
			}
			input.ProjectLUID, input.ProjectPath = projectID, projectPath
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			reporter := progress.New(command.ErrOrStderr())
			return runPublishSelection(command.Context(), "workbook.publish", "workbook", artifacts, preview, deps.Renderer, reporter, func(ctx context.Context, artifact string) (workbookpublish.Output, error) {
				item := input
				item.ArtifactPath = artifact
				return deps.Publisher.Execute(ctx, item, preview)
			})
		},
	}
	command.Flags().StringVar(&input.Workspace, "workspace", "", "logical workspace name; uses deterministic defaults when omitted")
	command.Flags().StringArrayVar(&artifacts, "artifact", nil, managedArtifactFlagHelp("workbook", "Finance--identity")+"; repeat for up to 100 items, processed sequentially")
	command.Flags().StringVar(&input.File, "file", "", "native .twb or .twbx file; no managed artifact required")
	command.Flags().StringVar(&input.ArtifactID, "id", "", "exact source workbook LUID within the resolved workspace")
	command.Flags().StringVar(&input.ArtifactName, "artifact-name", "", "unique exact managed workbook name within the resolved workspace")
	command.Flags().StringVar(&input.Environment, "environment", "", "write environment alias; may be omitted when exactly one environment is configured")
	command.Flags().StringVar(&input.Name, "name", "", "explicit published workbook name; defaults to artifact name")
	command.Flags().StringVar(&projectID, "project-id", "", "authoritative destination project LUID")
	command.Flags().StringVar(&projectPath, "project", "", "exact slash-delimited destination project path")
	command.Flags().BoolVar(&input.Overwrite, "overwrite", false, "replace the exact colliding workbook")
	command.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	command.Flags().Bool("no-wait", false, "start publication in the background and return a check-status command without polling")
	command.MarkFlagsMutuallyExclusive("preview", "no-wait")
	return command
}

func managedArtifactFlagHelp(kind, exampleName string) string {
	return "managed " + kind + " directory relative to the logical workspace, using forward slashes; for example, artifacts/" + kind + "/" + exampleName
}

func validateManagedArtifactPath(value, kind string) error {
	if pathspec.IsAbs(value) || strings.Contains(value, `\`) || path.Clean(value) != value {
		return errors.New("--artifact must be a workspace-relative slash-delimited managed " + kind + " path")
	}
	parts := strings.Split(value, "/")
	if len(parts) != 3 || parts[0] != "artifacts" || parts[1] != kind || parts[2] == "" {
		return errors.New("--artifact must identify one managed " + kind + " directory under artifacts/" + kind)
	}
	return nil
}
