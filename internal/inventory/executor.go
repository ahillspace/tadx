package inventory

import (
	"context"
	"errors"

	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
)

// AuthorizedExecutor enforces scope-specific administrative policy on each request.
// It serves both generation refresh and resource-specific inventory consumers.
type AuthorizedExecutor struct {
	Next       tableaucache.Executor
	CheckScope func(string) error
}

func (e AuthorizedExecutor) Do(ctx context.Context, request tableaucache.Request) (tableaucache.Response, error) {
	if err := CheckScopeCapabilities(e.CheckScope, []tableaucache.Scope{request.Scope}); err != nil {
		return tableaucache.Response{}, err
	}
	if e.Next == nil {
		return tableaucache.Response{}, errors.New("authenticated inventory executor is not configured")
	}
	return e.Next.Do(ctx, request)
}

// CheckScopeNames checks prerequisites for a normalized action-owned scope plan.
func CheckScopeNames(check func(string) error, names []string) error {
	scopes := make([]tableaucache.Scope, len(names))
	for i, name := range names {
		scopes[i] = tableaucache.Scope(name)
	}
	return CheckScopeCapabilities(check, scopes)
}

// CheckScopeCapabilities checks the fixed administrative prerequisites for scopes.
func CheckScopeCapabilities(check func(string) error, scopes []tableaucache.Scope) error {
	if check == nil {
		return nil
	}
	for _, scope := range scopes {
		var id string
		switch scope {
		case tableaucache.ScopeUsers:
			id = "admin.user.list"
		case tableaucache.ScopeGroups:
			id = "admin.group.list"
		case tableaucache.ScopePermissions:
			id = "admin.permission.inspect"
		}
		if id == "" {
			continue
		}
		if err := check(id); err != nil {
			return err
		}
	}
	return nil
}
