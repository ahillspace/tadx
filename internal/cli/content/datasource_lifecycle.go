package content

import (
	"context"
	"errors"

	datasourcedelete "github.com/ahillspace/tadx/actions/datasource/delete"
	datasourcepublish "github.com/ahillspace/tadx/actions/datasource/publish"
	datasourcepull "github.com/ahillspace/tadx/actions/datasource/pull"
	"github.com/ahillspace/tadx/internal/cli/clierr"
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
	var luid, name, projectPath string
	command := &cobra.Command{
		Use: "pull", Short: "Pull one unchanged native datasource artifact.",
		Annotations: map[string]string{"tadx.capability": "datasource.pull"},
		Args:        selectorArgs("datasource.pull", &luid, &name, &projectPath, input.SetSelector),
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.puller.PullDatasource(command.Context(), input)
			if err != nil {
				return err
			}
			return deps.renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().StringVar(&input.Workspace, "workspace", "", "logical workspace name; uses deterministic defaults when omitted")
	command.Flags().StringVar(&luid, "id", "", "authoritative datasource LUID")
	command.Flags().StringVar(&name, "name", "", "exact datasource name")
	command.Flags().StringVar(&projectPath, "project", "", "exact slash-delimited project path")
	command.Flags().BoolVar(&input.Overwrite, "overwrite", false, "replace a dirty local datasource artifact")
	return command
}

func newDatasourcePublish(deps datasourceLifecycleDependencies) *cobra.Command {
	var input datasourcepublish.Input
	var projectLUID, projectPath string
	var create, overwrite, appendMode, replace bool
	var apply bool
	command := &cobra.Command{
		Use: "publish", Short: "Preview or publish one native datasource artifact.",
		Annotations: map[string]string{"tadx.capability": "datasource.publish"},
		Args: func(command *cobra.Command, args []string) error {
			if err := noContentArgs("datasource.publish")(command, args); err != nil {
				return err
			}
			if err := validateManagedArtifactPath(input.ArtifactPath, "datasource"); err != nil {
				return clierr.Usage("datasource.publish", err)
			}
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
			result, err := deps.publisher.PublishDatasource(command.Context(), input, apply)
			if err != nil {
				return err
			}
			return deps.renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.ArtifactPath, "artifact", "", managedArtifactFlagHelp("datasource", "Sales--identity"))
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
	command.Flags().BoolVar(&apply, "apply", false, "apply the previewed remote mutation")
	return command
}

func newDatasourceDelete(deps datasourceLifecycleDependencies) *cobra.Command {
	var input datasourcedelete.Input
	var luid, name, projectPath string
	var apply bool
	command := &cobra.Command{
		Use: "delete", Short: "Preview or delete one exact remote datasource.",
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
			result, err := deps.deleter.DeleteDatasource(command.Context(), input, apply)
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
	command.Flags().BoolVar(&apply, "apply", false, "apply the previewed remote deletion")
	return command
}
