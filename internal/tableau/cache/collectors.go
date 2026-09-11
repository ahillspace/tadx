package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ahillspace/tadx/internal/tableau/cache/tabxml"
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
			email := strings.TrimSpace(element.Attr("email"))
			siteRole := strings.TrimSpace(element.Attr("siteRole"))
			lastLogin := strings.TrimSpace(element.Attr("lastLogin"))
			payload, err := listPayload(map[string]any{
				"luid": id, "name": name, "full_name": element.Attr("fullName"), "email": email,
				"site_role": siteRole, "last_login": lastLogin, "auth_setting": element.Attr("authSetting"),
				"domain": strings.TrimSpace(element.ChildAttr("domain", "name")),
			})
			return []any{id, name, email, siteRole, lastLogin, payload}, id, err
		},
	},
	ScopeGroups: {
		scope: ScopeGroups, path: "/groups", container: "groups", item: "group",
		row: func(element tabxml.Element) ([]any, string, error) {
			id, name, err := identity(element, ScopeGroups)
			if err != nil {
				return nil, "", err
			}
			domain := strings.TrimSpace(element.ChildAttr("domain", "name"))
			externalUserEnabled, err := optionalBoolean(element.Attr("externalUserEnabled"), "group external user enabled")
			if err != nil {
				return nil, "", err
			}
			minimumSiteRole := strings.TrimSpace(element.Attr("minimumSiteRole"))
			if minimumSiteRole == "" {
				minimumSiteRole = strings.TrimSpace(element.ChildAttr("import", "siteRole"))
			}
			payload, err := listPayload(map[string]any{
				"luid": id, "name": name, "domain": domain, "minimum_site_role": minimumSiteRole,
				"grant_license_mode":    strings.TrimSpace(element.ChildAttr("import", "grantLicenseMode")),
				"external_user_enabled": externalUserEnabled,
			})
			return []any{id, name, domain, payload}, id, err
		},
	},
	ScopeProjects: {
		scope: ScopeProjects, path: "/projects", container: "projects", item: "project",
		row: func(element tabxml.Element) ([]any, string, error) {
			id, name, err := identity(element, ScopeProjects)
			if err != nil {
				return nil, "", err
			}
			parentID := strings.TrimSpace(element.Attr("parentProjectId"))
			ownerID := strings.TrimSpace(element.ChildAttr("owner", "id"))
			topLevel, err := optionalBoolean(element.Attr("topLevelProject"), "project top-level status")
			if err != nil {
				return nil, "", err
			}
			counts := make([]any, 4)
			for index, field := range []string{"projectCount", "workbookCount", "viewCount", "datasourceCount"} {
				counts[index], err = optionalInteger(element.ChildAttr("contentCounts", field), "project "+field)
				if err != nil {
					return nil, "", err
				}
			}
			payload, err := listPayload(map[string]any{
				"luid": id, "name": name, "parent_luid": parentID, "description": element.Attr("description"),
				"owner_luid": ownerID, "top_level": topLevel, "content_permissions": element.Attr("contentPermissions"),
				"controlling_permissions_project_luid": element.Attr("controllingPermissionsProjectId"),
				"created_at":                           element.Attr("createdAt"), "updated_at": element.Attr("updatedAt"),
				"project_count": counts[0], "workbook_count": counts[1], "view_count": counts[2], "datasource_count": counts[3],
			})
			return []any{id, name, parentID, element.Attr("description"), ownerID, payload}, id, err
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
			projectID := strings.TrimSpace(element.ChildAttr("project", "id"))
			ownerID := strings.TrimSpace(element.ChildAttr("owner", "id"))
			updatedAt := strings.TrimSpace(element.Attr("updatedAt"))
			payload, err := listPayload(map[string]any{
				"luid": id, "name": name, "project_luid": projectID, "project_path": "", "content_url": element.Attr("contentUrl"),
				"updated_at": updatedAt, "description": element.Attr("description"), "owner_luid": ownerID,
				"created_at": element.Attr("createdAt"), "tags": tagLabels(element),
			})
			return []any{id, name, projectID, ownerID, size, updatedAt, payload}, id, err
		},
	},
	ScopeDatasources: {
		scope: ScopeDatasources, path: "/datasources", container: "datasources", item: "datasource",
		row: func(element tabxml.Element) ([]any, string, error) {
			id, name, err := identity(element, ScopeDatasources)
			if err != nil {
				return nil, "", err
			}
			projectID := strings.TrimSpace(element.ChildAttr("project", "id"))
			ownerID := strings.TrimSpace(element.ChildAttr("owner", "id"))
			updatedAt := strings.TrimSpace(element.Attr("updatedAt"))
			size, err := optionalInteger(element.Attr("size"), "datasource size")
			if err != nil {
				return nil, "", err
			}
			flags := make([]*bool, 4)
			for index, field := range []string{"encryptExtracts", "hasExtracts", "isCertified", "useRemoteQueryAgent"} {
				flags[index], err = optionalBoolean(element.Attr(field), "datasource "+field)
				if err != nil {
					return nil, "", err
				}
			}
			payload, err := listPayload(map[string]any{
				"luid": id, "name": name, "project_luid": projectID, "project_name": strings.TrimSpace(element.ChildAttr("project", "name")),
				"type": element.Attr("type"), "content_url": element.Attr("contentUrl"), "description": element.Attr("description"),
				"owner_luid": ownerID, "created_at": element.Attr("createdAt"), "updated_at": updatedAt, "size": size,
				"encrypt_extracts": flags[0], "has_extracts": flags[1], "is_certified": flags[2],
				"certification_note": element.Attr("certificationNote"), "use_remote_query_agent": flags[3],
				"webpage_url": element.Attr("webpageUrl"), "tags": tagLabels(element),
				"ask_data_enablement": strings.TrimSpace(element.ChildAttr("askData", "enablement")),
			})
			return []any{id, name, projectID, ownerID, updatedAt, payload}, id, err
		},
	},
	ScopeFlows: {
		scope: ScopeFlows, path: "/flows", container: "flows", item: "flow",
		row: func(element tabxml.Element) ([]any, string, error) {
			id, name, err := identity(element, ScopeFlows)
			if err != nil {
				return nil, "", err
			}
			projectID := strings.TrimSpace(element.ChildAttr("project", "id"))
			ownerID := strings.TrimSpace(element.ChildAttr("owner", "id"))
			fileType := strings.TrimSpace(element.Attr("fileType"))
			updatedAt := strings.TrimSpace(element.Attr("updatedAt"))
			payload, err := listPayload(map[string]any{
				"luid": id, "name": name, "project_luid": projectID, "project_name": strings.TrimSpace(element.ChildAttr("project", "name")),
				"file_type": fileType, "updated_at": updatedAt, "description": element.Attr("description"),
				"owner_luid": ownerID, "created_at": element.Attr("createdAt"), "tags": tagLabels(element),
			})
			return []any{id, name, projectID, ownerID, fileType, updatedAt, payload}, id, err
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
	skipped    int
}

func parseList(definition collectorDefinition, body []byte) (parsedPage, error) {
	return parseInventoryList(definition, body, false)
}

func parseInventoryList(definition collectorDefinition, body []byte, tolerateMalformed bool) (parsedPage, error) {
	result := parsedPage{}
	page, _, err := tabxml.DecodeList(body, definition.container, definition.item, func(element tabxml.Element) error {
		row, identity, err := definition.row(element)
		if err != nil {
			if tolerateMalformed {
				result.skipped++
				if id := strings.TrimSpace(element.Attr("id")); id != "" {
					result.identities = append(result.identities, id)
				}
				return nil
			}
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
			identities = append(identities, strings.Join([]string{"workbook", itemID, grantee.Type, grantee.ID, capability.Name}, "\x00"))
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

func optionalBoolean(value, field string) (*bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return nil, fmt.Errorf("%s %q is not a Boolean", field, value)
	}
	return &parsed, nil
}

func tagLabels(element tabxml.Element) []string {
	values := element.GrandchildAttrValues("tags", "tag", "label")
	labels := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			labels = append(labels, value)
		}
	}
	sort.Strings(labels)
	return labels
}

func listPayload(value map[string]any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode Tableau list projection: %w", err)
	}
	return string(encoded), nil
}
