package update

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type Input struct {
	Environment, Site, ID, Type, TargetID string
	TargetResolved                        bool
	Value, Message                        *string
	Active, Elevated                      *bool
}
type Reader interface {
	GetLabel(context.Context, string) (value.ContentLabel, error)
	GetLabels(context.Context, value.LabelTarget, []string) ([]value.ContentLabel, error)
	GetLabelValue(context.Context, string) (value.LabelValue, error)
}
type Writer interface {
	SetLabel(context.Context, value.LabelTarget, value.LabelUpdate) (value.ContentLabel, error)
	UpdateLabel(context.Context, string, value.LabelUpdate) (value.ContentLabel, error)
}
type Action struct {
	reader Reader
	writer Writer
}

func New(r Reader, w Writer) *Action { return &Action{reader: r, writer: w} }

type Plan struct {
	Mode        string              `json:"mode"`
	Operation   string              `json:"operation"`
	Environment string              `json:"environment"`
	Site        string              `json:"site"`
	Target      value.LabelTarget   `json:"target"`
	Before      *value.ContentLabel `json:"before,omitempty"`
	Desired     value.LabelUpdate   `json:"desired"`
	NoOp        bool                `json:"no_op"`
}
type Result struct {
	Status    string             `json:"status"`
	Item      value.ContentLabel `json:"item"`
	Completed []string           `json:"completed,omitempty"`
}
type Output struct {
	Plan   Plan    `json:"plan"`
	Result *Result `json:"result,omitempty"`
}

