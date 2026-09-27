package definition

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/readsource"
)

const (
	listDefaultLimit    = 25
	listMaxLimit        = 10000
	listMaxCursorLength = 4096
	listCursorVersion   = 1
)

// Reader is the action-owned Pulse definition list seam.
type ListReader interface {
	ListDefinitions(context.Context, ListPageRequest) (ListPage, error)
}

// List consumes validated input and reads a bounded view, scanning for exact names.
func List(ctx context.Context, reader ListReader, input ListInput) (ListOutput, error) {
	limit := input.limit
	fingerprint := listInputFingerprint(input)
	token := input.cursor.PageToken
	if input.All {
		limit = 10000
	}
	pageSize := min(limit, 100)
	if input.Name != "" || input.DatasourceLUID != "" {
		pageSize = 100
	}
	items := []ListDefinition{}
	seenTokens := map[string]bool{token: true}
	seenIDs := map[string]bool{}
	var requestID, nextToken string
	for pageNumber := 0; ; pageNumber++ {
		if err := ctx.Err(); err != nil {
			return ListOutput{}, err
		}
		if pageNumber >= 100 {
			return ListOutput{}, listError("pulse.definition.list.incomplete", errs.KindOperation, input, "Pulse listing exceeded its 100-page inventory bound; completeness cannot be established.", nil)
		}
		if !input.All && input.Name == "" && input.DatasourceLUID == "" {
			pageSize = min(100, limit-len(items))
		}
		page, err := reader.ListDefinitions(ctx, ListPageRequest{PageSize: pageSize, PageToken: token})
		if err != nil {
			var structured *errs.Error
			if input.Cache && errors.As(err, &structured) {
				return ListOutput{}, err
			}
			retryable, corrective := errs.CompleteRetryAdvice(err, "Review the Tableau response, then retry the listing.")
			return ListOutput{}, &errs.Error{ID: "pulse.definition.list.failed", Kind: errs.KindOperation, Operation: "pulse.definition.list", Environment: input.Environment, Site: input.Site, Summary: "Pulse definition listing failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err)}
		}
		if len(page.Definitions) > pageSize {
			return ListOutput{}, listError("pulse.definition.list.invalid_response", errs.KindOperation, input, "Pulse listing returned more records than requested.", nil)
		}
		for _, item := range page.Definitions {
			if strings.TrimSpace(item.LUID) == "" || strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.DatasourceLUID) == "" || seenIDs[item.LUID] {
				return ListOutput{}, listError("pulse.definition.list.invalid_response", errs.KindOperation, input, "Pulse listing returned an incomplete, mismatched, or duplicate identity.", nil)
			}
			seenIDs[item.LUID] = true
			if (input.Name == "" || item.Name == input.Name) && (input.DatasourceLUID == "" || item.DatasourceLUID == input.DatasourceLUID) {
				items = append(items, item)
			}
		}
		nextToken, requestID = page.NextPageToken, page.RequestID
		if nextToken != "" && (strings.TrimSpace(nextToken) == "" || seenTokens[nextToken]) {
			return ListOutput{}, listError("pulse.definition.list.invalid_response", errs.KindOperation, input, "Pulse listing returned an invalid or repeated continuation token.", nil)
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
	next, err := listEncodeCursor(nextToken, fingerprint)
	if err != nil {
		return ListOutput{}, err
	}
	help := []string{"No matching definitions were returned."}
	if len(items) > 0 {
		help = []string{commandhint.Environment(input.Environment, "pulse", "definition", "inspect", "--id", items[0].LUID)}
	}
	return ListOutput{Status: "listed", Environment: input.Environment, Site: input.Site, Page: ListOutputPage{Returned: len(items), Limit: limit, NextCursor: next, MoreAvailable: more}, Definitions: items, RequestID: requestID, Help: help}, nil
}

type listCursorValue struct {
	Version     int    `json:"v"`
	PageToken   string `json:"t"`
	Fingerprint string `json:"f"`
}

func listTargetFingerprint(environment, site, name string, limit int, cache bool) string {
	sum := sha256.Sum256([]byte(environment + "\x00" + site + "\x00" + name + "\x00" + fmt.Sprint(limit) + "\x00" + fmt.Sprint(cache)))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func listEncodeCursor(token, fingerprint string) (string, error) {
	if token == "" {
		return "", nil
	}
	data, err := json.Marshal(listCursorValue{Version: listCursorVersion, PageToken: token, Fingerprint: fingerprint})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func listError(id string, kind errs.Kind, input ListInput, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "pulse.definition.list", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Review the definition list input and request a new page."}
}

// Input selects one bounded Pulse definition page.
type ListInput struct {
	cursor         listCursorValue
	limit          int
	Environment    string
	Site           string
	Name           string
	DatasourceLUID string
	Cursor         string
	Limit          int
	Cache          bool
	All            bool
}

// PageRequest is the action-owned upstream continuation request.
type ListPageRequest struct {
	PageSize  int
	PageToken string
}

// Definition is one complete bounded Pulse definition summary.
type ListDefinition struct {
	LUID              string   `json:"luid"`
	Name              string   `json:"name"`
	Description       string   `json:"description,omitempty"`
	DatasourceLUID    string   `json:"datasource_luid"`
	MeasureField      string   `json:"measure_field,omitempty"`
	Aggregation       string   `json:"aggregation,omitempty"`
	TimeDimension     string   `json:"time_dimension,omitempty"`
	AllowedDimensions []string `json:"allowed_dimensions,omitempty"`
}

// Page is one provider page.
type ListPage struct {
	Definitions   []ListDefinition
	NextPageToken string
	RequestID     string
}

// OutputPage contains stable continuation metadata.
type ListOutputPage struct {
	Returned      int    `json:"returned"`
	Limit         int    `json:"limit"`
	NextCursor    string `json:"-"`
	MoreAvailable bool   `json:"more_available"`
}

// Output retains complete details before projection.
type ListOutput struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Page        ListOutputPage       `json:"page"`
	Definitions []ListDefinition     `json:"definitions"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// CompactDefinition identifies one definition and its datasource.
type ListCompactDefinition struct {
	LUID           string `json:"luid"`
	Name           string `json:"name"`
	DatasourceLUID string `json:"datasource_luid"`
}

// CompactResult is the default bounded projection.
type ListCompactResult struct {
	Status      string                  `json:"status"`
	Environment string                  `json:"environment,omitempty"`
	Site        string                  `json:"site,omitempty"`
	Page        ListOutputPage          `json:"page"`
	Definitions []ListCompactDefinition `json:"definitions"`
	Details     string                  `json:"details"`
	Help        []string                `json:"help"`
	Source      *readsource.Metadata    `json:"source,omitempty"`
}

// CompactOutput returns stable definition identities.
func (o ListOutput) CompactOutput() any {
	items := make([]ListCompactDefinition, len(o.Definitions))
	for index, item := range o.Definitions {
		items[index] = ListCompactDefinition{LUID: item.LUID, Name: item.Name, DatasourceLUID: item.DatasourceLUID}
	}
	return ListCompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Definitions: items, Details: "--full", Help: o.Help, Source: o.Source}
}

// FullOutput returns all fields from the same bounded provider page.
func (o ListOutput) FullOutput() any {
	items := slices.Clone(o.Definitions)
	for index := range items {
		items[index].AllowedDimensions = append([]string(nil), items[index].AllowedDimensions...)
	}
	o.Definitions = items
	return o
}

// ValidateInput checks bounded list inputs; resolved cursor ownership is checked later.
func ListValidateInput(input *ListInput) error {
	if input.Limit < 0 || input.Limit > listMaxLimit {
		return listError("pulse.definition.list.usage", errs.KindUsage, *input, "Pulse definition list limit must be between 1 and 10000.", nil)
	}
	if input.All && (input.Limit != 0 || input.Cursor != "") {
		return listError("pulse.definition.list.usage", errs.KindUsage, *input, "--all cannot be combined with --limit or --cursor; remove --limit and --cursor for all rows, or remove --all for a bounded result.", nil)
	}
	input.cursor = listCursorValue{}
	input.limit = input.Limit
	if input.limit == 0 {
		input.limit = listDefaultLimit
	}
	if input.Cursor != "" {
		data, err := base64.RawURLEncoding.DecodeString(input.Cursor)
		var value listCursorValue
		if len(input.Cursor) > listMaxCursorLength || err != nil || json.Unmarshal(data, &value) != nil || value.Version != listCursorVersion || value.PageToken == "" || value.Fingerprint == "" {
			return listError("pulse.definition.list.usage", errs.KindUsage, *input, "Invalid Pulse definition cursor.", nil)
		}
		input.cursor = value
	}
	return nil
}

// ValidateContinuation binds a cursor to a locally resolved target before sign-in.
func ListValidateContinuation(input ListInput) error {
	if input.Cursor != "" && input.cursor.Fingerprint != listInputFingerprint(input) {
		return listError("pulse.definition.list.usage", errs.KindUsage, input, "Pulse definition cursor does not match the selected target, name, and limit.", errors.New("invalid Pulse definition cursor"))
	}
	return nil
}

func listInputFingerprint(input ListInput) string {
	fingerprint := listTargetFingerprint(input.Environment, input.Site, input.Name, input.limit, input.Cache)
	if input.DatasourceLUID != "" {
		fingerprint = listTargetFingerprint(fingerprint, input.DatasourceLUID, "", input.limit, input.Cache)
	}
	return fingerprint
}
