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
	GetDatabase(context.Context, string) (value.MetadataDatabase, error)
	DiscoverDatabases(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataDatabase], error)
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
	Status      string                 `json:"status"`
	Environment string                 `json:"environment,omitempty"`
	Site        string                 `json:"site,omitempty"`
	Item        value.MetadataDatabase `json:"item"`
	ObservedAt  string                 `json:"observed_at,omitempty"`
	RequestID   string                 `json:"tableau_request_id,omitempty"`
}

func (o Output) CompactOutput() any {
	return struct {
		Status      string                 `json:"status"`
		Environment string                 `json:"environment,omitempty"`
		Site        string                 `json:"site,omitempty"`
		Item        value.MetadataIdentity `json:"item"`
		Details     string                 `json:"details"`
	}{o.Status, o.Environment, o.Site, o.Item.MetadataIdentity, "--full"}
}
func (o Output) FullOutput() any { return o }
func (a *Action) Execute(ctx context.Context, in Input) (Output, error) {
	if err := ValidateInput(in); err != nil {
		return Output{}, err
	}
	out := Output{Status: "inspected", Environment: in.Environment, Site: in.Site}
	if a == nil || a.reader == nil {
		return out, usage("catalog database inspection is not configured")
	}
	var err error
	if in.ID != "" {
		out.Item, err = a.reader.GetDatabase(ctx, in.ID)
		if err == nil && out.Item.LUID != in.ID {
			err = fmt.Errorf("returned database LUID does not match requested identity")
		}
	} else {
		var page value.MetadataPage[value.MetadataDatabase]
		page, err = a.reader.DiscoverDatabases(ctx, value.MetadataQuery{MetadataID: in.MetadataID, Limit: 2})
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
	return &errs.Error{ID: "catalog.database.inspect.usage", Kind: errs.KindUsage, Operation: "catalog.database.inspect", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Use an exact REST LUID or Metadata ID returned by discovery."}
}
func failure(in Input, cause error) error {
	retry, advice := errs.CompleteRetryAdvice(cause, "Check the exact catalog identity and permissions.")
	return &errs.Error{ID: "catalog.database.inspect.failed", Kind: errs.KindOperation, Operation: "catalog.database.inspect", Environment: in.Environment, Site: in.Site, Resource: in.ID, Summary: "Catalog database inspection failed.", Cause: cause, Retryable: retry, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause)}
}
