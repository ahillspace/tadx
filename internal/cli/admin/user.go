package admin

import (
	"errors"

	userops "github.com/ahillspace/tadx/actions/admin/user"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

func newUserList(deps Dependencies) *cobra.Command {
	var input userops.ListInput
	cmd := &cobra.Command{Use: "list", Short: "List site users with bounded live reads or explicit --all.", Annotations: map[string]string{"tadx.capability": "admin.user.list"}, Args: noArgs("admin.user.list"), RunE: func(cmd *cobra.Command, _ []string) error {
		out, err := deps.UserLister.ListAdminUsers(cmd.Context(), input)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	cmd.Flags().StringVar(&input.Name, "name", "", "exact username filter")
	cmd.Flags().StringVar(&input.SiteRole, "site-role", "", "exact site-role filter")
	cmd.Flags().BoolVar(&input.All, "all", false, "return all matching records, up to 10000; cannot combine with --limit")
	cmd.Flags().IntVar(&input.Limit, "limit", 0, "maximum users to render, up to 10000")
	cmd.Flags().StringVar(&input.Cursor, "cursor", "", "opaque continuation cursor")
	cmd.MarkFlagsMutuallyExclusive("all", "limit")
	cmd.MarkFlagsMutuallyExclusive("all", "cursor")
	cmd.Flags().BoolVar(&input.Cache, "cache", false, "read indexed local cache data without contacting Tableau")
	return cmd
}

func newUserInspect(deps Dependencies) *cobra.Command {
	var in userops.InspectInput
	var id, name, username string
	cmd := &cobra.Command{Use: "inspect", Short: "Inspect one exact site user.", Annotations: map[string]string{"tadx.capability": "admin.user.inspect"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := noArgs("admin.user.inspect")(cmd, args); err != nil {
			return err
		}
		if name != "" && username != "" {
			return clierr.Usage("admin.user.inspect", errors.New("--name and --username select the same exact login; provide only one"))
		}
		login := name
		if username != "" {
			login = username
		}
		if (id == "") == (login == "") {
			return clierr.Usage("admin.user.inspect", errors.New("use exactly one of --id, --name, or --username"))
		}
		in.Selector = userops.Selector{LUID: id, Username: login}
		return nil
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		out, err := deps.UserInspector.InspectAdminUser(cmd.Context(), in)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().StringVar(&in.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	cmd.Flags().StringVar(&id, "id", "", "authoritative user LUID")
	cmd.Flags().StringVar(&name, "name", "", "exact Tableau username")
	cmd.Flags().StringVar(&username, "username", "", "exact Tableau login; equivalent to --name, not a display name")
	cmd.MarkFlagsMutuallyExclusive("id", "name", "username")
	cmd.Flags().BoolVar(&in.Cache, "cache", false, "read indexed local cache data without contacting Tableau")
	return cmd
}

func newUserCreate(deps Dependencies) *cobra.Command {
	var in userops.CreateInput
	var preview bool
	cmd := mutation("create", "Add one exact site user.", "admin.user.create", func(cmd *cobra.Command, args []string) error {
		if err := noArgs("admin.user.create")(cmd, args); err != nil {
			return err
		}
		return requireMutation(cmd, "admin.user.create", in.Environment)
	}, func(cmd *cobra.Command) error {
		out, err := deps.UserCreator.CreateAdminUser(cmd.Context(), in, preview)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	})
	cmd.Long = "Add one exact site user.\n\nProvide exactly one of --auth-setting or --idp-configuration-id."
	cmd.Flags().StringVar(&in.Environment, "environment", "", "explicit write environment alias")
	cmd.Flags().StringVar(&in.Name, "name", "", "exact username or email")
	cmd.Flags().StringVar(&in.SiteRole, "site-role", "", "explicit site role")
	cmd.Flags().StringVar(&in.AuthSetting, "auth-setting", "", "authentication setting: ServerDefault, SAML, OpenID, or TableauIDWithMFA (availability depends on the site)")
	cmd.Flags().StringVar(&in.IdPConfigurationID, "idp-configuration-id", "", "explicit IdP configuration LUID")
	cmd.Flags().StringVar(&in.IdentityPoolName, "identity-pool", "", "explicit identity-pool name")
	cmd.Flags().StringVar(&in.Email, "email", "", "notification email address")
	cmd.Flags().StringVar(&in.Language, "language", "", "explicit language code")
	cmd.Flags().StringVar(&in.Locale, "locale", "", "explicit locale code")
	cmd.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return cmd
}

func newUserUpdate(deps Dependencies) *cobra.Command {
	var in userops.UpdateInput
	var preview bool
	var fullName, email, siteRole, auth, identityPool, idp, language, locale string
	cmd := mutation("update", "Update one exact site user.", "admin.user.update", func(cmd *cobra.Command, args []string) error {
		if err := noArgs("admin.user.update")(cmd, args); err != nil {
			return err
		}
		if err := requireMutation(cmd, "admin.user.update", in.Environment); err != nil {
			return err
		}
		if (in.UserLUID == "") == (in.Username == "") {
			return clierr.Usage("admin.user.update", errors.New("use exactly one of --id or --username"))
		}
		setString(cmd, "full-name", fullName, &in.FullName)
		setString(cmd, "email", email, &in.Email)
		setString(cmd, "site-role", siteRole, &in.SiteRole)
		setString(cmd, "auth-setting", auth, &in.AuthSetting)
		setString(cmd, "identity-pool", identityPool, &in.IdentityPoolName)
		setString(cmd, "idp-configuration-id", idp, &in.IdPConfigurationID)
		setString(cmd, "language", language, &in.Language)
		setString(cmd, "locale", locale, &in.Locale)
		return nil
	}, func(cmd *cobra.Command) error {
		out, err := deps.UserUpdater.UpdateAdminUser(cmd.Context(), in, preview)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	})
	cmd.Flags().StringVar(&in.Environment, "environment", "", "explicit write environment alias")
	cmd.Flags().StringVar(&in.UserLUID, "id", "", "authoritative user LUID")
	cmd.Flags().StringVar(&in.Username, "username", "", "exact Tableau username")
	cmd.MarkFlagsMutuallyExclusive("id", "username")
	cmd.Flags().StringVar(&fullName, "full-name", "", "full name (Tableau Server, server administrators only)")
	cmd.Flags().StringVar(&email, "email", "", "explicit notification email")
	cmd.Flags().StringVar(&siteRole, "site-role", "", "explicit site role")
	cmd.Flags().StringVar(&auth, "auth-setting", "", "authentication setting: ServerDefault, SAML, OpenID, or TableauIDWithMFA (availability depends on the site)")
	cmd.Flags().StringVar(&identityPool, "identity-pool", "", "explicit identity-pool name")
	cmd.Flags().StringVar(&idp, "idp-configuration-id", "", "explicit IdP configuration LUID")
	cmd.Flags().StringVar(&language, "language", "", "explicit language code")
	cmd.Flags().StringVar(&locale, "locale", "", "explicit locale code")
	cmd.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return cmd
}

func newUserDelete(deps Dependencies) *cobra.Command {
	var in userops.DeleteInput
	var preview bool
	cmd := mutation("delete", "Remove one exact site user.", "admin.user.delete", func(cmd *cobra.Command, args []string) error {
		if err := noArgs("admin.user.delete")(cmd, args); err != nil {
			return err
		}
		if err := requireMutation(cmd, "admin.user.delete", in.Environment); err != nil {
			return err
		}
		if (in.UserLUID == "") == (in.Username == "") {
			return clierr.Usage("admin.user.delete", errors.New("use exactly one of --id or --username"))
		}
		return nil
	}, func(cmd *cobra.Command) error {
		out, err := deps.UserDeleter.DeleteAdminUser(cmd.Context(), in, preview)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	})
	cmd.Flags().StringVar(&in.Environment, "environment", "", "explicit write environment alias")
	cmd.Flags().StringVar(&in.UserLUID, "id", "", "authoritative user LUID")
	cmd.Flags().StringVar(&in.Username, "username", "", "exact Tableau username")
	cmd.MarkFlagsMutuallyExclusive("id", "username")
	cmd.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return cmd
}
