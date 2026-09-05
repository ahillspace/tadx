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
	cursorVersion   = 1
	maxCursorLength = 2048
)

// Reader is the action-owned project listing seam.
type Reader interface {
	ListProjects(context.Context, PageRequest) (Page, error)
}

// Action lists one bounded project page.
type Action struct{ reader Reader }

// New creates a project list action.
func New(reader Reader) *Action { return &Action{reader: reader} }

// Execute lists one page without hidden continuation reads.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.reader == nil {
		return Output{}, errors.New("project list reader is not configured")
	}
	filterFingerprint, err := projectFilterFingerprint(input)
	if err != nil {
		return Output{}, fmt.Errorf("build project continuation cursor: %w", err)
	}
	pageNumber, pageSize, snapshotCursor, err := pageSelection(input.Cursor, input.Limit, filterFingerprint)
	if err != nil {
		return Output{}, err
	}
	request := PageRequest{PageNumber: pageNumber, PageSize: pageSize, Name: input.Name, ParentLUID: input.ParentLUID, OwnerName: input.OwnerName, TopLevel: input.TopLevel, SnapshotCursor: snapshotCursor}
	page, err := a.reader.ListProjects(ctx, request)
	if err != nil {
		return Output{}, err
	}
	if page.Number != pageNumber || page.Size <= 0 || page.Total < 0 || len(page.Projects) > page.Size {
		return Output{}, errors.New("project list reader returned inconsistent pagination")
	}
	next := ""
	if !page.SuppressContinuation && (page.SnapshotCursor != "" || page.Number*page.Size < page.Total) {
		next, err = encodeCursor(page.Number+1, page.Size, filterFingerprint, page.SnapshotCursor)
		if err != nil {
			return Output{}, err
		}
	}
	return Output{
		Status: "listed", Environment: input.Environment, Site: input.Site, Projects: page.Projects,
		Page:      OutputPage{Returned: len(page.Projects), Total: page.Total, Limit: page.Size, NextCursor: next},
		RequestID: page.RequestID,
		Help:      []string{"tadx content project inspect --project-id <project-luid>"},
	}, nil
}

func pageSelection(value string, requested int, expectedFilter string) (int, int, string, error) {
	if value == "" {
		if requested == 0 {
			requested = defaultLimit
		}
		if requested < 1 || requested > maxLimit {
			return 0, 0, "", fmt.Errorf("project list limit must be between 1 and %d", maxLimit)
		}
		return 1, requested, "", nil
	}
	if len(value) > maxCursorLength {
		return 0, 0, "", errs.New(errs.KindUsage, "invalid project continuation cursor")
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return 0, 0, "", errs.New(errs.KindUsage, "invalid project continuation cursor")
	}
	var cursor cursorValue
	if json.Unmarshal(data, &cursor) != nil || cursor.Version != cursorVersion || cursor.Page < 2 || cursor.Size < 1 || cursor.Size > maxLimit || cursor.Filter == "" || len(cursor.Snapshot) > 1024 {
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

func encodeCursor(page, size int, filter, snapshot string) (string, error) {
	data, err := json.Marshal(cursorValue{Version: cursorVersion, Page: page, Size: size, Filter: filter, Snapshot: snapshot})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

type cursorValue struct {
	Version  int    `json:"v"`
	Page     int    `json:"p"`
	Size     int    `json:"s"`
	Filter   string `json:"f"`
	Snapshot string `json:"c,omitempty"`
}

func projectFilterFingerprint(input Input) (string, error) {
	data, err := json.Marshal(struct {
		Environment string `json:"environment"`
		Site        string `json:"site"`
		Name        string `json:"name"`
		ParentLUID  string `json:"parent_luid"`
		OwnerName   string `json:"owner_name"`
		TopLevel    *bool  `json:"top_level"`
		Catalog     bool   `json:"catalog"`
	}{Environment: input.Environment, Site: input.Site, Name: input.Name, ParentLUID: input.ParentLUID, OwnerName: input.OwnerName, TopLevel: input.TopLevel, Catalog: input.Catalog})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}
