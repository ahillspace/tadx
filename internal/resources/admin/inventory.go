package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	group "github.com/ahillspace/tadx/actions/admin/group"
	user "github.com/ahillspace/tadx/actions/admin/user"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/inventory"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
)

// InventoryPorts binds one target to the shared collector and typed admin pages.
type InventoryPorts struct {
	Executor          tableaucache.Executor
	Store             *cache.Store
	Environment, Site string
	MaxConcurrency    int
	Now               func() time.Time
}

func (p InventoryPorts) CollectGroups(ctx context.Context, filter string, observedAt time.Time) (group.CollectedList, error) {
	collected, err := inventory.Collect(ctx, p.Executor, p.Store, tableaucache.ScopeGroups, p.Environment, p.Site, observedAt, inventory.Options{MaxConcurrency: p.MaxConcurrency, Filter: filter})
	if err != nil {
		return group.CollectedList{}, err
	}
	source, help := collected.SourceAndHelp(observedAt, p.Now)
	requestID := collected.FinalRequestID()
	return group.CollectedList{Reader: groupInventoryPages{entries: collected.Entries, requestID: requestID}, Source: source, RequestID: requestID, Help: help}, nil
}

func (p InventoryPorts) CollectUsers(ctx context.Context, filter string, observedAt time.Time) (user.CollectedList, error) {
	collected, err := inventory.Collect(ctx, p.Executor, p.Store, tableaucache.ScopeUsers, p.Environment, p.Site, observedAt, inventory.Options{MaxConcurrency: p.MaxConcurrency, Filter: filter})
	if err != nil {
		return user.CollectedList{}, err
	}
	source, help := collected.SourceAndHelp(observedAt, p.Now)
	requestID := collected.FinalRequestID()
	return user.CollectedList{Reader: userInventoryPages{entries: collected.Entries, requestID: requestID}, Source: source, RequestID: requestID, Help: help}, nil
}

func (p InventoryPorts) PublishGroupInspect(_ context.Context, output group.InspectOutput, membersRequested bool, observedAt time.Time) {
	payload, err := json.Marshal(struct {
		group.Record
		MembersFetched bool `json:"members_fetched"`
	}{Record: output.Group, MembersFetched: membersRequested})
	if err == nil {
		inventory.PublishDetail(p.Store, cache.ResourceEntry{Environment: output.Environment, Site: output.Site, Kind: "group", LUID: output.Group.LUID, Name: output.Group.Name, Payload: payload, Coverage: "detail", ObservedAt: observedAt})
	}
}

func (p InventoryPorts) PublishUserInspect(_ context.Context, output user.InspectOutput, observedAt time.Time) {
	payload, err := json.Marshal(output.User)
	if err == nil {
		inventory.PublishDetail(p.Store, cache.ResourceEntry{Environment: output.Environment, Site: output.Site, Kind: "user", LUID: output.User.LUID, Name: output.User.Name, Payload: payload, Coverage: "detail", ObservedAt: observedAt})
	}
}

type groupInventoryPages struct {
	entries   []cache.ResourceEntry
	requestID string
}

func (p groupInventoryPages) ListGroups(_ context.Context, input group.ListPageRequest) (group.ListPage, error) {
	start := min((input.PageNumber-1)*input.PageSize, len(p.entries))
	end := min(start+input.PageSize, len(p.entries))
	items := make([]group.Record, end-start)
	for index, entry := range p.entries[start:end] {
		if err := json.Unmarshal(entry.Payload, &items[index]); err != nil {
			return group.ListPage{}, fmt.Errorf("decode live inventory row: %w", err)
		}
	}
	return group.ListPage{Number: input.PageNumber, Size: input.PageSize, Total: len(p.entries), Groups: items, RequestID: p.requestID}, nil
}

type userInventoryPages struct {
	entries   []cache.ResourceEntry
	requestID string
}

func (p userInventoryPages) ListUsers(_ context.Context, input user.ListPageRequest) (user.ListPage, error) {
	start := min((input.PageNumber-1)*input.PageSize, len(p.entries))
	end := min(start+input.PageSize, len(p.entries))
	items := make([]user.Record, end-start)
	for index, entry := range p.entries[start:end] {
		if err := json.Unmarshal(entry.Payload, &items[index]); err != nil {
			return user.ListPage{}, fmt.Errorf("decode live inventory row: %w", err)
		}
	}
	return user.ListPage{Number: input.PageNumber, Size: input.PageSize, Total: len(p.entries), Users: items, RequestID: p.requestID}, nil
}
