package admin

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
)

// HelpExamples supplies examples and common notes owned by this command family.
func HelpExamples() []helpmeta.ExampleSet {
	return []helpmeta.ExampleSet{
		{Path: "admin", Note: "Use --preview to inspect supported remote changes. Users, groups, and permission principals use exact selectors.", Common: true, Examples: []string{}},
		{Path: "admin user", Note: "Create requires --name, --site-role, and exactly one of --auth-setting or --idp-configuration-id.\nInspect accepts exactly one of --id, --name, or --username; --username is an exact-login alias for --name, not a display name. Update and delete accept --id or --username.", Common: false, Examples: []string{"tadx admin user create --env dev --name <username> --site-role Viewer --auth-setting ServerDefault --preview"}},
		{Path: "admin group", Note: "Create requires --name. Inspect accepts --id or --name. Update and delete require --id.", Common: false, Examples: []string{"tadx admin group update --env dev --id <group-luid> --set-members --member-id <user-luid> --member-id <second-user-luid> --preview"}},
		{Path: "admin group-member", Note: "Add and remove require --group-id and exactly one of --user-id or --username.", Common: false, Examples: []string{}},
		{Path: "admin permission", Note: "Inspect requires --kind and --id. Create and delete also require --principal-type, a principal selector, --capability, and --mode.\nUse --principal-id, or --principal-username with --principal-type user. Repeat --capability for multiple rules.", Common: false, Examples: []string{"tadx admin permission create --env dev --kind workbook --id <workbook-luid> --principal-type group --principal-id <group-luid> --capability Read --mode Allow --preview"}},
		{Path: "admin label", Note: "Shared label definitions are separate from labels attached to assets under catalog label.", Common: true, Examples: []string{}},
		{Path: "admin label-value", Note: "Inspect, update, and delete select an exact --name. Creating a value through update also requires --category.", Common: false, Examples: []string{}},
		{Path: "admin label-category", Note: "Create, inspect, update, and delete select an exact --name.", Common: false, Examples: []string{}},
	}
}
