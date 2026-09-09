package admin

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/tableau"
)

const maxResponseBytes = 16 * 1024 * 1024

type Client struct {
	transport *tableau.Transport
	session   auth.Session
	serverURL string
}

func NewClient(transport *tableau.Transport, session auth.Session, serverURL string) *Client {
	return &Client{transport: transport, session: session, serverURL: serverURL}
}

func (c *Client) ListUsers(ctx context.Context, input ListUsersRequest) (UserPage, error) {
	filter, err := UserListFilter(input)
	if err != nil {
		return UserPage{}, err
	}
	query, err := pageQuery(input.PageNumber, input.PageSize, filter)
	if err != nil {
		return UserPage{}, err
	}
	response, err := c.do(ctx, http.MethodGet, "admin.user.list", []string{"users"}, query, nil)
	if err != nil {
		return UserPage{}, err
	}
	if err := exactStatus("admin.user.list", response, http.StatusOK); err != nil {
		return UserPage{}, err
	}
	var envelope usersEnvelope
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return UserPage{}, protocol("admin.user.list", response, err)
	}
	return normalizeUserPage("admin.user.list", response, envelope, input.PageNumber, input.PageSize)
}

func (c *Client) GetUser(ctx context.Context, luid string) (User, error) {
	if strings.TrimSpace(luid) == "" {
		return User{}, errors.New("user LUID is required")
	}
	response, err := c.do(ctx, http.MethodGet, "admin.user.get", []string{"users", luid}, nil, nil)
	if err != nil {
		return User{}, err
	}
	if err := exactStatus("admin.user.get", response, http.StatusOK); err != nil {
		return User{}, err
	}
	var envelope userEnvelope
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return User{}, protocol("admin.user.get", response, err)
	}
	user := normalizeUser(envelope.User)
	if user.LUID != luid || user.Name == "" {
		return User{}, protocol("admin.user.get", response, fmt.Errorf("user response identity %q does not match %q", user.LUID, luid))
	}
	user.RequestID = response.TableauRequestID
	return user, nil
}

func (c *Client) CreateUser(ctx context.Context, input CreateUserRequest) (User, error) {
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.SiteRole) == "" {
		return User{}, errors.New("user name and site role are required")
	}
	payload := userWriteXML{Name: optional(input.Name), SiteRole: optional(input.SiteRole), AuthSetting: optional(input.AuthSetting), IdentityPoolName: optional(input.IdentityPoolName), IdPConfigurationID: optional(input.IdPConfigurationID), Email: optional(input.Email), Language: optional(input.Language), Locale: optional(input.Locale)}
	response, err := c.write(ctx, http.MethodPost, "admin.user.create", []string{"users"}, payload)
	if err != nil {
		return User{}, err
	}
	unknown := User{MutationStatus: "unknown", RequestID: response.TableauRequestID}
	if err := exactStatus("admin.user.create", response, http.StatusCreated); err != nil {
		return unknown, err
	}
	user, err := decodeUser("admin.user.create", response, "")
	if err != nil {
		return unknown, err
	}
	user.MutationStatus = "succeeded"
	return user, nil
}

func (c *Client) UpdateUser(ctx context.Context, luid string, input UpdateUserRequest) (User, error) {
	if strings.TrimSpace(luid) == "" {
		return User{}, errors.New("user LUID is required")
	}
	payload := userWriteXML{FullName: input.FullName, Email: input.Email, SiteRole: input.SiteRole, AuthSetting: input.AuthSetting, IdentityPoolName: input.IdentityPoolName, IdPConfigurationID: input.IdPConfigurationID, Language: input.Language, Locale: input.Locale}
	if payload.empty() {
		return User{}, errors.New("at least one user update field is required")
	}
	response, err := c.write(ctx, http.MethodPut, "admin.user.update", []string{"users", luid}, payload)
	if err != nil {
		return User{}, err
	}
	unknown := User{LUID: luid, MutationStatus: "unknown", RequestID: response.TableauRequestID}
	if err := exactStatus("admin.user.update", response, http.StatusOK); err != nil {
		return unknown, err
	}
	user, err := decodeUser("admin.user.update", response, luid)
	if err != nil {
		return unknown, err
	}
	user.MutationStatus = "succeeded"
	return user, nil
}

