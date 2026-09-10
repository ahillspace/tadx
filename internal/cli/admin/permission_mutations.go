package admin

import (
	"context"
	"fmt"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/ahillspace/tadx/internal/contentbatch"
	"github.com/ahillspace/tadx/internal/errs"
	"strings"

	permissioncreate "github.com/ahillspace/tadx/actions/admin/permission/create"
	permissiondelete "github.com/ahillspace/tadx/actions/admin/permission/delete"
	"github.com/spf13/cobra"
)

type PermissionCreator interface {
	CreateAdminPermission(context.Context, permissioncreate.Input, bool) (permissioncreate.Output, error)
}
type PermissionDeleter interface {
	DeleteAdminPermission(context.Context, permissiondelete.Input, bool) (permissiondelete.Output, error)
}

func newPermissionCreate(deps Dependencies) *cobra.Command {
	var in permissioncreate.Input
	var capabilities []string
	var preview bool
	cmd := mutation("create", "Create one explicit capability and mode for an exact principal.", "admin.permission.create", deps.MutationsEnabled, func(cmd *cobra.Command, args []string) error {
		if err := noArgs("admin.permission.create")(cmd, args); err != nil {
			return err
		}
		if err := contentbatch.Validate(capabilities); err != nil {
			return clierr.Usage("admin.permission.create", err)
		}
		if err := validatePermissionCapabilities("admin.permission.create", in.ResourceKind, in.DefaultFor, capabilities, deps.PermissionCapabilities); err != nil {
			return err
		}
		in.Capability = capabilities[0]
		return permissioncreate.Validate(in)
	}, func(cmd *cobra.Command) error {
		return runPermissionCapabilities(cmd.Context(), "admin.permission.create", capabilities, deps.Renderer, func(ctx context.Context, capability string) (permissioncreate.Output, error) {
			item := in
			item.Capability = capability
			return deps.PermissionCreator.CreateAdminPermission(ctx, item, preview)
		})
	})
	cmd.Flags().StringVar(&in.Environment, "environment", "", "explicit write environment alias")
	cmd.Flags().StringVar(&in.ResourceKind, "kind", "", "exact resource kind: workbook, datasource, flow, or project")
	cmd.Flags().StringVar(&in.ResourceLUID, "id", "", "authoritative resource LUID")
	cmd.Flags().StringVar(&in.DefaultFor, "default-for", "", "project default content kind: workbooks, datasources, or flows")
	cmd.Flags().StringVar(&in.PrincipalType, "principal-type", "", "exact principal type: user or group")
	cmd.Flags().StringVar(&in.PrincipalLUID, "principal-id", "", "authoritative principal LUID")
	cmd.Flags().StringVar(&in.PrincipalUsername, "principal-username", "", "exact Tableau username for a user principal")
	cmd.MarkFlagsMutuallyExclusive("principal-id", "principal-username")
	cmd.Flags().StringArrayVar(&capabilities, "capability", nil, "exact Tableau capability name; repeat for multiple rules")
	cmd.Flags().StringVar(&in.Mode, "mode", "", "exact permission mode: Allow or Deny")
	cmd.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	addPermissionCapabilityHelp(cmd, deps.PermissionCapabilities)
	return cmd
}

