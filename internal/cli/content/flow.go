package content

import (
	"context"
	"errors"

	flowdelete "github.com/ahillspace/tadx/actions/flow/delete"
	flowinspect "github.com/ahillspace/tadx/actions/flow/inspect"
	flowlist "github.com/ahillspace/tadx/actions/flow/list"
	flowmove "github.com/ahillspace/tadx/actions/flow/move"
	flowpublish "github.com/ahillspace/tadx/actions/flow/publish"
	flowpull "github.com/ahillspace/tadx/actions/flow/pull"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/ahillspace/tadx/internal/cli/progress"
	"github.com/spf13/cobra"
)

type FlowLister interface {
	ListFlows(context.Context, flowlist.Input) (flowlist.Output, error)
}
type FlowInspector interface {
	InspectFlow(context.Context, flowinspect.Input) (flowinspect.Output, error)
}
type FlowPuller interface {
	PullFlow(context.Context, flowpull.Input) (flowpull.Output, error)
}
type FlowPublisher interface {
	PublishFlow(context.Context, flowpublish.Input, bool) (flowpublish.Output, error)
}
type FlowMover interface {
	MoveFlow(context.Context, flowmove.Input, bool) (flowmove.Output, error)
}
type FlowDeleter interface {
	DeleteFlow(context.Context, flowdelete.Input, bool) (flowdelete.Output, error)
}

func newFlow(deps Dependencies) *cobra.Command {
	command := &cobra.Command{Use: "flow", Short: "Operate Tableau flows"}
	command.AddCommand(newFlowList(deps), newFlowInspect(deps), newFlowPull(deps), newFlowPublish(deps), newFlowMove(deps), newFlowDelete(deps))
	if deps.FlowUpdater != nil {
		command.AddCommand(newFlowUpdate(deps))
	}
	return command
}

func newFlowList(deps Dependencies) *cobra.Command {
	var input flowlist.Input
	command := &cobra.Command{Use: "list", Short: "List flows with bounded live reads or explicit --all.", Annotations: map[string]string{"tadx.capability": "flow.list"}, Args: noContentArgs("flow.list"), RunE: func(command *cobra.Command, _ []string) error {
		result, err := deps.FlowLister.ListFlows(command.Context(), input)
		if err != nil {
			return clierr.WithOutput(result, err)
		}
		return deps.Renderer.Render(result)
	}}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().StringVar(&input.Name, "name", "", "exact flow-name filter")
	command.Flags().StringVar(&input.OwnerName, "owner", "", "exact owner-name filter")
	command.Flags().StringVar(&input.ProjectLUID, "project-id", "", "authoritative project LUID filter")
	command.Flags().StringVar(&input.ProjectName, "project-name", "", "exact leaf project name filter; not a project path")
	command.Flags().BoolVar(&input.All, "all", false, "return all matching records, up to 10000; cannot combine with --limit")
	command.Flags().IntVar(&input.Limit, "limit", 0, "maximum flows to render, from 1 to 10000 (default 25)")
	command.Flags().StringVar(&input.Cursor, "cursor", "", "opaque continuation cursor")
	command.MarkFlagsMutuallyExclusive("all", "limit")
	command.MarkFlagsMutuallyExclusive("all", "cursor")
	command.Flags().BoolVar(&input.Cache, "cache", false, "read indexed local cache data without contacting Tableau")
	return command
}

func newFlowInspect(deps Dependencies) *cobra.Command {
	var input flowinspect.Input
	var luid, name, projectPath, projectID string
	command := &cobra.Command{Use: "inspect", Short: "Inspect one exact flow.", Annotations: map[string]string{"tadx.capability": "flow.inspect"}, Args: selectorArgsWithProjectID("flow.inspect", &luid, &name, &projectPath, &projectID, input.SetSelectorWithProjectLUID), RunE: func(command *cobra.Command, _ []string) error {
		result, err := deps.FlowInspector.InspectFlow(command.Context(), input)
		if err != nil {
			return clierr.WithOutput(result, err)
		}
		return deps.Renderer.Render(result)
	}}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().StringVar(&luid, "id", "", "authoritative flow LUID")
	command.Flags().StringVar(&name, "name", "", "exact flow name")
	command.Flags().StringVar(&projectPath, "project", "", "exact slash-delimited project path")
	command.Flags().StringVar(&projectID, "project-id", "", "authoritative project LUID")
	command.Flags().BoolVar(&input.Cache, "cache", false, "read indexed local cache data without contacting Tableau")
	return command
}

