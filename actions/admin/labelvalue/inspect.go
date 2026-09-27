package labelvalue

import (
	"context"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type InspectInput struct{ Environment, Site, Name string }
type InspectReader interface {
	GetLabelValue(context.Context, string) (value.LabelValue, error)
}

type InspectOutput struct {
	Status      string           `json:"status"`
	Environment string           `json:"environment,omitempty"`
	Site        string           `json:"site,omitempty"`
	Item        value.LabelValue `json:"item"`
}

func (o InspectOutput) CompactOutput() any {
	return struct {
		Status      string       `json:"status"`
		Environment string       `json:"environment,omitempty"`
		Site        string       `json:"site,omitempty"`
		Item        CompactLabel `json:"item"`
	}{o.Status, o.Environment, o.Site, compact(o.Item)}
}
func (o InspectOutput) FullOutput() any { return o }
func ValidateInspectInput(in InspectInput) error {
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Name) != in.Name {
		return inspectUsage("an exact --name is required")
	}
	return nil
}
func Inspect(ctx context.Context, reader InspectReader, in InspectInput) (InspectOutput, error) {
	out := InspectOutput{Status: "inspected", Environment: in.Environment, Site: in.Site}
	item, err := reader.GetLabelValue(ctx, in.Name)
	if err != nil {
		return out, inspectFailure(in.Environment, in.Site, err)
	}
	out.Item = item
	return out, nil
}

func inspectUsage(s string) error {
	return &errs.Error{ID: "admin.label.value.inspect.usage", Kind: errs.KindUsage, Operation: "admin.label.value.inspect", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Correct the label selection and try again."}
}
func inspectFailure(env, site string, cause error) error {
	r, a := errs.CompleteRetryAdvice(cause, "Check the selected label and your access to it.")
	return &errs.Error{ID: "admin.label.value.inspect.failed", Kind: errs.KindOperation, Operation: "admin.label.value.inspect", Environment: env, Site: site, Summary: "The label read failed.", Cause: cause, Retryable: r, CorrectiveAction: a, TableauRequestID: errs.TableauRequestID(cause)}
}
