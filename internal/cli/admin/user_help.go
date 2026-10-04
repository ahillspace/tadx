package admin

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

func userHelpFacts(command *cobra.Command, action string) {
	if action == "create" || action == "update" {
		helpmeta.Choices(command, "auth-setting", "ServerDefault", "SAML", "OpenID", "TableauIDWithMFA")
		helpmeta.FlagNote(command, "site-role", "accepted roles depend on the Tableau site and version")
		helpmeta.FlagNote(command, "language", "Tableau language code supported by the site")
		helpmeta.FlagNote(command, "locale", "Tableau locale code supported by the site")
	}
	if action == "list" {
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"25"})
	}
	switch action {
	case "inspect":
		helpmeta.Group(command, "exactly-one", "id", "name", "username")
	case "create":
		helpmeta.Required(command, "name", "site-role")
		helpmeta.Group(command, "exactly-one", "auth-setting", "idp-configuration-id")
	case "delete":
		helpmeta.Group(command, "exactly-one", "id", "username")
	case "update":
		helpmeta.Group(command, "exactly-one", "id", "username")
		helpmeta.Group(command, "exclusive", "auth-setting", "idp-configuration-id")
		helpmeta.Group(command, "one-required", "full-name", "email", "site-role", "auth-setting", "identity-pool", "idp-configuration-id", "language", "locale")
	}
}

func userHelpExamples() []helpmeta.ExampleSet {
	return []helpmeta.ExampleSet{
		{Path: "admin user", Note: "Create requires --name, --site-role, and exactly one of --auth-setting or --idp-configuration-id.\nInspect accepts exactly one of --id, --name, or --username; --username is an exact-login alias for --name, not a display name. Update and delete accept --id or --username.", Common: false, Examples: []string{"tadx admin user create --env dev --name <username> --site-role Viewer --auth-setting ServerDefault --preview"}},
	}
}