func newFlowPull(deps Dependencies) *cobra.Command {
	var input flowpull.Input
	var ids []string
	var name, projectPath string
	command := &cobra.Command{Use: "pull", Short: "Pull unchanged native flow artifacts sequentially.", Annotations: map[string]string{"tadx.capability": "flow.pull"}, Args: batchPullArgs("flow.pull", &ids, &name, &projectPath, input.SetSelector), RunE: func(command *cobra.Command, _ []string) error {
		return runContentSelection(command.Context(), "flow.pull", ids, deps.Renderer, func(ctx context.Context, id string) (flowpull.Output, error) {
			item := input
			if id != "" {
				item.SetSelector(id, "", "")
			}
			return deps.FlowPuller.PullFlow(ctx, item)
		})
	}}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().StringArrayVar(&ids, "id", nil, "authoritative flow LUID; repeat for up to 100 items, processed sequentially")
	command.Flags().StringVar(&name, "name", "", "exact flow name")
	command.Flags().StringVar(&projectPath, "project", "", "exact slash-delimited project path")
	command.Flags().StringVar(&input.Workspace, "workspace", "", "logical workspace name; uses deterministic defaults when omitted")
	command.Flags().BoolVar(&input.Overwrite, "overwrite", false, "replace a dirty local flow artifact")
	command.Flags().BoolVar(&input.Preview, "preview", false, "resolve acquisition scope and local conflicts without writing artifacts")
	command.Flags().Bool("no-wait", false, "start the local download in the background and return one check-status command without polling")
	command.MarkFlagsMutuallyExclusive("preview", "no-wait")
	return command
}

func newFlowPublish(deps Dependencies) *cobra.Command {
	var input flowpublish.Input
	var artifacts []string
	var projectLUID, projectPath string
	var preview bool
	command := &cobra.Command{Use: "publish", Short: "Publish native flow artifacts sequentially.", Annotations: map[string]string{"tadx.capability": "flow.publish"}, Args: func(command *cobra.Command, args []string) error {
		if err := noContentArgs("flow.publish")(command, args); err != nil {
			return err
		}
		var err error
		artifacts, err = publishSelections(artifacts, "flow", input.Name, input.File, input.ArtifactID, input.ArtifactName)
		if err != nil {
			return clierr.Usage("flow.publish", err)
		}
		input.ArtifactPath = artifacts[0]
		if input.Environment != "" && (projectLUID == "") == (projectPath == "") {
			return clierr.Usage("flow.publish", errors.New("an explicit --environment requires exactly one of --project-id or --project"))
		}
		if input.Environment == "" && projectLUID != "" && projectPath != "" {
			return clierr.Usage("flow.publish", errors.New("use at most one of --project-id or --project"))
		}
		input.SetProjectSelector(projectLUID, projectPath)
		return nil
	}, RunE: func(command *cobra.Command, _ []string) error {
		reporter := progress.New(command.ErrOrStderr())
		return runPublishSelection(command.Context(), "flow.publish", "flow", artifacts, preview, deps.Renderer, reporter, func(ctx context.Context, artifact string) (flowpublish.Output, error) {
			item := input
			item.ArtifactPath = artifact
			return deps.FlowPublisher.PublishFlow(ctx, item, preview)
		})
	}}
	command.Flags().StringVar(&input.Workspace, "workspace", "", "logical workspace name; uses deterministic defaults when omitted")
	command.Flags().StringArrayVar(&artifacts, "artifact", nil, managedArtifactFlagHelp("flow", "DailyPrep--identity")+"; repeat for up to 100 items, processed sequentially")
	command.Flags().StringVar(&input.File, "file", "", "native .tfl or .tflx file; no managed artifact required")
	command.Flags().StringVar(&input.ArtifactID, "id", "", "exact source flow LUID within the resolved workspace")
	command.Flags().StringVar(&input.ArtifactName, "artifact-name", "", "unique exact managed flow name within the resolved workspace")
	command.Flags().StringVar(&input.Environment, "environment", "", "write environment alias; may be omitted when exactly one environment is configured")
	command.Flags().StringVar(&input.Name, "name", "", "published flow name; defaults to artifact name")
	command.Flags().StringVar(&projectLUID, "project-id", "", "authoritative destination project LUID")
	command.Flags().StringVar(&projectPath, "project", "", "exact destination project path")
	command.Flags().BoolVar(&input.Overwrite, "overwrite", false, "replace the exact colliding flow")
	command.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	command.Flags().Bool("no-wait", false, "start publication in the background and return a check-status command without polling")
	command.MarkFlagsMutuallyExclusive("preview", "no-wait")
	return command
}

