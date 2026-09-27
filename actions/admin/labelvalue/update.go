package labelvalue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type UpdateInput struct {
	Environment, Site              string
	Name                           string
	NewName, Category, Description *string
}
type UpdateReader interface {
	ListLabelValues(context.Context) ([]value.LabelValue, error)
	GetLabelValue(context.Context, string) (value.LabelValue, error)
}
type UpdateWriter interface {
	SetLabelValue(context.Context, string, value.LabelValue) (value.LabelValue, error)
}

type UpdatePlan struct {
	Mode        string            `json:"mode"`
	Operation   string            `json:"operation"`
	Environment string            `json:"environment"`
	Site        string            `json:"site"`
	Name        string            `json:"name"`
	Before      *value.LabelValue `json:"before,omitempty"`
	Desired     value.LabelValue  `json:"desired"`
	NoOp        bool              `json:"no_op"`
	Effect      string            `json:"effect"`
}
type UpdateResult struct {
	Status    string           `json:"status"`
	Item      value.LabelValue `json:"item"`
	Completed []string         `json:"completed,omitempty"`
}
type UpdateOutput struct {
	Plan   UpdatePlan    `json:"plan"`
	Result *UpdateResult `json:"result,omitempty"`
}

func (o UpdateOutput) CompactOutput() any { return o }
func (o UpdateOutput) FullOutput() any    { return o }
func ValidateUpdateInput(in UpdateInput) error {
	if !validName(in.Name) {
		return updateUsage("an exact --name containing 1 to 128 characters is required")
	}
	if in.NewName == nil && in.Category == nil && in.Description == nil {
		return updateUsage("supply a label value change")
	}
	if in.NewName != nil && !validName(*in.NewName) {
		return updateUsage("new name must contain 1 to 128 characters without surrounding whitespace")
	}
	if in.Category != nil && !validName(*in.Category) {
		return updateUsage("category must be an exact nonempty name")
	}
	if in.Description != nil && !validDescription(*in.Description) {
		return updateUsage("description must contain 1 to 500 characters")
	}
	return nil
}
func Update(ctx context.Context, reader UpdateReader, writer UpdateWriter, in UpdateInput, preview bool) (UpdateOutput, error) {
	out := UpdateOutput{Plan: UpdatePlan{Mode: "preview", Operation: "admin.label.value.update", Environment: in.Environment, Site: in.Site, Name: in.Name, Effect: "Native create-or-update; it may create the named value if absent."}}
	before, err := updateBaseline(ctx, reader, in)
	if err != nil {
		return out, updateFail(in, "read", errs.OutcomeNotAttempted, err)
	}
	out.Plan.Before = before
	desired := value.LabelValue{Name: in.Name}
	if before != nil {
		desired = *before
	}
	if in.NewName != nil {
		if before == nil {
			return out, updateFail(in, "baseline", errs.OutcomeNotAttempted, fmt.Errorf("rename requires an existing label value"))
		}
		desired.Name = *in.NewName
	}
	if in.Category != nil {
		if before != nil && before.Category != *in.Category {
			return out, updateFail(in, "category", errs.OutcomeNotAttempted, fmt.Errorf("an existing label value's category cannot be changed"))
		}
		desired.Category = *in.Category
	}
	if in.Description != nil {
		desired.Description = *in.Description
	}
	if !validName(desired.Category) || !validDescription(desired.Description) {
		return out, updateUsage("creating a label value requires --category and --description")
	}
	if before != nil && before.Internal {
		return out, updateFail(in, "value", errs.OutcomeNotAttempted, fmt.Errorf("system-managed label values cannot be edited here"))
	}
	out.Plan.Desired = desired
	out.Plan.NoOp = before != nil && equal(*before, desired)
	if preview {
		return out, nil
	}
	out.Plan.Mode = "perform"
	if out.Plan.NoOp {
		out.Result = &UpdateResult{Status: "unchanged", Item: *before}
		return out, nil
	}
	current, err := updateBaseline(ctx, reader, in)
	if err != nil {
		return out, updateFail(in, "recheck", errs.OutcomeNotAttempted, err)
	}
	if (before == nil) != (current == nil) || (before != nil && !equal(*before, *current)) {
		return out, updateFail(in, "conflict", errs.OutcomeNotAttempted, fmt.Errorf("label value changed after its baseline was read"))
	}
	oldName := ""
	if in.NewName != nil {
		oldName = in.Name
	}
	saved, err := writer.SetLabelValue(ctx, oldName, desired)
	if err != nil {
		out.Result = &UpdateResult{Status: "unknown", Item: saved}
		if acknowledged, ok := errors.AsType[interface {
			error
			WriteAcknowledged() bool
		}](err); ok && acknowledged.WriteAcknowledged() {
			out.Result.Status = "verification_pending"
			out.Result.Completed = []string{"definition_write"}
			return out, updateFail(in, "verification", errs.OutcomeConfirmed, err)
		}
		return out, updateFail(in, "submission", errs.OutcomeUnknown, err)
	}
	out.Result = &UpdateResult{Status: "verification_pending", Item: saved, Completed: []string{"definition_write"}}
	verified, err := reader.GetLabelValue(ctx, desired.Name)
	if err != nil {
		return out, updateFail(in, "verification", errs.OutcomeConfirmed, err)
	}
	if !equal(verified, desired) {
		return out, updateFail(in, "verification", errs.OutcomeConfirmed, fmt.Errorf("readback differs from the acknowledged definition"))
	}
	out.Result.Status = "updated"
	out.Result.Item = verified
	return out, nil
}
func updateBaseline(ctx context.Context, reader UpdateReader, in UpdateInput) (*value.LabelValue, error) {
	items, err := reader.ListLabelValues(ctx)
	if err != nil {
		return nil, err
	}
	var result *value.LabelValue
	for _, v := range items {
		if v.Name == in.Name {
			copy := v
			result = &copy
		}
	}
	return result, nil
}
func equal(a, b value.LabelValue) bool {
	return a.Name == b.Name && a.Category == b.Category && a.Description == b.Description
}
func validName(s string) bool {
	return strings.TrimSpace(s) == s && s != "" && utf8.RuneCountInString(s) <= 128
}
func validDescription(s string) bool {
	return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= 500
}
func updateUsage(s string) error {
	return &errs.Error{ID: "admin.label.value.update.usage", Kind: errs.KindUsage, Operation: "admin.label.value.update", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Correct the shared label definition and preview its changes."}
}
func updateFail(in UpdateInput, step string, outcome errs.Outcome, cause error) error {
	phase := errs.PhaseValidation
	if outcome == errs.OutcomeUnknown {
		phase = errs.PhaseSubmission
	}
	if step == "verification" {
		phase = errs.PhaseVerification
	}
	return &errs.Error{ID: "admin.label.value.update." + step, Kind: errs.KindOperation, Operation: "admin.label.value.update", Environment: in.Environment, Site: in.Site, Resource: in.Name, Summary: "Label value update failed.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact label definition before retrying; preserve any acknowledged write.", Phase: phase, Outcome: outcome, TableauRequestID: errs.TableauRequestID(cause)}
}
