package list

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

const (
	defaultLimit    = 25
	maxLimit        = 100
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

// Execute reads one bounded page without hidden continuation reads.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.reader == nil {
		return Output{}, listError("pulse.definition.list.unconfigured", errs.KindRuntime, input, "Pulse definition listing is not configured.", nil)
	}
	limit := input.Limit
	if limit == 0 {
		limit = defaultLimit
	}
	if limit < 1 || limit > maxLimit {
		return Output{}, listError("pulse.definition.list.usage", errs.KindUsage, input, "Pulse definition list limit must be between 1 and 100.", nil)
	}
	fingerprint := targetFingerprint(input.Environment, input.Site, limit, input.Catalog)
	token, err := decodeCursor(input.Cursor, fingerprint)
	if err != nil {
		return Output{}, listError("pulse.definition.list.usage", errs.KindUsage, input, "Pulse definition cursor does not match the selected target and limit.", err)
	}
	page, err := a.reader.ListDefinitions(ctx, PageRequest{PageSize: limit, PageToken: token})
	if err != nil {
		var structured *errs.Error
		if input.Catalog && errors.As(err, &structured) {
			return Output{}, err
		}
		retryable, corrective := errs.CompleteRetryAdvice(err, "Retry the same Pulse definition page after reviewing the Tableau response.")
		return Output{}, &errs.Error{ID: "pulse.definition.list.failed", Kind: errs.KindOperation, Operation: "pulse.definition.list", Environment: input.Environment, Site: input.Site, Summary: "Pulse definition listing failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err)}
	}
	if len(page.Definitions) > limit {
		return Output{}, listError("pulse.definition.list.invalid_response", errs.KindOperation, input, "Pulse definition listing returned more records than requested.", errors.New("provider page exceeded the requested limit"))
	}
	for _, item := range page.Definitions {
		if strings.TrimSpace(item.LUID) == "" || strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.DatasourceLUID) == "" {
			return Output{}, listError("pulse.definition.list.invalid_response", errs.KindOperation, input, "Pulse definition listing returned an incomplete identity.", errors.New("definition requires LUID, name, and datasource LUID"))
		}
	}
	next, err := encodeCursor(page.NextPageToken, fingerprint)
	if err != nil {
		return Output{}, fmt.Errorf("encode Pulse definition cursor: %w", err)
	}
	return Output{
		Status: "listed", Environment: input.Environment, Site: input.Site,
		Page:        OutputPage{Returned: len(page.Definitions), Limit: limit, NextCursor: next},
		Definitions: page.Definitions, RequestID: page.RequestID,
		Help: []string{"tadx pulse definition inspect --id <definition-luid>"},
	}, nil
}

type cursorValue struct {
	Version     int    `json:"v"`
	PageToken   string `json:"t"`
	Fingerprint string `json:"f"`
}

func targetFingerprint(environment, site string, limit int, catalog bool) string {
	sum := sha256.Sum256([]byte(environment + "\x00" + site + "\x00" + fmt.Sprint(limit) + "\x00" + fmt.Sprint(catalog)))
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
