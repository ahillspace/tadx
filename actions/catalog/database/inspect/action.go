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
	Status      string                  `json:"status"`
	Environment string                  `json:"environment,omitempty"`
	Site        string                  `json:"site,omitempty"`
	Item        *value.MetadataDatabase `json:"item,omitempty"`
	ObservedAt  string                  `json:"observed_at,omitempty"`
	RequestID   string                  `json:"tableau_request_id,omitempty"`
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
	Description    *string  `json:"description"`
	ContactLUID    string   `json:"contact_luid,omitempty"`
	ConnectionType string   `json:"connection_type,omitempty"`
	FilePath       string   `json:"file_path,omitempty"`
	Embedded       bool     `json:"embedded"`
	Tags           []string `json:"tags,omitempty"`
	TagsObserved   bool     `json:"tags_observed"`
}

func compact(v *value.MetadataDatabase) *compactItem {
	if v == nil {
		return nil
	}
	return &compactItem{MetadataIdentity: v.MetadataIdentity, Description: v.Description, ContactLUID: v.ContactLUID, ConnectionType: v.ConnectionType, FilePath: v.FilePath, Embedded: v.Embedded, Tags: v.Tags, TagsObserved: v.TagsObserved}
}
func (a *Action) Execute(ctx context.Context, in Input) (Output, error) {
	if err := ValidateInput(in); err != nil {
		return Output{}, err
	}
	out := Output{Status: "failed", Environment: in.Environment, Site: in.Site}
	if a == nil || a.reader == nil {
		return out, usage("catalog database inspection is not configured")
	}
	var err error
	if in.ID != "" {
		item, readErr := a.reader.GetDatabase(ctx, in.ID)
		err = readErr
		if err == nil && item.LUID != in.ID {
			err = fmt.Errorf("returned database LUID does not match requested identity")
		} else if item.LUID == in.ID {
			out.Item = &item
		}
	} else {
		var page value.MetadataPage[value.MetadataDatabase]
		page, err = a.reader.DiscoverDatabases(ctx, value.MetadataQuery{MetadataID: in.MetadataID, Limit: 2})
		out.ObservedAt = page.ObservedAt
		out.RequestID = page.TableauRequestID
		if len(page.Items) == 1 && page.Items[0].MetadataID == in.MetadataID {
			out.Item = &page.Items[0]
		}
		if err == nil {
			if out.Item == nil || !page.Complete || page.NextCursor != "" {
				err = fmt.Errorf("metadata selector did not resolve exactly one complete identity")
			}
		}
	}
	if err != nil {
		if out.Item != nil {
			out.Status = "partial"
		}
		return out, failure(in, err)
	}
	out.Status = "inspected"
	return out, nil
}
func usage(s string) error {
	return &errs.Error{ID: "catalog.database.inspect.usage", Kind: errs.KindUsage, Operation: "catalog.database.inspect", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Use an exact REST LUID or Metadata ID returned by discovery."}
}
func failure(in Input, cause error) error {
	retry, advice := errs.CompleteRetryAdvice(cause, "Check the exact catalog identity and permissions.")
	selector := in.ID
	if selector == "" {
		selector = in.MetadataID
	}
	return &errs.Error{ID: "catalog.database.inspect.failed", Kind: errs.KindOperation, Operation: "catalog.database.inspect", Environment: in.Environment, Site: in.Site, Selector: selector, Resource: in.ID, Summary: "Catalog database inspection failed.", Cause: cause, Retryable: retry, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause)}
}
