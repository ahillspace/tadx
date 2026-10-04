package catalog

import (
	"context"
	"fmt"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/paging"
	"github.com/ahillspace/tadx/internal/value"
	"reflect"
	"slices"
	"strings"
)

type SearchInput struct {
	Environment, Site, Query, TableID string
	Types                             []string
	Limit                             int
	All                               bool
}
type SearchReader interface {
	DiscoverDatabases(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataDatabase], error)
	DiscoverTables(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataTable], error)
	DiscoverColumns(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataColumn], error)
}

type SearchItem struct {
	value.MetadataIdentity
	Parent      value.MetadataIdentity `json:"parent"`
	Description *string                `json:"description,omitempty"`
	FullName    string                 `json:"full_name,omitempty"`
	Schema      string                 `json:"schema,omitempty"`
}
type SearchOutput struct {
	Status      string       `json:"status"`
	Environment string       `json:"environment,omitempty"`
	Site        string       `json:"site,omitempty"`
	Query       string       `json:"query"`
	Page        output.Page  `json:"page"`
	Items       []SearchItem `json:"items"`
	Complete    bool         `json:"complete"`
	Scanned     int          `json:"scanned"`
	MatchMode   string       `json:"match_mode"`
	ObservedAt  string       `json:"observed_at,omitempty"`
	RequestID   string       `json:"tableau_request_id,omitempty"`
	NextCommand string       `json:"next_command,omitempty"`
}
type compactSearchItem struct {
	LUID             string `json:"luid"`
	MetadataID       string `json:"metadata_id"`
	Type             string `json:"type"`
	Name             string `json:"name"`
	ParentLUID       string `json:"parent_luid"`
	ParentMetadataID string `json:"parent_metadata_id"`
	FullName         string `json:"full_name,omitempty"`
	Schema           string `json:"schema,omitempty"`
}

