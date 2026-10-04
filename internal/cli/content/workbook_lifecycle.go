package content

import (
	"context"
	"errors"

	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/ahillspace/tadx/internal/cli/progress"
	"github.com/ahillspace/tadx/internal/contentbatch"
	"github.com/spf13/cobra"
)

func newPull(deps Dependencies) *cobra.Command {
	use := deps.PullUse
	if use == "" {
		use = "pull"
	}
	short := deps.PullShort
	if short == "" {
		short = "Pull workbook artifacts sequentially."
	}
	var input workbookops.PullInput
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
			return runContentSelection(command.Context(), "workbook.pull", ids, deps.Renderer, func(ctx context.Context, id string) (workbookops.PullOutput, error) {
				item := input
				if id != "" {
					item.LUID = id
				}
				return deps.Puller.PullWorkbook(ctx, item)
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
	var input workbookops.PublishInput
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
			return runPublishSelection(command.Context(), "workbook.publish", "workbook", artifacts, preview, deps.Renderer, reporter, func(ctx context.Context, artifact string) (workbookops.PublishOutput, error) {
				item := input
				item.ArtifactPath = artifact
				return deps.Publisher.PublishWorkbook(ctx, item, preview)
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
