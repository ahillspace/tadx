package admin

import (
	"context"
	"github.com/ahillspace/tadx/internal/cli/clierr"
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
	var preview bool
	cmd := mutation("create", "Create one explicit capability and mode for an exact principal.", "admin.permission.create", deps.MutationsEnabled, func(cmd *cobra.Command, args []string) error {
		if err := noArgs("admin.permission.create")(cmd, args); err != nil {
			return err
		}
		return permissioncreate.Validate(in)
	}, func(cmd *cobra.Command) error {
		out, err := deps.PermissionCreator.CreateAdminPermission(cmd.Context(), in, preview)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	})
	cmd.Flags().StringVar(&in.Environment, "environment", "", "explicit write environment alias")
	cmd.Flags().StringVar(&in.ResourceKind, "kind", "", "exact resource kind: workbook, datasource, flow, or project")
	cmd.Flags().StringVar(&in.ResourceLUID, "id", "", "authoritative resource LUID")
	cmd.Flags().StringVar(&in.DefaultFor, "default-for", "", "project default content kind: workbooks, datasources, or flows")
	cmd.Flags().StringVar(&in.PrincipalType, "principal-type", "", "exact principal type: user or group")
	cmd.Flags().StringVar(&in.PrincipalLUID, "principal-id", "", "authoritative principal LUID")
	cmd.Flags().StringVar(&in.Capability, "capability", "", "exact Tableau capability name")
	cmd.Flags().StringVar(&in.Mode, "mode", "", "exact permission mode: Allow or Deny")
	cmd.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	addPermissionCapabilityHelp(cmd, deps.PermissionCapabilities)
	return cmd
}

func newPermissionDelete(deps Dependencies) *cobra.Command {
	var in permissiondelete.Input
	var preview bool
	cmd := mutation("delete", "Delete one explicit capability and mode for an exact principal.", "admin.permission.delete", deps.MutationsEnabled, func(cmd *cobra.Command, args []string) error {
		if err := noArgs("admin.permission.delete")(cmd, args); err != nil {
			return err
		}
		return permissiondelete.Validate(in)
	}, func(cmd *cobra.Command) error {
		out, err := deps.PermissionDeleter.DeleteAdminPermission(cmd.Context(), in, preview)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	})
	cmd.Flags().StringVar(&in.Environment, "environment", "", "explicit write environment alias")
	cmd.Flags().StringVar(&in.ResourceKind, "kind", "", "exact resource kind: workbook, datasource, flow, or project")
	cmd.Flags().StringVar(&in.ResourceLUID, "id", "", "authoritative resource LUID")
	cmd.Flags().StringVar(&in.DefaultFor, "default-for", "", "project default content kind: workbooks, datasources, or flows")
	cmd.Flags().StringVar(&in.PrincipalType, "principal-type", "", "exact principal type: user or group")
	cmd.Flags().StringVar(&in.PrincipalLUID, "principal-id", "", "authoritative principal LUID")
	cmd.Flags().StringVar(&in.Capability, "capability", "", "exact Tableau capability name")
	cmd.Flags().StringVar(&in.Mode, "mode", "", "exact permission mode: Allow or Deny")
	cmd.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	addPermissionCapabilityHelp(cmd, deps.PermissionCapabilities)
	return cmd
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
