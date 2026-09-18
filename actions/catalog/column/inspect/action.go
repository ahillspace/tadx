package inspect

import (
	"context"
	"fmt"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
	"strings"
)

type Input struct{ Environment, Site, ID, MetadataID, TableID string }
type Reader interface {
	GetColumn(context.Context, string, string) (value.MetadataColumn, error)
	DiscoverColumns(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataColumn], error)
}
type Action struct{ reader Reader }

func New(r Reader) *Action { return &Action{reader: r} }
func ValidateInput(in Input) error {
	for _, id := range []string{in.ID, in.MetadataID, in.TableID} {
		if id != "" && (id != strings.TrimSpace(id) || strings.ContainsAny(id, "\x00\r\n")) {
			return usage("identities must be exact and contain no surrounding whitespace or control characters")
		}
	}
	if (strings.TrimSpace(in.ID) == "") == (strings.TrimSpace(in.MetadataID) == "") {
		return usage("select exactly one --id or --metadata-id")
	}
	if in.ID != "" && strings.TrimSpace(in.TableID) == "" {
		return usage("--id requires --table-id for a column")
	}
	return nil
}

type Output struct {
	Status      string                `json:"status"`
	Environment string                `json:"environment,omitempty"`
	Site        string                `json:"site,omitempty"`
	Item        *value.MetadataColumn `json:"item,omitempty"`
	ObservedAt  string                `json:"observed_at,omitempty"`
	RequestID   string                `json:"tableau_request_id,omitempty"`
}

func (o Output) CompactOutput() any {
	return struct {
		Status      string       `json:"status"`
		Environment string       `json:"environment,omitempty"`
		Site        string       `json:"site,omitempty"`
		Item        *compactItem `json:"item,omitempty"`
		Details     string       `json:"details"`
	}{o.Status, o.Environment, o.Site, compact(o.Item), "--full"}
}
func (o Output) FullOutput() any { return o }

type compactItem struct {
	value.MetadataIdentity
	Description  *string                `json:"description"`
	Table        value.MetadataIdentity `json:"table"`
	RemoteType   string                 `json:"remote_type,omitempty"`
	Nullable     *bool                  `json:"nullable,omitempty"`
	Tags         []string               `json:"tags,omitempty"`
	TagsObserved bool                   `json:"tags_observed"`
}

func compact(v *value.MetadataColumn) *compactItem {
	if v == nil {
		return nil
	}
	return &compactItem{MetadataIdentity: v.MetadataIdentity, Description: v.Description, Table: v.Table, RemoteType: v.RemoteType, Nullable: v.Nullable, Tags: v.Tags, TagsObserved: v.TagsObserved}
}
func (a *Action) Execute(ctx context.Context, in Input) (Output, error) {
	if err := ValidateInput(in); err != nil {
		return Output{}, err
	}
	out := Output{Status: "inspected", Environment: in.Environment, Site: in.Site}
	if a == nil || a.reader == nil {
		return out, usage("catalog column inspection is not configured")
	}
	var err error
	if in.ID != "" {
		item, readErr := a.reader.GetColumn(ctx, in.TableID, in.ID)
		err = readErr
		if err == nil {
			out.Item = &item
		}
		if err == nil && out.Item.LUID != in.ID {
			err = fmt.Errorf("returned column LUID does not match requested identity")
		}
		if err == nil && out.Item.Table.LUID != in.TableID {
			err = fmt.Errorf("returned column parent does not match requested table")
		}
	} else {
		var page value.MetadataPage[value.MetadataColumn]
		page, err = a.reader.DiscoverColumns(ctx, value.MetadataQuery{MetadataID: in.MetadataID, Limit: 2})
		out.ObservedAt = page.ObservedAt
		out.RequestID = page.TableauRequestID
		if err == nil {
			if len(page.Items) != 1 || !page.Complete || page.NextCursor != "" || page.Items[0].MetadataID != in.MetadataID {
				err = fmt.Errorf("metadata selector did not resolve exactly one complete identity")
			} else {
				out.Item = &page.Items[0]
			}
		}
	}
	if err != nil {
		return out, failure(in, err)
	}
	return out, nil
}
func usage(s string) error {
	return &errs.Error{ID: "catalog.column.inspect.usage", Kind: errs.KindUsage, Operation: "catalog.column.inspect", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Use an exact REST LUID or Metadata ID returned by discovery."}
}
func failure(in Input, cause error) error {
	retry, advice := errs.CompleteRetryAdvice(cause, "Check the exact catalog identity and permissions.")
	selector := in.ID
	if selector == "" {
		selector = in.MetadataID
	}
	return &errs.Error{ID: "catalog.column.inspect.failed", Kind: errs.KindOperation, Operation: "catalog.column.inspect", Environment: in.Environment, Site: in.Site, Selector: selector, Resource: in.ID, Summary: "Catalog column inspection failed.", Cause: cause, Retryable: retry, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause)}
}
