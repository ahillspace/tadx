package datasource

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/paging"
)

const (
	listDefaultLimit    = 25
	listMaxLimit        = 10000
	listMaxCursorPage   = 1_000_000
	listCursorVersion   = 1
	listMaxCursorLength = 2048
)

// ListReader is the action-owned published datasource listing seam.
type ListReader interface {
	ListDatasources(context.Context, ListPageRequest) (ListPage, error)
}

// List lists one page without hidden continuation reads.
func List(ctx context.Context, reader ListReader, input ListInput) (ListOutput, error) {
	selection, err := listValidateInput(input)
	if err != nil {
		return ListOutput{}, err
	}
	if input.All {
		return listCollectAll(ctx, reader, input)
	}
	if reader == nil {
		return ListOutput{}, &errs.Error{ID: "datasource.list.unconfigured", Kind: errs.KindRuntime, Operation: "datasource.list", Summary: "Datasource list is not configured.", Retryable: new(false), CorrectiveAction: "Configure datasource listing before retrying."}
	}
	pageNumber, pageSize, snapshotCursor := selection.Page, selection.Size, selection.Snapshot
	request := ListPageRequest{
		PageNumber: pageNumber, PageSize: pageSize, Name: input.Name, OwnerName: input.OwnerName,
		ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, Type: input.Type, Tag: input.Tag,
		UpdatedAfter: input.UpdatedAfter, UpdatedBefore: input.UpdatedBefore,
		SnapshotCursor: snapshotCursor,
	}
	page, err := listReadWindow(ctx, reader, request)
	if err != nil {
		return ListOutput{}, err
	}
	if page.Number != pageNumber || page.Size <= 0 || page.Size > pageSize || page.Total < 0 || len(page.Datasources) > page.Size || int64(page.Number-1)*int64(page.Size)+int64(len(page.Datasources)) > int64(page.Total) {
		return ListOutput{}, errors.New("datasource list reader returned inconsistent pagination")
	}
	next := ""
	if !page.SuppressContinuation && (page.SnapshotCursor != "" || int64(page.Number)*int64(page.Size) < int64(page.Total)) {
		next, err = listEncodeCursor(page.Number+1, page.Size, selection.Filter, page.SnapshotCursor)
		if err != nil {
			return ListOutput{}, err
		}
	}
	return ListOutput{
		Status: "listed", Environment: input.Environment, Site: input.Site,
		Page:        ListOutputPage{Returned: len(page.Datasources), Total: page.Total, Limit: page.Size, NextCursor: next, MoreAvailable: next != "" || (page.SuppressContinuation && len(page.Datasources) < page.Total)},
		Datasources: page.Datasources, RequestID: page.RequestID,
		Help: listListHelp(input.Environment, page.Datasources),
	}, nil
}