func newPermissionDelete(deps Dependencies) *cobra.Command {
	var in permissiondelete.Input
	var capabilities []string
	var preview bool
	cmd := mutation("delete", "Delete one explicit capability and mode for an exact principal.", "admin.permission.delete", deps.MutationsEnabled, func(cmd *cobra.Command, args []string) error {
		if err := noArgs("admin.permission.delete")(cmd, args); err != nil {
			return err
		}
		if err := contentbatch.Validate(capabilities); err != nil {
			return clierr.Usage("admin.permission.delete", err)
		}
		if err := validatePermissionCapabilities("admin.permission.delete", in.ResourceKind, in.DefaultFor, capabilities, deps.PermissionCapabilities); err != nil {
			return err
		}
		in.Capability = capabilities[0]
		return permissiondelete.Validate(in)
	}, func(cmd *cobra.Command) error {
		return runPermissionCapabilities(cmd.Context(), "admin.permission.delete", capabilities, deps.Renderer, func(ctx context.Context, capability string) (permissiondelete.Output, error) {
			item := in
			item.Capability = capability
			return deps.PermissionDeleter.DeleteAdminPermission(ctx, item, preview)
		})
	})
	cmd.Flags().StringVar(&in.Environment, "environment", "", "explicit write environment alias")
	cmd.Flags().StringVar(&in.ResourceKind, "kind", "", "exact resource kind: workbook, datasource, flow, or project")
	cmd.Flags().StringVar(&in.ResourceLUID, "id", "", "authoritative resource LUID")
	cmd.Flags().StringVar(&in.DefaultFor, "default-for", "", "project default content kind: workbooks, datasources, or flows")
	cmd.Flags().StringVar(&in.PrincipalType, "principal-type", "", "exact principal type: user or group")
	cmd.Flags().StringVar(&in.PrincipalLUID, "principal-id", "", "authoritative principal LUID")
	cmd.Flags().StringVar(&in.PrincipalUsername, "principal-username", "", "exact Tableau username for a user principal")
	cmd.MarkFlagsMutuallyExclusive("principal-id", "principal-username")
	cmd.Flags().StringArrayVar(&capabilities, "capability", nil, "exact Tableau capability name; repeat for multiple rules")
	cmd.Flags().StringVar(&in.Mode, "mode", "", "exact permission mode: Allow or Deny")
	cmd.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	addPermissionCapabilityHelp(cmd, deps.PermissionCapabilities)
	return cmd
}

func validatePermissionCapabilities(operation, kind, defaults string, values []string, capabilities func(string) []string) error {
	if capabilities == nil {
		return nil
	}
	if defaults != "" {
		kind = strings.TrimSuffix(defaults, "s")
	}
	allowed := capabilities(kind)
	for _, value := range values {
		found := false
		for _, candidate := range allowed {
			if value == candidate {
				found = true
				break
			}
		}
		if !found {
			return &errs.Error{Kind: errs.KindUsage, Operation: operation, Summary: fmt.Sprintf("unsupported %s permission capability %q", kind, value), Retryable: errs.Bool(false), CorrectiveAction: "Supported " + kind + " capabilities: " + strings.Join(allowed, ", ") + ". Select an exact --capability and review a new --preview."}
		}
	}
	return nil
}

func runPermissionCapabilities[T any](ctx context.Context, operation string, capabilities []string, renderer Renderer, execute func(context.Context, string) (T, error)) error {
	if len(capabilities) == 1 {
		result, err := execute(ctx, capabilities[0])
		if err != nil {
			return clierr.WithOutput(result, err)
		}
		return renderer.Render(result)
	}
	result, err := contentbatch.Run(ctx, operation, capabilities, execute)
	if renderErr := renderer.Render(result); renderErr != nil {
		return renderErr
	}
	if err != nil {
		return clierr.Rendered(err)
	}
	return nil
}

func addPermissionCapabilityHelp(cmd *cobra.Command, capabilities func(string) []string) {
	if capabilities == nil {
		return
	}
	lines := []string{cmd.Short, "", "Supported capability names by resource kind:"}
	for _, kind := range []string{"project", "workbook", "datasource", "flow"} {
		lines = append(lines, "  "+kind+": "+strings.Join(capabilities(kind), ", "))
	}
	lines = append(lines, "", "For project defaults, --default-for selects the content kind's capabilities.")
	cmd.Long = strings.Join(lines, "\n")
	_ = cmd.RegisterFlagCompletionFunc("capability", func(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		kind, _ := cmd.Flags().GetString("kind")
		defaults, _ := cmd.Flags().GetString("default-for")
		if kind == "project" && defaults != "" {
			kind = strings.TrimSuffix(defaults, "s")
		}
		return capabilities(kind), cobra.ShellCompDirectiveNoFileComp
	})
}
