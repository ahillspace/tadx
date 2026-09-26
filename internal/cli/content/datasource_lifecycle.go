package content

import (
	"context"
	"errors"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"

	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/ahillspace/tadx/internal/cli/progress"
	"github.com/spf13/cobra"
)

// DatasourcePuller pulls one unchanged native datasource artifact.
type DatasourcePuller interface {
	PullDatasource(context.Context, datasourceops.PullInput) (datasourceops.PullOutput, error)
}

// DatasourcePublisher previews or applies one datasource publication.
type DatasourcePublisher interface {
	PublishDatasource(context.Context, datasourceops.PublishInput, bool) (datasourceops.PublishOutput, error)
}

// DatasourceDeleter previews or applies one exact datasource deletion.
type DatasourceDeleter interface {
	DeleteDatasource(context.Context, datasourceops.DeleteInput, bool) (datasourceops.DeleteOutput, error)
}

type datasourceLifecycleDependencies struct {
	puller           DatasourcePuller
	publisher        DatasourcePublisher
	deleter          DatasourceDeleter
	renderer         Renderer
	mutationsEnabled bool
}

// addDatasourceLifecycle adds lifecycle actions to an existing datasource root.
func addDatasourceLifecycle(command *cobra.Command, deps datasourceLifecycleDependencies) {
	command.AddCommand(newDatasourcePull(deps), newDatasourcePublish(deps), newDatasourceDelete(deps))
}

