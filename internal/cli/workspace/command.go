// Package workspace contains thin Cobra plumbing for named workspace commands.
package workspace

import (
	"context"
	"errors"

	artifactdelete "github.com/ahillspace/tadx/actions/workspace/artifact/delete"
	workspacecreate "github.com/ahillspace/tadx/actions/workspace/create"
	workspacelist "github.com/ahillspace/tadx/actions/workspace/list"
	workspacemove "github.com/ahillspace/tadx/actions/workspace/move"
	workspacestatus "github.com/ahillspace/tadx/actions/workspace/status"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/ahillspace/tadx/internal/pathspec"
	"github.com/spf13/cobra"
)

type Creator interface {
	Create(context.Context, workspacecreate.Input) (workspacecreate.Output, error)
}
type Lister interface {
	List(context.Context, workspacelist.Input) (workspacelist.Output, error)
}
type Statuser interface {
	Status(context.Context, workspacestatus.Input) (workspacestatus.Output, error)
}
type Mover interface {
	Move(context.Context, workspacemove.Input) (workspacemove.Output, error)
}
type Deleter interface {
	Delete(context.Context, artifactdelete.Input, bool) (artifactdelete.Output, error)
}
type Renderer interface{ Render(any) error }

type Dependencies struct {
	Creator  Creator
	Lister   Lister
	Statuser Statuser
	Mover    Mover
	Deleter  Deleter
	Renderer Renderer
	Uses     map[string]string
	Shorts   map[string]string
}

func New(deps Dependencies) *cobra.Command {
	command := &cobra.Command{Use: "workspace", Short: "Manage named local workspaces"}
	command.AddCommand(newCreate(deps), newList(deps), newStatus(deps), newMove(deps), newArtifact(deps))
	return command
}

func newCreate(deps Dependencies) *cobra.Command {
	var input workspacecreate.Input
	command := &cobra.Command{
		Use: use(deps, "workspace.create", "create <name>"), Short: short(deps, "workspace.create", "Create and register a named workspace."),
		Annotations: map[string]string{"tadx.capability": "workspace.create"},
		Args: func(command *cobra.Command, args []string) error {
			if err := exactArgs("workspace.create", 1)(command, args); err != nil {
				return err
			}
			if input.Path == "" {
				return clierr.Usage("workspace.create", errors.New("--path is required"))
			}
			input.Name = args[0]
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.Creator.Create(command.Context(), input)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Path, "path", "", "machine-local workspace root")
	return command
}

func newList(deps Dependencies) *cobra.Command {
	var input workspacelist.Input
	command := &cobra.Command{
		Use: use(deps, "workspace.list", "list"), Short: short(deps, "workspace.list", "List registered workspaces."),
		Annotations: map[string]string{"tadx.capability": "workspace.list"}, Args: noArgs("workspace.list"),
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.Lister.List(command.Context(), input)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().IntVar(&input.Limit, "limit", 0, "maximum workspaces to return")
	command.Flags().StringVar(&input.Cursor, "cursor", "", "opaque continuation cursor")
	return command
}

func newStatus(deps Dependencies) *cobra.Command {
	var input workspacestatus.Input
	command := &cobra.Command{
		Use: use(deps, "workspace.status", "status"), Short: short(deps, "workspace.status", "Inspect a named workspace."),
		Annotations: map[string]string{"tadx.capability": "workspace.status"}, Args: noArgs("workspace.status"),
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.Statuser.Status(command.Context(), input)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Workspace, "workspace", "", "logical workspace name; uses deterministic defaults when omitted")
	command.Flags().IntVar(&input.Limit, "limit", 0, "maximum artifacts to inspect")
	command.Flags().StringVar(&input.Cursor, "cursor", "", "opaque continuation cursor")
	return command
}

func newMove(deps Dependencies) *cobra.Command {
	var input workspacemove.Input
	command := &cobra.Command{
		Use: use(deps, "workspace.move", "move"), Short: short(deps, "workspace.move", "Move one exact managed artifact."),
		Annotations: map[string]string{"tadx.capability": "workspace.move"},
		Args: func(command *cobra.Command, args []string) error {
			if err := noArgs("workspace.move")(command, args); err != nil {
				return err
			}
			if input.SourceWorkspace == "" || input.DestinationWorkspace == "" {
				return clierr.Usage("workspace.move", errors.New("--source and --destination are required"))
			}
			return validateSelector("workspace.move", input.Path, input.Kind, input.LUID)
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.Mover.Move(command.Context(), input)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.SourceWorkspace, "source", "", "logical source workspace name")
	command.Flags().StringVar(&input.DestinationWorkspace, "destination", "", "logical destination workspace name")
	selectorFlags(command, &input.Path, &input.Kind, &input.LUID)
	return command
}

func newArtifact(deps Dependencies) *cobra.Command {
	artifact := &cobra.Command{Use: "artifact", Short: "Manage exact local artifacts"}
	var input artifactdelete.Input
	var apply bool
	command := &cobra.Command{
		Use: use(deps, "workspace.artifact.delete", "delete"), Short: short(deps, "workspace.artifact.delete", "Preview or delete one exact managed artifact."),
		Annotations: map[string]string{"tadx.capability": "workspace.artifact.delete"},
		Args: func(command *cobra.Command, args []string) error {
			if err := noArgs("workspace.artifact.delete")(command, args); err != nil {
				return err
			}
			if input.Workspace == "" {
				return clierr.Usage("workspace.artifact.delete", errors.New("--workspace is required"))
			}
			return validateSelector("workspace.artifact.delete", input.Path, input.Kind, input.LUID)
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.Deleter.Delete(command.Context(), input, apply)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Workspace, "workspace", "", "logical workspace name")
	selectorFlags(command, &input.Path, &input.Kind, &input.LUID)
	command.Flags().BoolVar(&input.Force, "force", false, "acknowledge deletion of a dirty artifact")
	command.Flags().BoolVar(&apply, "apply", false, "apply the previewed local deletion")
	artifact.AddCommand(command)
	return artifact
}

func selectorFlags(command *cobra.Command, path, kind, luid *string) {
	command.Flags().StringVar(path, "artifact", "", "exact workspace-relative managed artifact path")
	command.Flags().StringVar(kind, "kind", "", "managed artifact kind")
	command.Flags().StringVar(luid, "id", "", "authoritative Tableau LUID")
}

func validateSelector(operation, path, kind, luid string) error {
	if path != "" {
		if pathspec.Escapes(path) {
			return clierr.Usage(operation, errors.New("--artifact must be a workspace-relative managed path"))
		}
		if kind != "" || luid != "" {
			return clierr.Usage(operation, errors.New("use either --artifact or both --kind and --id"))
		}
		return nil
	}
	if kind == "" || luid == "" {
		return clierr.Usage(operation, errors.New("both --kind and --id are required when --artifact is omitted"))
	}
	return nil
}

func noArgs(operation string) cobra.PositionalArgs { return exactArgs(operation, 0) }
func exactArgs(operation string, count int) cobra.PositionalArgs {
	return func(command *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(count)(command, args); err != nil {
			return clierr.Usage(operation, err)
		}
		return nil
	}
}
func use(deps Dependencies, id, fallback string) string {
	if deps.Uses[id] != "" {
		return deps.Uses[id]
	}
	return fallback
}
func short(deps Dependencies, id, fallback string) string {
	if deps.Shorts[id] != "" {
		return deps.Shorts[id]
	}
	return fallback
}
