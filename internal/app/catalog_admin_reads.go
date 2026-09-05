package app

import (
	"context"
	"encoding/json"
	"path/filepath"

	groupinspect "github.com/ahillspace/tadx/actions/admin/group/inspect"
	grouplist "github.com/ahillspace/tadx/actions/admin/group/list"
	userinspect "github.com/ahillspace/tadx/actions/admin/user/inspect"
	userlist "github.com/ahillspace/tadx/actions/admin/user/list"
	"github.com/ahillspace/tadx/internal/catalog"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/readsource"
)

func (c *remoteAdminCommands) catalogStore() *catalog.Store {
	return catalog.NewStore(filepath.Dir(c.runtime.configPath), c.runtime.now)
}

func (c *remoteAdminCommands) resolveCatalogTarget(alias string) (string, string, error) {
	_, environment, err := c.runtime.environment(alias, false)
	if err != nil {
		return alias, "", err
	}
	return environment.Alias, environment.SiteContentURL, nil
}

type catalogUserListReader struct {
	store       *catalog.Store
	environment string
	site        string
	source      *readsource.Metadata
}

func (r *catalogUserListReader) ListUsers(ctx context.Context, input userlist.PageRequest) (userlist.Page, error) {
	if input.SiteRole != "" {
		return userlist.Page{}, unsupportedCatalogFilters("admin.user.list", r.environment, r.site)
	}
	result, err := r.store.ReadResources(ctx, catalog.ResourceQuery{Environment: r.environment, Site: r.site, Kind: "user", Name: input.Name, Offset: snapshotOffset(input.PageNumber, input.PageSize, input.SnapshotCursor), Limit: input.PageSize, Cursor: input.SnapshotCursor})
	if err != nil {
		return userlist.Page{}, catalogReadError("admin.user.list", r.environment, r.site, err)
	}
	r.source = catalogReadSource(result)
	items := make([]userlist.User, len(result.Entries))
	for index, entry := range result.Entries {
		if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &items[index]) == nil {
			continue
		}
		items[index] = userlist.User{LUID: entry.LUID, Name: entry.Name}
	}
	return userlist.Page{Number: input.PageNumber, Size: input.PageSize, Total: result.Total, Users: items, SnapshotCursor: result.NextCursor}, nil
}

type catalogUserGetResolver struct {
	store       *catalog.Store
	environment string
	site        string
	source      *readsource.Metadata
}

func (r *catalogUserGetResolver) ResolveUser(ctx context.Context, selector userinspect.Selector) (userinspect.User, error) {
	result, err := r.store.ReadResources(ctx, catalog.ResourceQuery{Environment: r.environment, Site: r.site, Kind: "user", LUID: selector.LUID, Name: selector.NameOrEmail, Limit: 2})
	if err != nil {
		return userinspect.User{}, catalogReadError("admin.user.inspect", r.environment, r.site, err)
	}
	entry := result.Entries[0]
	r.source = catalogRecordSource(result, entry)
	var item userinspect.User
	if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &item) == nil {
		return item, nil
	}
	return userinspect.User{LUID: entry.LUID, Name: entry.Name}, nil
}

type catalogGroupListReader struct {
	store       *catalog.Store
	environment string
	site        string
	source      *readsource.Metadata
}

func (r *catalogGroupListReader) ListGroups(ctx context.Context, input grouplist.PageRequest) (grouplist.Page, error) {
	if input.Domain != "" {
		return grouplist.Page{}, unsupportedCatalogFilters("admin.group.list", r.environment, r.site)
	}
	result, err := r.store.ReadResources(ctx, catalog.ResourceQuery{Environment: r.environment, Site: r.site, Kind: "group", Name: input.Name, Offset: snapshotOffset(input.PageNumber, input.PageSize, input.SnapshotCursor), Limit: input.PageSize, Cursor: input.SnapshotCursor})
	if err != nil {
		return grouplist.Page{}, catalogReadError("admin.group.list", r.environment, r.site, err)
	}
	r.source = catalogReadSource(result)
	items := make([]grouplist.Group, len(result.Entries))
	for index, entry := range result.Entries {
		if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &items[index]) == nil {
			continue
		}
		items[index] = grouplist.Group{LUID: entry.LUID, Name: entry.Name}
	}
	return grouplist.Page{Number: input.PageNumber, Size: input.PageSize, Total: result.Total, Groups: items, SnapshotCursor: result.NextCursor}, nil
}

type catalogGroupGetResolver struct {
	store       *catalog.Store
	environment string
	site        string
	source      *readsource.Metadata
}

func (r *catalogGroupGetResolver) ResolveGroup(ctx context.Context, selector groupinspect.Selector, members bool) (groupinspect.Group, error) {
	result, err := r.store.ReadResources(ctx, catalog.ResourceQuery{Environment: r.environment, Site: r.site, Kind: "group", LUID: selector.LUID, Name: selector.Name, Limit: 2})
	if err != nil {
		return groupinspect.Group{}, catalogReadError("admin.group.inspect", r.environment, r.site, err)
	}
	entry := result.Entries[0]
	if members && entry.Coverage != "detail" {
		return groupinspect.Group{}, &errs.Error{ID: "catalog.detail_not_indexed", Kind: errs.KindUsage, Operation: "admin.group.inspect", Environment: r.environment, Site: r.site, Summary: "Group membership is not indexed for this catalog record.", Retryable: errs.Bool(false), CorrectiveAction: "Run the command without --catalog to query Tableau and update the catalog."}
	}
	var item groupinspect.Group
	r.source = catalogRecordSource(result, entry)
	if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &item) == nil {
		return item, nil
	}
	return groupinspect.Group{LUID: entry.LUID, Name: entry.Name}, nil
}
