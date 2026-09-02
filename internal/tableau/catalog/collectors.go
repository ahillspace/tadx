package catalog

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ahillspace/tadx/internal/tableau/catalog/tabxml"
)

type collectorDefinition struct {
	scope     Scope
	path      string
	container string
	item      string
	row       func(tabxml.Element) ([]any, string, error)
}

var collectors = map[Scope]collectorDefinition{
	ScopeUsers: {
		scope: ScopeUsers, path: "/users", container: "users", item: "user",
		row: func(element tabxml.Element) ([]any, string, error) {
			id, name, err := identity(element, ScopeUsers)
			if err != nil {
				return nil, "", err
			}
			return []any{id, name, element.ChildText("email"), strings.TrimSpace(element.Attr("siteRole")), strings.TrimSpace(element.Attr("lastLogin"))}, id, nil
		},
	},
	ScopeGroups: {
		scope: ScopeGroups, path: "/groups", container: "groups", item: "group",
		row: func(element tabxml.Element) ([]any, string, error) {
			id, name, err := identity(element, ScopeGroups)
			if err != nil {
				return nil, "", err
			}
			return []any{id, name, strings.TrimSpace(element.Attr("domain"))}, id, nil
		},
	},
	ScopeProjects: {
		scope: ScopeProjects, path: "/projects", container: "projects", item: "project",
		row: func(element tabxml.Element) ([]any, string, error) {
			id, name, err := identity(element, ScopeProjects)
			if err != nil {
				return nil, "", err
			}
			return []any{id, name, strings.TrimSpace(element.Attr("parentProjectId")), element.Attr("description"), strings.TrimSpace(element.ChildAttr("owner", "id"))}, id, nil
		},
	},
	ScopeWorkbooks: {
		scope: ScopeWorkbooks, path: "/workbooks", container: "workbooks", item: "workbook",
		row: func(element tabxml.Element) ([]any, string, error) {
			id, name, err := identity(element, ScopeWorkbooks)
			if err != nil {
				return nil, "", err
			}
			size, err := optionalInteger(element.Attr("size"), "workbook size")
			if err != nil {
				return nil, "", err
			}
			return []any{id, name, strings.TrimSpace(element.ChildAttr("project", "id")), strings.TrimSpace(element.ChildAttr("owner", "id")), size, strings.TrimSpace(element.Attr("updatedAt"))}, id, nil
		},
	},
	ScopeDatasources: {
		scope: ScopeDatasources, path: "/datasources", container: "datasources", item: "datasource",
		row: func(element tabxml.Element) ([]any, string, error) {
			id, name, err := identity(element, ScopeDatasources)
			if err != nil {
				return nil, "", err
			}
			return []any{id, name, strings.TrimSpace(element.ChildAttr("project", "id")), strings.TrimSpace(element.ChildAttr("owner", "id")), strings.TrimSpace(element.Attr("updatedAt"))}, id, nil
		},
	},
	ScopeFlows: {
		scope: ScopeFlows, path: "/flows", container: "flows", item: "flow",
		row: func(element tabxml.Element) ([]any, string, error) {
			id, name, err := identity(element, ScopeFlows)
			if err != nil {
				return nil, "", err
			}
			return []any{id, name, strings.TrimSpace(element.ChildAttr("project", "id")), strings.TrimSpace(element.ChildAttr("owner", "id")), strings.TrimSpace(element.Attr("updatedAt"))}, id, nil
		},
	},
	ScopeViews: {
		scope: ScopeViews, path: "/views", container: "views", item: "view",
		row: func(element tabxml.Element) ([]any, string, error) {
			id, name, err := identity(element, ScopeViews)
			if err != nil {
				return nil, "", err
			}
			return []any{id, name, strings.TrimSpace(element.ChildAttr("workbook", "id"))}, id, nil
		},
	},
}

type parsedPage struct {
	page       tabxml.Pagination
	rows       [][]any
	identities []string
}

func parseList(definition collectorDefinition, body []byte) (parsedPage, error) {
	result := parsedPage{}
	page, _, err := tabxml.DecodeList(body, definition.container, definition.item, func(element tabxml.Element) error {
		row, identity, err := definition.row(element)
		if err != nil {
			return err
		}
		result.rows = append(result.rows, row)
		result.identities = append(result.identities, identity)
		return nil
	})
	if err != nil {
		return parsedPage{}, err
	}
	result.page = page
	return result, nil
}

func parsePermissions(itemID string, body []byte) ([][]any, []string, error) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return nil, nil, errors.New("permission request omitted workbook identity")
	}
	document, err := tabxml.DecodePermissionDocument(body)
	if err != nil {
		return nil, nil, err
	}
	if document.ContentType != "workbook" || document.ContentID != itemID {
		return nil, nil, fmt.Errorf("permission response returned %s identity %q; expected workbook %q", document.ContentType, document.ContentID, itemID)
	}
	var rows [][]any
	var identities []string
	for _, grantee := range document.Grantees {
		for _, capability := range grantee.Capabilities {
			row := []any{"workbook", itemID, grantee.Type, grantee.ID, capability.Name, capability.Mode}
			rows = append(rows, row)
			identities = append(identities, strings.Join([]string{"workbook", itemID, grantee.Type, grantee.ID, capability.Name, capability.Mode}, "\x00"))
		}
	}
	return rows, identities, nil
}

func identity(element tabxml.Element, scope Scope) (string, string, error) {
	id := strings.TrimSpace(element.Attr("id"))
	name := strings.TrimSpace(element.Attr("name"))
	if id == "" || name == "" {
		return "", "", fmt.Errorf("%s response returned an incomplete authoritative identity", scope)
	}
	return id, name, nil
}

func optionalInteger(value, field string) (any, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return nil, fmt.Errorf("%s %q is not a nonnegative integer", field, value)
	}
	return parsed, nil
}
