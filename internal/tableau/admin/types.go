// Package admin implements released Tableau administration REST operations.
package admin

import "context"

const MaxPageSize = 1000

// String returns a pointer for an explicitly selected string field.
func String(value string) *string { return &value }

// Bool returns a pointer for an explicitly selected Boolean field.
func Bool(value bool) *bool { return &value }

type PageRequest struct{ PageNumber, PageSize int }

type ListUsersRequest struct {
	PageNumber, PageSize int
	Name, SiteRole       string
}

type User struct {
	LUID, Name, FullName, Email, SiteRole, LastLogin, ExternalAuthUserID string
	AuthSetting, IdentityPoolName, IdPConfigurationID, Language, Locale  string
	Domain, RequestID                                                    string
	MutationStatus                                                       string
}

type UserPage struct {
	Number, Size, Total int
	Items               []User
	RequestID           string
}

type CreateUserRequest struct {
	Name, SiteRole, AuthSetting, IdentityPoolName, IdPConfigurationID string
	Email, Language, Locale                                           string
}

type UpdateUserRequest struct {
	FullName, Email, SiteRole, AuthSetting, IdentityPoolName *string
	IdPConfigurationID, Language, Locale                     *string
}

type ListGroupsRequest struct {
	PageNumber, PageSize int
	Name, Domain         string
}

type Group struct {
	LUID, Name, Domain, MinimumSiteRole, GrantLicenseMode string
	ExternalUserEnabled                                   *bool
	RequestID                                             string
	MutationStatus                                        string
}

type GroupPage struct {
	Number, Size, Total int
	Items               []Group
	RequestID           string
}

type CreateGroupRequest struct {
	Name, MinimumSiteRole string
	ExternalUserEnabled   *bool
}

type UpdateGroupRequest struct {
	Name, MinimumSiteRole *string
	ExternalUserEnabled   *bool
}

type MutationResult struct {
	Status, ResourceLUID, RequestID string
}

type PermissionRequest struct {
	ResourceKind, ResourceLUID, DefaultFor string
}

type PermissionRule struct {
	PrincipalType, PrincipalLUID, Capability, Mode string
}

type PermissionSet struct {
	ResourceKind, ResourceLUID, Source, ParentProjectLUID, RequestID string
	Rules                                                            []PermissionRule
}

type ClientContract interface {
	ListUsers(context.Context, ListUsersRequest) (UserPage, error)
	GetUser(context.Context, string) (User, error)
	CreateUser(context.Context, CreateUserRequest) (User, error)
	UpdateUser(context.Context, string, UpdateUserRequest) (User, error)
	DeleteUser(context.Context, string) (MutationResult, error)
	ListGroups(context.Context, ListGroupsRequest) (GroupPage, error)
	ListGroupUsers(context.Context, string, PageRequest) (UserPage, error)
	CreateGroup(context.Context, CreateGroupRequest) (Group, error)
	UpdateGroup(context.Context, string, UpdateGroupRequest) (Group, error)
	DeleteGroup(context.Context, string) (MutationResult, error)
	AddGroupUser(context.Context, string, string) (MutationResult, error)
	RemoveGroupUser(context.Context, string, string) (MutationResult, error)
	GetPermissions(context.Context, PermissionRequest) (PermissionSet, error)
}
