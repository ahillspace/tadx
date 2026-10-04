package contentlabel

import (
	"context"
	"fmt"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
	"strings"
)

type InspectInput struct {
	Environment, Site, ID string
	Type, TargetID        string
}
type InspectReader interface {
	GetLabel(context.Context, string) (value.ContentLabel, error)
}
type InspectOutput struct {
	Status      string              `json:"status"`
	Environment string              `json:"environment,omitempty"`
	Site        string              `json:"site,omitempty"`
	Item        *value.ContentLabel `json:"item,omitempty"`
}

type InspectCompactLabel struct {
	LUID       string `json:"luid"`
	TargetLUID string `json:"target_luid"`
	Type       string `json:"type"`
	Value      string `json:"value"`
	Category   string `json:"category"`
	Message    string `json:"message"`
	Active     bool   `json:"active"`
	Elevated   bool   `json:"elevated"`
}

func inspectCompactLabel(v value.ContentLabel) InspectCompactLabel {
	return InspectCompactLabel{v.LUID, v.TargetLUID, v.Type, v.Value, v.Category, v.Message, v.Active, v.Elevated}
}
func (o InspectOutput) CompactOutput() any {
	return struct {
		Status      string               `json:"status"`
		Environment string               `json:"environment,omitempty"`
		Site        string               `json:"site,omitempty"`
		Item        *InspectCompactLabel `json:"item,omitempty"`
	}{o.Status, o.Environment, o.Site, inspectCompactPtr(o.Item)}
}
func (o InspectOutput) FullOutput() any { return o }
func inspectCompactPtr(v *value.ContentLabel) *InspectCompactLabel {
	if v == nil {
		return nil
	}
	item := inspectCompactLabel(*v)
	return &item
}
func ValidateInspectInput(in InspectInput) error {
	if strings.TrimSpace(in.ID) == "" || strings.TrimSpace(in.ID) != in.ID {
		return inspectUsage("an exact attachment --id is required")
	}
	if (in.Type == "") != (in.TargetID == "") {
		return inspectUsage("--type and --target-id must be supplied together")
	}
	if in.Type != "" && !supportedKind(in.Type) {
		return inspectUsage("labels support database, table, column, datasource, or flow")
	}
	if strings.TrimSpace(in.TargetID) != in.TargetID {
		return inspectUsage("related asset ID must be exact")
	}
	return nil
}
func inspectLabel(ctx context.Context, reader InspectReader, in InspectInput) (InspectOutput, error) {
	out := InspectOutput{Status: "inspected", Environment: in.Environment, Site: in.Site}
	if reader == nil {
		return out, inspectUsage("label reader is not configured")
	}
	item, err := reader.GetLabel(ctx, in.ID)
	if err != nil {
		return out, inspectFailure(in.Environment, in.Site, err)
	}
	item.Type = value.CanonicalContentType(item.Type)
	if item.LUID != in.ID || (in.Type != "" && item.Type != in.Type) || (in.TargetID != "" && item.TargetLUID != in.TargetID) {
		return out, inspectFailure(in.Environment, in.Site, fmt.Errorf("label identity mismatch: requested attachment=%q type=%q target=%q, returned attachment=%q type=%q target=%q", in.ID, in.Type, in.TargetID, item.LUID, item.Type, item.TargetLUID))
	}
	out.Item = &item
	return out, nil
}

func inspectUsage(s string) error {
	return &errs.Error{ID: "content.label.inspect.usage", Kind: errs.KindUsage, Operation: "content.label.inspect", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Correct the label selection and try again."}
}
func inspectFailure(env, site string, cause error) error {
	r, a := errs.CompleteRetryAdvice(cause, "Check the selected label and your access to it.")
	return &errs.Error{ID: "content.label.inspect.failed", Kind: errs.KindOperation, Operation: "content.label.inspect", Environment: env, Site: site, Summary: "The label read failed.", Cause: cause, Retryable: r, CorrectiveAction: a, TableauRequestID: errs.TableauRequestID(cause)}
}
