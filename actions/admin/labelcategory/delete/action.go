package delete

import (
	"context"
	"fmt"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
	"strings"
	"unicode/utf8"
)

type Input struct {
	Environment, Site, Name string
	TargetResolved          bool
}
type Reader interface {
	GetLabelCategory(context.Context, string) (value.LabelCategory, error)
	ListLabelValues(context.Context) ([]value.LabelValue, error)
}
type Writer interface {
	DeleteLabelCategory(context.Context, string) error
}
type Action struct {
	reader Reader
	writer Writer
}

func New(r Reader, w Writer) *Action { return &Action{reader: r, writer: w} }

type Output struct {
	Mode        string              `json:"mode"`
	Operation   string              `json:"operation"`
	Environment string              `json:"environment"`
	Site        string              `json:"site"`
	Item        value.LabelCategory `json:"item"`
	Status      string              `json:"status"`
	Effect      string              `json:"effect"`
}

func (o Output) CompactOutput() any { return o }
func (o Output) FullOutput() any    { return o }
func ValidateInput(in Input) error {
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Name) != in.Name || utf8.RuneCountInString(in.Name) > 128 {
		return usage("an exact --name containing 1 to 128 characters is required")
	}
	return nil
}
func (a *Action) Execute(ctx context.Context, in Input, preview bool) (Output, error) {
	if e := ValidateInput(in); e != nil {
		return Output{}, e
	}
	out := Output{Mode: "preview", Operation: "admin.label.category.delete", Environment: in.Environment, Site: in.Site, Status: "planned"}
	if !in.TargetResolved || strings.TrimSpace(in.Environment) == "" {
		return out, usage("a resolved Tableau environment is required")
	}
	if a == nil || a.reader == nil {
		return out, usage("label category reader is not configured")
	}
	before, e := a.reader.GetLabelCategory(ctx, in.Name)
	if e != nil {
		return out, fail(in, "read", errs.OutcomeNotAttempted, e)
	}
	if before.Name != in.Name {
		return out, fail(in, "identity", errs.OutcomeNotAttempted, fmt.Errorf("label category name did not match"))
	}
	out.Item = before
	values, e := a.reader.ListLabelValues(ctx)
	if e != nil {
		return out, fail(in, "members", errs.OutcomeNotAttempted, e)
	}
	if len(values) > 10000 {
		return out, fail(in, "members", errs.OutcomeNotAttempted, fmt.Errorf("label value collection exceeds bound"))
	}
	for _, v := range values {
		if v.Category == in.Name {
			return out, fail(in, "members", errs.OutcomeNotAttempted, fmt.Errorf("category still contains label values; resolve them before deletion"))
		}
	}
	out.Effect = "Delete this empty shared label category; Tableau enforces built-in restrictions."
	if preview {
		return out, nil
	}
	if a.writer == nil {
		return out, usage("label category writer is not configured")
	}
	current, e := a.reader.GetLabelCategory(ctx, in.Name)
	if e != nil {
		return out, fail(in, "recheck", errs.OutcomeNotAttempted, e)
	}
	if current != before {
		return out, fail(in, "conflict", errs.OutcomeNotAttempted, fmt.Errorf("definition changed since baseline read"))
	}
	out.Mode = "perform"
	out.Status = "unknown"
	if e := a.writer.DeleteLabelCategory(ctx, in.Name); e != nil {
		return out, fail(in, "submission", errs.OutcomeUnknown, e)
	}
	out.Status = "deleted"
	return out, nil
}
func usage(s string) error {
	return &errs.Error{ID: "admin.label.category.delete.usage", Kind: errs.KindUsage, Operation: "admin.label.category.delete", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Select an exact shared label definition name and preview deletion."}
}
func fail(in Input, step string, outcome errs.Outcome, cause error) error {
	phase := errs.PhaseValidation
	if outcome == errs.OutcomeUnknown {
		phase = errs.PhaseSubmission
	}
	return &errs.Error{ID: "admin.label.category.delete." + step, Kind: errs.KindOperation, Operation: "admin.label.category.delete", Environment: in.Environment, Site: in.Site, Resource: in.Name, Summary: "Label category deletion failed.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the shared definition and its uses before retrying deletion.", Phase: phase, Outcome: outcome, TableauRequestID: errs.TableauRequestID(cause)}
}
