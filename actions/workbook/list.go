package workbook

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/paging"

	"github.com/ahillspace/tadx/internal/errs"
)

const (
	listDefaultLimit    = 25
	listMaxLimit        = 10000
	listMaxCursorPage   = 1_000_000
	listCursorVersion   = 1
	listMaxCursorLength = 2048
)

// ListReader is the action-owned list seam.
type ListReader interface {
	ListWorkbooks(context.Context, ListPageRequest) (ListPage, error)
}

// List reads one bounded page or the explicitly requested complete inventory.
func List(ctx context.Context, reader ListReader, input ListInput) (ListOutput, error) {
	selection, err := listValidateInput(input)
	if err != nil {
		return ListOutput{}, err
	}
	if input.All {
		return listCollectAll(ctx, reader, input)
	}
	if reader == nil {
		return ListOutput{}, &errs.Error{ID: "workbook.list.unconfigured", Kind: errs.KindRuntime, Operation: "workbook.list", Summary: "Workbook list is not configured.", Retryable: new(false), CorrectiveAction: "Configure workbook listing before retrying."}
	}
	page, err := listReadWindow(ctx, reader, ListPageRequest{PageNumber: selection.Page, PageSize: selection.Size, Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, Tag: input.Tag, SnapshotCursor: selection.Snapshot})
	if err != nil {
		return ListOutput{}, err
	}
	if page.Number != selection.Page || page.Size <= 0 || page.Size > selection.Size || page.Total < 0 || len(page.Workbooks) > page.Size || int64(page.Number-1)*int64(page.Size)+int64(len(page.Workbooks)) > int64(page.Total) {
		return ListOutput{}, errors.New("workbook list reader returned inconsistent pagination")
	}
	next := ""
	if !page.SuppressContinuation && (page.SnapshotCursor != "" || int64(page.Number)*int64(page.Size) < int64(page.Total)) {
		next = listEncodeCursor(page.Number+1, page.Size, selection.Filter, page.SnapshotCursor)
	}
	return ListOutput{Status: "listed", Environment: input.Environment, Site: input.Site, Page: ListOutputPage{Returned: len(page.Workbooks), Total: page.Total, Limit: page.Size, NextCursor: next, MoreAvailable: next != "" || (page.SuppressContinuation && len(page.Workbooks) < page.Total)}, Workbooks: page.Workbooks, RequestID: page.RequestID, Help: listHelp(input.Environment, page.Workbooks)}, nil
}

func listSelectPage(value string, requested int, expectedFilter string) (listCursorValue, error) {
	if value == "" {
		if requested == 0 {
			requested = listDefaultLimit
		}
		if requested < 1 || requested > listMaxLimit {
			return listCursorValue{}, errs.New(errs.KindUsage, fmt.Sprintf("workbook list limit must be between 1 and %d", listMaxLimit))
		}
		return listCursorValue{Version: listCursorVersion, Page: 1, Size: requested, Filter: expectedFilter}, nil
	}
	if len(value) > listMaxCursorLength {
		return listCursorValue{}, errs.New(errs.KindUsage, "invalid workbook continuation cursor")
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	var cursor listCursorValue
	if err != nil || json.Unmarshal(data, &cursor) != nil || cursor.Version != listCursorVersion || cursor.Page < 2 || cursor.Page > listMaxCursorPage || cursor.Size < 1 || cursor.Size > listMaxLimit || cursor.Filter == "" || len(cursor.Snapshot) > 1024 {
		return listCursorValue{}, errs.New(errs.KindUsage, "invalid workbook continuation cursor")
	}
	if requested != 0 && requested != cursor.Size {
		return listCursorValue{}, errs.New(errs.KindUsage, "workbook list limit must match the continuation cursor")
	}
	if cursor.Filter != expectedFilter {
		return listCursorValue{}, errs.New(errs.KindUsage, "workbook continuation cursor does not match the current filters")
	}
	return cursor, nil
}

func listEncodeCursor(page, size int, filter, snapshot string) string {
	// cursorValue contains only integers and strings, so JSON encoding cannot fail.
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

func listFilterFingerprint(input ListInput) string {
	// This fixed string/bool projection has no fallible JSON values or marshalers.
	data, _ := json.Marshal(struct {
		Environment string `json:"environment"`
		Site        string `json:"site"`
		Name        string `json:"name"`
		OwnerName   string `json:"owner_name"`
		ProjectLUID string `json:"project_luid"`
		ProjectName string `json:"project_name"`
		Tag         string `json:"tag"`
		Cache       bool   `json:"cache"`
	}{Environment: input.Environment, Site: input.Site, Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, Tag: input.Tag, Cache: input.Cache})
	sum := sha256.Sum256(data)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// listCollectAll follows private bounded pages and fails closed on incomplete inventories.
func listCollectAll(ctx context.Context, reader ListReader, input ListInput) (ListOutput, error) {
	if reader == nil {
		return ListOutput{}, errors.New("inventory reader is not configured")
	}
	requestID := ""
	items, err := paging.Collect(ctx, func(ctx context.Context, state paging.State) (paging.Page[Record], error) {
		page, err := reader.ListWorkbooks(ctx, ListPageRequest{PageNumber: state.Number, PageSize: state.Size, SnapshotCursor: state.Token, Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, Tag: input.Tag})
		requestID = page.RequestID
		return paging.Page[Record]{Number: page.Number, Size: page.Size, Total: page.Total, Items: page.Workbooks, Token: page.SnapshotCursor}, err
	}, func(item Record) string { return item.LUID })
	if err != nil {
		return ListOutput{}, err
	}
	return ListOutput{Status: "listed", Environment: input.Environment, Site: input.Site, Workbooks: items, Page: ListOutputPage{Returned: len(items), Total: len(items), Limit: 10000}, RequestID: requestID, Help: listHelp(input.Environment, items)}, nil
}

func listHelp(environment string, items []Record) []string {
	if len(items) == 0 {
		return nil
	}
	return []string{commandhint.Environment(environment, "content", "workbook", "inspect", "--id", items[0].LUID)}
}

func listReadWindow(ctx context.Context, reader ListReader, request ListPageRequest) (ListPage, error) {
	if request.PageSize <= 1000 {
		return reader.ListWorkbooks(ctx, request)
	}
	requestID := ""
	page, err := paging.Window(ctx, paging.State{Number: request.PageNumber, Size: request.PageSize, Token: request.SnapshotCursor}, 1000, func(ctx context.Context, state paging.State) (paging.Page[Record], error) {
		input := request
		input.PageNumber, input.PageSize, input.SnapshotCursor = state.Number, state.Size, state.Token
		page, err := reader.ListWorkbooks(ctx, input)
		requestID = page.RequestID
		return paging.Page[Record]{Number: page.Number, Size: page.Size, Total: page.Total, Items: page.Workbooks, Token: page.SnapshotCursor}, err
	}, func(item Record) string { return item.LUID })
	return ListPage{Number: page.Number, Size: page.Size, Total: page.Total, Workbooks: page.Items, SnapshotCursor: "", SuppressContinuation: true, RequestID: requestID}, err
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
	selection, err := listSelectPage(input.Cursor, input.Limit, listFilterFingerprint(input))
	if err != nil {
		return listCursorValue{}, errs.New(errs.KindUsage, err.Error())
	}
	return selection, nil
}
