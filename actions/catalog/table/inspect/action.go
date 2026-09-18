package inspect

import (
	"context"
	"fmt"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
	"strings"
)

type Input struct{ Environment, Site, ID, MetadataID string }
type Reader interface {
	GetTable(context.Context, string) (value.MetadataTable, error)
	DiscoverTables(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataTable], error)
}
type Action struct{ reader Reader }

func New(r Reader) *Action { return &Action{reader: r} }
func ValidateInput(in Input) error {
	for _, id := range []string{in.ID, in.MetadataID} {
		if id != "" && (id != strings.TrimSpace(id) || strings.ContainsAny(id, "\x00\r\n")) {
			return usage("identities must be exact and contain no surrounding whitespace or control characters")
		}
	}
	if (strings.TrimSpace(in.ID) == "") == (strings.TrimSpace(in.MetadataID) == "") {
		return usage("select exactly one --id or --metadata-id")
	}
	return nil
}

type Output struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Item        *value.MetadataTable `json:"item,omitempty"`
	ObservedAt  string               `json:"observed_at,omitempty"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
}

func (o Output) CompactOutput() any {
	return struct {
		Status      string                  `json:"status"`
		Environment string                  `json:"environment,omitempty"`
		Site        string                  `json:"site,omitempty"`
		Item        *compactItem            `json:"item,omitempty"`
		Parent      *value.MetadataIdentity `json:"parent,omitempty"`
		Details     string                  `json:"details"`
	}{o.Status, o.Environment, o.Site, compact(o.Item), parent(o.Item), "--full"}
}
func (o Output) FullOutput() any { return o }

type compactItem struct {
	value.MetadataIdentity
	Description  *string                `json:"description"`
	ContactLUID  string                 `json:"contact_luid,omitempty"`
	Database     value.MetadataIdentity `json:"database"`
	FullName     string                 `json:"full_name,omitempty"`
	Schema       string                 `json:"schema,omitempty"`
	Tags         []string               `json:"tags,omitempty"`
	TagsObserved bool                   `json:"tags_observed"`
}

func compact(v *value.MetadataTable) *compactItem {
	if v == nil {
		return nil
	}
	return &compactItem{MetadataIdentity: v.MetadataIdentity, Description: v.Description, ContactLUID: v.ContactLUID, Database: v.Database, FullName: v.FullName, Schema: v.Schema, Tags: v.Tags, TagsObserved: v.TagsObserved}
}
func parent(v *value.MetadataTable) *value.MetadataIdentity {
	if v == nil {
		return nil
	}
	return &v.Database
}
func (a *Action) Execute(ctx context.Context, in Input) (Output, error) {
	if err := ValidateInput(in); err != nil {
		return Output{}, err
	}
	out := Output{Status: "inspected", Environment: in.Environment, Site: in.Site}
	if a == nil || a.reader == nil {
		return out, usage("catalog table inspection is not configured")
	}
	var err error
	if in.ID != "" {
		item, readErr := a.reader.GetTable(ctx, in.ID)
		err = readErr
		if err == nil {
			out.Item = &item
		}
		if err == nil && out.Item.LUID != in.ID {
			err = fmt.Errorf("returned table LUID does not match requested identity")
		}
	} else {
		var page value.MetadataPage[value.MetadataTable]
		page, err = a.reader.DiscoverTables(ctx, value.MetadataQuery{MetadataID: in.MetadataID, Limit: 2})
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
	return &errs.Error{ID: "catalog.table.inspect.usage", Kind: errs.KindUsage, Operation: "catalog.table.inspect", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Use an exact REST LUID or Metadata ID returned by discovery."}
}
func failure(in Input, cause error) error {
	retry, advice := errs.CompleteRetryAdvice(cause, "Check the exact catalog identity and permissions.")
	selector := in.ID
	if selector == "" {
		selector = in.MetadataID
	}
	return &errs.Error{ID: "catalog.table.inspect.failed", Kind: errs.KindOperation, Operation: "catalog.table.inspect", Environment: in.Environment, Site: in.Site, Selector: selector, Resource: in.ID, Summary: "Catalog table inspection failed.", Cause: cause, Retryable: retry, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause)}
}
