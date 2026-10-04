package catalog

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type ColumnInput struct {
	Environment, Site, ID, TableID string
	TargetResolved                 bool
	Description                    *string
	AddTags, RemoveTags            []string
}
type ColumnReader interface {
	GetColumn(context.Context, string, string) (value.MetadataColumn, error)
}
type ColumnWriter interface {
	UpdateColumn(context.Context, string, string, value.MetadataUpdate) (value.MetadataColumn, error)
	TagWriter
}
type columnUpdate struct {
	reader ColumnReader
	writer ColumnWriter
}

type ColumnPlan struct {
	Mode        string                 `json:"mode"`
	Operation   string                 `json:"operation"`
	Environment string                 `json:"environment"`
	Site        string                 `json:"site"`
	Target      value.MetadataIdentity `json:"target"`
	TableLUID   string                 `json:"table_luid"`
	Changes     []Change               `json:"changes"`
	NoOp        bool                   `json:"no_op"`
}
type ColumnResult struct {
	Status       string                 `json:"status"`
	Identity     value.MetadataIdentity `json:"identity"`
	Description  *string                `json:"description,omitempty"`
	Tags         []string               `json:"tags,omitempty"`
	TagsObserved bool                   `json:"tags_observed"`
	Completed    []string               `json:"completed"`
	Failed       string                 `json:"failed,omitempty"`
}
type ColumnOutput struct {
	Plan   ColumnPlan    `json:"plan"`
	Result *ColumnResult `json:"result,omitempty"`
	Help   []string      `json:"help,omitempty"`
}

func (o ColumnOutput) CompactOutput() any { return o }
func (o ColumnOutput) FullOutput() any    { return o }
func ValidateColumnInput(in ColumnInput) error {
	for _, id := range []string{in.ID, in.TableID} {
		if id != "" && (id != strings.TrimSpace(id) || strings.ContainsAny(id, "\x00\r\n")) {
			return columnUsage("identities must be exact and contain no surrounding whitespace or control characters")
		}
	}
	if strings.TrimSpace(in.ID) == "" {
		return columnUsage("an exact REST --id is required")
	}
	if strings.TrimSpace(in.TableID) == "" {
		return columnUsage("--table-id is required for a column")
	}
	if in.Description == nil && len(in.AddTags) == 0 && len(in.RemoveTags) == 0 {
		return columnUsage("supply a description, supported contact, or tag change")
	}
	if in.Description != nil && ((*in.Description != "" && strings.TrimSpace(*in.Description) == "") || len(*in.Description) > 65536) {
		return columnUsage("description must be empty to clear or nonblank text of at most 65536 bytes")
	}

	if reason := validateTags(in.AddTags, in.RemoveTags); reason != "" {
		return columnUsage(reason)
	}
	return nil
}

