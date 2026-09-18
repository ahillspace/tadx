package capability_test

import (
	"testing"

	"github.com/ahillspace/tadx/internal/capability"
)

func TestSelectorContractsDescribeExecutableAlternatives(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"admin.group.member.add":    "Exact group LUID and exactly one user LUID or exact --username; environment/site (inferred only when one is configured)",
		"admin.group.member.remove": "Exact group LUID and exactly one user LUID or exact --username; environment/site (inferred only when one is configured)",
		"admin.permission.create":   "Resource kind/LUID, principal type, exactly one principal LUID or exact --principal-username, capability, mode; optional project default kind",
		"admin.permission.delete":   "Resource kind/LUID, principal type, exactly one principal LUID or exact --principal-username, capability, mode; optional project default kind",
		"admin.user.delete":         "User LUID or exact --username",
		"admin.user.update":         "User LUID or exact --username",
		"catalog.audit":             "Environment; required --type and --id; repeatable --check; optional --direct-only and --limit",
		"catalog.column.inspect":    "Environment; --metadata-id, or --id with --table-id",
		"catalog.column.list":       "Environment; required --table-id; optional exact --name; --limit or --all",
		"catalog.column.update":     "Environment; required --id and --table-id; optional description and repeated tags; --preview",
		"catalog.database.inspect":  "Environment; exactly one of --id or --metadata-id",
		"catalog.database.list":     "Environment; optional exact --name; --limit or --all",
		"catalog.database.update":   "Environment; required --id; optional description, contact, and repeated tags; --preview",
		"catalog.search":            "Environment; positional query; repeatable --type; optional --table-id; --limit or --all",
		"catalog.table.inspect":     "Environment; exactly one of --id or --metadata-id",
		"catalog.table.list":        "Environment; optional exact --name and --database-id; --limit or --all",
		"catalog.table.update":      "Environment; required --id; optional description, contact, and repeated tags; --preview",
		"flow.inspect":              "Flow LUID or exact name with --project or --project-id",
	}

	for id, expected := range want {
		definition, ok := capability.Lookup(id)
		if !ok {
			t.Errorf("Lookup(%q) did not find a definition", id)
			continue
		}
		if definition.Selectors != expected {
			t.Errorf("Lookup(%q).Selectors = %q, want %q", id, definition.Selectors, expected)
		}
	}
}
