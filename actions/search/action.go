package search

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type Source interface {
	Search(context.Context, Input) (Result, error)
}
type Action struct{ source Source }

func New(source Source) *Action { return &Action{source: source} }

// Types expands a public type selector in a stable order.
func Types(selector string) ([]string, error) {
	switch selector {
	case "":
		return []string{"datasource", "definition", "flow", "group", "metric", "project", "user", "workbook"}, nil
	case "content":
		return []string{"datasource", "flow", "project", "workbook"}, nil
	case "admin":
		return []string{"group", "user"}, nil
	case "pulse":
		return []string{"definition", "metric"}, nil
	case "workbook", "datasource", "flow", "project", "user", "group", "definition", "metric":
		return []string{selector}, nil
	default:
		return nil, errors.New("unsupported search type")
	}
}

func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	input.Terms = strings.TrimSpace(input.Terms)
	types, err := Types(input.Type)
	if err != nil || (input.Terms == "" && input.Type == "") || input.Limit < 0 || input.Limit > 100 || (input.Environment != "" && !input.SiteResolved) {
		return Output{}, searchError(errs.KindUsage, input, "Search requires a term or type, a supported type, and a limit from 0 through 100.", err)
	}
	if a == nil || a.source == nil {
		return Output{}, searchError(errs.KindRuntime, input, "Search is not configured.", nil)
	}
	if input.Limit == 0 {
		input.Limit = 20
	}
	request := input
	request.Cursor, err = decodeCursor(input.Cursor, input)
	if err != nil {
		return Output{}, searchError(errs.KindUsage, input, "Search cursor does not match the selected source and filters.", err)
	}
	result, err := a.source.Search(ctx, request)
	if err != nil {
		var invalid interface{ InvalidCatalogCursor() bool }
		var invalidSearch interface{ InvalidSearchCursor() bool }
		var unavailable interface{ CatalogScopeUnavailable() bool }
		if (errors.As(err, &invalid) && invalid.InvalidCatalogCursor()) || (errors.As(err, &invalidSearch) && invalidSearch.InvalidSearchCursor()) {
			return Output{}, searchError(errs.KindUsage, input, "Search cursor is invalid; start a new search.", err)
		}
		if input.Catalog && errors.As(err, &unavailable) && unavailable.CatalogScopeUnavailable() {
			return Output{}, searchError(errs.KindUsage, input, "The requested type is not available in the selected catalog.", err)
		}
		return Output{}, searchError(errs.KindOperation, input, "Search failed.", err)
	}
	if len(result.Items) > input.Limit {
		return Output{}, searchError(errs.KindOperation, input, "Search source exceeded the requested result bound.", nil)
	}
	allowed := make(map[string]bool, len(types))
	for _, kind := range types {
		allowed[kind] = true
	}
	seen := make(map[string]bool, len(result.Items))
	items := append([]Item{}, result.Items...)
	for _, item := range items {
		key := item.Type + ":" + item.LUID
		if strings.TrimSpace(item.LUID) == "" || (strings.TrimSpace(item.Name) == "" && item.Type != "metric") || !allowed[item.Type] || seen[key] {
			return Output{}, searchError(errs.KindOperation, input, "Search source returned an invalid or duplicate authoritative identity.", nil)
		}
		seen[key] = true
	}
	page := result.Page
	page.Returned = len(items)
	page.Limit = input.Limit
	if page.NextCursor != "" {
		page.NextCursor = encodeCursor(page.NextCursor, input)
	}
	sourceName := result.Source
	if sourceName == "" {
		sourceName = "live"
	}
	warnings := append([]string{}, result.Warnings...)
	if input.Catalog {
		sourceName = "catalog"
		warnings = append([]string{"Catalog absence does not establish remote absence; resolve authoritative LUIDs live before mutations."}, warnings...)
	}
	return Output{Source: sourceName, Page: page, Items: items, Generation: result.Generation, Warnings: output.BoundWarnings(warnings), Help: []string{"tadx capability list --resource <type>"}}, nil
}

type cursorEnvelope struct {
	Version     int    `json:"v"`
	Fingerprint string `json:"f"`
	Cursor      string `json:"c"`
	Checksum    string `json:"s"`
}

func fingerprint(input Input) string {
	input.Cursor = ""
	data, _ := json.Marshal(input)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func checksum(version int, fingerprint, cursor string) string {
	data, _ := json.Marshal([]any{version, fingerprint, cursor})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func encodeCursor(upstream string, input Input) string {
	e := cursorEnvelope{Version: 1, Fingerprint: fingerprint(input), Cursor: upstream}
	e.Checksum = checksum(e.Version, e.Fingerprint, e.Cursor)
	data, _ := json.Marshal(e)
	return base64.RawURLEncoding.EncodeToString(data)
}
func decodeCursor(value string, input Input) (string, error) {
	if value == "" {
		return "", nil
	}
	if len(value) > 16384 {
		return "", errors.New("oversized search cursor")
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	var e cursorEnvelope
	if err = json.Unmarshal(data, &e); err != nil {
		return "", err
	}
	if e.Version != 1 || e.Cursor == "" || e.Fingerprint != fingerprint(input) || e.Checksum != checksum(e.Version, e.Fingerprint, e.Cursor) {
		return "", errors.New("invalid search cursor")
	}
	return e.Cursor, nil
}
func searchError(kind errs.Kind, input Input, summary string, cause error) error {
	return &errs.Error{ID: "search." + string(kind), Kind: kind, Operation: "search", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Review the search source and filters, then retry."}
}
