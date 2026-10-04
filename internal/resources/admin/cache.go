package admin

import (
	"context"
	"encoding/json"

	group "github.com/ahillspace/tadx/actions/admin/group"
	user "github.com/ahillspace/tadx/actions/admin/user"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/readsource"
)

// CacheSupport binds the shared cache policy to typed administration readers.
type CacheSupport struct {
	CheckCapability   func(string) error
	ReadError         func(string, string, string, error) error
	UnsupportedFilter func(string, string, string) error
	ReadSource        func(cache.ResourceResult) *readsource.Metadata
	RecordSource      func(cache.ResourceResult, cache.ResourceEntry) *readsource.Metadata
}

func (s CacheSupport) check(operation string) error {
	if s.CheckCapability != nil {
		return s.CheckCapability(operation)
	}
	return nil
}

func cachePageOffset(number, size int, cursor string) int {
	if cursor != "" {
		return 0
	}
	return (number - 1) * size
}

type CachedUserListPort struct {
	store             *cache.Store
	environment, site string
	support           CacheSupport
	source            *readsource.Metadata
}

func NewCachedUserListPort(store *cache.Store, environment, site string, support CacheSupport) *CachedUserListPort {
	return &CachedUserListPort{store: store, environment: environment, site: site, support: support}
}

func (p *CachedUserListPort) Source() *readsource.Metadata { return p.source }

func (p *CachedUserListPort) ListUsers(ctx context.Context, input user.ListPageRequest) (user.ListPage, error) {
	if err := p.support.check("admin.user.list"); err != nil {
		return user.ListPage{}, err
	}
	if input.SiteRole != "" {
		return user.ListPage{}, p.support.UnsupportedFilter("admin.user.list", p.environment, p.site)
	}
	result, err := p.store.ReadResources(ctx, cache.ResourceQuery{Environment: p.environment, Site: p.site, Kind: "user", Name: input.Name, Offset: cachePageOffset(input.PageNumber, input.PageSize, input.SnapshotCursor), Limit: input.PageSize, Cursor: input.SnapshotCursor})
	if err != nil {
		return user.ListPage{}, p.support.ReadError("admin.user.list", p.environment, p.site, err)
	}
	p.source = p.support.ReadSource(result)
	items := make([]user.Record, len(result.Entries))
	for index, entry := range result.Entries {
		if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &items[index]) == nil {
			continue
		}
		items[index] = user.Record{LUID: entry.LUID, Name: entry.Name}
	}
	return user.ListPage{Number: input.PageNumber, Size: input.PageSize, Total: result.Total, Users: items, SnapshotCursor: result.NextCursor}, nil
}

type CachedUserInspectPort struct {
	store             *cache.Store
	environment, site string
	support           CacheSupport
	source            *readsource.Metadata
}

func NewCachedUserInspectPort(store *cache.Store, environment, site string, support CacheSupport) *CachedUserInspectPort {
	return &CachedUserInspectPort{store: store, environment: environment, site: site, support: support}
}

func (p *CachedUserInspectPort) Source() *readsource.Metadata { return p.source }

func (p *CachedUserInspectPort) ResolveUser(ctx context.Context, selector user.Selector) (user.Record, error) {
	if err := p.support.check("admin.user.inspect"); err != nil {
		return user.Record{}, err
	}
	result, err := p.store.ReadResources(ctx, cache.ResourceQuery{Environment: p.environment, Site: p.site, Kind: "user", LUID: selector.LUID, Name: selector.Username, Limit: 2, ExactlyOne: true})
	if err != nil {
		return user.Record{}, p.support.ReadError("admin.user.inspect", p.environment, p.site, err)
	}
	entry := result.Entries[0]
	p.source = p.support.RecordSource(result, entry)
	var item user.Record
	if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &item) == nil {
		return item, nil
	}
	return user.Record{LUID: entry.LUID, Name: entry.Name}, nil
}

type CachedGroupListPort struct {
	store             *cache.Store
	environment, site string
	support           CacheSupport
	source            *readsource.Metadata
}

func NewCachedGroupListPort(store *cache.Store, environment, site string, support CacheSupport) *CachedGroupListPort {
	return &CachedGroupListPort{store: store, environment: environment, site: site, support: support}
}

func (p *CachedGroupListPort) Source() *readsource.Metadata { return p.source }

func (p *CachedGroupListPort) ListGroups(ctx context.Context, input group.ListPageRequest) (group.ListPage, error) {
	if err := p.support.check("admin.group.list"); err != nil {
		return group.ListPage{}, err
	}
	if input.Domain != "" {
		return group.ListPage{}, p.support.UnsupportedFilter("admin.group.list", p.environment, p.site)
	}
	result, err := p.store.ReadResources(ctx, cache.ResourceQuery{Environment: p.environment, Site: p.site, Kind: "group", Name: input.Name, Offset: cachePageOffset(input.PageNumber, input.PageSize, input.SnapshotCursor), Limit: input.PageSize, Cursor: input.SnapshotCursor})
	if err != nil {
		return group.ListPage{}, p.support.ReadError("admin.group.list", p.environment, p.site, err)
	}
	p.source = p.support.ReadSource(result)
	items := make([]group.Record, len(result.Entries))
	for index, entry := range result.Entries {
		if len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &items[index]) == nil {
			continue
		}
		items[index] = group.Record{LUID: entry.LUID, Name: entry.Name}
	}
	return group.ListPage{Number: input.PageNumber, Size: input.PageSize, Total: result.Total, Groups: items, SnapshotCursor: result.NextCursor}, nil
}

type CachedGroupInspectPort struct {
	store             *cache.Store
	environment, site string
	support           CacheSupport
	source            *readsource.Metadata
}

func NewCachedGroupInspectPort(store *cache.Store, environment, site string, support CacheSupport) *CachedGroupInspectPort {
	return &CachedGroupInspectPort{store: store, environment: environment, site: site, support: support}
}

func (p *CachedGroupInspectPort) Source() *readsource.Metadata { return p.source }

func (p *CachedGroupInspectPort) ResolveGroup(ctx context.Context, selector group.Selector, members bool) (group.Record, error) {
	if err := p.support.check("admin.group.inspect"); err != nil {
		return group.Record{}, err
	}
	result, err := p.store.ReadResources(ctx, cache.ResourceQuery{Environment: p.environment, Site: p.site, Kind: "group", LUID: selector.LUID, Name: selector.Name, Limit: 2, ExactlyOne: true})
	if err != nil {
		return group.Record{}, p.support.ReadError("admin.group.inspect", p.environment, p.site, err)
	}
	entry := result.Entries[0]
	var item struct {
		group.Record
		MembersFetched bool `json:"members_fetched"`
	}
	decoded := len(entry.Payload) != 0 && json.Unmarshal(entry.Payload, &item) == nil
	if members && (entry.Coverage != "detail" || !decoded || !item.MembersFetched) {
		return group.Record{}, &errs.Error{ID: "cache.detail_not_indexed", Kind: errs.KindUsage, Operation: "admin.group.inspect", Environment: p.environment, Site: p.site, Summary: "Group membership is not indexed for this cache record.", Retryable: errs.Bool(false), CorrectiveAction: "Run the command without --cache to query Tableau and update the cache."}
	}
	p.source = p.support.RecordSource(result, entry)
	if decoded {
		return item.Record, nil
	}
	return group.Record{LUID: entry.LUID, Name: entry.Name}, nil
}
