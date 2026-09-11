package search

import (
	"context"
	"fmt"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/paging"
	"github.com/ahillspace/tadx/internal/value"
	"reflect"
	"slices"
	"strings"
)

type Input struct {
	Environment, Site, Query, TableID string
	Types                             []string
	Limit                             int
	All                               bool
}
type Reader interface {
	DiscoverDatabases(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataDatabase], error)
	DiscoverTables(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataTable], error)
	DiscoverColumns(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataColumn], error)
}
type Action struct{ reader Reader }

func New(r Reader) *Action { return &Action{reader: r} }

type Item struct {
	value.MetadataIdentity
	Parent      value.MetadataIdentity `json:"parent"`
	Description *string                `json:"description,omitempty"`
}
type Output struct {
	Status      string      `json:"status"`
	Environment string      `json:"environment,omitempty"`
	Site        string      `json:"site,omitempty"`
	Query       string      `json:"query"`
	Page        output.Page `json:"page"`
	Items       []Item      `json:"items"`
	Complete    bool        `json:"complete"`
	Scanned     int         `json:"scanned"`
	MatchMode   string      `json:"match_mode"`
	ObservedAt  string      `json:"observed_at,omitempty"`
	RequestID   string      `json:"tableau_request_id,omitempty"`
}
type compactItem struct {
	LUID             string `json:"luid"`
	MetadataID       string `json:"metadata_id"`
	Type             string `json:"type"`
	Name             string `json:"name"`
	ParentLUID       string `json:"parent_luid"`
	ParentMetadataID string `json:"parent_metadata_id"`
}