func listPageSelection(value string, requested int, expectedFilter string) (int, int, string, error) {
	if value == "" {
		if requested == 0 {
			requested = listDefaultLimit
		}
		if requested < 1 || requested > listMaxLimit {
			return 0, 0, "", errs.New(errs.KindUsage, fmt.Sprintf("datasource list limit must be between 1 and %d", listMaxLimit))
		}
		return 1, requested, "", nil
	}
	if len(value) > listMaxCursorLength {
		return 0, 0, "", errs.New(errs.KindUsage, "invalid datasource continuation cursor")
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	var cursor listCursorValue
	if err != nil || json.Unmarshal(data, &cursor) != nil || cursor.Version != listCursorVersion || cursor.Page < 2 || cursor.Page > listMaxCursorPage || cursor.Size < 1 || cursor.Size > listMaxLimit || cursor.Filter == "" || len(cursor.Snapshot) > 1024 {
		return 0, 0, "", errs.New(errs.KindUsage, "invalid datasource continuation cursor")
	}
	if requested != 0 && requested != cursor.Size {
		return 0, 0, "", errs.New(errs.KindUsage, "datasource list limit must match the continuation cursor")
	}
	if cursor.Filter != expectedFilter {
		return 0, 0, "", errs.New(errs.KindUsage, "datasource continuation cursor does not match the current filters")
	}
	return cursor.Page, cursor.Size, cursor.Snapshot, nil
}

func listEncodeCursor(page, size int, filter, snapshot string) (string, error) {
	data, err := json.Marshal(listCursorValue{Version: listCursorVersion, Page: page, Size: size, Filter: filter, Snapshot: snapshot})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

type listCursorValue struct {
	Version  int    `json:"v"`
	Page     int    `json:"p"`
	Size     int    `json:"s"`
	Filter   string `json:"f"`
	Snapshot string `json:"c,omitempty"`
}

func listDatasourceFilterFingerprint(input ListInput) (string, error) {
	data, err := json.Marshal(struct {
		Environment   string `json:"environment"`
		Site          string `json:"site"`
		Name          string `json:"name"`
		OwnerName     string `json:"owner_name"`
		ProjectLUID   string `json:"project_luid"`
		ProjectName   string `json:"project_name"`
		Type          string `json:"type"`
		Tag           string `json:"tag"`
		UpdatedAfter  string `json:"updated_after"`
		UpdatedBefore string `json:"updated_before"`
		Cache         bool   `json:"cache"`
	}{input.Environment, input.Site, input.Name, input.OwnerName, input.ProjectLUID, input.ProjectName, input.Type, input.Tag, input.UpdatedAfter, input.UpdatedBefore, input.Cache})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// listCollectAll follows private bounded pages and fails closed on incomplete inventories.
func listCollectAll(ctx context.Context, reader ListReader, input ListInput) (ListOutput, error) {
	if reader == nil {
		return ListOutput{}, errors.New("inventory reader is not configured")
	}
	requestID := ""
	items, err := paging.Collect(ctx, func(ctx context.Context, state paging.State) (paging.Page[Record], error) {
		page, err := reader.ListDatasources(ctx, ListPageRequest{PageNumber: state.Number, PageSize: state.Size, SnapshotCursor: state.Token, Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, Type: input.Type, Tag: input.Tag, UpdatedAfter: input.UpdatedAfter, UpdatedBefore: input.UpdatedBefore})
		requestID = page.RequestID
		return paging.Page[Record]{Number: page.Number, Size: page.Size, Total: page.Total, Items: page.Datasources, Token: page.SnapshotCursor}, err
	}, func(item Record) string { return item.LUID })
	if err != nil {
		return ListOutput{}, err
	}
	return ListOutput{Status: "listed", Environment: input.Environment, Site: input.Site, Datasources: items, Page: ListOutputPage{Returned: len(items), Total: len(items), Limit: 10000}, RequestID: requestID, Help: listListHelp(input.Environment, items)}, nil
}

func listListHelp(environment string, items []Record) []string {
	if len(items) == 0 {
		return nil
	}
	return []string{commandhint.Environment(environment, "content", "datasource", "inspect", "--id", items[0].LUID)}
}

func listReadWindow(ctx context.Context, reader ListReader, request ListPageRequest) (ListPage, error) {
	if request.PageSize <= 1000 {
		return reader.ListDatasources(ctx, request)
	}
	requestID := ""
	page, err := paging.Window(ctx, paging.State{Number: request.PageNumber, Size: request.PageSize, Token: request.SnapshotCursor}, 1000, func(ctx context.Context, state paging.State) (paging.Page[Record], error) {
		input := request
		input.PageNumber, input.PageSize, input.SnapshotCursor = state.Number, state.Size, state.Token
		page, err := reader.ListDatasources(ctx, input)
		requestID = page.RequestID
		return paging.Page[Record]{Number: page.Number, Size: page.Size, Total: page.Total, Items: page.Datasources, Token: page.SnapshotCursor}, err
	}, func(item Record) string { return item.LUID })
	return ListPage{Number: page.Number, Size: page.Size, Total: page.Total, Datasources: page.Items, SnapshotCursor: "", SuppressContinuation: true, RequestID: requestID}, err
}

// ValidateListInput validates bounds and continuation identity without a reader.
func ValidateListInput(input ListInput) error {
	_, err := listValidateInput(input)
	return err
}

func listValidateInput(input ListInput) (listCursorValue, error) {
	if input.All {
		if input.Limit != 0 || input.Cursor != "" {
			return listCursorValue{}, errs.New(errs.KindUsage, "--all cannot be combined with --limit or --cursor")
		}
		return listCursorValue{}, nil
	}
	fingerprint, err := listDatasourceFilterFingerprint(input)
	if err != nil {
		return listCursorValue{}, err
	}
	page, size, snapshot, err := listPageSelection(input.Cursor, input.Limit, fingerprint)
	if err != nil {
		return listCursorValue{}, errs.New(errs.KindUsage, err.Error())
	}
	return listCursorValue{Version: listCursorVersion, Page: page, Size: size, Snapshot: snapshot, Filter: fingerprint}, nil
}
