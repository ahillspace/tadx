package create

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
	Environment, Site string
	TargetResolved    bool
	Name, Description string
}
type Reader interface {
	ListLabelCategories(context.Context) ([]value.LabelCategory, error)
	GetLabelCategory(context.Context, string) (value.LabelCategory, error)
}
type Writer interface {
	CreateLabelCategory(context.Context, value.LabelCategory) (value.LabelCategory, error)
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
	if !validDescription(in.Description) {
		return usage("description must contain 1 to 500 characters")
	}
	return nil
}
func (a *Action) Execute(ctx context.Context, in Input, preview bool) (Output, error) {
	if e := ValidateInput(in); e != nil {
		return Output{}, e
	}
	out := Output{Plan: Plan{Mode: "preview", Operation: "admin.label.category.create", Environment: in.Environment, Site: in.Site, Name: in.Name, Effect: "Creates a new shared category; an existing exact name is rejected."}}
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
	if before != nil {
		return out, fail(in, "exists", errs.OutcomeNotAttempted, fmt.Errorf("category already exists; use update"))
	}
	desired := value.LabelCategory{Name: in.Name, Description: in.Description}
	out.Plan.Desired = desired
	if preview {
		return out, nil
	}
	out.Plan.Mode = "perform"
	if a.writer == nil {
		return out, usage("label category writer is not configured")
	}
	current, err := a.baseline(ctx, in)
	if err != nil {
		return out, fail(in, "recheck", errs.OutcomeNotAttempted, err)
	}
	if current != nil {
		return out, fail(in, "conflict", errs.OutcomeNotAttempted, fmt.Errorf("label category changed after its baseline was read"))
	}
	saved, err := a.writer.CreateLabelCategory(ctx, desired)
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
	out.Result.Status = "created"
	out.Result.Item = verified
	return out, nil
}
func (a *Action) baseline(ctx context.Context, in Input) (*value.LabelCategory, error) {
	items, err := a.reader.ListLabelCategories(ctx)
	if err != nil {
		return nil, err
	}
	if len(items) > 10000 {
		return nil, fmt.Errorf("label categories exceed collection bound")
	}
	for _, v := range items {
		if v.Name == in.Name {
			copy := v
			return &copy, nil
		}
	}
	return nil, nil
}
func equal(a, b value.LabelCategory) bool { return a.Name == b.Name && a.Description == b.Description }
func validName(s string) bool {
	return strings.TrimSpace(s) == s && s != "" && utf8.RuneCountInString(s) <= 128
}
func validDescription(s string) bool {
	return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= 500
}
func usage(s string) error {
	return &errs.Error{ID: "admin.label.category.create.usage", Kind: errs.KindUsage, Operation: "admin.label.category.create", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Correct the shared label definition and preview its changes."}
}
func fail(in Input, step string, outcome errs.Outcome, cause error) error {
	phase := errs.PhaseValidation
	if outcome == errs.OutcomeUnknown {
		phase = errs.PhaseSubmission
	}
	if step == "verification" {
		phase = errs.PhaseVerification
	}
	return &errs.Error{ID: "admin.label.category.create." + step, Kind: errs.KindOperation, Operation: "admin.label.category.create", Environment: in.Environment, Site: in.Site, Resource: in.Name, Summary: "Label category create failed.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact label definition before retrying; preserve any acknowledged write.", Phase: phase, Outcome: outcome, TableauRequestID: errs.TableauRequestID(cause)}
}