func (c *Client) DeleteUser(ctx context.Context, luid string) (MutationResult, error) {
	return c.delete(ctx, "admin.user.delete", []string{"users", luid}, luid)
}

func (c *Client) ListGroups(ctx context.Context, input ListGroupsRequest) (GroupPage, error) {
	filter, err := GroupListFilter(input)
	if err != nil {
		return GroupPage{}, err
	}
	query, err := pageQuery(input.PageNumber, input.PageSize, filter)
	if err != nil {
		return GroupPage{}, err
	}
	response, err := c.do(ctx, http.MethodGet, "admin.group.list", []string{"groups"}, query, nil)
	if err != nil {
		return GroupPage{}, err
	}
	if err := exactStatus("admin.group.list", response, http.StatusOK); err != nil {
		return GroupPage{}, err
	}
	var envelope groupsEnvelope
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return GroupPage{}, protocol("admin.group.list", response, err)
	}
	page, err := normalizePage(envelope.Pagination, input.PageNumber, input.PageSize, len(envelope.Groups.Items))
	if err != nil {
		return GroupPage{}, protocol("admin.group.list", response, err)
	}
	items := make([]Group, len(envelope.Groups.Items))
	for i, item := range envelope.Groups.Items {
		items[i] = normalizeGroup(item)
		if items[i].LUID == "" || items[i].Name == "" {
			return GroupPage{}, protocol("admin.group.list", response, errors.New("group response omitted authoritative identity"))
		}
	}
	return GroupPage{Number: page.Number, Size: page.Size, Total: page.Total, Items: items, RequestID: response.TableauRequestID}, nil
}

func (c *Client) ListGroupUsers(ctx context.Context, groupLUID string, input PageRequest) (UserPage, error) {
	if strings.TrimSpace(groupLUID) == "" {
		return UserPage{}, errors.New("group LUID is required")
	}
	query, err := pageQuery(input.PageNumber, input.PageSize, "")
	if err != nil {
		return UserPage{}, err
	}
	response, err := c.do(ctx, http.MethodGet, "admin.group.members", []string{"groups", groupLUID, "users"}, query, nil)
	if err != nil {
		return UserPage{}, err
	}
	if err := exactStatus("admin.group.members", response, http.StatusOK); err != nil {
		return UserPage{}, err
	}
	var envelope usersEnvelope
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return UserPage{}, protocol("admin.group.members", response, err)
	}
	return normalizeUserPage("admin.group.members", response, envelope, input.PageNumber, input.PageSize)
}

func (c *Client) CreateGroup(ctx context.Context, input CreateGroupRequest) (Group, error) {
	if strings.TrimSpace(input.Name) == "" {
		return Group{}, errors.New("group name is required")
	}
	payload := groupWriteXML{Name: optional(input.Name), MinimumSiteRole: optional(input.MinimumSiteRole), ExternalUserEnabled: boolAttr(input.ExternalUserEnabled)}
	response, err := c.write(ctx, http.MethodPost, "admin.group.create", []string{"groups"}, payload)
	if err != nil {
		return Group{}, err
	}
	unknown := Group{MutationStatus: "unknown", RequestID: response.TableauRequestID}
	if err := exactStatus("admin.group.create", response, http.StatusCreated); err != nil {
		return unknown, err
	}
	group, err := decodeGroup("admin.group.create", response, "")
	if err != nil {
		return unknown, err
	}
	group.MutationStatus = "succeeded"
	return group, nil
}

func (c *Client) UpdateGroup(ctx context.Context, luid string, input UpdateGroupRequest) (Group, error) {
	if strings.TrimSpace(luid) == "" {
		return Group{}, errors.New("group LUID is required")
	}
	payload := groupWriteXML{Name: input.Name, MinimumSiteRole: input.MinimumSiteRole, ExternalUserEnabled: boolAttr(input.ExternalUserEnabled)}
	if payload.empty() {
		return Group{}, errors.New("at least one group update field is required")
	}
	response, err := c.write(ctx, http.MethodPut, "admin.group.update", []string{"groups", luid}, payload)
	if err != nil {
		return Group{}, err
	}
	unknown := Group{LUID: luid, MutationStatus: "unknown", RequestID: response.TableauRequestID}
	if err := exactStatus("admin.group.update", response, http.StatusOK); err != nil {
		return unknown, err
	}
	group, err := decodeGroup("admin.group.update", response, luid)
	if err != nil {
		return unknown, err
	}
	group.MutationStatus = "succeeded"
	return group, nil
}

