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
	GetLabelValue(context.Context, string) (value.LabelValue, error)
}
type Writer interface {
	DeleteLabelValue(context.Context, string) error
}
type Action struct {
	reader Reader
	writer Writer
}

func New(r Reader, w Writer) *Action { return &Action{reader: r, writer: w} }

type Output struct {
	Mode        string           `json:"mode"`
	Operation   string           `json:"operation"`
	Environment string           `json:"environment"`
	Site        string           `json:"site"`
	Item        value.LabelValue `json:"item"`
	Status      string           `json:"status"`
	Effect      string           `json:"effect"`
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
	out := Output{Mode: "preview", Operation: "admin.label.value.delete", Environment: in.Environment, Site: in.Site, Status: "planned"}
	if !in.TargetResolved || strings.TrimSpace(in.Environment) == "" {
		return out, usage("a resolved Tableau environment is required")
	}
	if a == nil || a.reader == nil {
		return out, usage("label value reader is not configured")
	}
	before, e := a.reader.GetLabelValue(ctx, in.Name)
	if e != nil {
		return out, fail(in, "read", errs.OutcomeNotAttempted, e)
	}
	if before.Name != in.Name {
		return out, fail(in, "identity", errs.OutcomeNotAttempted, fmt.Errorf("label value name did not match"))
	}
	out.Item = before
	if before.Internal {
		return out, fail(in, "value", errs.OutcomeNotAttempted, fmt.Errorf("system-managed label values cannot be deleted here"))
	}
	if before.BuiltIn {
		out.Effect = "Reset the customized built-in label definition to its default; existing uses may change."
	} else {
		out.Effect = "Delete this shared label definition; existing uses may be affected."
	}
	if preview {
		return out, nil
	}
	if a.writer == nil {
		return out, usage("label value writer is not configured")
	}
	current, e := a.reader.GetLabelValue(ctx, in.Name)
	if e != nil {
		return out, fail(in, "recheck", errs.OutcomeNotAttempted, e)
	}
	if current != before {
		return out, fail(in, "conflict", errs.OutcomeNotAttempted, fmt.Errorf("definition changed since baseline read"))
	}
	out.Mode = "perform"
	out.Status = "unknown"
	if e := a.writer.DeleteLabelValue(ctx, in.Name); e != nil {
		return out, fail(in, "submission", errs.OutcomeUnknown, e)
	}
	if before.BuiltIn {
		out.Status = "reset"
	} else {
		out.Status = "deleted"
	}
	return out, nil
}
func usage(s string) error {
	return &errs.Error{ID: "admin.label.value.delete.usage", Kind: errs.KindUsage, Operation: "admin.label.value.delete", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Select an exact shared label definition name and preview deletion."}
}
func fail(in Input, step string, outcome errs.Outcome, cause error) error {
	phase := errs.PhaseValidation
	if outcome == errs.OutcomeUnknown {
		phase = errs.PhaseSubmission
	}
	return &errs.Error{ID: "admin.label.value.delete." + step, Kind: errs.KindOperation, Operation: "admin.label.value.delete", Environment: in.Environment, Site: in.Site, Resource: in.Name, Summary: "Label value deletion failed.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the shared definition and its uses before retrying deletion.", Phase: phase, Outcome: outcome, TableauRequestID: errs.TableauRequestID(cause)}
}
