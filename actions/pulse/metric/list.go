package metric

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/readsource"
)

const listDefaultLimit = 25

type ListReader interface {
	ListMetrics(context.Context, string, ListPageRequest) (ListPage, error)
}

func List(ctx context.Context, reader ListReader, input ListInput) (ListOutput, error) {
	input.DefinitionLUID = strings.TrimSpace(input.DefinitionLUID)
	limit := input.limit
	token := input.cursor.Token
	if input.All {
		limit = 10000
	}
	pageSize := min(limit, 100)

	items := []ListMetric{}
	seenTokens := map[string]bool{token: true}
	seenIDs := map[string]bool{}
	var requestID, nextToken string
	for pageNumber := 0; ; pageNumber++ {
		if pageNumber >= 100 {
			return ListOutput{}, listFail("pulse.metric.list.incomplete", errs.KindOperation, input, "Pulse listing exceeded its 100-page inventory bound; completeness cannot be established.", nil)
		}
		if !input.All {
			pageSize = min(100, limit-len(items))
		}
		page, err := reader.ListMetrics(ctx, input.DefinitionLUID, ListPageRequest{PageSize: pageSize, PageToken: token})
		if err != nil {
			var structured *errs.Error
			if input.Cache && errors.As(err, &structured) {
				return ListOutput{}, err
			}
			retryable, corrective := errs.CompleteRetryAdvice(err, "Review the Tableau response, then retry the listing.")
			return ListOutput{}, &errs.Error{ID: "pulse.metric.list.failed", Kind: errs.KindOperation, Operation: "pulse.metric.list", Environment: input.Environment, Site: input.Site, Summary: "Pulse metric listing failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err)}
		}
		if len(page.Metrics) > pageSize {
			return ListOutput{}, listFail("pulse.metric.list.invalid_response", errs.KindOperation, input, "Pulse listing returned more records than requested.", nil)
		}
		for _, item := range page.Metrics {
			if strings.TrimSpace(item.LUID) == "" || item.DefinitionLUID != input.DefinitionLUID || seenIDs[item.LUID] {
				return ListOutput{}, listFail("pulse.metric.list.invalid_response", errs.KindOperation, input, "Pulse listing returned an incomplete, mismatched, or duplicate identity.", nil)
			}
			seenIDs[item.LUID] = true
			items = append(items, item)
		}
		nextToken, requestID = page.NextPageToken, page.RequestID
		if nextToken != "" && (strings.TrimSpace(nextToken) == "" || seenTokens[nextToken]) {
			return ListOutput{}, listFail("pulse.metric.list.invalid_response", errs.KindOperation, input, "Pulse listing returned an invalid or repeated continuation token.", nil)
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
	next, err := listEncodeCursor(nextToken, input.Environment, input.Site, input.DefinitionLUID, limit, input.Cache)
	if err != nil {
		return ListOutput{}, err
	}
	help := []string{"No matching metrics were returned."}
	if len(items) > 0 {
		help = []string{commandhint.Environment(input.Environment, "pulse", "metric", "inspect", "--id", items[0].LUID)}
	}
	return ListOutput{Status: "listed", Environment: input.Environment, Site: input.Site, DefinitionLUID: input.DefinitionLUID, Page: ListOutputPage{Returned: len(items), Limit: limit, NextCursor: next, MoreAvailable: more}, Metrics: items, RequestID: requestID, Help: help}, nil
}

type listCursor struct {
	Version     int    `json:"v"`
	Token       string `json:"t"`
	Definition  string `json:"d"`
	Environment string `json:"e"`
	Site        string `json:"s"`
	Limit       int    `json:"l"`
	Cache       bool   `json:"c"`
}

func listEncodeCursor(token, environment, site, definition string, limit int, cache bool) (string, error) {
	if token == "" {
		return "", nil
	}
	data, err := json.Marshal(listCursor{Version: 1, Token: token, Definition: definition, Environment: environment, Site: site, Limit: limit, Cache: cache})
	return base64.RawURLEncoding.EncodeToString(data), err
}
func listFail(id string, kind errs.Kind, input ListInput, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "pulse.metric.list", Resource: input.DefinitionLUID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Provide an exact definition LUID and request one bounded page."}
}

type ListInput struct {
	cursor         listCursor
	limit          int
	Environment    string
	Site           string
	DefinitionLUID string
	Cursor         string
	Limit          int
	Cache          bool
	All            bool
}
type ListPageRequest struct {
	PageSize  int
	PageToken string
}
type ListMetric struct {
	LUID           string         `json:"luid"`
	Name           string         `json:"name,omitempty"`
	DefinitionLUID string         `json:"definition_luid"`
	IsDefault      bool           `json:"is_default"`
	Specification  map[string]any `json:"specification,omitempty"`
}
type ListPage struct {
	Metrics       []ListMetric
	NextPageToken string
	RequestID     string
}
type ListOutputPage struct {
	Returned      int    `json:"returned"`
	Limit         int    `json:"limit"`
	NextCursor    string `json:"-"`
	MoreAvailable bool   `json:"more_available"`
}
type ListOutput struct {
	Status         string
	Environment    string
	Site           string
	DefinitionLUID string
	Page           ListOutputPage
	Metrics        []ListMetric
	RequestID      string
	Help           []string
	Source         *readsource.Metadata
}
type ListCompactMetric struct {
	LUID      string `json:"luid"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
}
type ListCompactResult struct {
	Status         string               `json:"status"`
	Environment    string               `json:"environment,omitempty"`
	Site           string               `json:"site,omitempty"`
	DefinitionLUID string               `json:"definition_luid"`
	Page           ListOutputPage       `json:"page"`
	Metrics        []ListCompactMetric  `json:"metrics"`
	Details        string               `json:"details"`
	Help           []string             `json:"help"`
	Source         *readsource.Metadata `json:"source,omitempty"`
}
type ListFullResult struct {
	Status         string               `json:"status"`
	Environment    string               `json:"environment,omitempty"`
	Site           string               `json:"site,omitempty"`
	DefinitionLUID string               `json:"definition_luid"`
	Page           ListOutputPage       `json:"page"`
	Metrics        []ListMetric         `json:"metrics"`
	RequestID      string               `json:"tableau_request_id,omitempty"`
	Help           []string             `json:"help"`
	Source         *readsource.Metadata `json:"source,omitempty"`
}

func (o ListOutput) CompactOutput() any {
	items := make([]ListCompactMetric, len(o.Metrics))
	for i, item := range o.Metrics {
		items[i] = ListCompactMetric{LUID: item.LUID, Name: item.Name, IsDefault: item.IsDefault}
	}
	return ListCompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, DefinitionLUID: o.DefinitionLUID, Page: o.Page, Metrics: items, Details: "--full", Help: o.Help, Source: o.Source}
}
func (o ListOutput) FullOutput() any {
	return ListFullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, DefinitionLUID: o.DefinitionLUID, Page: o.Page, Metrics: o.Metrics, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}

// ValidateInput checks bounded list inputs; resolved cursor ownership is checked later.
func ListValidateInput(input *ListInput) error {
	if strings.TrimSpace(input.DefinitionLUID) == "" {
		return listFail("pulse.metric.list.usage", errs.KindUsage, *input, "Pulse metric list requires an exact definition LUID.", nil)
	}
	if input.Limit < 0 || input.Limit > 10000 {
		return listFail("pulse.metric.list.usage", errs.KindUsage, *input, "Pulse metric list limit must be between 1 and 10000.", nil)
	}
	if input.All && (input.Limit != 0 || input.Cursor != "") {
		return listFail("pulse.metric.list.usage", errs.KindUsage, *input, "--all cannot be combined with --limit or --cursor; remove --limit and --cursor for all rows, or remove --all for a bounded result.", nil)
	}
	input.cursor = listCursor{}
	input.limit = input.Limit
	if input.limit == 0 {
		input.limit = listDefaultLimit
	}
	if input.Cursor != "" {
		data, err := base64.RawURLEncoding.DecodeString(input.Cursor)
		var value listCursor
		if len(input.Cursor) > 4096 || err != nil || json.Unmarshal(data, &value) != nil || value.Version != 1 || value.Token == "" || value.Definition != strings.TrimSpace(input.DefinitionLUID) || value.Limit < 1 || value.Limit > 10000 {
			return listFail("pulse.metric.list.usage", errs.KindUsage, *input, "Invalid Pulse metric cursor.", nil)
		}
		input.cursor = value
	}
	return nil
}

// ValidateContinuation binds a cursor to a locally resolved target before sign-in.
func ListValidateContinuation(input ListInput) error {
	cursor := input.cursor
	if input.Cursor != "" && (cursor.Definition != strings.TrimSpace(input.DefinitionLUID) || cursor.Environment != input.Environment || cursor.Site != input.Site || cursor.Limit != input.limit || cursor.Cache != input.Cache) {
		return listFail("pulse.metric.list.usage", errs.KindUsage, input, "Pulse metric cursor does not match this definition, limit, and source.", errors.New("invalid cursor"))
	}
	return nil
}