func (c *Client) DeleteGroup(ctx context.Context, luid string) (MutationResult, error) {
	return c.delete(ctx, "admin.group.delete", []string{"groups", luid}, luid)
}

func (c *Client) AddGroupUser(ctx context.Context, groupLUID, userLUID string) (MutationResult, error) {
	if strings.TrimSpace(groupLUID) == "" || strings.TrimSpace(userLUID) == "" {
		return MutationResult{}, errors.New("group and user LUIDs are required")
	}
	payload := userReferenceXML{ID: userLUID}
	response, err := c.write(ctx, http.MethodPost, "admin.group.member.add", []string{"groups", groupLUID, "users"}, payload)
	if err != nil {
		return MutationResult{}, err
	}
	unknown := MutationResult{Status: "unknown", ResourceLUID: userLUID, RequestID: response.TableauRequestID}
	if err := exactStatus("admin.group.member.add", response, http.StatusOK); err != nil {
		return unknown, err
	}
	var envelope userEnvelope
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return unknown, mutationProtocol("admin.group.member.add", response, err)
	}
	user := normalizeUser(envelope.User)
	if user.LUID != userLUID || user.Name == "" {
		return unknown, mutationProtocol("admin.group.member.add", response, fmt.Errorf("group member response identity %q does not match %q", user.LUID, userLUID))
	}
	return MutationResult{Status: "added", ResourceLUID: userLUID, RequestID: response.TableauRequestID}, nil
}

func (c *Client) RemoveGroupUser(ctx context.Context, groupLUID, userLUID string) (MutationResult, error) {
	return c.delete(ctx, "admin.group.member.remove", []string{"groups", groupLUID, "users", userLUID}, userLUID)
}

func (c *Client) GetPermissions(ctx context.Context, input PermissionRequest) (PermissionSet, error) {
	parts, source, err := permissionPath(input)
	if err != nil {
		return PermissionSet{}, err
	}
	response, err := c.do(ctx, http.MethodGet, "admin.permission.get", parts, nil, nil)
	if err != nil {
		return PermissionSet{}, err
	}
	if err := exactStatus("admin.permission.get", response, http.StatusOK); err != nil {
		return PermissionSet{}, err
	}
	rules, err := decodePermissionRules("admin.permission.get", response, input, true)
	if err != nil {
		return PermissionSet{}, err
	}
	var envelope permissionEnvelope
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return PermissionSet{}, protocol("admin.permission.get", response, err)
	}
	parent := envelope.Parent
	if envelope.Permissions.Parent.ID != "" {
		parent = envelope.Permissions.Parent
	}
	if parent.ID != "" {
		source = "inherited"
	}
	return PermissionSet{ResourceKind: input.ResourceKind, ResourceLUID: input.ResourceLUID, Source: source, ParentProjectLUID: parent.ID, Rules: rules, RequestID: response.TableauRequestID}, nil
}

func (c *Client) write(ctx context.Context, method, operation string, parts []string, value any) (tableau.Response, error) {
	body, err := xml.Marshal(struct {
		XMLName xml.Name `xml:"tsRequest"`
		Value   any      `xml:",any"`
	}{Value: value})
	if err != nil {
		return tableau.Response{}, err
	}
	// Values already carrying tsRequest must not be nested.
	if strings.Contains(string(body), "<Value>") || strings.Contains(string(body), "<value>") {
		return tableau.Response{}, errors.New("invalid administration XML payload")
	}
	return c.do(ctx, method, operation, parts, nil, body)
}

func (c *Client) do(ctx context.Context, method, operation string, parts []string, query url.Values, body []byte) (tableau.Response, error) {
	if c == nil || c.transport == nil || c.session == nil || strings.TrimSpace(c.serverURL) == "" {
		return tableau.Response{}, errors.New("Tableau administration client is not configured")
	}
	if strings.TrimSpace(c.session.SiteLUID()) == "" {
		return tableau.Response{}, errors.New("Tableau administration session omitted site LUID")
	}
	segments := []string{"api", c.transport.APIVersion(), "sites", c.session.SiteLUID()}
	segments = append(segments, parts...)
	for i := range segments {
		segments[i] = url.PathEscape(segments[i])
	}
	request := tableau.Request{Method: method, ServerURL: c.serverURL, Path: "/" + strings.Join(segments, "/"), Query: query, Body: body, Operation: operation, Accept: "application/xml", MaxResponseBytes: maxResponseBytes}
	if len(body) > 0 {
		request.ContentType = "application/xml"
	}
	return c.transport.Do(ctx, c.session, request)
}

