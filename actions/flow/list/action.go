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
	maxLimit        = 100
	cursorVersion   = 1
	maxCursorLength = 2048
)

// Reader is the action-owned list seam.
type Reader interface {
	ListFlows(context.Context, PageRequest) (Page, error)
}

// Action lists flows.
type Action struct{ reader Reader }

// New creates a flow list action.
func New(reader Reader) *Action { return &Action{reader: reader} }

// Execute reads one bounded page.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if input.All {
		return a.collectAll(ctx, input)
	}
	if a == nil || a.reader == nil {
		return Output{}, errors.New("flow list reader is not configured")
	}
	filterFingerprint, err := flowFilterFingerprint(input)
	if err != nil {
		return Output{}, fmt.Errorf("build flow continuation cursor: %w", err)
	}
	pageNumber, pageSize, snapshotCursor, err := selectPage(input.Cursor, input.Limit, filterFingerprint)
	if err != nil {
		return Output{}, err
	}
	page, err := a.reader.ListFlows(ctx, PageRequest{PageNumber: pageNumber, PageSize: pageSize, Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, SnapshotCursor: snapshotCursor})
	if err != nil {
		return Output{}, err
	}
	if page.Number != pageNumber || page.Size <= 0 || len(page.Flows) > page.Size {
		return Output{}, errors.New("flow list reader returned inconsistent pagination")
	}
	next := ""
	if !page.SuppressContinuation && (page.SnapshotCursor != "" || page.Number*page.Size < page.Total) {
		next, err = encodeCursor(page.Number+1, page.Size, filterFingerprint, page.SnapshotCursor)
		if err != nil {
			return Output{}, err
		}
	}
	return Output{Status: "listed", Environment: input.Environment, Site: input.Site, Page: OutputPage{Returned: len(page.Flows), Total: page.Total, Limit: page.Size, NextCursor: next, MoreAvailable: next != "" || (page.SuppressContinuation && len(page.Flows) < page.Total)}, Flows: page.Flows, RequestID: page.RequestID, Help: []string{"tadx content flow inspect --id <flow-luid>"}}, nil
}

func selectPage(value string, requested int, expectedFilter string) (int, int, string, error) {
	if value == "" {
		if requested == 0 {
			requested = 25
		}
		if requested < 1 || requested > maxLimit {
			return 0, 0, "", fmt.Errorf("flow list limit must be between 1 and %d", maxLimit)
		}
		return 1, requested, "", nil
	}
	if len(value) > maxCursorLength {
		return 0, 0, "", errs.New(errs.KindUsage, "invalid flow continuation cursor")
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	var cursor cursorValue
	if err != nil || json.Unmarshal(data, &cursor) != nil || cursor.Version != cursorVersion || cursor.Page < 2 || cursor.Size < 1 || cursor.Size > maxLimit || cursor.Filter == "" || len(cursor.Snapshot) > 1024 {
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

func encodeCursor(page, size int, filter, snapshot string) (string, error) {
	data, err := json.Marshal(cursorValue{Version: cursorVersion, Page: page, Size: size, Filter: filter, Snapshot: snapshot})
	return base64.RawURLEncoding.EncodeToString(data), err
}

type cursorValue struct {
	Version  int    `json:"v"`
	Page     int    `json:"p"`
	Size     int    `json:"s"`
	Filter   string `json:"f"`
	Snapshot string `json:"c,omitempty"`
}

func flowFilterFingerprint(input Input) (string, error) {
	data, err := json.Marshal(struct {
		Environment string `json:"environment"`
		Site        string `json:"site"`
		Name        string `json:"name"`
		OwnerName   string `json:"owner_name"`
		ProjectLUID string `json:"project_luid"`
		ProjectName string `json:"project_name"`
		Catalog     bool   `json:"catalog"`
	}{Environment: input.Environment, Site: input.Site, Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, Catalog: input.Catalog})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// collectAll follows private bounded pages and fails closed on incomplete inventories.
func (a *Action) collectAll(ctx context.Context, input Input) (Output, error) {
	if input.Limit != 0 || input.Cursor != "" {
		return Output{}, errs.New(errs.KindUsage, "--all cannot be combined with --limit or --cursor")
	}
	input.All = false
	input.Limit = 100
	var result Output
	seen := map[string]bool{}
	cursors := map[string]bool{}
	for pageNumber := 0; pageNumber < 100; pageNumber++ {
		page, err := a.Execute(ctx, input)
		if err != nil {
			return Output{}, err
		}
		if pageNumber == 0 {
			result = page
			result.Flows = nil
		} else if page.Page.Total != result.Page.Total {
			return Output{}, errors.New("flow inventory changed during pagination; retry")
		}
		for _, item := range page.Flows {
			if item.LUID == "" || seen[item.LUID] {
				return Output{}, errors.New("flow inventory returned missing or repeated identities")
			}
			seen[item.LUID] = true
			result.Flows = append(result.Flows, item)
		}
		result.RequestID = page.RequestID
		if !page.Page.MoreAvailable && page.Page.NextCursor == "" {
			if len(result.Flows) != result.Page.Total {
				return Output{}, errors.New("flow inventory completeness could not be established")
			}
			result.Page = OutputPage{Returned: len(result.Flows), Total: result.Page.Total, Limit: 10000}
			return result, nil
		}
		if len(page.Flows) == 0 || page.Page.NextCursor == "" || cursors[page.Page.NextCursor] {
			return Output{}, errors.New("flow inventory pagination did not advance; completeness could not be established")
		}
		cursors[page.Page.NextCursor] = true
		input.Cursor = page.Page.NextCursor
	}
	return Output{}, errors.New("flow --all exceeds the 10000-record bound; use narrower filters")
}
