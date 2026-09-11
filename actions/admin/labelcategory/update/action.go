package update

import (
	"context"
	"errors"
	"fmt"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
	"strings"
	"unicode/utf8"
)

type Input struct {
	Environment, Site    string
	TargetResolved       bool
	Name                 string
	NewName, Description *string
}
type Reader interface {
	GetLabelCategory(context.Context, string) (value.LabelCategory, error)
}
type Writer interface {
	UpdateLabelCategory(context.Context, string, value.LabelCategory) (value.LabelCategory, error)
}
type Action struct {
	reader Reader
	writer Writer
}

func New(r Reader, w Writer) *Action { return &Action{reader: r, writer: w} }

type Plan struct {
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
type Result struct {
	Status    string              `json:"status"`
	Item      value.LabelCategory `json:"item"`
	Completed []string            `json:"completed,omitempty"`
}
type Output struct {
	Plan   Plan    `json:"plan"`
	Result *Result `json:"result,omitempty"`
}

func (o Output) CompactOutput() any { return o }
func (o Output) FullOutput() any    { return o }
func ValidateInput(in Input) error {
	if !validName(in.Name) {
		return usage("an exact --name containing 1 to 128 characters is required")
	}
	if in.NewName == nil && in.Description == nil {
		return usage("supply a new name or description")
	}
	if in.NewName != nil && !validName(*in.NewName) {
		return usage("new name must contain 1 to 128 characters")
	}
	if in.Description != nil && !validDescription(*in.Description) {
		return usage("description must contain 1 to 500 characters")
	}
	return nil
}
func (a *Action) Execute(ctx context.Context, in Input, preview bool) (Output, error) {
	if e := ValidateInput(in); e != nil {
		return Output{}, e
	}
	out := Output{Plan: Plan{Mode: "preview", Operation: "admin.label.category.update", Environment: in.Environment, Site: in.Site, Name: in.Name, Effect: "Updates the selected category definition; all uses reference that shared definition."}}
	if !in.TargetResolved || strings.TrimSpace(in.Environment) == "" {
		return out, usage("a resolved Tableau environment is required")
	}
	if a == nil || a.reader == nil {
		return out, usage("label category reader is not configured")
	}
	before, err := a.baseline(ctx, in)
	if err != nil {
		return out, fail(in, "read", errs.OutcomeNotAttempted, err)
	}
	out.Plan.Before = before
	desired := *before
	if in.NewName != nil {
		desired.Name = *in.NewName
	}
	if in.Description != nil {
		desired.Description = *in.Description
	}
	out.Plan.Desired = desired
	out.Plan.NoOp = before != nil && equal(*before, desired)
	if preview {
		return out, nil
	}
	out.Plan.Mode = "perform"
	if out.Plan.NoOp {
		out.Result = &Result{Status: "unchanged", Item: *before}
		return out, nil
	}
	if a.writer == nil {
		return out, usage("label category writer is not configured")
	}
	current, err := a.baseline(ctx, in)
	if err != nil {
		return out, fail(in, "recheck", errs.OutcomeNotAttempted, err)
	}
	if (before == nil) != (current == nil) || (before != nil && !equal(*before, *current)) {
		return out, fail(in, "conflict", errs.OutcomeNotAttempted, fmt.Errorf("label category changed after its baseline was read"))
	}
	saved, err := a.writer.UpdateLabelCategory(ctx, in.Name, desired)
	if err != nil {
		out.Result = &Result{Status: "unknown", Item: saved}
		var acknowledged interface{ WriteAcknowledged() bool }
		if errors.As(err, &acknowledged) && acknowledged.WriteAcknowledged() {
			out.Result.Status = "verification_pending"
			out.Result.Completed = []string{"definition_write"}
			return out, fail(in, "verification", errs.OutcomeConfirmed, err)
		}
		return out, fail(in, "submission", errs.OutcomeUnknown, err)
	}
	out.Result = &Result{Status: "verification_pending", Item: saved, Completed: []string{"definition_write"}}
	if !equal(saved, desired) {
		return out, fail(in, "verification", errs.OutcomeConfirmed, fmt.Errorf("saved definition differs from requested values"))
	}
	verified, err := a.reader.GetLabelCategory(ctx, desired.Name)
	if err != nil {
		return out, fail(in, "verification", errs.OutcomeConfirmed, err)
	}
	if !equal(verified, desired) {
		return out, fail(in, "verification", errs.OutcomeConfirmed, fmt.Errorf("readback differs from the acknowledged definition"))
	}
	out.Result.Status = "updated"
	out.Result.Item = verified
	return out, nil
}
func (a *Action) baseline(ctx context.Context, in Input) (*value.LabelCategory, error) {
	v, err := a.reader.GetLabelCategory(ctx, in.Name)
	if err != nil {
		return nil, err
	}
	if v.Name != in.Name {
		return nil, fmt.Errorf("category name does not match")
	}
	return &v, nil
}
func equal(a, b value.LabelCategory) bool { return a.Name == b.Name && a.Description == b.Description }
func validName(s string) bool {
	return strings.TrimSpace(s) == s && s != "" && utf8.RuneCountInString(s) <= 128
}
func validDescription(s string) bool {
	return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= 500
}
func usage(s string) error {
	return &errs.Error{ID: "admin.label.category.update.usage", Kind: errs.KindUsage, Operation: "admin.label.category.update", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Correct the shared label definition and preview its changes."}
}
func fail(in Input, step string, outcome errs.Outcome, cause error) error {
	phase := errs.PhaseValidation
	if outcome == errs.OutcomeUnknown {
		phase = errs.PhaseSubmission
	}
	if step == "verification" {
		phase = errs.PhaseVerification
	}
	return &errs.Error{ID: "admin.label.category.update." + step, Kind: errs.KindOperation, Operation: "admin.label.category.update", Environment: in.Environment, Site: in.Site, Resource: in.Name, Summary: "Label category update failed.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact label definition before retrying; preserve any acknowledged write.", Phase: phase, Outcome: outcome, TableauRequestID: errs.TableauRequestID(cause)}
}
