package project

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/paging"
	"regexp"

	"github.com/ahillspace/tadx/internal/errs"
)

const (
	listDefaultLimit    = 25
	listMaxLimit        = 10000
	listCursorVersion   = 1
	listMaxCursorLength = 2048
)

var listOwnerLUIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$`)

const listOwnerLUIDCursor = "owner_luid"

// MayBeOwnerLUID recognizes the UUID form Tableau returns for owner LUIDs.
func ListMayBeOwnerLUID(value string) bool { return listOwnerLUIDPattern.MatchString(value) }

// Reader is the action-owned project listing seam.
type ListReader interface {
	ListProjects(context.Context, ListPageRequest) (ListPage, error)
}

// ValidateInput validates bounds and continuation identity without a reader.
func ValidateListInput(input ListInput) error {
	_, err := listParseInput(input)
	return err
}

type listSelection struct {
	fingerprint string
	pageNumber  int
	pageSize    int
	snapshot    string
}

func listParseInput(input ListInput) (listSelection, error) {
	if input.All {
		if input.Limit != 0 || input.Cursor != "" {
			return listSelection{}, errs.New(errs.KindUsage, "--all cannot be combined with --limit or --cursor")
		}
		return listSelection{}, nil
	}
	fingerprint := listProjectFilterFingerprint(input)
	pageNumber, pageSize, snapshot, err := listPageSelection(input.Cursor, input.Limit, fingerprint)
	if err != nil {
		return listSelection{}, errs.New(errs.KindUsage, err.Error())
	}
	return listSelection{fingerprint: fingerprint, pageNumber: pageNumber, pageSize: pageSize, snapshot: snapshot}, nil
}

// List reads one page without hidden continuation reads.
func (a *Service) List(ctx context.Context, input ListInput) (ListOutput, error) {
	selected, err := listParseInput(input)
	if err != nil {
		return ListOutput{}, err
	}
	if input.All {
		out, err := a.collectAll(ctx, input)
		if err == nil && len(out.Projects) == 0 && listOwnerLUIDPattern.MatchString(input.OwnerName) {
			return a.listByOwnerLUID(ctx, input, selected)
		}
		return out, err
	}
	if a == nil || a.ListReader == nil {
		return ListOutput{}, errors.New("project list reader is not configured")
	}
	if selected.snapshot == listOwnerLUIDCursor && listOwnerLUIDPattern.MatchString(input.OwnerName) {
		return a.listByOwnerLUID(ctx, input, selected)
	}
	request := ListPageRequest{PageNumber: selected.pageNumber, PageSize: selected.pageSize, Name: input.Name, ParentLUID: input.ParentLUID, OwnerName: input.OwnerName, TopLevel: input.TopLevel, SnapshotCursor: selected.snapshot}
	page, err := a.readWindow(ctx, request)
	if err != nil {
		return ListOutput{}, err
	}
	if page.Number != selected.pageNumber || page.Size <= 0 || page.Total < 0 || len(page.Projects) > page.Size {
		return ListOutput{}, errors.New("project list reader returned inconsistent pagination")
	}
	next := ""
	if !page.SuppressContinuation && (page.SnapshotCursor != "" || page.Number*page.Size < page.Total) {
		next = listEncodeCursor(page.Number+1, page.Size, selected.fingerprint, page.SnapshotCursor)
	}
	out := ListOutput{
		Status: "listed", Environment: input.Environment, Site: input.Site, Projects: page.Projects,
		Page:      ListOutputPage{Returned: len(page.Projects), Total: page.Total, Limit: page.Size, NextCursor: next, MoreAvailable: next != "" || (page.SuppressContinuation && len(page.Projects) < page.Total)},
		RequestID: page.RequestID,
		Help:      listHelp(input.Environment, page.Projects),
	}
	if page.Total == 0 && listOwnerLUIDPattern.MatchString(input.OwnerName) {
		return a.listByOwnerLUID(ctx, input, selected)
	}
	return out, nil
}

// Tableau's project list supports ownerName but not owner LUID filtering.
// When the documented name filter finds no rows, a UUID-shaped selector is
// resolved against a complete inventory using authoritative owner LUIDs.
func (a *Service) listByOwnerLUID(ctx context.Context, input ListInput, selected listSelection) (ListOutput, error) {
	if a == nil || a.ListReader == nil {
		return ListOutput{}, errors.New("project list reader is not configured")
	}
	if input.Cache {
		return ListOutput{}, errs.New(errs.KindUsage, "owner LUID filtering requires live project inventory; remove --cache")
	}
	requestID := ""
	items, err := paging.Collect(ctx, func(ctx context.Context, state paging.State) (paging.Page[ListProject], error) {
		page, readErr := a.ListReader.ListProjects(ctx, ListPageRequest{PageNumber: state.Number, PageSize: state.Size, SnapshotCursor: state.Token, Name: input.Name, ParentLUID: input.ParentLUID, TopLevel: input.TopLevel})
		requestID = page.RequestID
		return paging.Page[ListProject]{Number: page.Number, Size: page.Size, Total: page.Total, Items: page.Projects, Token: page.SnapshotCursor}, readErr
	}, func(item ListProject) string { return item.LUID })
	if err != nil {
		return ListOutput{}, err
	}
	owned := make([]ListProject, 0)
	for _, item := range items {
		if item.OwnerLUID == input.OwnerName {
			owned = append(owned, item)
		}
	}
	if input.All {
		return ListOutput{Status: "listed", Environment: input.Environment, Site: input.Site, Projects: owned, Page: ListOutputPage{Returned: len(owned), Total: len(owned), Limit: listMaxLimit}, RequestID: requestID, Help: listHelp(input.Environment, owned)}, nil
	}
	start := len(owned)
	if selected.pageNumber <= len(owned)/selected.pageSize+1 {
		start = min((selected.pageNumber-1)*selected.pageSize, len(owned))
	}
	end := min(start+selected.pageSize, len(owned))
	page := owned[start:end]
	next := ""
	if end < len(owned) {
		next = listEncodeCursor(selected.pageNumber+1, selected.pageSize, selected.fingerprint, listOwnerLUIDCursor)
	}
	return ListOutput{Status: "listed", Environment: input.Environment, Site: input.Site, Projects: page, Page: ListOutputPage{Returned: len(page), Total: len(owned), Limit: selected.pageSize, NextCursor: next, MoreAvailable: next != ""}, RequestID: requestID, Help: listHelp(input.Environment, page)}, nil
}

func listPageSelection(value string, requested int, expectedFilter string) (int, int, string, error) {
	if value == "" {
		if requested == 0 {
			requested = listDefaultLimit
		}
		if requested < 1 || requested > listMaxLimit {
			return 0, 0, "", fmt.Errorf("project list limit must be between 1 and %d", listMaxLimit)
		}
		return 1, requested, "", nil
	}
	if len(value) > listMaxCursorLength {
		return 0, 0, "", errs.New(errs.KindUsage, "invalid project continuation cursor")
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return 0, 0, "", errs.New(errs.KindUsage, "invalid project continuation cursor")
	}
	var cursor listCursorValue
	if json.Unmarshal(data, &cursor) != nil || cursor.Version != listCursorVersion || cursor.Page < 2 || cursor.Size < 1 || cursor.Size > listMaxLimit || cursor.Filter == "" || len(cursor.Snapshot) > 1024 {
		return 0, 0, "", errs.New(errs.KindUsage, "invalid project continuation cursor")
	}
	if requested != 0 && requested != cursor.Size {
		return 0, 0, "", errs.New(errs.KindUsage, "project list limit must match the continuation cursor")
	}
	if cursor.Filter != expectedFilter {
		return 0, 0, "", errs.New(errs.KindUsage, "project continuation cursor does not match the current filters")
	}
	return cursor.Page, cursor.Size, cursor.Snapshot, nil
}

func listEncodeCursor(page, size int, filter, snapshot string) string {
	data, _ := json.Marshal(listCursorValue{Version: listCursorVersion, Page: page, Size: size, Filter: filter, Snapshot: snapshot})
	return base64.RawURLEncoding.EncodeToString(data)
}

type listCursorValue struct {
	Version  int    `json:"v"`
	Page     int    `json:"p"`
	Size     int    `json:"s"`
	Filter   string `json:"f"`
	Snapshot string `json:"c,omitempty"`
}

func listProjectFilterFingerprint(input ListInput) string {
	data, _ := json.Marshal(struct {
		Environment string `json:"environment"`
		Site        string `json:"site"`
		Name        string `json:"name"`
		ParentLUID  string `json:"parent_luid"`
		OwnerName   string `json:"owner_name"`
		TopLevel    *bool  `json:"top_level"`
		Cache       bool   `json:"cache"`
	}{Environment: input.Environment, Site: input.Site, Name: input.Name, ParentLUID: input.ParentLUID, OwnerName: input.OwnerName, TopLevel: input.TopLevel, Cache: input.Cache})
	sum := sha256.Sum256(data)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// collectAll follows private bounded pages and fails closed on incomplete inventories.
func (a *Service) collectAll(ctx context.Context, input ListInput) (ListOutput, error) {
	if a == nil || a.ListReader == nil {
		return ListOutput{}, errors.New("inventory reader is not configured")
	}
	requestID := ""
	items, err := paging.Collect(ctx, func(ctx context.Context, state paging.State) (paging.Page[ListProject], error) {
		page, err := a.ListReader.ListProjects(ctx, ListPageRequest{PageNumber: state.Number, PageSize: state.Size, SnapshotCursor: state.Token, Name: input.Name, ParentLUID: input.ParentLUID, OwnerName: input.OwnerName, TopLevel: input.TopLevel})
		requestID = page.RequestID
		return paging.Page[ListProject]{Number: page.Number, Size: page.Size, Total: page.Total, Items: page.Projects, Token: page.SnapshotCursor}, err
	}, func(item ListProject) string { return item.LUID })
	if err != nil {
		return ListOutput{}, err
	}
	return ListOutput{Status: "listed", Environment: input.Environment, Site: input.Site, Projects: items, Page: ListOutputPage{Returned: len(items), Total: len(items), Limit: 10000}, RequestID: requestID, Help: listHelp(input.Environment, items)}, nil
}

func listHelp(environment string, items []ListProject) []string {
	if len(items) == 0 {
		return nil
	}
	return []string{commandhint.Environment(environment, "content", "project", "inspect", "--project-id", items[0].LUID)}
}

func (a *Service) readWindow(ctx context.Context, request ListPageRequest) (ListPage, error) {
	if request.PageSize <= 1000 {
		return a.ListReader.ListProjects(ctx, request)
	}
	requestID := ""
	page, err := paging.Window(ctx, paging.State{Number: request.PageNumber, Size: request.PageSize, Token: request.SnapshotCursor}, 1000, func(ctx context.Context, state paging.State) (paging.Page[ListProject], error) {
		input := request
		input.PageNumber, input.PageSize, input.SnapshotCursor = state.Number, state.Size, state.Token
		page, err := a.ListReader.ListProjects(ctx, input)
		requestID = page.RequestID
		return paging.Page[ListProject]{Number: page.Number, Size: page.Size, Total: page.Total, Items: page.Projects, Token: page.SnapshotCursor}, err
	}, func(item ListProject) string { return item.LUID })
	return ListPage{Number: page.Number, Size: page.Size, Total: page.Total, Projects: page.Items, SnapshotCursor: "", SuppressContinuation: true, RequestID: requestID}, err
}
