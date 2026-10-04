package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/paging"
	"github.com/ahillspace/tadx/internal/value"
)

type ColumnListInput struct {
	Environment, Site, Name, TableID string
	Limit                            int
	All                              bool
}
type ColumnListReader interface {
	DiscoverColumns(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataColumn], error)
}

func ValidateColumnListInput(in ColumnListInput) error {
	if identityReason(in.TableID) != "" {
		return columnListUsage("parent identity must be exact and contain no surrounding whitespace or control characters")
	}
	if reason := listLimitReason(in.Limit, in.All); reason != "" {
		return columnListUsage(reason)
	}
	if strings.TrimSpace(in.TableID) == "" {
		return columnListUsage("column listing requires an exact --table-id")
	}
	return nil
}

type ColumnListOutput struct {
	Status      string                 `json:"status"`
	Environment string                 `json:"environment,omitempty"`
	Site        string                 `json:"site,omitempty"`
	Page        output.Page            `json:"page"`
	Items       []value.MetadataColumn `json:"items"`
	Complete    bool                   `json:"complete"`
	ObservedAt  string                 `json:"observed_at,omitempty"`
	RequestID   string                 `json:"tableau_request_id,omitempty"`
	NextCommand string                 `json:"next_command,omitempty"`
}
type ColumnListItem struct {
	LUID             string `json:"luid"`
	MetadataID       string `json:"metadata_id"`
	Name             string `json:"name"`
	Type             string `json:"type"`
	ParentLUID       string `json:"parent_luid"`
	ParentMetadataID string `json:"parent_metadata_id"`
}

func (o ColumnListOutput) CompactOutput() any {
	rows := make([]ColumnListItem, len(o.Items))
	for i, v := range o.Items {
		rows[i] = ColumnListItem{LUID: v.LUID, MetadataID: v.MetadataID, Name: v.Name, Type: v.Type, ParentLUID: v.Table.LUID, ParentMetadataID: v.Table.MetadataID}
	}
	return struct {
		Status      string           `json:"status"`
		Environment string           `json:"environment,omitempty"`
		Site        string           `json:"site,omitempty"`
		Page        output.Page      `json:"page"`
		Items       []ColumnListItem `json:"items"`
		Complete    bool             `json:"complete"`
		ObservedAt  string           `json:"observed_at,omitempty"`
		Details     string           `json:"details"`
		NextCommand string           `json:"next_command,omitempty"`
	}{o.Status, o.Environment, o.Site, o.Page, rows, o.Complete, o.ObservedAt, "--full", o.NextCommand}
}
func (o ColumnListOutput) FullOutput() any { return o }
func listColumnsValidated(ctx context.Context, reader ColumnListReader, in ColumnListInput) (out ColumnListOutput, err error) {
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
			out.NextCommand = columnListNextCommand(in)
		}
	}()
	out = ColumnListOutput{Status: "listed", Environment: in.Environment, Site: in.Site, Items: []value.MetadataColumn{}}
	if reader == nil {
		return out, columnListUsage("catalog column reader is not configured")
	}
	limit := listLimit(in.Limit, in.All)
	out.Page.Limit = limit
	result, readErr := paging.CollectMetadata(ctx, limit, func(ctx context.Context, size int, cursor string) (value.MetadataPage[value.MetadataColumn], error) {
		return reader.DiscoverColumns(ctx, value.MetadataQuery{Name: in.Name, Limit: size, Cursor: cursor, ParentLUID: in.TableID})
	}, func(item value.MetadataColumn) string {
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
		return out, columnListFailure(in, readErr)
	}
	if !out.Complete && !out.Page.MoreAvailable {
		out.Status = "partial"
	}
	return out, nil
}
func columnListNextCommand(in ColumnListInput) string {
	args := []string{"catalog", "column", "list", "--all", "--table-id", in.TableID}
	if in.Name != "" {
		args = append(args, "--name", in.Name)
	}
	return commandhint.Environment(in.Environment, args...)
}
func columnListUsage(message string) error {
	return &errs.Error{ID: "catalog.column.list.usage", Kind: errs.KindUsage, Operation: "catalog.column.list", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the list selectors or bounds."}
}
func columnListFailure(in ColumnListInput, cause error) error {
	retry, advice := errs.CompleteRetryAdvice(cause, "Review the exact scope and retry the read.")
	return &errs.Error{ID: "catalog.column.list.failed", Kind: errs.KindOperation, Operation: "catalog.column.list", Environment: in.Environment, Site: in.Site, Summary: "Catalog column listing failed.", Cause: cause, Retryable: retry, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause)}
}

type ColumnInspectInput struct{ Environment, Site, ID, MetadataID, TableID string }
type ColumnInspectReader interface {
	GetColumn(context.Context, string, string) (value.MetadataColumn, error)
	DiscoverColumns(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataColumn], error)
}

