package list

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ahillspace/tadx/internal/commandhint"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

const (
	defaultLimit    = 25
	maxLimit        = 10000
	maxCursorLength = 4096
	cursorVersion   = 1
)

// Reader is the action-owned Pulse definition list seam.
type Reader interface {
	ListDefinitions(context.Context, PageRequest) (Page, error)
}

// Action lists Pulse definitions.
type Action struct{ reader Reader }

// New creates a Pulse definition list action.
func New(reader Reader) *Action { return &Action{reader: reader} }

// Execute reads a bounded view, scanning internally for all results or exact names.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if err := ValidateInput(input); err != nil {
		return Output{}, err
	}
	if a == nil || a.reader == nil {
		return Output{}, listError("pulse.definition.list.unconfigured", errs.KindRuntime, input, "Pulse definition listing is not configured.", nil)
	}
	limit := input.Limit
	if limit == 0 {
		limit = defaultLimit
	}
	fingerprint := targetFingerprint(input.Environment, input.Site, input.Name, limit, input.Cache)
	if input.DatasourceLUID != "" {
		fingerprint = targetFingerprint(fingerprint, input.DatasourceLUID, "", limit, input.Cache)
	}
	token, err := decodeCursor(input.Cursor, fingerprint)
	if err != nil {
		return Output{}, listError("pulse.definition.list.usage", errs.KindUsage, input, "Pulse definition cursor does not match the selected target, name, and limit.", err)
	}
	if input.All {
		limit = 10000
	}
	pageSize := min(limit, 100)
	if input.Name != "" || input.DatasourceLUID != "" {
		pageSize = 100
	}
	items := []Definition{}
	seenTokens := map[string]bool{token: true}
	seenIDs := map[string]bool{}
	var requestID, nextToken string
	for pageNumber := 0; ; pageNumber++ {
		if err := ctx.Err(); err != nil {
			return Output{}, err
		}
		if pageNumber >= 100 {
			return Output{}, listError("pulse.definition.list.incomplete", errs.KindOperation, input, "Pulse listing exceeded its 100-page inventory bound; completeness cannot be established.", nil)
		}
		if !input.All && input.Name == "" && input.DatasourceLUID == "" {
			pageSize = min(100, limit-len(items))
		}
		page, err := a.reader.ListDefinitions(ctx, PageRequest{PageSize: pageSize, PageToken: token})
		if err != nil {
			var structured *errs.Error
			if input.Cache && errors.As(err, &structured) {
				return Output{}, err
			}
			retryable, corrective := errs.CompleteRetryAdvice(err, "Review the Tableau response, then retry the listing.")
			return Output{}, &errs.Error{ID: "pulse.definition.list.failed", Kind: errs.KindOperation, Operation: "pulse.definition.list", Environment: input.Environment, Site: input.Site, Summary: "Pulse definition listing failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err)}
		}
		if len(page.Definitions) > pageSize {
			return Output{}, listError("pulse.definition.list.invalid_response", errs.KindOperation, input, "Pulse listing returned more records than requested.", nil)
		}
		for _, item := range page.Definitions {
			if strings.TrimSpace(item.LUID) == "" || strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.DatasourceLUID) == "" || seenIDs[item.LUID] {
				return Output{}, listError("pulse.definition.list.invalid_response", errs.KindOperation, input, "Pulse listing returned an incomplete, mismatched, or duplicate identity.", nil)
			}
			seenIDs[item.LUID] = true
			if (input.Name == "" || item.Name == input.Name) && (input.DatasourceLUID == "" || item.DatasourceLUID == input.DatasourceLUID) {
				items = append(items, item)
			}
		}
		nextToken, requestID = page.NextPageToken, page.RequestID
		if nextToken != "" && (strings.TrimSpace(nextToken) == "" || seenTokens[nextToken]) {
			return Output{}, listError("pulse.definition.list.invalid_response", errs.KindOperation, input, "Pulse listing returned an invalid or repeated continuation token.", nil)
		}
		if nextToken == "" || (!input.All && (len(items) >= limit || (limit <= 100 && input.Name == "" && input.DatasourceLUID == ""))) {
			break
		}
		seenTokens[nextToken] = true
		token = nextToken
	}
	more := nextToken != "" || len(items) > limit
	if len(items) > limit {
		items = items[:limit]
	}
	next, err := encodeCursor(nextToken, fingerprint)
	if err != nil {
		return Output{}, err
	}
	help := []string{"No matching definitions were returned."}
	if len(items) > 0 {
		help = []string{commandhint.Environment(input.Environment, "pulse", "definition", "inspect", "--id", items[0].LUID)}
	}
	return Output{Status: "listed", Environment: input.Environment, Site: input.Site, Page: OutputPage{Returned: len(items), Limit: limit, NextCursor: next, MoreAvailable: more}, Definitions: items, RequestID: requestID, Help: help}, nil
}

type cursorValue struct {
	Version     int    `json:"v"`
	PageToken   string `json:"t"`
	Fingerprint string `json:"f"`
}

func targetFingerprint(environment, site, name string, limit int, cache bool) string {
	sum := sha256.Sum256([]byte(environment + "\x00" + site + "\x00" + name + "\x00" + fmt.Sprint(limit) + "\x00" + fmt.Sprint(cache)))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func encodeCursor(token, fingerprint string) (string, error) {
	if token == "" {
		return "", nil
	}
	data, err := json.Marshal(cursorValue{Version: cursorVersion, PageToken: token, Fingerprint: fingerprint})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func decodeCursor(value, fingerprint string) (string, error) {
	if value == "" {
		return "", nil
	}
	if len(value) > maxCursorLength {
		return "", errors.New("cursor exceeds its byte limit")
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	var cursor cursorValue
	if err != nil || json.Unmarshal(data, &cursor) != nil || cursor.Version != cursorVersion || cursor.PageToken == "" || cursor.Fingerprint != fingerprint {
		return "", errors.New("invalid Pulse definition cursor")
	}
	return cursor.PageToken, nil
}

func listError(id string, kind errs.Kind, input Input, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "pulse.definition.list", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Review the definition list input and request a new page."}
}
