package content

import (
	"context"
	"errors"

	datasourcedelete "github.com/ahillspace/tadx/actions/datasource/delete"
	datasourcepublish "github.com/ahillspace/tadx/actions/datasource/publish"
	datasourcepull "github.com/ahillspace/tadx/actions/datasource/pull"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/ahillspace/tadx/internal/cli/progress"
	"github.com/spf13/cobra"
)

// DatasourcePuller pulls one unchanged native datasource artifact.
type DatasourcePuller interface {
	PullDatasource(context.Context, datasourcepull.Input) (datasourcepull.Output, error)
}

// DatasourcePublisher previews or applies one datasource publication.
type DatasourcePublisher interface {
	PublishDatasource(context.Context, datasourcepublish.Input, bool) (datasourcepublish.Output, error)
}

// DatasourceDeleter previews or applies one exact datasource deletion.
type DatasourceDeleter interface {
	DeleteDatasource(context.Context, datasourcedelete.Input, bool) (datasourcedelete.Output, error)
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
	var input datasourcepull.Input
	var ids []string
	var name, projectPath string
	command := &cobra.Command{
		Use: "pull", Short: "Pull unchanged native datasource artifacts sequentially.",
		Annotations: map[string]string{"tadx.capability": "datasource.pull"},
		Args:        batchPullArgs("datasource.pull", &ids, &name, &projectPath, input.SetSelector),
		RunE: func(command *cobra.Command, _ []string) error {
			return runContentSelection(command.Context(), "datasource.pull", ids, deps.renderer, func(ctx context.Context, id string) (datasourcepull.Output, error) {
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
	command.Flags().StringVar(&name, "name", "", "exact datasource name")
	command.Flags().StringVar(&projectPath, "project", "", "exact slash-delimited project path")
	command.Flags().BoolVar(&input.Overwrite, "overwrite", false, "replace a dirty local datasource artifact")
	return command
}

func newDatasourcePublish(deps datasourceLifecycleDependencies) *cobra.Command {
	var input datasourcepublish.Input
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
			if err := validateArtifactSelection(artifacts, "datasource", input.Name); err != nil {
				return clierr.Usage("datasource.publish", err)
			}
			input.ArtifactPath = artifacts[0]
			if input.Environment == "" {
				if projectLUID != "" || projectPath != "" {
					return clierr.Usage("datasource.publish", errors.New("--project-id and --project require an explicit --environment"))
				}
				input.SourceDefaulted = true
			} else {
				if (projectLUID == "") == (projectPath == "") {
					return clierr.Usage("datasource.publish", errors.New("an explicit --environment requires exactly one of --project-id or --project"))
				}
				input.SourceDefaulted = false
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
				input.Mode = datasourcepublish.ModeCreate
			case overwrite:
				input.Mode = datasourcepublish.ModeOverwrite
			case appendMode:
				input.Mode = datasourcepublish.ModeAppend
			case replace:
				input.Mode = datasourcepublish.ModeReplace
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			reporter := progress.New(command.ErrOrStderr())
			return runPublishSelection(command.Context(), "datasource.publish", "datasource", artifacts, preview, deps.renderer, reporter, func(ctx context.Context, artifact string) (datasourcepublish.Output, error) {
				item := input
				item.ArtifactPath = artifact
				return deps.publisher.PublishDatasource(ctx, item, preview)
			})
		},
	}
	command.Flags().StringArrayVar(&artifacts, "artifact", nil, managedArtifactFlagHelp("datasource", "Sales--identity")+"; repeat for up to 100 items, processed sequentially")
	command.Flags().StringVar(&input.Workspace, "workspace", "", "logical workspace name; uses deterministic defaults when omitted")
	command.Flags().StringVar(&input.Environment, "environment", "", "explicit write environment alias; defaults to the artifact source")
	command.Flags().StringVar(&input.Name, "name", "", "published datasource name; defaults to the artifact name")
	command.Flags().StringVar(&projectLUID, "project-id", "", "authoritative destination project LUID")
	command.Flags().StringVar(&projectPath, "project", "", "exact slash-delimited destination project path")
	command.Flags().BoolVar(&create, "create", false, "create a new datasource and fail on an exact collision")
	command.Flags().BoolVar(&overwrite, "overwrite", false, "overwrite the exact colliding datasource")
	command.Flags().BoolVar(&appendMode, "append", false, "append to the exact colliding datasource")
	command.Flags().BoolVar(&replace, "replace", false, "replace data in the exact colliding datasource")
	command.Flags().BoolVar(&input.AsJob, "as-job", false, "publish asynchronously and poll to a bounded terminal result")
	command.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return command
}

func newDatasourceDelete(deps datasourceLifecycleDependencies) *cobra.Command {
	var input datasourcedelete.Input
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
				return err
			}
			return deps.renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "explicit write environment alias")
	command.Flags().StringVar(&luid, "id", "", "authoritative datasource LUID")
	command.Flags().StringVar(&name, "name", "", "exact datasource name")
	command.Flags().StringVar(&projectPath, "project", "", "exact slash-delimited project path")
	command.Flags().BoolVar(&preview, "preview", false, "preview the remote deletion without performing it")
	return command
}