func (o Output) CompactOutput() any {
	rows := make([]compactItem, len(o.Items))
	for i, v := range o.Items {
		rows[i] = compactItem{v.LUID, v.MetadataID, v.Type, v.Name, v.Parent.LUID, v.Parent.MetadataID}
	}
	return struct {
		Status      string        `json:"status"`
		Environment string        `json:"environment,omitempty"`
		Site        string        `json:"site,omitempty"`
		Page        output.Page   `json:"page"`
		Items       []compactItem `json:"items"`
		Complete    bool          `json:"complete"`
		Scanned     int           `json:"scanned"`
		MatchMode   string        `json:"match_mode"`
		Details     string        `json:"details"`
	}{o.Status, o.Environment, o.Site, o.Page, rows, o.Complete, o.Scanned, o.MatchMode, "--full"}
}
func (o Output) FullOutput() any { return o }
func ValidateInput(in Input) error {
	if in.TableID != strings.TrimSpace(in.TableID) || strings.ContainsAny(in.TableID, "\x00\r\n") {
		return usage("table identity must be exact and contain no whitespace or control characters")
	}
	if strings.TrimSpace(in.Query) == "" || len(in.Query) > 4096 {
		return usage("provide a nonempty bounded catalog search query")
	}
	if in.Limit < 0 || in.Limit > 10000 || in.All && in.Limit != 0 {
		return usage("use --limit between 1 and 10000, or --all")
	}
	seen := map[string]bool{}
	for _, kind := range in.Types {
		if !slices.Contains([]string{"database", "table", "column"}, kind) || seen[kind] {
			return usage("types must be unique database, table, or column values")
		}
		seen[kind] = true
		if kind == "column" && strings.TrimSpace(in.TableID) == "" {
			return usage("column search requires --table-id to bound the local scan")
		}
	}
	return nil
}
func (a *Action) Execute(ctx context.Context, in Input) (out Output, err error) {
	defer func() {
		if err != nil && out.Status != "" {
			out.Status, out.Complete, out.Page.MoreAvailable = "partial", false, true
			out.Page.Returned = len(out.Items)
		}
	}()
	if err := ValidateInput(in); err != nil {
		return Output{}, err
	}
	if a == nil || a.reader == nil {
		return Output{}, usage("catalog search is not configured")
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
	out = Output{Status: "found", Environment: in.Environment, Site: in.Site, Query: in.Query, Page: output.Page{Limit: limit}, Items: []Item{}, Complete: true, MatchMode: "metadata_text"}
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
		identities := map[string]Item{}
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
			page := value.MetadataPage[Item]{}
			var err error
			switch kind {
			case "database":
				var p value.MetadataPage[value.MetadataDatabase]
				p, err = a.reader.DiscoverDatabases(ctx, query)
				page = mapPage(p, func(v value.MetadataDatabase) Item {
					return Item{MetadataIdentity: v.MetadataIdentity, Description: v.Description}
				})
			case "table":
				var p value.MetadataPage[value.MetadataTable]
				p, err = a.reader.DiscoverTables(ctx, query)
				page = mapPage(p, func(v value.MetadataTable) Item {
					return Item{MetadataIdentity: v.MetadataIdentity, Parent: v.Database, Description: v.Description}
				})
			case "column":
				query.Text = ""
				query.ParentLUID = in.TableID
				var p value.MetadataPage[value.MetadataColumn]
				p, err = a.reader.DiscoverColumns(ctx, query)
				page = mapPage(p, func(v value.MetadataColumn) Item {
					return Item{MetadataIdentity: v.MetadataIdentity, Parent: v.Table, Description: v.Description}
				})
			}
			if err != nil {
				out.Complete = false
				out.Status = "partial"
				out.Page.MoreAvailable = true
				return out, failure(in, err)
			}
			if len(page.Items) > size {
				return out, failure(in, fmt.Errorf("provider exceeded bounded page size"))
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
					return out, failure(in, fmt.Errorf("catalog search returned incomplete identity"))
				}
				if prior, ok := identities[key]; ok {
					if !reflect.DeepEqual(prior, item) {
						return out, failure(in, fmt.Errorf("conflicting duplicate search identity"))
					}
					continue
				}
				identities[key] = item
				if kind == "column" && !matches(item, in.Query) {
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
				return out, failure(in, e)
			}
			if page.NextCursor == "" {
				out.Complete = out.Complete && page.Complete
				break
			}
			if seen[page.NextCursor] {
				return out, failure(in, fmt.Errorf("repeated metadata search cursor"))
			}
			seen[page.NextCursor] = true
			cursor = page.NextCursor
			if len(out.Items) >= limit || scanned >= scanLimit {
				out.Complete = false
				break
			}
			if pageNumber == 999 {
				return out, failure(in, fmt.Errorf("catalog search traversal exceeded page bound"))
			}
		}
	}
	out.Page.Total = len(out.Items)
	out.Page.MoreAvailable = !out.Complete
	if !out.Complete {
		out.Status = "partial"
	}
	return out, nil
}
func mapPage[T any](p value.MetadataPage[T], f func(T) Item) value.MetadataPage[Item] {
	out := value.MetadataPage[Item]{NextCursor: p.NextCursor, Complete: p.Complete, Total: p.Total, ObservedAt: p.ObservedAt, TableauRequestID: p.TableauRequestID}
	for _, v := range p.Items {
		out.Items = append(out.Items, f(v))
	}
	return out
}
func matches(v Item, q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(strings.ToLower(v.Name), q) || (v.Description != nil && strings.Contains(strings.ToLower(*v.Description), q))
}
func usage(s string) error {
	return &errs.Error{ID: "catalog.search.usage", Kind: errs.KindUsage, Operation: "catalog.search", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Correct the catalog search query and scope."}
}
func failure(in Input, cause error) error {
	retry, advice := errs.CompleteRetryAdvice(cause, "Review catalog search scope and permissions.")
	return &errs.Error{ID: "catalog.search.failed", Kind: errs.KindOperation, Operation: "catalog.search", Environment: in.Environment, Site: in.Site, Summary: "Catalog search failed.", Cause: cause, Retryable: retry, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause)}
}
