package app

import (
	"context"
	"encoding/json"

	groupops "github.com/ahillspace/tadx/actions/admin/group"
	userops "github.com/ahillspace/tadx/actions/admin/user"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/readsource"
)

func (c *remoteAdminCommands) cacheStore(alias string) *cache.Store {
	_, environment, _ := c.runtime.environment(alias, false)
	return c.runtime.cacheStore(environment)
}

func (c *remoteAdminCommands) resolveCacheTarget(alias string) (string, string, error) {
	_, environment, err := c.runtime.environment(alias, false)
	if err != nil {
		return alias, "", err
	}
	return environment.Alias, environment.SiteContentURL, nil
}

type cacheUserListReader struct {
	checkCapability func(string) error
	store           *cache.Store
	environment     string
	site            string
	source          *readsource.Metadata
}

func (r *cacheUserListReader) ListUsers(ctx context.Context, input userops.ListPageRequest) (userops.ListPage, error) {
	if r.checkCapability != nil {
		if err := r.checkCapability("admin.user.list"); err != nil {
			return userops.ListPage{}, err
		}
	}
	if input.SiteRole != "" {
		return userops.ListPage{}, unsupportedCacheFilters("admin.user.list", r.environment, r.site)
	}
	result, err := r.store.ReadResources(ctx, cache.ResourceQuery{Environment: r.environment, Site: r.site, Kind: "user", Name: input.Name, Offset: snapshotOffset(input.PageNumber, input.PageSize, input.SnapshotCursor), Limit: input.PageSize, Cursor: input.SnapshotCursor})
	if err != nil {
		return userops.ListPage{}, cacheReadError("admin.user.list", r.environment, r.site, err)
	}
	r.source = cacheReadSource(result)
	items := make([]userops.Record, len(result.Entries))
	for index, entry := range result.Entries {
		if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &items[index]) == nil {
			continue
		}
		items[index] = userops.Record{LUID: entry.LUID, Name: entry.Name}
	}
	return userops.ListPage{Number: input.PageNumber, Size: input.PageSize, Total: result.Total, Users: items, SnapshotCursor: result.NextCursor}, nil
}

type cacheUserGetResolver struct {
	checkCapability func(string) error
	store           *cache.Store
	environment     string
	site            string
	source          *readsource.Metadata
}

func (r *cacheUserGetResolver) ResolveUser(ctx context.Context, selector userops.Selector) (userops.Record, error) {
	if r.checkCapability != nil {
		if err := r.checkCapability("admin.user.inspect"); err != nil {
			return userops.Record{}, err
		}
	}
	result, err := r.store.ReadResources(ctx, cache.ResourceQuery{Environment: r.environment, Site: r.site, Kind: "user", LUID: selector.LUID, Name: selector.Username, Limit: 2, ExactlyOne: true})
	if err != nil {
		return userops.Record{}, cacheReadError("admin.user.inspect", r.environment, r.site, err)
	}
	entry := result.Entries[0]
	r.source = cacheRecordSource(result, entry)
	var item userops.Record
	if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &item) == nil {
		return item, nil
	}
	return userops.Record{LUID: entry.LUID, Name: entry.Name}, nil
}

type cacheGroupListReader struct {
	checkCapability func(string) error
	store           *cache.Store
	environment     string
	site            string
	source          *readsource.Metadata
}

func (r *cacheGroupListReader) ListGroups(ctx context.Context, input groupops.ListPageRequest) (groupops.ListPage, error) {
	if r.checkCapability != nil {
		if err := r.checkCapability("admin.group.list"); err != nil {
			return groupops.ListPage{}, err
		}
	}
	if input.Domain != "" {
		return groupops.ListPage{}, unsupportedCacheFilters("admin.group.list", r.environment, r.site)
	}
	result, err := r.store.ReadResources(ctx, cache.ResourceQuery{Environment: r.environment, Site: r.site, Kind: "group", Name: input.Name, Offset: snapshotOffset(input.PageNumber, input.PageSize, input.SnapshotCursor), Limit: input.PageSize, Cursor: input.SnapshotCursor})
	if err != nil {
		return groupops.ListPage{}, cacheReadError("admin.group.list", r.environment, r.site, err)
	}
	r.source = cacheReadSource(result)
	items := make([]groupops.Record, len(result.Entries))
	for index, entry := range result.Entries {
		if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &items[index]) == nil {
			continue
		}
		items[index] = groupops.Record{LUID: entry.LUID, Name: entry.Name}
	}
	return groupops.ListPage{Number: input.PageNumber, Size: input.PageSize, Total: result.Total, Groups: items, SnapshotCursor: result.NextCursor}, nil
}

type cacheGroupGetResolver struct {
	checkCapability func(string) error
	store           *cache.Store
	environment     string
	site            string
	source          *readsource.Metadata
}

func (r *cacheGroupGetResolver) ResolveGroup(ctx context.Context, selector groupops.Selector, members bool) (groupops.Record, error) {
	if r.checkCapability != nil {
		if err := r.checkCapability("admin.group.inspect"); err != nil {
			return groupops.Record{}, err
		}
	}
	result, err := r.store.ReadResources(ctx, cache.ResourceQuery{Environment: r.environment, Site: r.site, Kind: "group", LUID: selector.LUID, Name: selector.Name, Limit: 2, ExactlyOne: true})
	if err != nil {
		return groupops.Record{}, cacheReadError("admin.group.inspect", r.environment, r.site, err)
	}
	entry := result.Entries[0]
	var item struct {
		groupops.Record
		MembersFetched bool `json:"members_fetched"`
	}
	decoded := len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &item) == nil
	if members && (entry.Coverage != "detail" || !decoded || !item.MembersFetched) {
		return groupops.Record{}, &errs.Error{ID: "cache.detail_not_indexed", Kind: errs.KindUsage, Operation: "admin.group.inspect", Environment: r.environment, Site: r.site, Summary: "Group membership is not indexed for this cache record.", Retryable: errs.Bool(false), CorrectiveAction: "Run the command without --cache to query Tableau and update the cache."}
	}
	r.source = cacheRecordSource(result, entry)
	if decoded {
		return item.Record, nil
	}
	return groupops.Record{LUID: entry.LUID, Name: entry.Name}, nil
}
