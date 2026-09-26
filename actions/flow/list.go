package flow

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
	listMaxLimit        = 10000
	listCursorVersion   = 1
	listMaxCursorLength = 2048
)

// ListReader is the action-owned list seam.
type ListReader interface {
	ListFlows(context.Context, ListPageRequest) (ListPage, error)
}

// List reads one bounded page.
func List(ctx context.Context, reader ListReader, input ListInput) (ListOutput, error) {
	selection, err := listValidateInput(input)
	if err != nil {
		return ListOutput{}, err
	}
	if input.All {
		return listCollectAll(ctx, reader, input)
	}
	if reader == nil {
		return ListOutput{}, errors.New("flow list reader is not configured")
	}
	page, err := listReadWindow(ctx, reader, ListPageRequest{PageNumber: selection.Page, PageSize: selection.Size, Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, SnapshotCursor: selection.Snapshot})
	if err != nil {
		return ListOutput{}, err
	}
	if page.Number != selection.Page || page.Size <= 0 || len(page.Flows) > page.Size {
		return ListOutput{}, errors.New("flow list reader returned inconsistent pagination")
	}
	next := ""
	if !page.SuppressContinuation && (page.SnapshotCursor != "" || page.Number*page.Size < page.Total) {
		next, err = listEncodeCursor(page.Number+1, page.Size, selection.Filter, page.SnapshotCursor)
		if err != nil {
			return ListOutput{}, err
		}
	}
	return ListOutput{Status: "listed", Environment: input.Environment, Site: input.Site, Page: ListOutputPage{Returned: len(page.Flows), Total: page.Total, Limit: page.Size, NextCursor: next, MoreAvailable: next != "" || (page.SuppressContinuation && len(page.Flows) < page.Total)}, Flows: page.Flows, RequestID: page.RequestID, Help: listListHelp(input.Environment, page.Flows)}, nil
}

func listSelectPage(value string, requested int, expectedFilter string) (int, int, string, error) {
	if value == "" {
		if requested == 0 {
			requested = 25
		}
		if requested < 1 || requested > listMaxLimit {
			return 0, 0, "", fmt.Errorf("flow list limit must be between 1 and %d", listMaxLimit)
		}
		return 1, requested, "", nil
	}
	if len(value) > listMaxCursorLength {
		return 0, 0, "", errs.New(errs.KindUsage, "invalid flow continuation cursor")
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	var cursor listCursorValue
	if err != nil || json.Unmarshal(data, &cursor) != nil || cursor.Version != listCursorVersion || cursor.Page < 2 || cursor.Size < 1 || cursor.Size > listMaxLimit || cursor.Filter == "" || len(cursor.Snapshot) > 1024 {
		return 0, 0, "", errs.New(errs.KindUsage, "invalid flow continuation cursor")
	}
	if requested != 0 && requested != cursor.Size {
		return 0, 0, "", errs.New(errs.KindUsage, "flow list limit must match the continuation cursor")
	}
	if cursor.Filter != expectedFilter {
		return 0, 0, "", errs.New(errs.KindUsage, "flow continuation cursor does not match the current filters")
	}
	return cursor.Page, cursor.Size, cursor.Snapshot, nil
}

func listEncodeCursor(page, size int, filter, snapshot string) (string, error) {
	data, err := json.Marshal(listCursorValue{Version: listCursorVersion, Page: page, Size: size, Filter: filter, Snapshot: snapshot})
	return base64.RawURLEncoding.EncodeToString(data), err
}

type listCursorValue struct {
	Version  int    `json:"v"`
	Page     int    `json:"p"`
	Size     int    `json:"s"`
	Filter   string `json:"f"`
	Snapshot string `json:"c,omitempty"`
}

func listFlowFilterFingerprint(input ListInput) (string, error) {
	data, err := json.Marshal(struct {
		Environment string `json:"environment"`
		Site        string `json:"site"`
		Name        string `json:"name"`
		OwnerName   string `json:"owner_name"`
		ProjectLUID string `json:"project_luid"`
		ProjectName string `json:"project_name"`
		Cache       bool   `json:"cache"`
	}{Environment: input.Environment, Site: input.Site, Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, Cache: input.Cache})
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
		page, err := reader.ListFlows(ctx, ListPageRequest{PageNumber: state.Number, PageSize: state.Size, SnapshotCursor: state.Token, Name: input.Name, OwnerName: input.OwnerName, ProjectName: input.ProjectName, ProjectLUID: input.ProjectLUID})
		requestID = page.RequestID
		return paging.Page[Record]{Number: page.Number, Size: page.Size, Total: page.Total, Items: page.Flows, Token: page.SnapshotCursor}, err
	}, func(item Record) string { return item.LUID })
	if err != nil {
		return ListOutput{}, err
	}
	return ListOutput{Status: "listed", Environment: input.Environment, Site: input.Site, Flows: items, Page: ListOutputPage{Returned: len(items), Total: len(items), Limit: 10000}, RequestID: requestID, Help: listListHelp(input.Environment, items)}, nil
}

func listListHelp(environment string, items []Record) []string {
	if len(items) == 0 {
		return nil
	}
	return []string{commandhint.Environment(environment, "content", "flow", "inspect", "--id", items[0].LUID)}
}

func listReadWindow(ctx context.Context, reader ListReader, request ListPageRequest) (ListPage, error) {
	if request.PageSize <= 1000 {
		return reader.ListFlows(ctx, request)
	}
	requestID := ""
	page, err := paging.Window(ctx, paging.State{Number: request.PageNumber, Size: request.PageSize, Token: request.SnapshotCursor}, 1000, func(ctx context.Context, state paging.State) (paging.Page[Record], error) {
		input := request
		input.PageNumber, input.PageSize, input.SnapshotCursor = state.Number, state.Size, state.Token
		page, err := reader.ListFlows(ctx, input)
		requestID = page.RequestID
		return paging.Page[Record]{Number: page.Number, Size: page.Size, Total: page.Total, Items: page.Flows, Token: page.SnapshotCursor}, err
	}, func(item Record) string { return item.LUID })
	return ListPage{Number: page.Number, Size: page.Size, Total: page.Total, Flows: page.Items, SnapshotCursor: "", SuppressContinuation: true, RequestID: requestID}, err
}

type listSelection struct {
	Page     int
	Size     int
	Snapshot string
	Filter   string
}

// ValidateListInput validates bounds and continuation identity without a reader.
func ValidateListInput(input ListInput) error {
	_, err := listValidateInput(input)
	return err
}

func listValidateInput(input ListInput) (listSelection, error) {
	if input.All {
		if input.Limit != 0 || input.Cursor != "" {
			return listSelection{}, errs.New(errs.KindUsage, "--all cannot be combined with --limit or --cursor")
		}
		return listSelection{}, nil
	}
	fingerprint, err := listFlowFilterFingerprint(input)
	if err != nil {
		return listSelection{}, err
	}
	page, size, snapshot, err := listSelectPage(input.Cursor, input.Limit, fingerprint)
	if err != nil {
		return listSelection{}, errs.New(errs.KindUsage, err.Error())
	}
	return listSelection{Page: page, Size: size, Snapshot: snapshot, Filter: fingerprint}, nil
}
