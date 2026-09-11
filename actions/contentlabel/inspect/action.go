package inspect

import (
	"context"
	"fmt"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
	"strings"
)

type Input struct {
	Environment, Site, ID string
	Type, TargetID        string
}
type Reader interface {
	GetLabel(context.Context, string) (value.ContentLabel, error)
}
type Action struct{ reader Reader }

func New(r Reader) *Action { return &Action{reader: r} }

type Output struct {
	Status      string             `json:"status"`
	Environment string             `json:"environment,omitempty"`
	Site        string             `json:"site,omitempty"`
	Item        value.ContentLabel `json:"item"`
}

type CompactLabel struct {
	ID       string `json:"id"`
	TargetID string `json:"target_id"`
	Type     string `json:"type"`
	Value    string `json:"value"`
	Category string `json:"category"`
	Active   bool   `json:"active"`
	Elevated bool   `json:"elevated"`
}

func compact(v value.ContentLabel) CompactLabel {
	return CompactLabel{v.LUID, v.TargetLUID, v.Type, v.Value, v.Category, v.Active, v.Elevated}
}
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
	if strings.TrimSpace(in.ID) == "" || strings.TrimSpace(in.ID) != in.ID {
		return usage("an exact attachment --id is required")
	}
	if (in.Type == "") != (in.TargetID == "") {
		return usage("--type and --target-id must be supplied together")
	}
	if in.Type != "" && (in.Type != "database" && in.Type != "table" && in.Type != "column" && in.Type != "datasource" && in.Type != "flow") {
		return usage("labels support database, table, column, datasource, or flow")
	}
	if strings.TrimSpace(in.TargetID) != in.TargetID {
		return usage("related asset ID must be exact")
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
	item, err := a.reader.GetLabel(ctx, in.ID)
	if err != nil {
		return out, failure(in.Environment, in.Site, err)
	}
	if item.LUID != in.ID || (in.Type != "" && item.Type != in.Type) || (in.TargetID != "" && item.TargetLUID != in.TargetID) {
		return out, failure(in.Environment, in.Site, fmt.Errorf("label identity or related asset did not match"))
	}
	out.Item = item
	return out, nil
}

func usage(s string) error {
	return &errs.Error{ID: "content.label.inspect.usage", Kind: errs.KindUsage, Operation: "content.label.inspect", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Correct the label selection and try again."}
}
func failure(env, site string, cause error) error {
	r, a := errs.CompleteRetryAdvice(cause, "Check the selected label and your access to it.")
	return &errs.Error{ID: "content.label.inspect.failed", Kind: errs.KindOperation, Operation: "content.label.inspect", Environment: env, Site: site, Summary: "The label read failed.", Cause: cause, Retryable: r, CorrectiveAction: a, TableauRequestID: errs.TableauRequestID(cause)}
}
