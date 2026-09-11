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
	Environment, Site              string
	TargetResolved                 bool
	Name                           string
	NewName, Category, Description *string
}
type Reader interface {
	ListLabelValues(context.Context) ([]value.LabelValue, error)
	GetLabelValue(context.Context, string) (value.LabelValue, error)
}
type Writer interface {
	SetLabelValue(context.Context, string, value.LabelValue) (value.LabelValue, error)
}
type Action struct {
	reader Reader
	writer Writer
}

func New(r Reader, w Writer) *Action { return &Action{reader: r, writer: w} }

type Plan struct {
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
type Result struct {
	Status    string           `json:"status"`
	Item      value.LabelValue `json:"item"`
	Completed []string         `json:"completed,omitempty"`
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
	if in.NewName == nil && in.Category == nil && in.Description == nil {
		return usage("supply a label value change")
	}
	if in.NewName != nil && !validName(*in.NewName) {
		return usage("new name must contain 1 to 128 characters without surrounding whitespace")
	}
	if in.Category != nil && !validName(*in.Category) {
		return usage("category must be an exact nonempty name")
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
	out := Output{Plan: Plan{Mode: "preview", Operation: "admin.label.value.update", Environment: in.Environment, Site: in.Site, Name: in.Name, Effect: "Native create-or-update; it may create the named value if absent."}}
	if !in.TargetResolved || strings.TrimSpace(in.Environment) == "" {
		return out, usage("a resolved Tableau environment is required")
	}
	if a == nil || a.reader == nil {
		return out, usage("label value reader is not configured")
	}
	before, err := a.baseline(ctx, in)
	if err != nil {
		return out, fail(in, "read", errs.OutcomeNotAttempted, err)
	}
	out.Plan.Before = before
	desired := value.LabelValue{Name: in.Name}
	if before != nil {
		desired = *before
	}
	if in.NewName != nil {
		if before == nil {
			return out, fail(in, "baseline", errs.OutcomeNotAttempted, fmt.Errorf("rename requires an existing label value"))
		}
		desired.Name = *in.NewName
	}
	if in.Category != nil {
		if before != nil && before.Category != *in.Category {
			return out, fail(in, "category", errs.OutcomeNotAttempted, fmt.Errorf("an existing label value's category cannot be changed"))
		}
		desired.Category = *in.Category
	}
	if in.Description != nil {
		desired.Description = *in.Description
	}
	if !validName(desired.Category) || !validDescription(desired.Description) {
		return out, usage("creating a label value requires --category and --description")
	}
	if before != nil && before.Internal {
		return out, fail(in, "value", errs.OutcomeNotAttempted, fmt.Errorf("system-managed label values cannot be edited here"))
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
		return out, usage("label value writer is not configured")
	}
	current, err := a.baseline(ctx, in)
	if err != nil {
		return out, fail(in, "recheck", errs.OutcomeNotAttempted, err)
	}
	if (before == nil) != (current == nil) || (before != nil && !equal(*before, *current)) {
		return out, fail(in, "conflict", errs.OutcomeNotAttempted, fmt.Errorf("label value changed after its baseline was read"))
	}
	oldName := ""
	if in.NewName != nil {
		oldName = in.Name
	}
	saved, err := a.writer.SetLabelValue(ctx, oldName, desired)
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
	verified, err := a.reader.GetLabelValue(ctx, desired.Name)
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
func (a *Action) baseline(ctx context.Context, in Input) (*value.LabelValue, error) {
	items, err := a.reader.ListLabelValues(ctx)
	if err != nil {
		return nil, err
	}
	if len(items) > 10000 {
		return nil, fmt.Errorf("label values exceed collection bound")
	}
	var result *value.LabelValue
	for _, v := range items {
		if v.Name == in.Name {
			if result != nil {
				return nil, fmt.Errorf("label name is ambiguous")
			}
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
func usage(s string) error {
	return &errs.Error{ID: "admin.label.value.update.usage", Kind: errs.KindUsage, Operation: "admin.label.value.update", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Correct the shared label definition and preview its changes."}
}
func fail(in Input, step string, outcome errs.Outcome, cause error) error {
	phase := errs.PhaseValidation
	if outcome == errs.OutcomeUnknown {
		phase = errs.PhaseSubmission
	}
	if step == "verification" {
		phase = errs.PhaseVerification
	}
	return &errs.Error{ID: "admin.label.value.update." + step, Kind: errs.KindOperation, Operation: "admin.label.value.update", Environment: in.Environment, Site: in.Site, Resource: in.Name, Summary: "Label value update failed.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact label definition before retrying; preserve any acknowledged write.", Phase: phase, Outcome: outcome, TableauRequestID: errs.TableauRequestID(cause)}
}