func newFlowMove(deps Dependencies) *cobra.Command {
	var input flowmove.Input
	var flowLUID, name, sourceProject, projectLUID, projectPath string
	var preview bool
	command := &cobra.Command{Use: "move", Short: "Move one exact flow.", Annotations: map[string]string{"tadx.capability": "flow.move"}, Args: func(command *cobra.Command, args []string) error {
		if err := selectorArgs("flow.move", &flowLUID, &name, &sourceProject, input.SetFlowSelector)(command, args); err != nil {
			return err
		}
		if input.Environment == "" {
			return clierr.Usage("flow.move", errors.New("--environment is required for a remote move"))
		}
		if (projectLUID == "") == (projectPath == "") {
			return clierr.Usage("flow.move", errors.New("use exactly one of --destination-project-id or --destination-project"))
		}
		input.SetProjectSelector(projectLUID, projectPath)
		return nil
	}, RunE: func(command *cobra.Command, _ []string) error {
		result, err := deps.FlowMover.MoveFlow(command.Context(), input, preview)
		if err != nil {
			return clierr.WithOutput(result, err)
		}
		return deps.Renderer.Render(result)
	}}
	readTargetFlags(command, &input.Environment, &flowLUID, &name, &sourceProject)
	command.Flags().StringVar(&projectLUID, "destination-project-id", "", "authoritative destination project LUID")
	command.Flags().StringVar(&projectPath, "destination-project", "", "exact destination project path")
	command.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return command
}

func newFlowDelete(deps Dependencies) *cobra.Command {
	var input flowdelete.Input
	var luid, name, projectPath string
	var preview bool
	command := &cobra.Command{Use: "delete", Short: "Delete one exact remote flow.", Annotations: map[string]string{"tadx.capability": "flow.delete"}, Args: func(command *cobra.Command, args []string) error {
		if err := selectorArgs("flow.delete", &luid, &name, &projectPath, input.SetSelector)(command, args); err != nil {
			return err
		}
		if input.Environment == "" {
			return clierr.Usage("flow.delete", errors.New("--environment is required for remote deletion"))
		}
		return nil
	}, RunE: func(command *cobra.Command, _ []string) error {
		result, err := deps.FlowDeleter.DeleteFlow(command.Context(), input, preview)
		if err != nil {
			return clierr.WithOutput(result, err)
		}
		return deps.Renderer.Render(result)
	}}
	readTargetFlags(command, &input.Environment, &luid, &name, &projectPath)
	command.Flags().BoolVar(&preview, "preview", false, "preview the remote deletion without performing it")
	return command
}

func readTargetFlags(command *cobra.Command, environment, luid, name, projectPath *string) {
	command.Flags().StringVar(environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().StringVar(luid, "id", "", "authoritative flow LUID")
	command.Flags().StringVar(name, "name", "", "exact flow name")
	command.Flags().StringVar(projectPath, "project", "", "exact slash-delimited project path")
}

func selectorArgs(operation string, luid, name, projectPath *string, set func(string, string, string)) cobra.PositionalArgs {
	return func(command *cobra.Command, args []string) error {
		if err := noContentArgs(operation)(command, args); err != nil {
			return err
		}
		if *luid != "" {
			if *name != "" || *projectPath != "" {
				return clierr.Usage(operation, errors.New("use either --id or exact --name and --project"))
			}
			set(*luid, "", "")
			return nil
		}
		if *name == "" || *projectPath == "" {
			return clierr.Usage(operation, errors.New("use --id or both --name and --project"))
		}
		set("", *name, *projectPath)
		return nil
	}
}

func selectorArgsWithProjectID(operation string, luid, name, projectPath, projectID *string, set func(string, string, string, string)) cobra.PositionalArgs {
	return func(command *cobra.Command, args []string) error {
		if err := noContentArgs(operation)(command, args); err != nil {
			return err
		}
		if *luid != "" {
			if *name != "" || *projectPath != "" || *projectID != "" {
				return clierr.Usage(operation, errors.New("use either --id or exact --name with --project or --project-id"))
			}
			set(*luid, "", "", "")
			return nil
		}
		if *name == "" || (*projectPath == "" && *projectID == "") {
			return clierr.Usage(operation, errors.New("use --id or exact --name with --project or --project-id"))
		}
		if *projectPath != "" && *projectID != "" {
			return clierr.Usage(operation, errors.New("use exactly one of --project or --project-id"))
		}
		set("", *name, *projectPath, *projectID)
		return nil
	}
}