func (c *Client) delete(ctx context.Context, operation string, parts []string, luid string) (MutationResult, error) {
	if strings.TrimSpace(luid) == "" {
		return MutationResult{}, errors.New("resource LUID is required")
	}
	response, err := c.do(ctx, http.MethodDelete, operation, parts, nil, nil)
	if err != nil {
		return MutationResult{}, err
	}
	unknown := MutationResult{Status: "unknown", ResourceLUID: luid, RequestID: response.TableauRequestID}
	if response.StatusCode != http.StatusNoContent || len(response.Body) != 0 {
		return unknown, tableau.NewProtocolError(operation, response, fmt.Errorf("%s returned HTTP %d with %d response bytes, expected empty HTTP 204", operation, response.StatusCode, len(response.Body)), false)
	}
	return MutationResult{Status: "deleted", ResourceLUID: luid, RequestID: response.TableauRequestID}, nil
}

func exactStatus(operation string, response tableau.Response, expected int) error {
	if response.StatusCode == expected {
		return nil
	}
	return tableau.NewProtocolError(operation, response, fmt.Errorf("%s returned HTTP %d, expected %d", operation, response.StatusCode, expected), false)
}

func pageQuery(number, size int, filter string) (url.Values, error) {
	if number <= 0 || size <= 0 || size > MaxPageSize {
		return nil, fmt.Errorf("administration page number must be positive and size must be between 1 and %d", MaxPageSize)
	}
	query := url.Values{"pageNumber": {strconv.Itoa(number)}, "pageSize": {strconv.Itoa(size)}}
	if filter != "" {
		query.Set("filter", filter)
	}
	return query, nil
}

// UserListFilter validates and encodes user selectors for paged and full lists.
func UserListFilter(input ListUsersRequest) (string, error) {
	return listFilter(map[string]string{"name": input.Name, "siteRole": input.SiteRole})
}

// GroupListFilter validates and encodes group selectors for paged and full lists.
func GroupListFilter(input ListGroupsRequest) (string, error) {
	return listFilter(map[string]string{"name": input.Name, "domainName": input.Domain})
}

func listFilter(filters map[string]string) (string, error) {
	values := make([]string, 0, len(filters))
	for _, key := range []string{"name", "siteRole", "domainName"} {
		item := filters[key]
		if item == "" {
			continue
		}
		if strings.ContainsAny(item, ",&") {
			return "", fmt.Errorf("administration %s filter contains an unsupported comma or ampersand", key)
		}
		values = append(values, key+":eq:"+item)
	}
	return strings.Join(values, ","), nil
}

type page struct{ Number, Size, Total int }

func normalizePage(value paginationXML, expectedNumber, expectedSize, count int) (page, error) {
	if value.Number == nil || value.Size == nil || value.Total == nil {
		return page{}, errors.New("response omitted pagination attributes")
	}
	number, size, total := *value.Number, *value.Size, *value.Total
	if number != expectedNumber || size <= 0 || size > expectedSize || total < 0 || count > size || (number-1)*size+count > total {
		return page{}, fmt.Errorf("inconsistent pagination number=%d size=%d total=%d count=%d", number, size, total, count)
	}
	return page{Number: number, Size: size, Total: total}, nil
}

func normalizeUserPage(operation string, response tableau.Response, envelope usersEnvelope, expectedNumber, expectedSize int) (UserPage, error) {
	page, err := normalizePage(envelope.Pagination, expectedNumber, expectedSize, len(envelope.Users.Items))
	if err != nil {
		return UserPage{}, protocol(operation, response, err)
	}
	items := make([]User, len(envelope.Users.Items))
	for i, item := range envelope.Users.Items {
		items[i] = normalizeUser(item)
		if items[i].LUID == "" || items[i].Name == "" {
			return UserPage{}, protocol(operation, response, errors.New("user response omitted authoritative identity"))
		}
	}
	return UserPage{Number: page.Number, Size: page.Size, Total: page.Total, Items: items, RequestID: response.TableauRequestID}, nil
}

