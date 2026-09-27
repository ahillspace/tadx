package labelcategory

import (
	"context"
	"fmt"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type DeleteInput struct {
	Environment, Site, Name string
}
type DeleteReader interface {
	GetLabelCategory(context.Context, string) (value.LabelCategory, error)
	ListLabelValues(context.Context) ([]value.LabelValue, error)
}
type DeleteWriter interface {
	DeleteLabelCategory(context.Context, string) error
}

type DeleteOutput struct {
	Mode        string              `json:"mode"`
	Operation   string              `json:"operation"`
	Environment string              `json:"environment"`
	Site        string              `json:"site"`
	Item        value.LabelCategory `json:"item"`
	Status      string              `json:"status"`
	Effect      string              `json:"effect"`
}

func (o DeleteOutput) CompactOutput() any { return o }
func (o DeleteOutput) FullOutput() any    { return o }
func ValidateDeleteInput(in DeleteInput) error {
	if !validName(in.Name) {
		return deleteUsage("an exact --name containing 1 to 128 characters is required")
	}
	return nil
}
func Delete(ctx context.Context, reader DeleteReader, writer DeleteWriter, in DeleteInput, preview bool) (DeleteOutput, error) {
	out := DeleteOutput{Mode: "preview", Operation: "admin.label.category.delete", Environment: in.Environment, Site: in.Site, Status: "planned"}
	before, e := reader.GetLabelCategory(ctx, in.Name)
	if e != nil {
		return out, deleteFail(in, "read", errs.OutcomeNotAttempted, e)
	}
	out.Item = before
	values, e := reader.ListLabelValues(ctx)
	if e != nil {
		return out, deleteFail(in, "members", errs.OutcomeNotAttempted, e)
	}
	for _, v := range values {
		if v.Category == in.Name {
			return out, deleteFail(in, "members", errs.OutcomeNotAttempted, fmt.Errorf("category still contains label values; resolve them before deletion"))
		}
	}
	out.Effect = "Delete this empty shared label category; Tableau enforces built-in restrictions."
	if preview {
		return out, nil
	}
	current, e := reader.GetLabelCategory(ctx, in.Name)
	if e != nil {
		return out, deleteFail(in, "recheck", errs.OutcomeNotAttempted, e)
	}
	if current != before {
		return out, deleteFail(in, "conflict", errs.OutcomeNotAttempted, fmt.Errorf("definition changed since baseline read"))
	}
	out.Mode = "perform"
	out.Status = "unknown"
	if e := writer.DeleteLabelCategory(ctx, in.Name); e != nil {
		return out, deleteFail(in, "submission", errs.OutcomeUnknown, e)
	}
	out.Status = "deleted"
	return out, nil
}
func deleteUsage(s string) error {
	return &errs.Error{ID: "admin.label.category.delete.usage", Kind: errs.KindUsage, Operation: "admin.label.category.delete", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Select an exact shared label definition name and preview deletion."}
}
func deleteFail(in DeleteInput, step string, outcome errs.Outcome, cause error) error {
	phase := errs.PhaseValidation
	if outcome == errs.OutcomeUnknown {
		phase = errs.PhaseSubmission
	}
	return &errs.Error{ID: "admin.label.category.delete." + step, Kind: errs.KindOperation, Operation: "admin.label.category.delete", Environment: in.Environment, Site: in.Site, Resource: in.Name, Summary: "Label category deletion failed.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the shared definition and its uses before retrying deletion.", Phase: phase, Outcome: outcome, TableauRequestID: errs.TableauRequestID(cause)}
}
