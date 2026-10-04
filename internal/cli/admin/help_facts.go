package admin

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

// ApplyHelpFacts attaches this command family's presentation-only contracts.
func ApplyHelpFacts(command *cobra.Command, path string) {
	if path == "admin" {
		helpmeta.Summary(command, "Users, groups, memberships, permissions, and label definitions")
	}
	switch path {
	case "admin permission inspect", "admin permission create", "admin permission delete":
		helpmeta.Choices(command, "kind", "workbook", "datasource", "flow", "project")
		helpmeta.Choices(command, "default-for", "workbooks", "datasources", "flows")
		helpmeta.Choices(command, "principal-type", "user", "group")
		helpmeta.Choices(command, "mode", "Allow", "Deny")
		helpmeta.FlagNote(command, "default-for", "requires --kind project")
		// The action's Long help already receives authoritative per-kind
		// capability lists through its composition-root dependency.
	case "admin user create", "admin user update":
		helpmeta.Choices(command, "auth-setting", "ServerDefault", "SAML", "OpenID", "TableauIDWithMFA")
		helpmeta.FlagNote(command, "site-role", "accepted roles depend on the Tableau site and version")
		helpmeta.FlagNote(command, "language", "Tableau language code supported by the site")
		helpmeta.FlagNote(command, "locale", "Tableau locale code supported by the site")
	case "admin group create", "admin group update":
		helpmeta.FlagNote(command, "minimum-site-role", "accepted roles depend on the Tableau site and version")
	}
	switch path {
	case "admin user list":
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"25"})
	case "admin group list":
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"25"})
	case "admin label-value list":
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"20"})
	case "admin label-category list":
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"20"})
	}
	switch path {
	case "admin group create":
		helpmeta.Required(command, "name")
		if command.Annotations == nil {
			command.Annotations = map[string]string{}
		}
		command.Annotations["tadx.help.batch-example"] = `{"items":[{"name":"analysts"},{"name":"publishers"}]}`
	case "admin group inspect":
		helpmeta.Group(command, "exactly-one", "id", "name")
	case "admin user inspect":
		helpmeta.Group(command, "exactly-one", "id", "name", "username")
	case "admin group delete":
		helpmeta.Required(command, "id")
	case "admin group update":
		helpmeta.Required(command, "id")
		helpmeta.Group(command, "one-required", "new-name", "minimum-site-role", "external-user-enabled", "set-members")
		helpmeta.Constraint(command, "--member-id requires --set-members; --set-members without --member-id removes all direct members.")
	case "admin group-member add", "admin group-member remove":
		helpmeta.Required(command, "group-id")
		helpmeta.Group(command, "exactly-one", "user-id", "username")
	case "admin user create":
		helpmeta.Required(command, "name", "site-role")
		helpmeta.Group(command, "exactly-one", "auth-setting", "idp-configuration-id")
	case "admin user delete":
		helpmeta.Group(command, "exactly-one", "id", "username")
	case "admin user update":
		helpmeta.Group(command, "exactly-one", "id", "username")
		helpmeta.Group(command, "exclusive", "auth-setting", "idp-configuration-id")
		helpmeta.Group(command, "one-required", "full-name", "email", "site-role", "auth-setting", "identity-pool", "idp-configuration-id", "language", "locale")
	case "admin permission inspect":
		helpmeta.Required(command, "kind", "id")
	case "admin permission create", "admin permission delete":
		helpmeta.Required(command, "kind", "id", "principal-type", "capability", "mode")
		helpmeta.Group(command, "exactly-one", "principal-id", "principal-username")
		helpmeta.Constraint(command, "--principal-username requires --principal-type user; --default-for requires --kind project.")
	case "admin label-value inspect", "admin label-value delete", "admin label-category inspect", "admin label-category delete":
		helpmeta.Required(command, "name")
	case "admin label-category create":
		helpmeta.Required(command, "name", "description")
	case "admin label-category update":
		helpmeta.Required(command, "name")
		helpmeta.Group(command, "one-required", "new-name", "description")
	case "admin label-value update":
		helpmeta.Required(command, "name")
		helpmeta.Group(command, "one-required", "new-name", "category", "description")
		helpmeta.Constraint(command, "Creating a missing label value requires --category and --description; renaming requires an existing value.")
	}
}
