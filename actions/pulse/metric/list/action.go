package list

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
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
	if err := ValidateInput(input); err != nil {
		return Output{}, err
	}
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
	if limit < 1 || limit > 10000 {
		return Output{}, fail("pulse.metric.list.usage", errs.KindUsage, input, "Pulse metric list limit must be between 1 and 10000.", nil)
	}
	token, err := decodeCursor(input.Cursor, input.Environment, input.Site, input.DefinitionLUID, limit, input.Catalog)
	if err != nil {
		return Output{}, fail("pulse.metric.list.usage", errs.KindUsage, input, "Pulse metric cursor does not match this definition, limit, and source.", err)
	}
	if input.All {
		if input.Limit != 0 || input.Cursor != "" {
			return Output{}, fail("pulse.metric.list.usage", errs.KindUsage, input, "--all cannot be combined with --limit or --cursor.", nil)
		}
		limit = 10000
	}
	pageSize := min(limit, 100)

	items := []Metric{}
	seenTokens := map[string]bool{token: true}
	seenIDs := map[string]bool{}
	var requestID, nextToken string
	for pageNumber := 0; ; pageNumber++ {
		if pageNumber >= 100 {
			return Output{}, fail("pulse.metric.list.incomplete", errs.KindOperation, input, "Pulse listing exceeded its 100-page inventory bound; completeness cannot be established.", nil)
		}
		if !input.All {
			pageSize = min(100, limit-len(items))
		}
		page, err := a.reader.ListMetrics(ctx, input.DefinitionLUID, PageRequest{PageSize: pageSize, PageToken: token})
		if err != nil {
			var structured *errs.Error
			if input.Catalog && errors.As(err, &structured) {
				return Output{}, err
			}
			retryable, corrective := errs.CompleteRetryAdvice(err, "Review the Tableau response, then retry the listing.")
			return Output{}, &errs.Error{ID: "pulse.metric.list.failed", Kind: errs.KindOperation, Operation: "pulse.metric.list", Environment: input.Environment, Site: input.Site, Summary: "Pulse metric listing failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err)}
		}
		if len(page.Metrics) > pageSize {
			return Output{}, fail("pulse.metric.list.invalid_response", errs.KindOperation, input, "Pulse listing returned more records than requested.", nil)
		}
		for _, item := range page.Metrics {
			if strings.TrimSpace(item.LUID) == "" || item.DefinitionLUID != input.DefinitionLUID || seenIDs[item.LUID] {
				return Output{}, fail("pulse.metric.list.invalid_response", errs.KindOperation, input, "Pulse listing returned an incomplete, mismatched, or duplicate identity.", nil)
			}
			seenIDs[item.LUID] = true
			items = append(items, item)
		}
		nextToken, requestID = page.NextPageToken, page.RequestID
		if nextToken != "" && (strings.TrimSpace(nextToken) == "" || seenTokens[nextToken]) {
			return Output{}, fail("pulse.metric.list.invalid_response", errs.KindOperation, input, "Pulse listing returned an invalid or repeated continuation token.", nil)
		}
		if nextToken == "" || (!input.All && (len(items) >= limit || limit <= 100)) {
			break
		}
		seenTokens[nextToken] = true
		token = nextToken
	}
	more := nextToken != "" || len(items) > limit
	if len(items) > limit {
		items = items[:limit]
	}
	next, err := encodeCursor(nextToken, input.Environment, input.Site, input.DefinitionLUID, limit, input.Catalog)
	if err != nil {
		return Output{}, err
	}
	help := []string{"No matching metrics were returned."}
	if len(items) > 0 {
		help = []string{commandhint.Environment(input.Environment, "pulse", "metric", "inspect", "--id", items[0].LUID)}
	}
	return Output{Status: "listed", Environment: input.Environment, Site: input.Site, DefinitionLUID: input.DefinitionLUID, Page: OutputPage{Returned: len(items), Limit: limit, NextCursor: next, MoreAvailable: more}, Metrics: items, RequestID: requestID, Help: help}, nil
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
