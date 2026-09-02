package list

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ahillspace/tadx/internal/errs"
)

const (
	defaultLimit    = 25
	maxLimit        = 100
	maxCursorPage   = 1_000_000
	cursorVersion   = 1
	maxCursorLength = 256
)

// Reader is the action-owned list seam.
type Reader interface {
	ListWorkbooks(context.Context, PageRequest) (Page, error)
}

// Action lists workbooks.
type Action struct{ reader Reader }

// New creates a workbook list action.
func New(reader Reader) *Action { return &Action{reader: reader} }

// Execute reads one bounded page.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.reader == nil {
		return Output{}, &errs.Error{ID: "workbook.list.unconfigured", Kind: errs.KindRuntime, Operation: "workbook.list", Summary: "Workbook list is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure workbook listing before retrying."}
	}
	fingerprint, err := filterFingerprint(input)
	if err != nil {
		return Output{}, fmt.Errorf("build workbook continuation cursor: %w", err)
	}
	pageNumber, pageSize, err := selectPage(input.Cursor, input.Limit, fingerprint)
	if err != nil {
		return Output{}, err
	}
	page, err := a.reader.ListWorkbooks(ctx, PageRequest{PageNumber: pageNumber, PageSize: pageSize, Name: input.Name, OwnerName: input.OwnerName, ProjectName: input.ProjectName, Tag: input.Tag})
	if err != nil {
		return Output{}, err
	}
	if page.Number != pageNumber || page.Size <= 0 || page.Size > pageSize || page.Total < 0 || len(page.Workbooks) > page.Size || int64(page.Number-1)*int64(page.Size)+int64(len(page.Workbooks)) > int64(page.Total) {
		return Output{}, errors.New("workbook list reader returned inconsistent pagination")
	}
	next := ""
	if int64(page.Number)*int64(page.Size) < int64(page.Total) {
		next, err = encodeCursor(page.Number+1, page.Size, fingerprint)
		if err != nil {
			return Output{}, err
		}
	}
	return Output{Status: "listed", Environment: input.Environment, Site: input.Site, Page: OutputPage{Returned: len(page.Workbooks), Total: page.Total, Limit: page.Size, NextCursor: next}, Workbooks: page.Workbooks, RequestID: page.RequestID, Help: []string{"tadx content workbook get --id <workbook-luid>"}}, nil
}

func selectPage(value string, requested int, expectedFilter string) (int, int, error) {
	if value == "" {
		if requested == 0 {
			requested = defaultLimit
		}
		if requested < 1 || requested > maxLimit {
			return 0, 0, errs.New(errs.KindUsage, fmt.Sprintf("workbook list limit must be between 1 and %d", maxLimit))
		}
		return 1, requested, nil
	}
	if len(value) > maxCursorLength {
		return 0, 0, errs.New(errs.KindUsage, "invalid workbook continuation cursor")
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	var cursor cursorValue
	if err != nil || json.Unmarshal(data, &cursor) != nil || cursor.Version != cursorVersion || cursor.Page < 2 || cursor.Page > maxCursorPage || cursor.Size < 1 || cursor.Size > maxLimit || cursor.Filter == "" {
		return 0, 0, errs.New(errs.KindUsage, "invalid workbook continuation cursor")
	}
	if requested != 0 && requested != cursor.Size {
		return 0, 0, errs.New(errs.KindUsage, "workbook list limit must match the continuation cursor")
	}
	if cursor.Filter != expectedFilter {
		return 0, 0, errs.New(errs.KindUsage, "workbook continuation cursor does not match the current filters")
	}
	return cursor.Page, cursor.Size, nil
}

func encodeCursor(page, size int, filter string) (string, error) {
	data, err := json.Marshal(cursorValue{Version: cursorVersion, Page: page, Size: size, Filter: filter})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

type cursorValue struct {
	Version int    `json:"v"`
	Page    int    `json:"p"`
	Size    int    `json:"s"`
	Filter  string `json:"f"`
}

func filterFingerprint(input Input) (string, error) {
	data, err := json.Marshal(struct {
		Environment string `json:"environment"`
		Site        string `json:"site"`
		Name        string `json:"name"`
		OwnerName   string `json:"owner_name"`
		ProjectName string `json:"project_name"`
		Tag         string `json:"tag"`
	}{Environment: input.Environment, Site: input.Site, Name: input.Name, OwnerName: input.OwnerName, ProjectName: input.ProjectName, Tag: input.Tag})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}