func normalizeUser(item userXML) User {
	return User{LUID: strings.TrimSpace(item.ID), Name: strings.TrimSpace(item.Name), FullName: item.FullName, Email: item.Email, SiteRole: item.SiteRole, LastLogin: item.LastLogin, ExternalAuthUserID: item.ExternalAuthUserID, AuthSetting: item.AuthSetting, IdentityPoolName: item.IdentityPoolName, IdPConfigurationID: item.IdPConfigurationID, Language: item.Language, Locale: item.Locale, Domain: item.Domain.Name}
}
func normalizeGroup(item groupXML) Group {
	return Group{LUID: strings.TrimSpace(item.ID), Name: strings.TrimSpace(item.Name), Domain: item.Domain.Name, MinimumSiteRole: first(item.MinimumSiteRole, item.Import.SiteRole), GrantLicenseMode: item.Import.GrantLicenseMode, ExternalUserEnabled: item.ExternalUserEnabled}
}
func first(values ...string) string {
	for _, item := range values {
		if item != "" {
			return item
		}
	}
	return ""
}

// optional returns a non-nil pointer only for a set create field so unset
// create fields stay omitted from the request.
func optional(item string) *string {
	if item == "" {
		return nil
	}
	return &item
}

// boolAttr renders an optional Boolean as a serializable string pointer.
func boolAttr(item *bool) *string {
	if item == nil {
		return nil
	}
	value := strconv.FormatBool(*item)
	return &value
}
func protocol(operation string, response tableau.Response, err error) error {
	return tableau.NewProtocolError(operation, response, err, true)
}

func mutationProtocol(operation string, response tableau.Response, err error) error {
	return tableau.NewProtocolError(operation, response, err, false)
}

func decodeUser(operation string, response tableau.Response, expected string) (User, error) {
	var envelope userEnvelope
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return User{}, mutationProtocol(operation, response, err)
	}
	user := normalizeUser(envelope.User)
	if expected != "" && user.LUID == "" {
		user.LUID = expected
	}
	if user.LUID == "" || user.Name == "" || (expected != "" && user.LUID != expected) {
		return User{}, mutationProtocol(operation, response, errors.New("user mutation response omitted authoritative identity"))
	}
	user.RequestID = response.TableauRequestID
	return user, nil
}
func decodeGroup(operation string, response tableau.Response, expected string) (Group, error) {
	var envelope groupEnvelope
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return Group{}, mutationProtocol(operation, response, err)
	}
	group := normalizeGroup(envelope.Group)
	if expected != "" && group.LUID == "" {
		group.LUID = expected
	}
	if group.LUID == "" || group.Name == "" || (expected != "" && group.LUID != expected) {
		return Group{}, mutationProtocol(operation, response, errors.New("group mutation response omitted authoritative identity"))
	}
	group.RequestID = response.TableauRequestID
	return group, nil
}

func permissionPath(input PermissionRequest) ([]string, string, error) {
	if strings.TrimSpace(input.ResourceLUID) == "" {
		return nil, "", errors.New("permission resource LUID is required")
	}
	if input.DefaultFor != "" {
		if input.ResourceKind != "project" {
			return nil, "", errors.New("default permissions require a project resource")
		}
		allowed := map[string]bool{"workbooks": true, "datasources": true, "flows": true}
		if !allowed[input.DefaultFor] {
			return nil, "", errors.New("default permission kind must be workbooks, datasources, or flows")
		}
		return []string{"projects", input.ResourceLUID, "default-permissions", input.DefaultFor}, "default", nil
	}
	plural := map[string]string{"workbook": "workbooks", "datasource": "datasources", "flow": "flows", "project": "projects"}[input.ResourceKind]
	if plural == "" {
		return nil, "", errors.New("permission resource kind must be workbook, datasource, flow, or project")
	}
	return []string{plural, input.ResourceLUID, "permissions"}, "direct", nil
}

