package content

import (
	"context"
	"errors"

	flowdelete "github.com/ahillspace/tadx/actions/flow/delete"
	flowget "github.com/ahillspace/tadx/actions/flow/get"
	flowlist "github.com/ahillspace/tadx/actions/flow/list"
	flowmove "github.com/ahillspace/tadx/actions/flow/move"
	flowpublish "github.com/ahillspace/tadx/actions/flow/publish"
	flowpull "github.com/ahillspace/tadx/actions/flow/pull"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

type FlowLister interface {
	ListFlows(context.Context, flowlist.Input) (flowlist.Output, error)
}
type FlowGetter interface {
	GetFlow(context.Context, flowget.Input) (flowget.Output, error)
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
	command.AddCommand(newFlowList(deps), newFlowGet(deps), newFlowPull(deps), newFlowPublish(deps), newFlowMove(deps), newFlowDelete(deps))
	return command
}

func newFlowList(deps Dependencies) *cobra.Command {
	var input flowlist.Input
	command := &cobra.Command{Use: "list", Short: "List one bounded flow page.", Annotations: map[string]string{"tadx.capability": "flow.list"}, Args: noContentArgs("flow.list"), RunE: func(command *cobra.Command, _ []string) error {
		result, err := deps.FlowLister.ListFlows(command.Context(), input)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(result)
	}}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().StringVar(&input.Name, "name", "", "exact flow-name filter")
	command.Flags().StringVar(&input.OwnerName, "owner", "", "exact owner-name filter")
	command.Flags().StringVar(&input.ProjectLUID, "project-id", "", "authoritative project LUID filter")
	command.Flags().StringVar(&input.ProjectName, "project-name", "", "exact project-name filter")
	command.Flags().IntVar(&input.Limit, "limit", 0, "maximum flows to return")
	command.Flags().StringVar(&input.Cursor, "cursor", "", "opaque continuation cursor")
	return command
}

func newFlowGet(deps Dependencies) *cobra.Command {
	var input flowget.Input
	var luid, name, projectPath string
	command := &cobra.Command{Use: "get", Short: "Inspect one exact flow.", Annotations: map[string]string{"tadx.capability": "flow.get"}, Args: selectorArgs("flow.get", &luid, &name, &projectPath, input.SetSelector), RunE: func(command *cobra.Command, _ []string) error {
		result, err := deps.FlowGetter.GetFlow(command.Context(), input)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(result)
	}}
	readTargetFlags(command, &input.Environment, &luid, &name, &projectPath)
	return command
}

func newFlowPull(deps Dependencies) *cobra.Command {
	var input flowpull.Input
	var luid, name, projectPath string
	command := &cobra.Command{Use: "pull", Short: "Pull one unchanged native flow artifact.", Annotations: map[string]string{"tadx.capability": "flow.pull"}, Args: selectorArgs("flow.pull", &luid, &name, &projectPath, input.SetSelector), RunE: func(command *cobra.Command, _ []string) error {
		result, err := deps.FlowPuller.PullFlow(command.Context(), input)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(result)
	}}
	readTargetFlags(command, &input.Environment, &luid, &name, &projectPath)
	command.Flags().StringVar(&input.Workspace, "workspace", "", "logical workspace name; uses deterministic defaults when omitted")
	command.Flags().BoolVar(&input.Overwrite, "overwrite", false, "replace a dirty local flow artifact")
	return command
}

func newFlowPublish(deps Dependencies) *cobra.Command {
	var input flowpublish.Input
	var projectLUID, projectPath string
	var apply bool
	command := &cobra.Command{Use: "publish", Short: "Preview or publish one native flow artifact.", Hidden: !deps.MutationsEnabled, Annotations: map[string]string{"tadx.capability": "flow.publish"}, Args: func(command *cobra.Command, args []string) error {
		if err := noContentArgs("flow.publish")(command, args); err != nil {
			return err
		}
		if err := validateManagedArtifactPath(input.ArtifactPath, "flow"); err != nil {
			return clierr.Usage("flow.publish", err)
		}
		if input.Environment != "" && (projectLUID == "") == (projectPath == "") {
			return clierr.Usage("flow.publish", errors.New("an explicit --environment requires exactly one of --project-id or --project"))
		}
		if input.Environment == "" && projectLUID != "" && projectPath != "" {
			return clierr.Usage("flow.publish", errors.New("use at most one of --project-id or --project"))
		}
		input.SetProjectSelector(projectLUID, projectPath)
		return nil
	}, RunE: func(command *cobra.Command, _ []string) error {
		result, err := deps.FlowPublisher.PublishFlow(command.Context(), input, apply)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(result)
	}}
	command.Flags().StringVar(&input.Workspace, "workspace", "", "logical workspace name; uses deterministic defaults when omitted")
	command.Flags().StringVar(&input.ArtifactPath, "artifact", "", "exact workspace-relative managed flow path")
	command.Flags().StringVar(&input.Environment, "environment", "", "explicit write environment alias; defaults to artifact source")
	command.Flags().StringVar(&input.Name, "name", "", "published flow name; defaults to artifact name")
	command.Flags().StringVar(&projectLUID, "project-id", "", "authoritative destination project LUID")
	command.Flags().StringVar(&projectPath, "project", "", "exact destination project path")
	command.Flags().BoolVar(&input.Overwrite, "overwrite", false, "replace the exact colliding flow")
	command.Flags().BoolVar(&apply, "apply", false, "apply the previewed remote mutation")
	return command
}

func newFlowMove(deps Dependencies) *cobra.Command {
	var input flowmove.Input
	var flowLUID, name, sourceProject, projectLUID, projectPath string
	var apply bool
	command := &cobra.Command{Use: "move", Short: "Preview or move one exact flow.", Hidden: !deps.MutationsEnabled, Annotations: map[string]string{"tadx.capability": "flow.move"}, Args: func(command *cobra.Command, args []string) error {
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
		result, err := deps.FlowMover.MoveFlow(command.Context(), input, apply)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(result)
	}}
	readTargetFlags(command, &input.Environment, &flowLUID, &name, &sourceProject)
	command.Flags().StringVar(&projectLUID, "destination-project-id", "", "authoritative destination project LUID")
	command.Flags().StringVar(&projectPath, "destination-project", "", "exact destination project path")
	command.Flags().BoolVar(&apply, "apply", false, "apply the previewed remote mutation")
	return command
}

func newFlowDelete(deps Dependencies) *cobra.Command {
	var input flowdelete.Input
	var luid, name, projectPath string
	var apply bool
	command := &cobra.Command{Use: "delete", Short: "Preview or delete one exact remote flow.", Hidden: !deps.MutationsEnabled, Annotations: map[string]string{"tadx.capability": "flow.delete"}, Args: func(command *cobra.Command, args []string) error {
		if err := selectorArgs("flow.delete", &luid, &name, &projectPath, input.SetSelector)(command, args); err != nil {
			return err
		}
		if input.Environment == "" {
			return clierr.Usage("flow.delete", errors.New("--environment is required for remote deletion"))
		}
		return nil
	}, RunE: func(command *cobra.Command, _ []string) error {
		result, err := deps.FlowDeleter.DeleteFlow(command.Context(), input, apply)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(result)
	}}
	readTargetFlags(command, &input.Environment, &luid, &name, &projectPath)
	command.Flags().BoolVar(&apply, "apply", false, "apply the previewed remote deletion")
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
