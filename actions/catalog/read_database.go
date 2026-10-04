package catalog

import (
	"context"
	"fmt"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/paging"
	"github.com/ahillspace/tadx/internal/value"
)

type DatabaseListInput struct {
	Environment, Site, Name string
	Limit                   int
	All                     bool
}
type DatabaseListReader interface {
	DiscoverDatabases(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataDatabase], error)
}

func ValidateDatabaseListInput(in DatabaseListInput) error {
	if reason := listLimitReason(in.Limit, in.All); reason != "" {
		return databaseListUsage(reason)
	}

	return nil
}

type DatabaseListOutput struct {
	Status      string                   `json:"status"`
	Environment string                   `json:"environment,omitempty"`
	Site        string                   `json:"site,omitempty"`
	Page        output.Page              `json:"page"`
	Items       []value.MetadataDatabase `json:"items"`
	Complete    bool                     `json:"complete"`
	ObservedAt  string                   `json:"observed_at,omitempty"`
	RequestID   string                   `json:"tableau_request_id,omitempty"`
	NextCommand string                   `json:"next_command,omitempty"`
}
type DatabaseListItem struct {
	LUID       string `json:"luid"`
	MetadataID string `json:"metadata_id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
}

func (o DatabaseListOutput) CompactOutput() any {
	rows := make([]DatabaseListItem, len(o.Items))
	for i, v := range o.Items {
		rows[i] = DatabaseListItem{LUID: v.LUID, MetadataID: v.MetadataID, Name: v.Name, Type: v.Type}
	}
	return struct {
		Status      string             `json:"status"`
		Environment string             `json:"environment,omitempty"`
		Site        string             `json:"site,omitempty"`
		Page        output.Page        `json:"page"`
		Items       []DatabaseListItem `json:"items"`
		Complete    bool               `json:"complete"`
		ObservedAt  string             `json:"observed_at,omitempty"`
		Details     string             `json:"details"`
		NextCommand string             `json:"next_command,omitempty"`
	}{o.Status, o.Environment, o.Site, o.Page, rows, o.Complete, o.ObservedAt, "--full", o.NextCommand}
}
func (o DatabaseListOutput) FullOutput() any { return o }
func listDatabasesValidated(ctx context.Context, reader DatabaseListReader, in DatabaseListInput) (out DatabaseListOutput, err error) {
	defer func() {
		if err != nil && out.Status != "" {
			out.Status = "partial"
			out.Complete = false
			out.Page.MoreAvailable = true
			out.Page.Returned = len(out.Items)
		}
	}()
	defer func() {
		if err == nil && out.Page.MoreAvailable {
			out.NextCommand = databaseListNextCommand(in)
		}
	}()
	out = DatabaseListOutput{Status: "listed", Environment: in.Environment, Site: in.Site, Items: []value.MetadataDatabase{}}
	if reader == nil {
		return out, databaseListUsage("catalog database reader is not configured")
	}
	limit := listLimit(in.Limit, in.All)
	out.Page.Limit = limit
	result, readErr := paging.CollectMetadata(ctx, limit, func(ctx context.Context, size int, cursor string) (value.MetadataPage[value.MetadataDatabase], error) {
		return reader.DiscoverDatabases(ctx, value.MetadataQuery{Name: in.Name, Limit: size, Cursor: cursor})
	}, func(item value.MetadataDatabase) string {
		if item.MetadataID != "" {
			return item.MetadataID
		}
		return item.LUID
	})
	out.Items = result.Items
	out.Page.Returned = len(result.Items)
	out.Page.Total = result.Total
	out.Page.MoreAvailable = result.MoreAvailable
	out.Complete = result.Complete
	out.ObservedAt = result.ObservedAt
	out.RequestID = result.TableauRequestID
	if readErr != nil {
		return out, databaseListFailure(in, readErr)
	}
	if !out.Complete && !out.Page.MoreAvailable {
		out.Status = "partial"
	}
	return out, nil
}

func databaseListNextCommand(in DatabaseListInput) string {
	args := []string{"catalog", "database", "list", "--all"}
	if in.Name != "" {
		args = append(args, "--name", in.Name)
	}
	return commandhint.Environment(in.Environment, args...)
}
func databaseListUsage(message string) error {
	return &errs.Error{ID: "catalog.database.list.usage", Kind: errs.KindUsage, Operation: "catalog.database.list", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the list selectors or bounds."}
}
func databaseListFailure(in DatabaseListInput, cause error) error {
	retry, advice := errs.CompleteRetryAdvice(cause, "Review the exact scope and retry the read.")
	return &errs.Error{ID: "catalog.database.list.failed", Kind: errs.KindOperation, Operation: "catalog.database.list", Environment: in.Environment, Site: in.Site, Summary: "Catalog database listing failed.", Cause: cause, Retryable: retry, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause)}
}

type DatabaseInspectInput struct{ Environment, Site, ID, MetadataID string }
type DatabaseInspectReader interface {
	GetDatabase(context.Context, string) (value.MetadataDatabase, error)
	DiscoverDatabases(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataDatabase], error)
}

func ValidateDatabaseInspectInput(in DatabaseInspectInput) error {
	if reason := identityReason(in.ID, in.MetadataID); reason != "" {
		return databaseInspectUsage(reason)
	}
	if reason := selectorReason(in.ID, in.MetadataID); reason != "" {
		return databaseInspectUsage(reason)
	}
	return nil
}

type DatabaseInspectOutput struct {
	Status      string                  `json:"status"`
	Environment string                  `json:"environment,omitempty"`
	Site        string                  `json:"site,omitempty"`
	Item        *value.MetadataDatabase `json:"item,omitempty"`
	ObservedAt  string                  `json:"observed_at,omitempty"`
	RequestID   string                  `json:"tableau_request_id,omitempty"`
}

func (o DatabaseInspectOutput) CompactOutput() any {
	return struct {
		Status      string               `json:"status"`
		Environment string               `json:"environment,omitempty"`
		Site        string               `json:"site,omitempty"`
		Item        *databaseInspectItem `json:"item,omitempty"`
		Details     string               `json:"details"`
	}{o.Status, o.Environment, o.Site, compactDatabaseInspect(o.Item), "--full"}
}
func (o DatabaseInspectOutput) FullOutput() any { return o }

type databaseInspectItem struct {
	value.MetadataIdentity
	Description    *string  `json:"description"`
	ContactLUID    string   `json:"contact_luid,omitempty"`
	ConnectionType string   `json:"connection_type,omitempty"`
	FilePath       string   `json:"file_path,omitempty"`
	Embedded       bool     `json:"embedded"`
	Tags           []string `json:"tags,omitempty"`
	TagsObserved   bool     `json:"tags_observed"`
}

func compactDatabaseInspect(v *value.MetadataDatabase) *databaseInspectItem {
	if v == nil {
		return nil
	}
	return &databaseInspectItem{MetadataIdentity: v.MetadataIdentity, Description: v.Description, ContactLUID: v.ContactLUID, ConnectionType: v.ConnectionType, FilePath: v.FilePath, Embedded: v.Embedded, Tags: v.Tags, TagsObserved: v.TagsObserved}
}
func inspectDatabaseValidated(ctx context.Context, reader DatabaseInspectReader, in DatabaseInspectInput) (DatabaseInspectOutput, error) {
	out := DatabaseInspectOutput{Status: "failed", Environment: in.Environment, Site: in.Site}
	if reader == nil {
		return out, databaseInspectUsage("catalog database inspection is not configured")
	}
	var err error
	if in.ID != "" {
		item, readErr := reader.GetDatabase(ctx, in.ID)
		err = readErr
		if err == nil && item.LUID != in.ID {
			err = fmt.Errorf("returned database LUID does not match requested identity")
		} else if item.LUID == in.ID {
			out.Item = &item
		}
	} else {
		var page value.MetadataPage[value.MetadataDatabase]
		page, err = reader.DiscoverDatabases(ctx, value.MetadataQuery{MetadataID: in.MetadataID, Limit: 2})
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
		return out, databaseInspectFailure(in, err)
	}
	out.Status = "inspected"
	return out, nil
}
func databaseInspectUsage(s string) error {
	return &errs.Error{ID: "catalog.database.inspect.usage", Kind: errs.KindUsage, Operation: "catalog.database.inspect", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Use an exact REST LUID or Metadata ID returned by discovery."}
}
func databaseInspectFailure(in DatabaseInspectInput, cause error) error {
	retry, advice := errs.CompleteRetryAdvice(cause, "Check the exact catalog identity and permissions.")
	selector := in.ID
	if selector == "" {
		selector = in.MetadataID
	}
	return &errs.Error{ID: "catalog.database.inspect.failed", Kind: errs.KindOperation, Operation: "catalog.database.inspect", Environment: in.Environment, Site: in.Site, Selector: selector, Resource: in.ID, Summary: "Catalog database inspection failed.", Cause: cause, Retryable: retry, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause)}
}