type paginationXML struct {
	Number *int `xml:"pageNumber,attr"`
	Size   *int `xml:"pageSize,attr"`
	Total  *int `xml:"totalAvailable,attr"`
}
type domainXML struct {
	Name string `xml:"name,attr"`
}
type userXML struct {
	ID                 string    `xml:"id,attr"`
	Name               string    `xml:"name,attr"`
	FullName           string    `xml:"fullName,attr"`
	Email              string    `xml:"email,attr"`
	SiteRole           string    `xml:"siteRole,attr"`
	LastLogin          string    `xml:"lastLogin,attr"`
	ExternalAuthUserID string    `xml:"externalAuthUserId,attr"`
	AuthSetting        string    `xml:"authSetting,attr"`
	IdentityPoolName   string    `xml:"identityPoolName,attr"`
	IdPConfigurationID string    `xml:"idpConfigurationId,attr"`
	Language           string    `xml:"language,attr"`
	Locale             string    `xml:"locale,attr"`
	Domain             domainXML `xml:"domain"`
}
type usersEnvelope struct {
	Pagination paginationXML `xml:"pagination"`
	Users      struct {
		Items []userXML `xml:"user"`
	} `xml:"users"`
}
type userEnvelope struct {
	User userXML `xml:"user"`
}
type importXML struct {
	GrantLicenseMode string `xml:"grantLicenseMode,attr"`
	SiteRole         string `xml:"siteRole,attr"`
}
type groupXML struct {
	ID                  string    `xml:"id,attr"`
	Name                string    `xml:"name,attr"`
	MinimumSiteRole     string    `xml:"minimumSiteRole,attr"`
	ExternalUserEnabled *bool     `xml:"externalUserEnabled,attr"`
	Domain              domainXML `xml:"domain"`
	Import              importXML `xml:"import"`
}
type groupsEnvelope struct {
	Pagination paginationXML `xml:"pagination"`
	Groups     struct {
		Items []groupXML `xml:"group"`
	} `xml:"groups"`
}
type groupEnvelope struct {
	Group groupXML `xml:"group"`
}
type userWriteXML struct {
	XMLName            xml.Name `xml:"user"`
	Name               *string  `xml:"name,attr,omitempty"`
	FullName           *string  `xml:"fullName,attr,omitempty"`
	Email              *string  `xml:"email,attr,omitempty"`
	SiteRole           *string  `xml:"siteRole,attr,omitempty"`
	AuthSetting        *string  `xml:"authSetting,attr,omitempty"`
	IdentityPoolName   *string  `xml:"identityPoolName,attr,omitempty"`
	IdPConfigurationID *string  `xml:"idpConfigurationId,attr,omitempty"`
	Language           *string  `xml:"language,attr,omitempty"`
	Locale             *string  `xml:"locale,attr,omitempty"`
}

// empty reports whether an update carries no explicitly selected field. A
// non-nil pointer to "" is an explicit field clear and is therefore not empty.
func (u userWriteXML) empty() bool {
	return u.FullName == nil && u.Email == nil && u.SiteRole == nil && u.AuthSetting == nil && u.IdentityPoolName == nil && u.IdPConfigurationID == nil && u.Language == nil && u.Locale == nil
}

type groupWriteXML struct {
	XMLName             xml.Name `xml:"group"`
	Name                *string  `xml:"name,attr,omitempty"`
	MinimumSiteRole     *string  `xml:"minimumSiteRole,attr,omitempty"`
	ExternalUserEnabled *string  `xml:"externalUserEnabled,attr,omitempty"`
}

func (g groupWriteXML) empty() bool {
	return g.Name == nil && g.MinimumSiteRole == nil && g.ExternalUserEnabled == nil
}

type idXML struct {
	ID string `xml:"id,attr"`
}
type userReferenceXML struct {
	XMLName xml.Name `xml:"user"`
	ID      string   `xml:"id,attr"`
}
type capabilityXML struct {
	Name string `xml:"name,attr"`
	Mode string `xml:"mode,attr"`
}
type granteeXML struct {
	User         idXML `xml:"user"`
	Group        idXML `xml:"group"`
	Capabilities struct {
		Items []capabilityXML `xml:"capability"`
	} `xml:"capabilities"`
}
type permissionParentXML struct {
	Type string `xml:"type,attr"`
	ID   string `xml:"id,attr"`
}
type permissionEnvelope struct {
	Parent      permissionParentXML `xml:"parent"`
	Permissions struct {
		Parent   permissionParentXML `xml:"parent"`
		Grantees []granteeXML        `xml:"granteeCapabilities"`
	} `xml:"permissions"`
}
