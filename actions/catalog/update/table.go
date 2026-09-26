package update

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type TableInput struct {
	Environment, Site, ID string
	TargetResolved        bool
	Description           *string
	ContactLUID           *string
	AddTags, RemoveTags   []string
}
type TableReader interface {
	GetTable(context.Context, string) (value.MetadataTable, error)
}
type TableWriter interface {
	UpdateTable(context.Context, string, value.MetadataUpdate) (value.MetadataTable, error)
	AddTableTags(context.Context, string, []string) ([]string, error)
	DeleteTableTag(context.Context, string, string) error
}
type TableAction struct {
	reader TableReader
	writer TableWriter
}

func NewTable(r TableReader, w TableWriter) *TableAction { return &TableAction{reader: r, writer: w} }

type TablePlan struct {
	Mode        string                 `json:"mode"`
	Operation   string                 `json:"operation"`
	Environment string                 `json:"environment"`
	Site        string                 `json:"site"`
	Target      value.MetadataIdentity `json:"target"`
	Changes     []Change               `json:"changes"`
	NoOp        bool                   `json:"no_op"`
}
type TableResult struct {
	Status       string                 `json:"status"`
	Identity     value.MetadataIdentity `json:"identity"`
	Description  *string                `json:"description,omitempty"`
	ContactLUID  string                 `json:"contact_luid,omitempty"`
	Tags         []string               `json:"tags,omitempty"`
	TagsObserved bool                   `json:"tags_observed"`
	Completed    []string               `json:"completed"`
	Failed       string                 `json:"failed,omitempty"`
}
type TableOutput struct {
	Plan   TablePlan    `json:"plan"`
	Result *TableResult `json:"result,omitempty"`
	Help   []string     `json:"help,omitempty"`
}

func (o TableOutput) CompactOutput() any { return o }
func (o TableOutput) FullOutput() any    { return o }
func ValidateTableInput(in TableInput) error {
	for _, id := range []string{in.ID} {
		if id != "" && (id != strings.TrimSpace(id) || strings.ContainsAny(id, "\x00\r\n")) {
			return tableUsage("identities must be exact and contain no surrounding whitespace or control characters")
		}
	}
	if strings.TrimSpace(in.ID) == "" {
		return tableUsage("an exact REST --id is required")
	}

	if in.Description == nil && in.ContactLUID == nil && len(in.AddTags) == 0 && len(in.RemoveTags) == 0 {
		return tableUsage("supply a description, supported contact, or tag change")
	}
	if in.Description != nil && (strings.TrimSpace(*in.Description) == "" || len(*in.Description) > 65536) {
		return tableUsage("description must be nonempty and bounded; clearing is not verified")
	}
	if in.ContactLUID != nil && strings.TrimSpace(*in.ContactLUID) == "" {
		return tableUsage("contact clearing is not verified; supply an exact contact LUID")
	}
	if reason := validateTags(in.AddTags, in.RemoveTags); reason != "" {
		return tableUsage(reason)
	}
	return nil
}