func (o Output) CompactOutput() any { return o }
func (o Output) FullOutput() any    { return o }
func ValidateInput(in Input) error {
	if in.ID == "" {
		if !allowed(in.Type) || strings.TrimSpace(in.TargetID) == "" || in.Value == nil {
			return usage("select an attachment --id, or --type and --target-id with --value")
		}
	} else if strings.TrimSpace(in.ID) != in.ID {
		return usage("attachment ID must be exact")
	}
	if (in.Type == "") != (in.TargetID == "") {
		return usage("--type and --target-id must be supplied together")
	}
	if in.Type != "" && !allowed(in.Type) {
		return usage("labels support database, table, column, datasource, or flow")
	}
	if strings.TrimSpace(in.TargetID) != in.TargetID {
		return usage("related asset ID must be exact")
	}
	if in.Value == nil && in.Message == nil && in.Active == nil && in.Elevated == nil {
		return usage("supply at least one label change")
	}
	if in.Value != nil && (strings.TrimSpace(*in.Value) == "" || strings.TrimSpace(*in.Value) != *in.Value || len(*in.Value) > 512) {
		return usage("label value must be an exact nonempty name")
	}
	if in.Message != nil && len(*in.Message) > 65536 {
		return usage("label message exceeds 65536 bytes")
	}
	return nil
}
func (a *Action) Execute(ctx context.Context, in Input, preview bool) (Output, error) {
	if err := ValidateInput(in); err != nil {
		return Output{}, err
	}
	out := Output{Plan: Plan{Mode: "preview", Operation: "content.label.update", Environment: in.Environment, Site: in.Site}}
	if !in.TargetResolved || strings.TrimSpace(in.Environment) == "" {
		return out, usage("a resolved Tableau environment is required")
	}
	if a == nil || a.reader == nil {
		return out, usage("label update reader is not configured")
	}
	before, err := a.baseline(ctx, in)
	if err != nil {
		return out, fail(in, "read", errs.OutcomeNotAttempted, err)
	}
	if before != nil {
		if in.Value != nil && *in.Value != before.Value {
			previous, readErr := a.reader.GetLabelValue(ctx, before.Value)
			if readErr != nil {
				return out, fail(in, "value", errs.OutcomeNotAttempted, readErr)
			}
			if previous.Name != before.Value || previous.Internal {
				return out, fail(in, "value", errs.OutcomeNotAttempted, fmt.Errorf("system-managed label attachments cannot be changed here"))
			}
		}
		out.Plan.Before = before
		out.Plan.Target = value.LabelTarget{Type: before.Type, LUID: before.TargetLUID}
		out.Plan.Desired = value.LabelUpdate{Value: before.Value, Message: before.Message, Active: before.Active, Elevated: before.Elevated}
	} else {
		out.Plan.Target = value.LabelTarget{Type: in.Type, LUID: in.TargetID}
		out.Plan.Desired.Active = true
	}
	if in.Value != nil {
		out.Plan.Desired.Value = *in.Value
	}
	if in.Message != nil {
		out.Plan.Desired.Message = *in.Message
	}
	if in.Active != nil {
		out.Plan.Desired.Active = *in.Active
	}
	if in.Elevated != nil {
		out.Plan.Desired.Elevated = *in.Elevated
	}
	definition, err := a.reader.GetLabelValue(ctx, out.Plan.Desired.Value)
	if err != nil {
		return out, fail(in, "value", errs.OutcomeNotAttempted, err)
	}
	if definition.Name != out.Plan.Desired.Value || definition.Internal {
		return out, fail(in, "value", errs.OutcomeNotAttempted, fmt.Errorf("label value is mismatched or system-managed"))
	}
	if before == nil && in.Elevated == nil {
		out.Plan.Desired.Elevated = definition.ElevatedDefault
	}
	out.Plan.NoOp = before != nil && matches(*before, out.Plan.Desired)
	if preview {
		return out, nil
	}
	out.Plan.Mode = "perform"
	if out.Plan.NoOp {
		out.Result = &Result{Status: "unchanged", Item: *before}
		return out, nil
	}
	if a.writer == nil {
		return out, usage("label update writer is not configured")
	}
	current, err := a.baseline(ctx, in)
	if err != nil {
		return out, fail(in, "recheck", errs.OutcomeNotAttempted, err)
	}
	if !sameBaseline(before, current) {
		return out, fail(in, "conflict", errs.OutcomeNotAttempted, fmt.Errorf("label changed after preview baseline was read"))
	}
	var saved value.ContentLabel
	if before == nil {
		saved, err = a.writer.SetLabel(ctx, out.Plan.Target, out.Plan.Desired)
	} else {
		saved, err = a.writer.UpdateLabel(ctx, before.LUID, out.Plan.Desired)
	}
	if err != nil {
		out.Result = &Result{Status: "unknown", Item: saved}
		var acknowledged interface{ WriteAcknowledged() bool }
		if errors.As(err, &acknowledged) && acknowledged.WriteAcknowledged() {
			out.Result.Status = "verification_pending"
			out.Result.Completed = []string{"label_write"}
			return out, fail(in, "verification", errs.OutcomeConfirmed, err)
		}
		return out, fail(in, "submission", errs.OutcomeUnknown, err)
	}
	out.Result = &Result{Status: "verification_pending", Item: saved, Completed: []string{"label_write"}}
	if saved.LUID == "" || saved.TargetLUID != out.Plan.Target.LUID || saved.Type != out.Plan.Target.Type || (before != nil && saved.LUID != before.LUID) || !matches(saved, out.Plan.Desired) {
		return out, fail(in, "verification", errs.OutcomeConfirmed, fmt.Errorf("write response does not confirm the selected label and requested values"))
	}
	verified, err := a.reader.GetLabel(ctx, saved.LUID)
	if err != nil {
		return out, fail(in, "verification", errs.OutcomeConfirmed, err)
	}
	if verified.LUID != saved.LUID || verified.TargetLUID != saved.TargetLUID || verified.Type != saved.Type || !matches(verified, out.Plan.Desired) {
		return out, fail(in, "verification", errs.OutcomeConfirmed, fmt.Errorf("label readback differs from the acknowledged write"))
	}
	out.Result.Status = "updated"
	out.Result.Item = verified
	return out, nil
}
func (a *Action) baseline(ctx context.Context, in Input) (*value.ContentLabel, error) {
	if in.ID != "" {
		v, e := a.reader.GetLabel(ctx, in.ID)
		if e != nil {
			return nil, e
		}
		if v.LUID != in.ID || !allowed(v.Type) || v.TargetLUID == "" || (in.Type != "" && (v.Type != in.Type || v.TargetLUID != in.TargetID)) {
			return nil, fmt.Errorf("attachment ID or related asset differs from request")
		}
		return &v, nil
	}
	items, e := a.reader.GetLabels(ctx, value.LabelTarget{Type: in.Type, LUID: in.TargetID}, nil)
	if e != nil {
		return nil, e
	}
	if len(items) > 10000 {
		return nil, fmt.Errorf("label collection exceeds bound")
	}
	var found *value.ContentLabel
	for _, v := range items {
		if v.TargetLUID != in.TargetID || v.Type != in.Type || v.LUID == "" {
			return nil, fmt.Errorf("label collection includes an unrelated or incomplete attachment")
		}
		if v.Value == *in.Value {
			if found != nil {
				return nil, fmt.Errorf("label value matches multiple attachments; select --id")
			}
			copy := v
			found = &copy
		}
	}
	return found, nil
}
func matches(v value.ContentLabel, p value.LabelUpdate) bool {
	return v.Value == p.Value && v.Message == p.Message && v.Active == p.Active && v.Elevated == p.Elevated
}
func sameBaseline(a, b *value.ContentLabel) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.LUID == b.LUID && a.TargetLUID == b.TargetLUID && a.Type == b.Type && a.Value == b.Value && a.Message == b.Message && a.Active == b.Active && a.Elevated == b.Elevated
}
func allowed(s string) bool {
	return s == "database" || s == "table" || s == "column" || s == "datasource" || s == "flow"
}
func usage(s string) error {
	return &errs.Error{ID: "content.label.update.usage", Kind: errs.KindUsage, Operation: "content.label.update", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Choose the exact label or asset, then preview explicit changes."}
}
func fail(in Input, step string, outcome errs.Outcome, cause error) error {
	phase := errs.PhaseValidation
	if outcome == errs.OutcomeUnknown {
		phase = errs.PhaseSubmission
	}
	if step == "verification" {
		phase = errs.PhaseVerification
	}
	return &errs.Error{ID: "content.label.update." + step, Kind: errs.KindOperation, Operation: "content.label.update", Environment: in.Environment, Site: in.Site, Resource: in.ID, Summary: "Label update failed.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact attachment before retrying; preserve any acknowledged write.", Phase: phase, Outcome: outcome, TableauRequestID: errs.TableauRequestID(cause)}
}
