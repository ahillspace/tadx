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
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Item        value.MetadataColumn `json:"item"`
	ObservedAt  string               `json:"observed_at,omitempty"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
}

func (o Output) CompactOutput() any {
	return struct {
		Status      string                 `json:"status"`
		Environment string                 `json:"environment,omitempty"`
		Site        string                 `json:"site,omitempty"`
		Item        value.MetadataIdentity `json:"item"`
		Parent      value.MetadataIdentity `json:"parent"`
		Details     string                 `json:"details"`
	}{o.Status, o.Environment, o.Site, o.Item.MetadataIdentity, o.Item.Table, "--full"}
}
func (o Output) FullOutput() any { return o }
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
		out.Item, err = a.reader.GetColumn(ctx, in.TableID, in.ID)
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
				out.Item = page.Items[0]
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
	return &errs.Error{ID: "catalog.column.inspect.failed", Kind: errs.KindOperation, Operation: "catalog.column.inspect", Environment: in.Environment, Site: in.Site, Resource: in.ID, Summary: "Catalog column inspection failed.", Cause: cause, Retryable: retry, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause)}
}