func newDatasourcePull(deps datasourceLifecycleDependencies) *cobra.Command {
	var input datasourceops.PullInput
	var ids []string
	var name, projectPath string
	command := &cobra.Command{
		Use: "pull", Short: "Pull unchanged native datasource artifacts sequentially.",
		Annotations: map[string]string{"tadx.capability": "datasource.pull"},
		Args:        batchPullArgs("datasource.pull", &ids, &name, &projectPath, input.SetSelector),
		RunE: func(command *cobra.Command, _ []string) error {
			return runContentSelection(command.Context(), "datasource.pull", ids, deps.renderer, func(ctx context.Context, id string) (datasourceops.PullOutput, error) {
				item := input
				if id != "" {
					item.SetSelector(id, "", "")
				}
				return deps.puller.PullDatasource(ctx, item)
			})
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().StringVar(&input.Workspace, "workspace", "", "logical workspace name; uses deterministic defaults when omitted")
	command.Flags().StringArrayVar(&ids, "id", nil, "authoritative datasource LUID; repeat for up to 100 items, processed sequentially")
	command.Flags().StringVar(&name, "name", "", "exact datasource name; requires --project instead of --id")
	command.Flags().StringVar(&projectPath, "project", "", "exact slash-delimited project path; required with --name")
	command.Flags().BoolVar(&input.Overwrite, "overwrite", false, "replace a dirty local datasource artifact")
	command.Flags().BoolVar(&input.Preview, "preview", false, "resolve acquisition scope and local conflicts without writing artifacts")
	command.Flags().Bool("no-wait", false, "start the local download in the background and return one check-status command without polling")
	command.MarkFlagsMutuallyExclusive("preview", "no-wait")
	return command
}

func newDatasourcePublish(deps datasourceLifecycleDependencies) *cobra.Command {
	var input datasourceops.PublishInput
	var artifacts []string
	var projectLUID, projectPath string
	var create, overwrite, appendMode, replace bool
	var preview bool
	command := &cobra.Command{
		Use: "publish", Short: "Publish native datasource artifacts sequentially.",
		Annotations: map[string]string{"tadx.capability": "datasource.publish"},
		Args: func(command *cobra.Command, args []string) error {
			if err := noContentArgs("datasource.publish")(command, args); err != nil {
				return err
			}
			var err error
			artifacts, err = publishSelections(artifacts, "datasource", input.Name, input.File, input.ArtifactID, input.ArtifactName)
			if err != nil {
				return clierr.Usage("datasource.publish", err)
			}
			input.ArtifactPath = artifacts[0]
			if (projectLUID == "") == (projectPath == "") {
				return clierr.Usage("datasource.publish", errors.New("use exactly one of --project-id or --project"))
			}
			input.SetProjectSelector(projectLUID, projectPath)
			modes := 0
			for _, selected := range []bool{create, overwrite, appendMode, replace} {
				if selected {
					modes++
				}
			}
			if modes != 1 {
				return clierr.Usage("datasource.publish", errors.New("use exactly one of --create, --overwrite, --append, or --replace"))
			}
			switch {
			case create:
				input.Mode = datasourceops.ModeCreate
			case overwrite:
				input.Mode = datasourceops.ModeOverwrite
			case appendMode:
				input.Mode = datasourceops.ModeAppend
			case replace:
				input.Mode = datasourceops.ModeReplace
			}
			return datasourceops.ValidatePublishInput(input)
		},
		RunE: func(command *cobra.Command, _ []string) error {
			reporter := progress.New(command.ErrOrStderr())
			return runPublishSelection(command.Context(), "datasource.publish", "datasource", artifacts, preview, deps.renderer, reporter, func(ctx context.Context, artifact string) (datasourceops.PublishOutput, error) {
				item := input
				item.ArtifactPath = artifact
				return deps.publisher.PublishDatasource(ctx, item, preview)
			})
		},
	}
	command.Flags().StringArrayVar(&artifacts, "artifact", nil, managedArtifactFlagHelp("datasource", "Sales--identity")+"; repeat for up to 100 items, processed sequentially")
	command.Flags().StringVar(&input.File, "file", "", "native .tds or .tdsx file; no managed artifact required")
	command.Flags().StringVar(&input.ArtifactID, "id", "", "exact source datasource LUID within the resolved workspace")
	command.Flags().StringVar(&input.ArtifactName, "artifact-name", "", "unique exact managed datasource name within the resolved workspace")
	command.Flags().StringVar(&input.Workspace, "workspace", "", "logical workspace name; uses deterministic defaults when omitted")
	command.Flags().StringVar(&input.Environment, "environment", "", "write environment alias; may be omitted when exactly one environment is configured")
	command.Flags().StringVar(&input.Name, "name", "", "published datasource name; defaults to the artifact name")
	command.Flags().StringVar(&projectLUID, "project-id", "", "authoritative destination project LUID")
	command.Flags().StringVar(&projectPath, "project", "", "exact slash-delimited destination project path")
	command.Flags().BoolVar(&create, "create", false, "create a new datasource and fail on an exact collision")
	command.Flags().BoolVar(&overwrite, "overwrite", false, "overwrite the exact colliding datasource")
	command.Flags().BoolVar(&appendMode, "append", false, "append to the exact colliding datasource")
	command.Flags().BoolVar(&replace, "replace", false, "replace data in the exact colliding datasource")
	command.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	command.Flags().Bool("no-wait", false, "start publication in the background and return a check-status command without polling")
	command.MarkFlagsMutuallyExclusive("preview", "no-wait")
	return command
}

func newDatasourceDelete(deps datasourceLifecycleDependencies) *cobra.Command {
	var input datasourceops.DeleteInput
	var luid, name, projectPath string
	var preview bool
	command := &cobra.Command{
		Use: "delete", Short: "Delete one exact remote datasource.",
		Annotations: map[string]string{"tadx.capability": "datasource.delete"},
		Args: func(command *cobra.Command, args []string) error {
			if err := selectorArgs("datasource.delete", &luid, &name, &projectPath, input.SetSelector)(command, args); err != nil {
				return err
			}
			if input.Environment == "" {
				return clierr.Usage("datasource.delete", errors.New("--environment is required for remote deletion"))
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.deleter.DeleteDatasource(command.Context(), input, preview)
			if err != nil {
				return clierr.WithOutput(result, err)
			}
			return deps.renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "explicit write environment alias")
	command.Flags().StringVar(&luid, "id", "", "authoritative datasource LUID")
	command.Flags().StringVar(&name, "name", "", "exact datasource name; requires --project instead of --id")
	command.Flags().StringVar(&projectPath, "project", "", "exact slash-delimited project path; required with --name")
	command.Flags().BoolVar(&preview, "preview", false, "preview the remote deletion without performing it")
	return command
}
