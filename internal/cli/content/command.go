// Package content contains thin Cobra plumbing for content lifecycle commands.
package content

import (
	"context"
	"errors"

	workbookpublish "github.com/ahillspace/tadx/actions/workbook/publish"
	workbookpull "github.com/ahillspace/tadx/actions/workbook/pull"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

// Puller executes workbook.pull.
type Puller interface {
	Execute(context.Context, workbookpull.Input) (workbookpull.Output, error)
}

// Publisher executes workbook.publish preview or apply.
type Publisher interface {
	Execute(context.Context, workbookpublish.Input, bool) (workbookpublish.Output, error)
}

// Renderer writes one structured result.
type Renderer interface{ Render(any) error }

// Dependencies contains content command wiring.
type Dependencies struct {
	Puller           Puller
	Publisher        Publisher
	Renderer         Renderer
	MutationsEnabled bool
	PullUse          string
	PullShort        string
	PublishUse       string
	PublishShort     string
}

// New creates the content workbook command tree.
func New(deps Dependencies) *cobra.Command {
	content := &cobra.Command{Use: "content", Short: "Operate Tableau content lifecycle"}
	workbook := &cobra.Command{Use: "workbook", Short: "Operate Tableau workbooks"}
	workbook.AddCommand(newPull(deps), newPublish(deps))
	content.AddCommand(workbook)
	return content
}

func newPull(deps Dependencies) *cobra.Command {
	use := deps.PullUse
	if use == "" {
		use = "pull"
	}
	short := deps.PullShort
	if short == "" {
		short = "Pull one workbook artifact."
	}
	var input workbookpull.Input
	var id, name, project string
	var includeExtract bool
	command := &cobra.Command{
		Use: use, Short: short, Annotations: map[string]string{"tadx.capability": "workbook.pull"},
		Args: func(command *cobra.Command, args []string) error {
			if err := cobra.NoArgs(command, args); err != nil {
				return clierr.Usage("workbook.pull", err)
			}
			if id == "" && name == "" {
				return clierr.Usage("workbook.pull", errors.New("one of --id or --name is required"))
			}
			input.LUID, input.Name, input.ProjectPath = id, name, project
			if command.Flags().Changed("include-extract") {
				value := includeExtract
				input.IncludeExtract = &value
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.Puller.Execute(command.Context(), input)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to configured read environment")
	command.Flags().StringVar(&input.Workspace, "workspace", "", "existing workspace path")
	command.Flags().StringVar(&id, "id", "", "authoritative workbook LUID")
	command.Flags().StringVar(&name, "name", "", "exact workbook name")
	command.Flags().StringVar(&project, "project", "", "exact slash-delimited project path")
	command.Flags().BoolVar(&includeExtract, "include-extract", true, "include workbook extracts")
	command.Flags().BoolVar(&input.Overwrite, "overwrite", false, "replace a dirty local artifact")
	return command
}

func newPublish(deps Dependencies) *cobra.Command {
	use := deps.PublishUse
	if use == "" {
		use = "publish"
	}
	short := deps.PublishShort
	if short == "" {
		short = "Preview or publish one workbook artifact."
	}
	var input workbookpublish.Input
	var projectID, projectPath string
	var apply bool
	command := &cobra.Command{
		Use: use, Short: short, Hidden: !deps.MutationsEnabled, Annotations: map[string]string{"tadx.capability": "workbook.publish"},
		Args: func(command *cobra.Command, args []string) error {
			if err := cobra.NoArgs(command, args); err != nil {
				return clierr.Usage("workbook.publish", err)
			}
			if input.Environment == "" || input.ArtifactPath == "" {
				return clierr.Usage("workbook.publish", errors.New("--environment and --artifact are required"))
			}
			if projectID == "" && projectPath == "" {
				return clierr.Usage("workbook.publish", errors.New("one of --project-id or --project is required"))
			}
			input.ProjectLUID, input.ProjectPath = projectID, projectPath
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.Publisher.Execute(command.Context(), input, apply)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.ArtifactPath, "artifact", "", "workbook artifact directory or canonical payload")
	command.Flags().StringVar(&input.Environment, "environment", "", "explicit write environment alias")
	command.Flags().StringVar(&input.Name, "name", "", "explicit published workbook name; defaults to artifact name")
	command.Flags().StringVar(&projectID, "project-id", "", "authoritative destination project LUID")
	command.Flags().StringVar(&projectPath, "project", "", "exact slash-delimited destination project path")
	command.Flags().BoolVar(&input.Overwrite, "overwrite", false, "replace the exact colliding workbook")
	command.Flags().BoolVar(&input.AsJob, "as-job", false, "publish asynchronously and poll to a bounded terminal result")
	command.Flags().BoolVar(&apply, "apply", false, "apply the previewed remote mutation")
	return command
}
