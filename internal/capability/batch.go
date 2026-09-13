package capability

import "github.com/ahillspace/tadx/internal/batchspec"

// BatchSelectors declares bounded repetitions of one action, not workflows.
// Empty selectors support files only; identifiers enable repeated scalar flags.
// The first selector is retained for callers using the original metadata API.
func BatchSelectors() map[string]string {
	result := map[string]string{}
	for id, options := range BatchOptions() {
		result[id] = ""
		if len(options.Selectors) != 0 {
			result[id] = options.Selectors[0]
		}
	}
	return result
}

// BatchOptions declares every executable file-batch surface and its alternative
// shorthand dimensions. Properties such as filters, members and maps stay local
// to each action input. Global defaults and interactive credential entry remain
// single operations.
func BatchOptions() map[string]batchspec.Options {
	legacy := map[string]string{
		"content.label.inspect":        "id",
		"content.label.update":         "id",
		"content.label.delete":         "id",
		"admin.label.value.inspect":    "name",
		"admin.label.value.update":     "name",
		"admin.label.value.delete":     "name",
		"admin.label.category.inspect": "name",
		"admin.label.category.create":  "name",
		"admin.label.category.update":  "name",
		"admin.label.category.delete":  "name",
		"catalog.database.inspect":     "id", "catalog.database.update": "id", "catalog.table.inspect": "id", "catalog.table.update": "id", "catalog.column.inspect": "id", "catalog.column.update": "id", "catalog.audit": "id",
		"workbook.inspect": "id", "workbook.pull": "id", "workbook.publish": "id", "workbook.move": "id", "workbook.update": "id", "workbook.delete": "id",
		"datasource.inspect": "id", "datasource.schema": "id", "datasource.pull": "id", "datasource.publish": "id", "datasource.move": "id", "datasource.update": "id", "datasource.delete": "id",
		"flow.inspect": "id", "flow.pull": "id", "flow.publish": "id", "flow.move": "id", "flow.update": "id", "flow.delete": "id",
		"project.create": "", "project.inspect": "id", "project.update": "id", "project.move": "id", "project.delete": "id",
		"admin.user.create": "", "admin.user.inspect": "id", "admin.user.update": "id", "admin.user.delete": "id",
		"admin.group.create": "", "admin.group.inspect": "id", "admin.group.update": "id", "admin.group.delete": "id",
		"admin.group.member.add": "user-id", "admin.group.member.remove": "user-id",
		"admin.permission.inspect": "id", "admin.permission.create": "id", "admin.permission.delete": "id",
		"pulse.definition.create": "", "pulse.definition.inspect": "id", "pulse.definition.pull": "id", "pulse.definition.publish": "id", "pulse.definition.delete": "id",
		"pulse.metric.inspect": "id", "pulse.metric.fork": "id", "pulse.metric.follow": "id", "pulse.metric.unfollow": "id", "pulse.metric.followers": "id", "pulse.metric.delete": "id",
		"lineage.pull": "id",
	}
	result := make(map[string]batchspec.Options, len(legacy))
	for id, selector := range legacy {
		options := batchspec.Options{}
		if selector != "" {
			options.Selectors = []string{selector}
		}
		result[id] = options
	}
	selectors := func(id string, names ...string) { result[id] = batchspec.Options{Selectors: names} }
	for _, kind := range []string{"workbook", "datasource", "flow"} {
		selectors(kind+".inspect", "id", "name")
		selectors(kind+".pull", "id", "name")
		selectors(kind+".publish", "id", "artifact", "artifact-name", "file")
	}
	selectors("project.create", "name")
	selectors("project.inspect", "id", "project")
	selectors("project.update", "id", "project")
	selectors("project.list", "parent-id")
	selectors("content.label.list", "target-id")
	selectors("content.label.update", "id", "target-id")
	for _, kind := range []string{"database", "table", "column"} {
		selectors("catalog."+kind+".inspect", "id", "metadata-id")
	}
	selectors("catalog.table.list", "database-id")
	selectors("catalog.column.list", "table-id")
	selectors("pulse.metric.list", "definition-id")
	for _, kind := range []string{"user", "group"} {
		selectors("admin."+kind+".create", "name")
		selectors("admin."+kind+".inspect", "id", "name")
	}
	selectors("admin.user.update", "id", "username")
	selectors("admin.user.delete", "id", "username")
	for _, operation := range []string{"add", "remove"} {
		selectors("admin.group.member."+operation, "user-id", "username", "group-id")
	}
	for _, operation := range []string{"inspect", "create", "delete"} {
		selectors("admin.permission."+operation, "id", "principal-id", "principal-username")
	}
	for _, operation := range []string{"create", "delete"} {
		id := "admin.permission." + operation
		options := result[id]
		options.NativeSelections = []string{"capability"}
		result[id] = options
	}
	selectors("pulse.definition.create", "name")
	selectors("pulse.definition.publish", "id", "artifact", "artifact-name")
	selectors("pulse.metric.follow", "id", "user-id", "group-id")
	selectors("pulse.metric.unfollow", "id", "subscription-id", "user-id", "group-id")
	selectors("workspace.move", "artifact", "id")
	selectors("workspace.artifact.delete", "artifact", "id")
	selectors("workspace.clean", "workspace")
	for _, id := range []string{"workspace.delete", "workspace.unregister"} {
		result[id] = batchspec.Options{Positional: true}
	}
	return result
}
