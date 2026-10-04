package admin

import (
	"strings"

	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

// ApplyHelpFacts attaches the admin command family's presentation contracts.
func ApplyHelpFacts(command *cobra.Command, path string) {
	if path == "admin" {
		helpmeta.Summary(command, "Users, groups, memberships, permissions, and label definitions")
	}
	switch {
	case strings.HasPrefix(path, "admin user "):
		userHelpFacts(command, strings.TrimPrefix(path, "admin user "))
	case strings.HasPrefix(path, "admin group "):
		groupHelpFacts(command, strings.TrimPrefix(path, "admin group "))
	case strings.HasPrefix(path, "admin group-member "):
		groupMemberHelpFacts(command, strings.TrimPrefix(path, "admin group-member "))
	case strings.HasPrefix(path, "admin permission "):
		permissionHelpFacts(command, strings.TrimPrefix(path, "admin permission "))
	case strings.HasPrefix(path, "admin label-value "):
		labelValueHelpFacts(command, strings.TrimPrefix(path, "admin label-value "))
	case strings.HasPrefix(path, "admin label-category "):
		labelCategoryHelpFacts(command, strings.TrimPrefix(path, "admin label-category "))
	}
}
