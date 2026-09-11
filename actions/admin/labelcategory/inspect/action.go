package inspect

import (
	"context"
	"fmt"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
	"strings"
)

type Input struct{ Environment, Site, Name string }
type Reader interface {
	GetLabelCategory(context.Context, string) (value.LabelCategory, error)
}
type Action struct{ reader Reader }

func New(r Reader) *Action { return &Action{reader: r} }

type Output struct {
	Status      string              `json:"status"`
	Environment string              `json:"environment,omitempty"`
	Site        string              `json:"site,omitempty"`
	Item        value.LabelCategory `json:"item"`
}

type CompactLabel struct {
	Name string `json:"name"`
}

func compact(v value.LabelCategory) CompactLabel { return CompactLabel{v.Name} }
func (o Output) CompactOutput() any {
	return struct {
		Status      string       `json:"status"`
		Environment string       `json:"environment,omitempty"`
		Site        string       `json:"site,omitempty"`
		Item        CompactLabel `json:"item"`
	}{o.Status, o.Environment, o.Site, compact(o.Item)}
}
func (o Output) FullOutput() any { return o }
func ValidateInput(in Input) error {
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Name) != in.Name {
		return usage("an exact --name is required")
	}
	return nil
}
func (a *Action) Execute(ctx context.Context, in Input) (Output, error) {
	if err := ValidateInput(in); err != nil {
		return Output{}, err
	}
	out := Output{Status: "inspected", Environment: in.Environment, Site: in.Site}
	if a == nil || a.reader == nil {
		return out, usage("label reader is not configured")
	}
	item, err := a.reader.GetLabelCategory(ctx, in.Name)
	if err != nil {
		return out, failure(in.Environment, in.Site, err)
	}
	if item.Name != in.Name {
		return out, failure(in.Environment, in.Site, fmt.Errorf("label category name did not match"))
	}
	out.Item = item
	return out, nil
}

func usage(s string) error {
	return &errs.Error{ID: "admin.label.category.inspect.usage", Kind: errs.KindUsage, Operation: "admin.label.category.inspect", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Correct the label selection and try again."}
}
func failure(env, site string, cause error) error {
	r, a := errs.CompleteRetryAdvice(cause, "Check the selected label and your access to it.")
	return &errs.Error{ID: "admin.label.category.inspect.failed", Kind: errs.KindOperation, Operation: "admin.label.category.inspect", Environment: env, Site: site, Summary: "The label read failed.", Cause: cause, Retryable: r, CorrectiveAction: a, TableauRequestID: errs.TableauRequestID(cause)}
}
