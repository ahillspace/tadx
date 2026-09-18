package list

import (
	"context"
	"fmt"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/paging"
	"github.com/ahillspace/tadx/internal/value"
	"reflect"
	"strings"
)

type Input struct {
	Environment, Site, Name, DatabaseID string
	Limit                               int
	All                                 bool
}
type Reader interface {
	DiscoverTables(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataTable], error)
}
type Action struct{ reader Reader }

func New(reader Reader) *Action { return &Action{reader: reader} }
func ValidateInput(in Input) error {
	if id := in.DatabaseID; id != "" && (id != strings.TrimSpace(id) || strings.ContainsAny(id, "\x00\r\n")) {
		return usage("parent identity must be exact and contain no surrounding whitespace or control characters")
	}
	if in.Limit < 0 || in.Limit > 10000 {
		return usage("limit must be between 1 and 10000")
	}
	if in.All && in.Limit != 0 {
		return usage("--all cannot be combined with --limit")
	}

	return nil
}

type Output struct {
	Status      string                `json:"status"`
	Environment string                `json:"environment,omitempty"`
	Site        string                `json:"site,omitempty"`
	Page        output.Page           `json:"page"`
	Items       []value.MetadataTable `json:"items"`
	Complete    bool                  `json:"complete"`
	ObservedAt  string                `json:"observed_at,omitempty"`
	RequestID   string                `json:"tableau_request_id,omitempty"`
	NextCommand string                `json:"next_command,omitempty"`
}
type CompactItem struct {
	LUID             string `json:"luid"`
	MetadataID       string `json:"metadata_id"`
	Name             string `json:"name"`
	Type             string `json:"type"`
	ParentLUID       string `json:"parent_luid"`
	ParentMetadataID string `json:"parent_metadata_id"`
	FullName         string `json:"full_name,omitempty"`
	Schema           string `json:"schema,omitempty"`
}

func (o Output) CompactOutput() any {
	rows := make([]CompactItem, len(o.Items))
	for i, v := range o.Items {
		rows[i] = CompactItem{LUID: v.LUID, MetadataID: v.MetadataID, Name: v.Name, Type: v.Type, ParentLUID: v.Database.LUID, ParentMetadataID: v.Database.MetadataID, FullName: v.FullName, Schema: v.Schema}
	}
	return struct {
		Status      string        `json:"status"`
		Environment string        `json:"environment,omitempty"`
		Site        string        `json:"site,omitempty"`
		Page        output.Page   `json:"page"`
		Items       []CompactItem `json:"items"`
		Complete    bool          `json:"complete"`
		ObservedAt  string        `json:"observed_at,omitempty"`
		Details     string        `json:"details"`
		NextCommand string        `json:"next_command,omitempty"`
	}{o.Status, o.Environment, o.Site, o.Page, rows, o.Complete, o.ObservedAt, "--full", o.NextCommand}
}
func (o Output) FullOutput() any { return o }
func (a *Action) Execute(ctx context.Context, in Input) (out Output, err error) {
	defer func() {
		if err != nil && out.Status != "" {
			out.Status = "partial"
			out.Complete = false
			out.Page.MoreAvailable = true
			out.Page.Returned = len(out.Items)
		}
	}()
	if err := ValidateInput(in); err != nil {
		return Output{}, err
	}
	defer func() {
		if err == nil && out.Page.MoreAvailable {
			out.NextCommand = nextCommand(in)
		}
	}()
	out = Output{Status: "listed", Environment: in.Environment, Site: in.Site, Items: []value.MetadataTable{}}
	if a == nil || a.reader == nil {
		return out, usage("catalog table reader is not configured")
	}
	limit := in.Limit
	if limit == 0 {
		limit = 25
	}
	if in.All {
		limit = 10000
	}
	out.Page.Limit = limit
	cursor := ""
	seen := map[string]bool{}
	identities := map[string]value.MetadataTable{}
	var coverage paging.MetadataCoverage
	for pageNumber := 0; pageNumber < 1000; pageNumber++ {
		size := limit - len(out.Items)
		if size > 100 {
			size = 100
		}
		page, err := a.reader.DiscoverTables(ctx, value.MetadataQuery{Name: in.Name, Limit: size, Cursor: cursor, ParentLUID: in.DatabaseID})
		if err != nil {
			out.Status = "partial"
			return out, failure(in, err)
		}
		out.ObservedAt = page.ObservedAt
		out.RequestID = page.TableauRequestID
		if len(page.Items) > size || page.Total < 0 {
			return out, failure(in, fmt.Errorf("inconsistent metadata page"))
		}
		for _, item := range page.Items {
			key := item.MetadataID
			if key == "" {
				key = item.LUID
			}
			if strings.TrimSpace(key) == "" {
				return out, failure(in, fmt.Errorf("metadata item has no authoritative identity"))
			}
			if prior, ok := identities[key]; ok {
				if !reflect.DeepEqual(prior, item) {
					return out, failure(in, fmt.Errorf("conflicting duplicate metadata identity"))
				}
				continue
			}
			identities[key] = item
			out.Items = append(out.Items, item)
		}
		out.Page.Returned = len(out.Items)
		out.Page.Total = page.Total
		if e := coverage.Page(page.Total, len(identities), page.NextCursor == ""); e != nil {
			return out, failure(in, e)
		}
		out.Complete = page.Complete && page.NextCursor == ""
		out.Page.MoreAvailable = page.NextCursor != "" || !page.Complete
		if page.NextCursor == "" {
			if !out.Complete {
				out.Status = "partial"
			}
			return out, nil
		}
		if seen[page.NextCursor] {
			return out, failure(in, fmt.Errorf("repeated metadata continuation"))
		}
		seen[page.NextCursor] = true
		if len(out.Items) >= limit {
			return out, nil
		}
		cursor = page.NextCursor
	}
	out.Status = "partial"
	return out, failure(in, fmt.Errorf("metadata traversal exceeded page bound"))
}

func nextCommand(in Input) string {
	args := []string{"catalog", "table", "list", "--all"}
	if in.Name != "" {
		args = append(args, "--name", in.Name)
	}
	if in.DatabaseID != "" {
		args = append(args, "--database-id", in.DatabaseID)
	}
	return commandhint.Environment(in.Environment, args...)
}
func usage(message string) error {
	return &errs.Error{ID: "catalog.table.list.usage", Kind: errs.KindUsage, Operation: "catalog.table.list", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the list selectors or bounds."}
}
func failure(in Input, cause error) error {
	retry, advice := errs.CompleteRetryAdvice(cause, "Review the exact scope and retry the read.")
	return &errs.Error{ID: "catalog.table.list.failed", Kind: errs.KindOperation, Operation: "catalog.table.list", Environment: in.Environment, Site: in.Site, Summary: "Catalog table listing failed.", Cause: cause, Retryable: retry, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause)}
}
