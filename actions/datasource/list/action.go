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

// Reader is the action-owned published datasource listing seam.
type Reader interface {
	ListDatasources(context.Context, PageRequest) (Page, error)
}

// Action lists one bounded published datasource page.
type Action struct{ reader Reader }

// New creates a datasource list action.
func New(reader Reader) *Action { return &Action{reader: reader} }

// Execute lists one page without hidden continuation reads.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.reader == nil {
		return Output{}, &errs.Error{ID: "datasource.list.unconfigured", Kind: errs.KindRuntime, Operation: "datasource.list", Summary: "Datasource list is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure datasource listing before retrying."}
	}
	fingerprint, err := datasourceFilterFingerprint(input)
	if err != nil {
		return Output{}, fmt.Errorf("build datasource continuation cursor: %w", err)
	}
	pageNumber, pageSize, err := pageSelection(input.Cursor, input.Limit, fingerprint)
	if err != nil {
		return Output{}, err
	}
	request := PageRequest{
		PageNumber: pageNumber, PageSize: pageSize, Name: input.Name, OwnerName: input.OwnerName,
		ProjectName: input.ProjectName, Type: input.Type, Tag: input.Tag,
		UpdatedAfter: input.UpdatedAfter, UpdatedBefore: input.UpdatedBefore,
	}
	page, err := a.reader.ListDatasources(ctx, request)
	if err != nil {
		return Output{}, err
	}
	if page.Number != pageNumber || page.Size <= 0 || page.Size > pageSize || page.Total < 0 || len(page.Datasources) > page.Size || int64(page.Number-1)*int64(page.Size)+int64(len(page.Datasources)) > int64(page.Total) {
		return Output{}, errors.New("datasource list reader returned inconsistent pagination")
	}
	next := ""
	if int64(page.Number)*int64(page.Size) < int64(page.Total) {
		next, err = encodeCursor(page.Number+1, page.Size, fingerprint)
		if err != nil {
			return Output{}, err
		}
	}
	return Output{
		Status: "listed", Environment: input.Environment, Site: input.Site,
		Page:        OutputPage{Returned: len(page.Datasources), Total: page.Total, Limit: page.Size, NextCursor: next},
		Datasources: page.Datasources, RequestID: page.RequestID,
		Help: []string{"tadx content datasource get --id <datasource-luid>"},
	}, nil
}

func pageSelection(value string, requested int, expectedFilter string) (int, int, error) {
	if value == "" {
		if requested == 0 {
			requested = defaultLimit
		}
		if requested < 1 || requested > maxLimit {
			return 0, 0, errs.New(errs.KindUsage, fmt.Sprintf("datasource list limit must be between 1 and %d", maxLimit))
		}
		return 1, requested, nil
	}
	if len(value) > maxCursorLength {
		return 0, 0, errs.New(errs.KindUsage, "invalid datasource continuation cursor")
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	var cursor cursorValue
	if err != nil || json.Unmarshal(data, &cursor) != nil || cursor.Version != cursorVersion || cursor.Page < 2 || cursor.Page > maxCursorPage || cursor.Size < 1 || cursor.Size > maxLimit || cursor.Filter == "" {
		return 0, 0, errs.New(errs.KindUsage, "invalid datasource continuation cursor")
	}
	if requested != 0 && requested != cursor.Size {
		return 0, 0, errs.New(errs.KindUsage, "datasource list limit must match the continuation cursor")
	}
	if cursor.Filter != expectedFilter {
		return 0, 0, errs.New(errs.KindUsage, "datasource continuation cursor does not match the current filters")
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

func datasourceFilterFingerprint(input Input) (string, error) {
	data, err := json.Marshal(struct {
		Environment   string `json:"environment"`
		Site          string `json:"site"`
		Name          string `json:"name"`
		OwnerName     string `json:"owner_name"`
		ProjectName   string `json:"project_name"`
		Type          string `json:"type"`
		Tag           string `json:"tag"`
		UpdatedAfter  string `json:"updated_after"`
		UpdatedBefore string `json:"updated_before"`
		Catalog       bool   `json:"catalog"`
	}{input.Environment, input.Site, input.Name, input.OwnerName, input.ProjectName, input.Type, input.Tag, input.UpdatedAfter, input.UpdatedBefore, input.Catalog})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}
