package cache

import "sort"

var canonicalScopes = []Scope{
	ScopeUsers,
	ScopeGroups,
	ScopeProjects,
	ScopeWorkbooks,
	ScopeDatasources,
	ScopeFlows,
	ScopeViews,
	ScopePermissions,
}

var fixedColumns = map[Scope][]Column{
	ScopeUsers: {
		{Name: "id", Type: ColumnText},
		{Name: "name", Type: ColumnText},
		{Name: "email", Type: ColumnText},
		{Name: "site_role", Type: ColumnText},
		{Name: "last_login", Type: ColumnTimestamp},
		{Name: "list_payload", Type: ColumnText},
	},
	ScopeGroups: {
		{Name: "id", Type: ColumnText},
		{Name: "name", Type: ColumnText},
		{Name: "domain", Type: ColumnText},
		{Name: "list_payload", Type: ColumnText},
	},
	ScopeProjects: {
		{Name: "id", Type: ColumnText},
		{Name: "name", Type: ColumnText},
		{Name: "parent_project_id", Type: ColumnText},
		{Name: "description", Type: ColumnText},
		{Name: "owner_id", Type: ColumnText},
		{Name: "list_payload", Type: ColumnText},
	},
	ScopeWorkbooks: {
		{Name: "id", Type: ColumnText},
		{Name: "name", Type: ColumnText},
		{Name: "project_id", Type: ColumnText},
		{Name: "owner_id", Type: ColumnText},
		{Name: "size", Type: ColumnInteger},
		{Name: "updated_at", Type: ColumnTimestamp},
		{Name: "list_payload", Type: ColumnText},
	},
	ScopeDatasources: {
		{Name: "id", Type: ColumnText},
		{Name: "name", Type: ColumnText},
		{Name: "project_id", Type: ColumnText},
		{Name: "owner_id", Type: ColumnText},
		{Name: "updated_at", Type: ColumnTimestamp},
		{Name: "list_payload", Type: ColumnText},
	},
	ScopeFlows: {
		{Name: "id", Type: ColumnText},
		{Name: "name", Type: ColumnText},
		{Name: "project_id", Type: ColumnText},
		{Name: "owner_id", Type: ColumnText},
		{Name: "file_type", Type: ColumnText},
		{Name: "updated_at", Type: ColumnTimestamp},
		{Name: "list_payload", Type: ColumnText},
	},
	ScopeViews: {
		{Name: "id", Type: ColumnText},
		{Name: "name", Type: ColumnText},
		{Name: "workbook_id", Type: ColumnText},
	},
	ScopePermissions: {
		{Name: "content_type", Type: ColumnText},
		{Name: "content_id", Type: ColumnText},
		{Name: "grantee_type", Type: ColumnText},
		{Name: "grantee_id", Type: ColumnText},
		{Name: "capability", Type: ColumnText},
		{Name: "mode", Type: ColumnText},
	},
}

// Scopes returns all supported scopes in stable order.
func Scopes() []Scope { return append([]Scope(nil), canonicalScopes...) }

// ColumnsForScope returns a defensive copy of one fixed schema.
func ColumnsForScope(scope Scope) ([]Column, bool) {
	columns, ok := fixedColumns[scope]
	if !ok {
		return nil, false
	}
	return append([]Column(nil), columns...), true
}

func sortScopes(scopes []Scope) {
	order := make(map[Scope]int, len(canonicalScopes))
	for index, scope := range canonicalScopes {
		order[scope] = index
	}
	sort.Slice(scopes, func(left, right int) bool { return order[scopes[left]] < order[scopes[right]] })
}