func ValidateColumnInspectInput(in ColumnInspectInput) error {
	if reason := identityReason(in.ID, in.MetadataID, in.TableID); reason != "" {
		return columnInspectUsage(reason)
	}
	if reason := selectorReason(in.ID, in.MetadataID); reason != "" {
		return columnInspectUsage(reason)
	}
	if in.ID != "" && strings.TrimSpace(in.TableID) == "" {
		return columnInspectUsage("--id requires --table-id for a column")
	}
	return nil
}

type ColumnInspectOutput struct {
	Status      string                `json:"status"`
	Environment string                `json:"environment,omitempty"`
	Site        string                `json:"site,omitempty"`
	Item        *value.MetadataColumn `json:"item,omitempty"`
	ObservedAt  string                `json:"observed_at,omitempty"`
	RequestID   string                `json:"tableau_request_id,omitempty"`
}

func (o ColumnInspectOutput) CompactOutput() any {
	return struct {
		Status      string             `json:"status"`
		Environment string             `json:"environment,omitempty"`
		Site        string             `json:"site,omitempty"`
		Item        *columnInspectItem `json:"item,omitempty"`
		Details     string             `json:"details"`
	}{o.Status, o.Environment, o.Site, compactColumnInspect(o.Item), "--full"}
}
func (o ColumnInspectOutput) FullOutput() any { return o }

type columnInspectItem struct {
	value.MetadataIdentity
	Description  *string                `json:"description"`
	Table        value.MetadataIdentity `json:"table"`
	RemoteType   string                 `json:"remote_type,omitempty"`
	Nullable     *bool                  `json:"nullable,omitempty"`
	Tags         []string               `json:"tags,omitempty"`
	TagsObserved bool                   `json:"tags_observed"`
}

func compactColumnInspect(v *value.MetadataColumn) *columnInspectItem {
	if v == nil {
		return nil
	}
	return &columnInspectItem{MetadataIdentity: v.MetadataIdentity, Description: v.Description, Table: v.Table, RemoteType: v.RemoteType, Nullable: v.Nullable, Tags: v.Tags, TagsObserved: v.TagsObserved}
}
func inspectColumnValidated(ctx context.Context, reader ColumnInspectReader, in ColumnInspectInput) (ColumnInspectOutput, error) {
	out := ColumnInspectOutput{Status: "failed", Environment: in.Environment, Site: in.Site}
	if reader == nil {
		return out, columnInspectUsage("catalog column inspection is not configured")
	}
	var err error
	if in.ID != "" {
		item, readErr := reader.GetColumn(ctx, in.TableID, in.ID)
		err = readErr
		if err == nil && item.LUID != in.ID {
			err = fmt.Errorf("returned column LUID does not match requested identity")
		} else if item.LUID == in.ID && item.Table.LUID == in.TableID {
			out.Item = &item
		}
		if err == nil && item.Table.LUID != in.TableID {
			err = fmt.Errorf("returned column parent does not match requested table")
		}
	} else {
		var page value.MetadataPage[value.MetadataColumn]
		page, err = reader.DiscoverColumns(ctx, value.MetadataQuery{MetadataID: in.MetadataID, Limit: 2})
		out.ObservedAt = page.ObservedAt
		out.RequestID = page.TableauRequestID
		if len(page.Items) == 1 && page.Items[0].MetadataID == in.MetadataID {
			out.Item = &page.Items[0]
		}
		if err == nil && (out.Item == nil || !page.Complete || page.NextCursor != "") {
			err = fmt.Errorf("metadata selector did not resolve exactly one complete identity")
		}
	}
	if err != nil {
		if out.Item != nil {
			out.Status = "partial"
		}
		return out, columnInspectFailure(in, err)
	}
	out.Status = "inspected"
	return out, nil
}
func columnInspectUsage(s string) error {
	return &errs.Error{ID: "catalog.column.inspect.usage", Kind: errs.KindUsage, Operation: "catalog.column.inspect", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Use an exact REST LUID or Metadata ID returned by discovery."}
}
func columnInspectFailure(in ColumnInspectInput, cause error) error {
	retry, advice := errs.CompleteRetryAdvice(cause, "Check the exact catalog identity and permissions.")
	selector := in.ID
	if selector == "" {
		selector = in.MetadataID
	}
	return &errs.Error{ID: "catalog.column.inspect.failed", Kind: errs.KindOperation, Operation: "catalog.column.inspect", Environment: in.Environment, Site: in.Site, Selector: selector, Resource: in.ID, Summary: "Catalog column inspection failed.", Cause: cause, Retryable: retry, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause)}
}
