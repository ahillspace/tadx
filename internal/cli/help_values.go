package cli

import (
	"strings"

	admincli "github.com/ahillspace/tadx/internal/cli/admin"
	agentcli "github.com/ahillspace/tadx/internal/cli/agent"
	authcli "github.com/ahillspace/tadx/internal/cli/auth"
	cachecli "github.com/ahillspace/tadx/internal/cli/cache"
	capabilitycli "github.com/ahillspace/tadx/internal/cli/capability"
	catalogcli "github.com/ahillspace/tadx/internal/cli/catalog"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
	doctorcli "github.com/ahillspace/tadx/internal/cli/doctor"
	envcli "github.com/ahillspace/tadx/internal/cli/env"
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	lastcli "github.com/ahillspace/tadx/internal/cli/last"
	mutationcli "github.com/ahillspace/tadx/internal/cli/mutation"
	pulsecli "github.com/ahillspace/tadx/internal/cli/pulse"
	updatecli "github.com/ahillspace/tadx/internal/cli/update"
	versioncli "github.com/ahillspace/tadx/internal/cli/version"
	workspacecli "github.com/ahillspace/tadx/internal/cli/workspace"
	"github.com/spf13/cobra"
)

// applyHelpValues applies generic syntax and delegates facts to command owners.
func applyHelpValues(root *cobra.Command) {
	var visit func(*cobra.Command, string)
	visit = func(command *cobra.Command, path string) {
		applyHelpSemantics(command, path)
		category, _, _ := strings.Cut(path, " ")
		switch category {
		case "search":
			applySearchHelpFacts(command, path)
		case "content":
			contentcli.ApplyHelpFacts(command, path)
		case "catalog":
			catalogcli.ApplyHelpFacts(command, path)
		case "cache":
			cachecli.ApplyHelpFacts(command, path)
		case "admin":
			admincli.ApplyHelpFacts(command, path)
		case "workspace":
			workspacecli.ApplyHelpFacts(command, path)
		case "pulse":
			pulsecli.ApplyHelpFacts(command, path)
		case "update":
			updatecli.ApplyHelpFacts(command, path)
		case "env":
			envcli.ApplyHelpFacts(command, path)
		case "auth":
			authcli.ApplyHelpFacts(command, path)
		case "agent":
			agentcli.ApplyHelpFacts(command, path)
		case "capability":
			capabilitycli.ApplyHelpFacts(command, path)
		case "last":
			lastcli.ApplyHelpFacts(command, path)
		case "mutation":
			mutationcli.ApplyHelpFacts(command, path)
		case "doctor":
			doctorcli.ApplyHelpFacts(command, path)
		case "version":
			versioncli.ApplyHelpFacts(command, path)
		case "completion":
			applyCompletionHelpFacts(command, path)
		}
		for _, child := range command.Commands() {
			visit(child, strings.TrimSpace(path+" "+child.Name()))
		}
	}
	visit(root, "")
}

// Semantic names explain existing values without changing accepted syntax.
func applyHelpSemantics(command *cobra.Command, path string) {
	resource := "resource"
	parts := strings.Fields(path)
	if len(parts) > 1 {
		resource = parts[len(parts)-2]
	}
	for _, flag := range collectEffectiveFlags(command) {
		if len(flag.Annotations["tadx.help.value"]) > 0 || len(flag.Annotations["tadx.help.choices"]) > 0 {
			continue
		}
		kind := flag.Value.Type()
		if kind != "string" && kind != "stringArray" && kind != "stringSlice" {
			continue
		}
		value := flag.Name
		switch flag.Name {
		case "config", "file", "batch-file", "path", "source", "destination":
			value = "path"
		case "environment":
			value = "environment-name"
		case "workspace":
			value = "workspace-name"
		case "artifact":
			value = "workspace-relative-path"
		case "id":
			value = resource + "-luid"
			if resource == "artifact" || resource == "permission" || resource == "lineage" {
				value = "resource-luid"
			}
		case "name", "new-name":
			value = resource + "-name"
		case "project", "parent", "destination-project":
			value = "project-path"
		case "query":
			value = "text"
		case "description", "message":
			value = "text"
		case "cursor":
			value = "cursor"
		case "pat-name-env", "pat-secret-env":
			value = "environment-variable"
		case "site":
			value = "site-content-url"
		case "url":
			value = "https://server"
		case "metadata-id":
			value = "metadata-api-id"
		case "field-id":
			value = "field-id"
		case "measure-field", "date-field", "dimension":
			value = "field-id-or-name"
		default:
			if strings.HasSuffix(flag.Name, "-id") {
				value = strings.TrimSuffix(flag.Name, "-id") + "-luid"
			}
		}
		helpmeta.FlagAnnotation(command, flag.Name, "tadx.help.value", []string{value})
	}
	if command.Name() == "update" {
		for _, name := range []string{"new-name", "full-name", "email", "description", "owner-id", "site-role", "auth-setting", "idp-configuration-id", "identity-pool", "language", "locale", "content-permissions", "minimum-site-role", "external-user-enabled", "contact-id", "value", "message", "active", "elevated"} {
			helpmeta.FlagAnnotation(command, name, "tadx.help.omission", []string{"unchanged"})
		}
	}
}