func (o SearchOutput) CompactOutput() any {
	rows := make([]compactSearchItem, len(o.Items))
	for i, v := range o.Items {
		rows[i] = compactSearchItem{LUID: v.LUID, MetadataID: v.MetadataID, Type: v.Type, Name: v.Name, ParentLUID: v.Parent.LUID, ParentMetadataID: v.Parent.MetadataID, FullName: v.FullName, Schema: v.Schema}
	}
	return struct {
		Status      string              `json:"status"`
		Environment string              `json:"environment,omitempty"`
		Site        string              `json:"site,omitempty"`
		Page        output.Page         `json:"page"`
		Items       []compactSearchItem `json:"items"`
		Complete    bool                `json:"complete"`
		Scanned     int                 `json:"scanned"`
		MatchMode   string              `json:"match_mode"`
		Details     string              `json:"details"`
		NextCommand string              `json:"next_command,omitempty"`
	}{o.Status, o.Environment, o.Site, o.Page, rows, o.Complete, o.Scanned, o.MatchMode, "--full", o.NextCommand}
}
func (o SearchOutput) FullOutput() any { return o }
func ValidateSearchInput(in SearchInput) error {
	if in.TableID != strings.TrimSpace(in.TableID) || strings.ContainsAny(in.TableID, "\x00\r\n") {
		return searchUsage("table identity must be exact and contain no whitespace or control characters")
	}
	if strings.TrimSpace(in.Query) == "" || len(in.Query) > 4096 {
		return searchUsage("provide a nonempty bounded catalog search query")
	}
	if in.Limit < 0 || in.Limit > 10000 || in.All && in.Limit != 0 {
		return searchUsage("use --limit between 1 and 10000, or --all")
	}
	seen := map[string]bool{}
	for _, kind := range in.Types {
		if !slices.Contains([]string{"database", "table", "column"}, kind) || seen[kind] {
			return searchUsage("types must be unique database, table, or column values")
		}
		seen[kind] = true
		if kind == "column" && strings.TrimSpace(in.TableID) == "" {
			return searchUsage("column search requires --table-id to bound the local scan")
		}
	}
	return nil
}
func searchCatalogValidated(ctx context.Context, reader SearchReader, in SearchInput) (out SearchOutput, err error) {
	defer func() {
		if err != nil && out.Status != "" {
			out.Status, out.Complete, out.Page.MoreAvailable = "partial", false, true
			out.Page.Returned = len(out.Items)
		}
	}()
	if reader == nil {
		return SearchOutput{}, searchUsage("catalog search is not configured")
	}
	limit := in.Limit
	if limit == 0 {
		limit = 25
	}
	if in.All {
		limit = 10000
	}
	types := in.Types
	if len(types) == 0 {
		types = []string{"database", "table"}
	}
	out = SearchOutput{Status: "found", Environment: in.Environment, Site: in.Site, Query: in.Query, Page: output.Page{Limit: limit}, Items: []SearchItem{}, Complete: true, MatchMode: "metadata_text"}
	if slices.Contains(types, "column") {
		out.MatchMode = "metadata_text_and_scoped_column_substring"
	}
	for _, kind := range types {
		if len(out.Items) >= limit {
			out.Complete = false
			break
		}
		cursor := ""
		seen := map[string]bool{}
		identities := map[string]SearchItem{}
		var coverage paging.MetadataCoverage
		scanLimit := 1000
		if in.All {
			scanLimit = 10000
		}
		scanned := 0
		for pageNumber := 0; pageNumber < 1000; pageNumber++ {
			size := min(100, limit-len(out.Items))
			if kind == "column" {
				size = min(100, scanLimit-scanned)
			}
			query := value.MetadataQuery{Text: in.Query, Limit: size, Cursor: cursor}
			page := value.MetadataPage[SearchItem]{}
			var err error
			switch kind {
			case "database":
				var p value.MetadataPage[value.MetadataDatabase]
				p, err = reader.DiscoverDatabases(ctx, query)
				page = searchMapPage(p, func(v value.MetadataDatabase) SearchItem {
					return SearchItem{MetadataIdentity: v.MetadataIdentity, Description: v.Description}
				})
			case "table":
				var p value.MetadataPage[value.MetadataTable]
				p, err = reader.DiscoverTables(ctx, query)
				page = searchMapPage(p, func(v value.MetadataTable) SearchItem {
					return SearchItem{MetadataIdentity: v.MetadataIdentity, Parent: v.Database, Description: v.Description, FullName: v.FullName, Schema: v.Schema}
				})
			case "column":
				query.Text = ""
				query.ParentLUID = in.TableID
				var p value.MetadataPage[value.MetadataColumn]
				p, err = reader.DiscoverColumns(ctx, query)
				page = searchMapPage(p, func(v value.MetadataColumn) SearchItem {
					return SearchItem{MetadataIdentity: v.MetadataIdentity, Parent: v.Table, Description: v.Description}
				})
			}
			if err != nil {
				out.Complete = false
				out.Status = "partial"
				out.Page.MoreAvailable = true
				return out, searchFailure(in, err)
			}
			if len(page.Items) > size {
				return out, searchFailure(in, fmt.Errorf("provider exceeded bounded page size"))
			}
			out.ObservedAt = page.ObservedAt
			out.RequestID = page.TableauRequestID
			for _, item := range page.Items {
				out.Scanned++
				scanned++
				key := item.MetadataID
				if key == "" {
					key = item.LUID
				}
				if key == "" {
					return out, searchFailure(in, fmt.Errorf("catalog search returned incomplete identity"))
				}
				if prior, ok := identities[key]; ok {
					if !reflect.DeepEqual(prior, item) {
						return out, searchFailure(in, fmt.Errorf("conflicting duplicate search identity"))
					}
					continue
				}
				identities[key] = item
				if kind == "column" && !searchMatches(item, in.Query) {
					continue
				}
				if len(out.Items) < limit {
					out.Items = append(out.Items, item)
				} else {
					out.Complete = false
				}
			}
			out.Page.Returned = len(out.Items)
			if e := coverage.Page(page.Total, len(identities), page.NextCursor == ""); e != nil {
				return out, searchFailure(in, e)
			}
			if page.NextCursor == "" {
				out.Complete = out.Complete && page.Complete
				break
			}
			if seen[page.NextCursor] {
				return out, searchFailure(in, fmt.Errorf("repeated metadata search cursor"))
			}
			seen[page.NextCursor] = true
			cursor = page.NextCursor
			if len(out.Items) >= limit || scanned >= scanLimit {
				out.Complete = false
				break
			}
			if pageNumber == 999 {
				return out, searchFailure(in, fmt.Errorf("catalog search traversal exceeded page bound"))
			}
		}
	}
	out.Page.Total = len(out.Items)
	out.Page.MoreAvailable = !out.Complete
	if !out.Complete {
		out.Status = "partial"
		out.NextCommand = searchNextCommand(in)
	}
	return out, nil
}
func searchNextCommand(in SearchInput) string {
	args := []string{"catalog", "search", in.Query, "--all"}
	for _, kind := range in.Types {
		args = append(args, "--type", kind)
	}
	if in.TableID != "" {
		args = append(args, "--table-id", in.TableID)
	}
	return commandhint.Environment(in.Environment, args...)
}
func searchMapPage[T any](p value.MetadataPage[T], f func(T) SearchItem) value.MetadataPage[SearchItem] {
	out := value.MetadataPage[SearchItem]{NextCursor: p.NextCursor, Complete: p.Complete, Total: p.Total, ObservedAt: p.ObservedAt, TableauRequestID: p.TableauRequestID}
	for _, v := range p.Items {
		out.Items = append(out.Items, f(v))
	}
	return out
}
func searchMatches(v SearchItem, q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(strings.ToLower(v.Name), q) || (v.Description != nil && strings.Contains(strings.ToLower(*v.Description), q))
}
func searchUsage(s string) error {
	return &errs.Error{ID: "catalog.search.usage", Kind: errs.KindUsage, Operation: "catalog.search", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Correct the catalog search query and scope."}
}
func searchFailure(in SearchInput, cause error) error {
	retry, advice := errs.CompleteRetryAdvice(cause, "Review catalog search scope and permissions.")
	return &errs.Error{ID: "catalog.search.failed", Kind: errs.KindOperation, Operation: "catalog.search", Environment: in.Environment, Site: in.Site, Summary: "Catalog search failed.", Cause: cause, Retryable: retry, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause)}
}
