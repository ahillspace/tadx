package labelcategory

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
	Environment, Site    string
	Name                 string
	NewName, Description *string
}
type UpdateReader interface {
	GetLabelCategory(context.Context, string) (value.LabelCategory, error)
}
type UpdateWriter interface {
	UpdateLabelCategory(context.Context, string, value.LabelCategory) (value.LabelCategory, error)
}

type WritePlan struct {
	Mode        string               `json:"mode"`
	Operation   string               `json:"operation"`
	Environment string               `json:"environment"`
	Site        string               `json:"site"`
	Name        string               `json:"name"`
	Before      *value.LabelCategory `json:"before,omitempty"`
	Desired     value.LabelCategory  `json:"desired"`
	NoOp        bool                 `json:"no_op"`
	Effect      string               `json:"effect"`
}
type WriteResult struct {
	Status    string              `json:"status"`
	Item      value.LabelCategory `json:"item"`
	Completed []string            `json:"completed,omitempty"`
}
type WriteOutput struct {
	Plan   WritePlan    `json:"plan"`
	Result *WriteResult `json:"result,omitempty"`
}

func (o WriteOutput) CompactOutput() any { return o }
func (o WriteOutput) FullOutput() any    { return o }
func ValidateUpdateInput(in UpdateInput) error {
	if !validName(in.Name) {
		return updateUsage("an exact --name containing 1 to 128 characters is required")
	}
	if in.NewName == nil && in.Description == nil {
		return updateUsage("supply a new name or description")
	}
	if in.NewName != nil && !validName(*in.NewName) {
		return updateUsage("new name must contain 1 to 128 characters")
	}
	if in.Description != nil && !validDescription(*in.Description) {
		return updateUsage("description must contain 1 to 500 characters")
	}
	return nil
}
func Update(ctx context.Context, reader UpdateReader, writer UpdateWriter, in UpdateInput, preview bool) (WriteOutput, error) {
	out := WriteOutput{Plan: WritePlan{Mode: "preview", Operation: "admin.label.category.update", Environment: in.Environment, Site: in.Site, Name: in.Name, Effect: "Updates the selected category definition; all uses reference that shared definition."}}
	before, err := reader.GetLabelCategory(ctx, in.Name)
	if err != nil {
		return out, updateFail(in, "read", errs.OutcomeNotAttempted, err)
	}
	out.Plan.Before = &before
	desired := before
	if in.NewName != nil {
		desired.Name = *in.NewName
	}
	if in.Description != nil {
		desired.Description = *in.Description
	}
	out.Plan.Desired = desired
	out.Plan.NoOp = equal(before, desired)
	if preview {
		return out, nil
	}
	out.Plan.Mode = "perform"
	if out.Plan.NoOp {
		out.Result = &WriteResult{Status: "unchanged", Item: before}
		return out, nil
	}
	current, err := reader.GetLabelCategory(ctx, in.Name)
	if err != nil {
		return out, updateFail(in, "recheck", errs.OutcomeNotAttempted, err)
	}
	if !equal(before, current) {
		return out, updateFail(in, "conflict", errs.OutcomeNotAttempted, fmt.Errorf("label category changed after its baseline was read"))
	}
	saved, err := writer.UpdateLabelCategory(ctx, in.Name, desired)
	if err != nil {
		out.Result = &WriteResult{Status: "unknown", Item: saved}
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
	out.Result = &WriteResult{Status: "verification_pending", Item: saved, Completed: []string{"definition_write"}}
	verified, err := reader.GetLabelCategory(ctx, desired.Name)
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
func equal(a, b value.LabelCategory) bool {
	return a.Name == b.Name && a.Description == b.Description
}
func validName(s string) bool {
	return strings.TrimSpace(s) == s && s != "" && utf8.RuneCountInString(s) <= 128
}
func validDescription(s string) bool {
	return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= 500
}
func updateUsage(s string) error {
	return &errs.Error{ID: "admin.label.category.update.usage", Kind: errs.KindUsage, Operation: "admin.label.category.update", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Correct the shared label definition and preview its changes."}
}
func updateFail(in UpdateInput, step string, outcome errs.Outcome, cause error) error {
	phase := errs.PhaseValidation
	if outcome == errs.OutcomeUnknown {
		phase = errs.PhaseSubmission
	}
	if step == "verification" {
		phase = errs.PhaseVerification
	}
	return &errs.Error{ID: "admin.label.category.update." + step, Kind: errs.KindOperation, Operation: "admin.label.category.update", Environment: in.Environment, Site: in.Site, Resource: in.Name, Summary: "Label category update failed.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact label definition before retrying; preserve any acknowledged write.", Phase: phase, Outcome: outcome, TableauRequestID: errs.TableauRequestID(cause)}
}
