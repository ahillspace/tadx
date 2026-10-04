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

type TableListInput struct {
	Environment, Site, Name, DatabaseID string
	Limit                               int
	All                                 bool
}
type TableListReader interface {
	DiscoverTables(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataTable], error)
}

func ValidateTableListInput(in TableListInput) error {
	if identityReason(in.DatabaseID) != "" {
		return tableListUsage("parent identity must be exact and contain no surrounding whitespace or control characters")
	}
	if reason := listLimitReason(in.Limit, in.All); reason != "" {
		return tableListUsage(reason)
	}

	return nil
}

type TableListOutput struct {
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
type TableListItem struct {
	LUID             string `json:"luid"`
	MetadataID       string `json:"metadata_id"`
	Name             string `json:"name"`
	Type             string `json:"type"`
	ParentLUID       string `json:"parent_luid"`
	ParentMetadataID string `json:"parent_metadata_id"`
	FullName         string `json:"full_name,omitempty"`
	Schema           string `json:"schema,omitempty"`
}

func (o TableListOutput) CompactOutput() any {
	rows := make([]TableListItem, len(o.Items))
	for i, v := range o.Items {
		rows[i] = TableListItem{LUID: v.LUID, MetadataID: v.MetadataID, Name: v.Name, Type: v.Type, ParentLUID: v.Database.LUID, ParentMetadataID: v.Database.MetadataID, FullName: v.FullName, Schema: v.Schema}
	}
	return struct {
		Status      string          `json:"status"`
		Environment string          `json:"environment,omitempty"`
		Site        string          `json:"site,omitempty"`
		Page        output.Page     `json:"page"`
		Items       []TableListItem `json:"items"`
		Complete    bool            `json:"complete"`
		ObservedAt  string          `json:"observed_at,omitempty"`
		Details     string          `json:"details"`
		NextCommand string          `json:"next_command,omitempty"`
	}{o.Status, o.Environment, o.Site, o.Page, rows, o.Complete, o.ObservedAt, "--full", o.NextCommand}
}
func (o TableListOutput) FullOutput() any { return o }
func listTablesValidated(ctx context.Context, reader TableListReader, in TableListInput) (out TableListOutput, err error) {
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
			out.NextCommand = tableListNextCommand(in)
		}
	}()
	out = TableListOutput{Status: "listed", Environment: in.Environment, Site: in.Site, Items: []value.MetadataTable{}}
	if reader == nil {
		return out, tableListUsage("catalog table reader is not configured")
	}
	limit := listLimit(in.Limit, in.All)
	out.Page.Limit = limit
	result, readErr := paging.CollectMetadata(ctx, limit, func(ctx context.Context, size int, cursor string) (value.MetadataPage[value.MetadataTable], error) {
		return reader.DiscoverTables(ctx, value.MetadataQuery{Name: in.Name, Limit: size, Cursor: cursor, ParentLUID: in.DatabaseID})
	}, func(item value.MetadataTable) string {
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
		return out, tableListFailure(in, readErr)
	}
	if !out.Complete && !out.Page.MoreAvailable {
		out.Status = "partial"
	}
	return out, nil
}

func tableListNextCommand(in TableListInput) string {
	args := []string{"catalog", "table", "list", "--all"}
	if in.Name != "" {
		args = append(args, "--name", in.Name)
	}
	if in.DatabaseID != "" {
		args = append(args, "--database-id", in.DatabaseID)
	}
	return commandhint.Environment(in.Environment, args...)
}
func tableListUsage(message string) error {
	return &errs.Error{ID: "catalog.table.list.usage", Kind: errs.KindUsage, Operation: "catalog.table.list", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the list selectors or bounds."}
}
func tableListFailure(in TableListInput, cause error) error {
	retry, advice := errs.CompleteRetryAdvice(cause, "Review the exact scope and retry the read.")
	return &errs.Error{ID: "catalog.table.list.failed", Kind: errs.KindOperation, Operation: "catalog.table.list", Environment: in.Environment, Site: in.Site, Summary: "Catalog table listing failed.", Cause: cause, Retryable: retry, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause)}
}

