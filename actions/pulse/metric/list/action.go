package list

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

const defaultLimit = 25

type Reader interface {
	ListMetrics(context.Context, string, PageRequest) (Page, error)
}
type Action struct{ reader Reader }

func New(reader Reader) *Action { return &Action{reader: reader} }
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.reader == nil {
		return Output{}, fail("pulse.metric.list.unconfigured", errs.KindRuntime, input, "Pulse metric listing is not configured.", nil)
	}
	input.DefinitionLUID = strings.TrimSpace(input.DefinitionLUID)
	if input.DefinitionLUID == "" {
		return Output{}, fail("pulse.metric.list.usage", errs.KindUsage, input, "Pulse metric list requires an exact definition LUID.", nil)
	}
	limit := input.Limit
	if limit == 0 {
		limit = defaultLimit
	}
	if limit < 1 || limit > 100 {
		return Output{}, fail("pulse.metric.list.usage", errs.KindUsage, input, "Pulse metric list limit must be between 1 and 100.", nil)
	}
	token, err := decodeCursor(input.Cursor, input.Environment, input.Site, input.DefinitionLUID, limit, input.Catalog)
	if err != nil {
		return Output{}, fail("pulse.metric.list.usage", errs.KindUsage, input, "Pulse metric cursor does not match this definition, limit, and source.", err)
	}
	page, err := a.reader.ListMetrics(ctx, input.DefinitionLUID, PageRequest{PageSize: limit, PageToken: token})
	if err != nil {
		var structured *errs.Error
		if input.Catalog && errors.As(err, &structured) {
			return Output{}, err
		}
		retryable, corrective := errs.CompleteRetryAdvice(err, "Review the exact definition LUID, then retry.")
		return Output{}, &errs.Error{ID: "pulse.metric.list.failed", Kind: errs.KindOperation, Operation: "pulse.metric.list", Resource: input.DefinitionLUID, Environment: input.Environment, Site: input.Site, Summary: "Pulse metric listing failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err)}
	}
	if len(page.Metrics) > limit {
		return Output{}, fail("pulse.metric.list.invalid_response", errs.KindOperation, input, "Tableau returned more Pulse metrics than requested.", errors.New("provider page exceeded limit"))
	}
	for _, item := range page.Metrics {
		if item.LUID == "" || item.DefinitionLUID != input.DefinitionLUID {
			return Output{}, fail("pulse.metric.list.invalid_response", errs.KindOperation, input, "Tableau returned an incomplete or mismatched Pulse metric.", errors.New("metric identity or definition ownership mismatch"))
		}
	}
	next, err := encodeCursor(page.NextPageToken, input.Environment, input.Site, input.DefinitionLUID, limit, input.Catalog)
	if err != nil {
		return Output{}, err
	}
	return Output{Status: "listed", Environment: input.Environment, Site: input.Site, DefinitionLUID: input.DefinitionLUID, Page: OutputPage{Returned: len(page.Metrics), Limit: limit, NextCursor: next}, Metrics: page.Metrics, RequestID: page.RequestID, Help: []string{"tadx pulse metric inspect --id <metric-luid>"}}, nil
}

type cursor struct {
	Version     int    `json:"v"`
	Token       string `json:"t"`
	Definition  string `json:"d"`
	Environment string `json:"e"`
	Site        string `json:"s"`
	Limit       int    `json:"l"`
	Catalog     bool   `json:"c"`
}

func encodeCursor(token, environment, site, definition string, limit int, catalog bool) (string, error) {
	if token == "" {
		return "", nil
	}
	data, err := json.Marshal(cursor{Version: 1, Token: token, Definition: definition, Environment: environment, Site: site, Limit: limit, Catalog: catalog})
	return base64.RawURLEncoding.EncodeToString(data), err
}
func decodeCursor(value, environment, site, definition string, limit int, catalog bool) (string, error) {
	if value == "" {
		return "", nil
	}
	if len(value) > 4096 {
		return "", errors.New("cursor too long")
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	var c cursor
	if err != nil || json.Unmarshal(data, &c) != nil || c.Version != 1 || c.Token == "" || c.Definition != definition || c.Environment != environment || c.Site != site || c.Limit != limit || c.Catalog != catalog {
		return "", errors.New("invalid cursor")
	}
	return c.Token, nil
}
func fail(id string, kind errs.Kind, input Input, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "pulse.metric.list", Resource: input.DefinitionLUID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Provide an exact definition LUID and request one bounded page."}
}