func (a *columnUpdate) executeValidated(ctx context.Context, in ColumnInput, preview bool) (ColumnOutput, error) {
	if strings.TrimSpace(in.Environment) == "" || (!in.TargetResolved && strings.TrimSpace(in.Site) == "") {
		return ColumnOutput{}, columnUsage("resolve an explicit mutation environment")
	}
	if a == nil || a.reader == nil || a.writer == nil {
		return ColumnOutput{}, columnUsage("catalog column update is not configured")
	}
	target, err := a.reader.GetColumn(ctx, in.TableID, in.ID)
	if err != nil {
		return ColumnOutput{}, columnFailure(in, "read", nil, errs.OutcomeNotAttempted, err)
	}
	if err = columnValidTarget(target, in); err != nil {
		return ColumnOutput{}, columnFailure(in, "identity", nil, errs.OutcomeNotAttempted, err)
	}
	request, changes, add, remove := columnChanged(target, in)
	out := ColumnOutput{Plan: ColumnPlan{Mode: "preview", Operation: "catalog.column.update", Environment: in.Environment, Site: in.Site, Target: target.MetadataIdentity, TableLUID: in.TableID, Changes: changes, NoOp: len(changes) == 0}}
	if preview {
		return out, nil
	}
	current, err := a.reader.GetColumn(ctx, in.TableID, in.ID)
	if err != nil {
		return out, columnFailure(in, "revalidate", nil, errs.OutcomeNotAttempted, err)
	}
	if err = columnValidTarget(current, in); err != nil {
		return out, columnFailure(in, "identity", nil, errs.OutcomeNotAttempted, err)
	}
	request, changes, add, remove = columnChanged(current, in)
	out.Plan.Mode = "execute"
	out.Plan.Target = current.MetadataIdentity
	out.Plan.Changes = changes
	out.Plan.NoOp = len(changes) == 0
	out.Result = &ColumnResult{Status: "updated", Identity: current.MetadataIdentity, Description: current.Description, Tags: slices.Clone(current.Tags), TagsObserved: current.TagsObserved, Completed: []string{}}
	out.Help = []string{commandhint.Environment(in.Environment, "catalog", "column", "inspect", "--table-id", in.TableID, "--id", in.ID)}
	if out.Plan.NoOp {
		out.Result.Status = "unchanged"
		return out, nil
	}
	if request.Description != nil {
		updated, e := a.writer.UpdateColumn(ctx, in.TableID, in.ID, request)
		if e != nil {
			out.Result.Status = "partial"
			out.Result.Failed = "properties"
			outcome := errs.OutcomeUnknown
			if updated.LUID == in.ID {
				out.Result.Identity = retainedIdentity(current.MetadataIdentity, updated.MetadataIdentity)
				out.Result.Description = updated.Description
				out.Result.Tags = slices.Clone(updated.Tags)
				out.Result.TagsObserved = updated.TagsObserved
				outcome = errs.OutcomeConfirmed
			}
			return out, columnFailure(in, "properties", out.Result.Completed, outcome, e)
		}
		if updated.LUID != in.ID {
			out.Result.Status = "partial"
			out.Result.Failed = "properties"
			return out, columnFailure(in, "properties", out.Result.Completed, errs.OutcomeUnknown, fmt.Errorf("updated identity does not match target"))
		}
		out.Result.Identity = retainedIdentity(current.MetadataIdentity, updated.MetadataIdentity)
		out.Result.Description = updated.Description
		out.Result.Tags = slices.Clone(updated.Tags)
		out.Result.TagsObserved = updated.TagsObserved
		if request.Description != nil && (updated.Description == nil || *updated.Description != *request.Description) {
			out.Result.Status = "partial"
			out.Result.Failed = "properties"
			return out, columnFailure(in, "properties", out.Result.Completed, errs.OutcomeConfirmed, propertyVerification("description"))
		}
		if request.Description != nil {
			out.Result.Completed = append(out.Result.Completed, "description")
		}
	}
	if len(add) > 0 {
		acknowledged, e := a.writer.AddTags(ctx, value.LabelTarget{Type: "column", LUID: in.ID}, add)
		if e == nil {
			e = verifyTagAcknowledgment(add, acknowledged)
		}
		if e != nil {
			out.Result.Status = "partial"
			out.Result.Failed = "add_tags"
			return out, columnFailure(in, "add_tags", out.Result.Completed, errs.OutcomeUnknown, e)
		}
		out.Result.Completed = append(out.Result.Completed, "add_tags")
		out.Result.Tags = slices.Clone(acknowledged)
		out.Result.TagsObserved = true
	}
	for _, tag := range remove {
		if e := a.writer.DeleteTag(ctx, value.LabelTarget{Type: "column", LUID: in.ID}, tag); e != nil {
			out.Result.Status = "partial"
			out.Result.Failed = "remove_tag:" + tag
			return out, columnFailure(in, "remove_tag", out.Result.Completed, errs.OutcomeUnknown, e)
		}
		out.Result.Completed = append(out.Result.Completed, "remove_tag:"+tag)
		out.Result.Tags = nil
		out.Result.TagsObserved = false
	}
	return out, nil
}
func columnValidTarget(v value.MetadataColumn, in ColumnInput) error {
	if v.LUID != in.ID {
		return fmt.Errorf("returned column has a different REST identity")
	}
	if v.Table.LUID != in.TableID {
		return fmt.Errorf("returned column has a different parent table")
	}
	return nil
}
func columnChanged(v value.MetadataColumn, in ColumnInput) (value.MetadataUpdate, []Change, []string, []string) {
	patch := value.MetadataUpdate{}
	changes := []Change{}
	if in.Description != nil && (v.Description == nil || *v.Description != *in.Description) {
		patch.Description = new(*in.Description)
		changes = append(changes, Change{Property: "description", Before: v.Description, After: *in.Description})
	}

	tags, add, remove := tagChanges(v.Tags, v.TagsObserved, in.AddTags, in.RemoveTags)
	changes = append(changes, tags...)
	return patch, changes, add, remove
}
func columnUsage(s string) error {
	return &errs.Error{ID: "catalog.column.update.usage", Kind: errs.KindUsage, Operation: "catalog.column.update", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Correct the requested metadata update and preview it."}
}
func columnFailure(in ColumnInput, step string, completed []string, outcome errs.Outcome, cause error) error {
	retry, advice := errs.CompleteRetryAdvice(cause, "Inspect the exact asset before retrying: "+commandhint.Environment(in.Environment, "catalog", "column", "inspect", "--table-id", in.TableID, "--id", in.ID))
	return &errs.Error{ID: "catalog.column.update." + step, Kind: errs.KindOperation, Operation: "catalog.column.update", Environment: in.Environment, Site: in.Site, Resource: in.ID, Summary: "Catalog column metadata update failed.", Cause: cause, Retryable: retry, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause), Completed: completed, Failed: step, Phase: failurePhase(cause, outcome), Outcome: outcome}
}