type TableInspectInput struct{ Environment, Site, ID, MetadataID string }
type TableInspectReader interface {
	GetTable(context.Context, string) (value.MetadataTable, error)
	DiscoverTables(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataTable], error)
}

func ValidateTableInspectInput(in TableInspectInput) error {
	if reason := identityReason(in.ID, in.MetadataID); reason != "" {
		return tableInspectUsage(reason)
	}
	if reason := selectorReason(in.ID, in.MetadataID); reason != "" {
		return tableInspectUsage(reason)
	}
	return nil
}

type TableInspectOutput struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Item        *value.MetadataTable `json:"item,omitempty"`
	ObservedAt  string               `json:"observed_at,omitempty"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
}

func (o TableInspectOutput) CompactOutput() any {
	return struct {
		Status      string            `json:"status"`
		Environment string            `json:"environment,omitempty"`
		Site        string            `json:"site,omitempty"`
		Item        *tableInspectItem `json:"item,omitempty"`
		Details     string            `json:"details"`
	}{o.Status, o.Environment, o.Site, compactTableInspect(o.Item), "--full"}
}
func (o TableInspectOutput) FullOutput() any { return o }

type tableInspectItem struct {
	value.MetadataIdentity
	Description  *string                `json:"description"`
	ContactLUID  string                 `json:"contact_luid,omitempty"`
	Database     value.MetadataIdentity `json:"database"`
	FullName     string                 `json:"full_name,omitempty"`
	Schema       string                 `json:"schema,omitempty"`
	Tags         []string               `json:"tags,omitempty"`
	TagsObserved bool                   `json:"tags_observed"`
}

func compactTableInspect(v *value.MetadataTable) *tableInspectItem {
	if v == nil {
		return nil
	}
	return &tableInspectItem{MetadataIdentity: v.MetadataIdentity, Description: v.Description, ContactLUID: v.ContactLUID, Database: v.Database, FullName: v.FullName, Schema: v.Schema, Tags: v.Tags, TagsObserved: v.TagsObserved}
}
func inspectTableValidated(ctx context.Context, reader TableInspectReader, in TableInspectInput) (TableInspectOutput, error) {
	out := TableInspectOutput{Status: "failed", Environment: in.Environment, Site: in.Site}
	if reader == nil {
		return out, tableInspectUsage("catalog table inspection is not configured")
	}
	var err error
	if in.ID != "" {
		item, readErr := reader.GetTable(ctx, in.ID)
		err = readErr
		if err == nil && item.LUID != in.ID {
			err = fmt.Errorf("returned table LUID does not match requested identity")
		} else if item.LUID == in.ID {
			out.Item = &item
		}
	} else {
		var page value.MetadataPage[value.MetadataTable]
		page, err = reader.DiscoverTables(ctx, value.MetadataQuery{MetadataID: in.MetadataID, Limit: 2})
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
		return out, tableInspectFailure(in, err)
	}
	out.Status = "inspected"
	return out, nil
}
func tableInspectUsage(s string) error {
	return &errs.Error{ID: "catalog.table.inspect.usage", Kind: errs.KindUsage, Operation: "catalog.table.inspect", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Use an exact REST LUID or Metadata ID returned by discovery."}
}
func tableInspectFailure(in TableInspectInput, cause error) error {
	retry, advice := errs.CompleteRetryAdvice(cause, "Check the exact catalog identity and permissions.")
	selector := in.ID
	if selector == "" {
		selector = in.MetadataID
	}
	return &errs.Error{ID: "catalog.table.inspect.failed", Kind: errs.KindOperation, Operation: "catalog.table.inspect", Environment: in.Environment, Site: in.Site, Selector: selector, Resource: in.ID, Summary: "Catalog table inspection failed.", Cause: cause, Retryable: retry, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause)}
}