func (a *TableAction) Execute(ctx context.Context, in TableInput, preview bool) (TableOutput, error) {
	if err := ValidateTableInput(in); err != nil {
		return TableOutput{}, err
	}
	if strings.TrimSpace(in.Environment) == "" || (!in.TargetResolved && strings.TrimSpace(in.Site) == "") {
		return TableOutput{}, tableUsage("resolve an explicit mutation environment")
	}
	if a == nil || a.reader == nil || a.writer == nil {
		return TableOutput{}, tableUsage("catalog table update is not configured")
	}
	target, err := a.reader.GetTable(ctx, in.ID)
	if err != nil {
		return TableOutput{}, tableFailure(in, "read", nil, errs.OutcomeNotAttempted, err)
	}
	if err = tableValidTarget(target, in); err != nil {
		return TableOutput{}, tableFailure(in, "identity", nil, errs.OutcomeNotAttempted, err)
	}
	request, changes, add, remove := tableChanged(target, in)
	out := TableOutput{Plan: TablePlan{Mode: "preview", Operation: "catalog.table.update", Environment: in.Environment, Site: in.Site, Target: target.MetadataIdentity, Changes: changes, NoOp: len(changes) == 0}}
	if preview {
		return out, nil
	}
	current, err := a.reader.GetTable(ctx, in.ID)
	if err != nil {
		return out, tableFailure(in, "revalidate", nil, errs.OutcomeNotAttempted, err)
	}
	if err = tableValidTarget(current, in); err != nil {
		return out, tableFailure(in, "identity", nil, errs.OutcomeNotAttempted, err)
	}
	request, changes, add, remove = tableChanged(current, in)
	out.Plan.Mode = "execute"
	out.Plan.Target = current.MetadataIdentity
	out.Plan.Changes = changes
	out.Plan.NoOp = len(changes) == 0
	out.Result = &TableResult{Status: "updated", Identity: current.MetadataIdentity, Description: current.Description, ContactLUID: current.ContactLUID, Tags: slices.Clone(current.Tags), TagsObserved: current.TagsObserved, Completed: []string{}}
	out.Help = []string{commandhint.Environment(in.Environment, "catalog", "table", "inspect", "--id", in.ID)}
	if out.Plan.NoOp {
		out.Result.Status = "unchanged"
		return out, nil
	}
	if request.Description != nil || request.ContactLUID != nil {
		updated, e := a.writer.UpdateTable(ctx, in.ID, request)
		if e != nil {
			out.Result.Status = "partial"
			out.Result.Failed = "properties"
			outcome := errs.OutcomeUnknown
			if updated.LUID == in.ID {
				out.Result.Identity = retainedIdentity(current.MetadataIdentity, updated.MetadataIdentity)
				out.Result.Description = updated.Description
				out.Result.ContactLUID = updated.ContactLUID
				out.Result.Tags = slices.Clone(updated.Tags)
				out.Result.TagsObserved = updated.TagsObserved
				outcome = errs.OutcomeConfirmed
			}
			return out, tableFailure(in, "properties", out.Result.Completed, outcome, e)
		}
		if updated.LUID != in.ID {
			out.Result.Status = "partial"
			out.Result.Failed = "properties"
			return out, tableFailure(in, "properties", out.Result.Completed, errs.OutcomeUnknown, fmt.Errorf("updated identity does not match target"))
		}
		out.Result.Identity = retainedIdentity(current.MetadataIdentity, updated.MetadataIdentity)
		out.Result.Description = updated.Description
		out.Result.ContactLUID = updated.ContactLUID
		out.Result.Tags = slices.Clone(updated.Tags)
		out.Result.TagsObserved = updated.TagsObserved
		if request.Description != nil && (updated.Description == nil || *updated.Description != *request.Description) {
			out.Result.Status = "partial"
			out.Result.Failed = "properties"
			return out, tableFailure(in, "properties", out.Result.Completed, errs.OutcomeConfirmed, propertyVerification("description"))
		}
		if request.ContactLUID != nil && updated.ContactLUID != *request.ContactLUID {
			out.Result.Status = "partial"
			out.Result.Failed = "properties"
			return out, tableFailure(in, "properties", out.Result.Completed, errs.OutcomeConfirmed, propertyVerification("contact"))
		}
		if request.Description != nil {
			out.Result.Completed = append(out.Result.Completed, "description")
		}
		if request.ContactLUID != nil {
			out.Result.Completed = append(out.Result.Completed, "contact")
		}
	}
	if len(add) > 0 {
		acknowledged, e := a.writer.AddTableTags(ctx, in.ID, add)
		if e == nil {
			e = verifyTagAcknowledgment(add, acknowledged)
		}
		if e != nil {
			out.Result.Status = "partial"
			out.Result.Failed = "add_tags"
			return out, tableFailure(in, "add_tags", out.Result.Completed, errs.OutcomeUnknown, e)
		}
		out.Result.Completed = append(out.Result.Completed, "add_tags")
		out.Result.Tags = slices.Clone(acknowledged)
		out.Result.TagsObserved = true
	}
	for _, tag := range remove {
		if e := a.writer.DeleteTableTag(ctx, in.ID, tag); e != nil {
			out.Result.Status = "partial"
			out.Result.Failed = "remove_tag:" + tag
			return out, tableFailure(in, "remove_tag", out.Result.Completed, errs.OutcomeUnknown, e)
		}
		out.Result.Completed = append(out.Result.Completed, "remove_tag:"+tag)
		out.Result.Tags = nil
		out.Result.TagsObserved = false
	}
	return out, nil
}
func tableValidTarget(v value.MetadataTable, in TableInput) error {
	if v.LUID != in.ID {
		return fmt.Errorf("returned table has a different REST identity")
	}
	return nil
}
func tableChanged(v value.MetadataTable, in TableInput) (value.MetadataUpdate, []Change, []string, []string) {
	patch := value.MetadataUpdate{}
	changes := []Change{}
	if in.Description != nil && (v.Description == nil || *v.Description != *in.Description) {
		patch.Description = new(*in.Description)
		changes = append(changes, Change{Property: "description", Before: v.Description, After: *in.Description})
	}
	if in.ContactLUID != nil && v.ContactLUID != *in.ContactLUID {
		patch.ContactLUID = new(*in.ContactLUID)
		changes = append(changes, Change{Property: "contact", Before: new(v.ContactLUID), After: *in.ContactLUID})
	}
	tags, add, remove := tagChanges(v.Tags, v.TagsObserved, in.AddTags, in.RemoveTags)
	changes = append(changes, tags...)
	return patch, changes, add, remove
}
func tableUsage(s string) error {
	return &errs.Error{ID: "catalog.table.update.usage", Kind: errs.KindUsage, Operation: "catalog.table.update", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Correct the requested metadata update and preview it."}
}
func tableFailure(in TableInput, step string, completed []string, outcome errs.Outcome, cause error) error {
	retry, advice := errs.CompleteRetryAdvice(cause, "Inspect the exact asset before retrying: "+commandhint.Environment(in.Environment, "catalog", "table", "inspect", "--id", in.ID))
	phase := errs.PhaseSubmission
	structured, ok := errors.AsType[*errs.Error](cause)
	var verification interface{ VerificationFailed() bool }
	if (ok && structured.Phase == errs.PhaseVerification) || (errors.As(cause, &verification) && verification.VerificationFailed()) {
		phase = errs.PhaseVerification
	}
	if outcome == errs.OutcomeNotAttempted {
		phase = errs.PhaseValidation
	}
	return &errs.Error{ID: "catalog.table.update." + step, Kind: errs.KindOperation, Operation: "catalog.table.update", Environment: in.Environment, Site: in.Site, Resource: in.ID, Summary: "Catalog table metadata update failed.", Cause: cause, Retryable: retry, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause), Completed: completed, Failed: step, Phase: phase, Outcome: outcome}
}
