package contentlabel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type UpdateInput struct {
	Environment, Site, ID, Type, TargetID string
	Value, Message                        *string
	Active, Elevated                      *bool
}
type UpdateReader interface {
	GetLabel(context.Context, string) (value.ContentLabel, error)
	GetLabels(context.Context, value.LabelTarget, []string) ([]value.ContentLabel, error)
	GetLabelValue(context.Context, string) (value.LabelValue, error)
}
type UpdateWriter interface {
	SetLabel(context.Context, value.LabelTarget, value.LabelUpdate) (value.ContentLabel, error)
	UpdateLabel(context.Context, string, value.LabelUpdate) (value.ContentLabel, error)
}
type UpdatePlan struct {
	Mode        string              `json:"mode"`
	Operation   string              `json:"operation"`
	Environment string              `json:"environment"`
	Site        string              `json:"site"`
	Target      value.LabelTarget   `json:"target"`
	Before      *value.ContentLabel `json:"before,omitempty"`
	Desired     value.LabelUpdate   `json:"desired"`
	NoOp        bool                `json:"no_op"`
}
type UpdateResult struct {
	Status    string              `json:"status"`
	Item      *value.ContentLabel `json:"item,omitempty"`
	Completed []string            `json:"completed,omitempty"`
}
type UpdateOutput struct {
	Plan   *UpdatePlan   `json:"plan,omitempty"`
	Result *UpdateResult `json:"result,omitempty"`
}

func (o UpdateOutput) CompactOutput() any { return o }
func (o UpdateOutput) FullOutput() any    { return o }
func ValidateUpdateInput(in UpdateInput) error {
	if in.ID == "" {
		if !supportedKind(in.Type) || strings.TrimSpace(in.TargetID) == "" || in.Value == nil {
			return updateUsage("select an attachment --id, or --type and --target-id with --value")
		}
	} else if strings.TrimSpace(in.ID) != in.ID {
		return updateUsage("attachment ID must be exact")
	}
	if (in.Type == "") != (in.TargetID == "") {
		return updateUsage("--type and --target-id must be supplied together")
	}
	if in.Type != "" && !supportedKind(in.Type) {
		return updateUsage("labels support database, table, column, datasource, or flow")
	}
	if strings.TrimSpace(in.TargetID) != in.TargetID {
		return updateUsage("related asset ID must be exact")
	}
	if in.Value == nil && in.Message == nil && in.Active == nil && in.Elevated == nil {
		return updateUsage("supply at least one label change")
	}
	if in.Value != nil && (strings.TrimSpace(*in.Value) == "" || strings.TrimSpace(*in.Value) != *in.Value || len(*in.Value) > 512) {
		return updateUsage("label value must be an exact nonempty name")
	}
	if in.Message != nil && len(*in.Message) > 65536 {
		return updateUsage("label message exceeds 65536 bytes")
	}
	return nil
}
func updateLabel(ctx context.Context, reader UpdateReader, writer UpdateWriter, in UpdateInput, preview bool) (UpdateOutput, error) {
	out := UpdateOutput{}
	if reader == nil {
		return out, updateUsage("label update reader is not configured")
	}
	before, err := updateBaseline(ctx, reader, in)
	if err != nil {
		return out, updateFailure(in, "read", errs.OutcomeNotAttempted, err)
	}
	if before != nil {
		if in.Value != nil && *in.Value != before.Value {
			previous, readErr := reader.GetLabelValue(ctx, before.Value)
			if readErr != nil {
				return out, updateFailure(in, "value", errs.OutcomeNotAttempted, readErr)
			}
			if previous.Name != before.Value || previous.Internal {
				return out, updateFailure(in, "value", errs.OutcomeNotAttempted, fmt.Errorf("system-managed label attachments cannot be changed here"))
			}
		}
		out.Plan = &UpdatePlan{Mode: "preview", Operation: "content.label.update", Environment: in.Environment, Site: in.Site, Before: before}
		out.Plan.Target = value.LabelTarget{Type: before.Type, LUID: before.TargetLUID}
		out.Plan.Desired = value.LabelUpdate{Value: before.Value, Message: before.Message, Active: before.Active, Elevated: before.Elevated}
	} else {
		out.Plan = &UpdatePlan{Mode: "preview", Operation: "content.label.update", Environment: in.Environment, Site: in.Site}
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
	definition, err := reader.GetLabelValue(ctx, out.Plan.Desired.Value)
	if err != nil {
		return out, updateFailure(in, "value", errs.OutcomeNotAttempted, err)
	}
	if definition.Name != out.Plan.Desired.Value || definition.Internal {
		return out, updateFailure(in, "value", errs.OutcomeNotAttempted, fmt.Errorf("label value is mismatched or system-managed"))
	}
	if before == nil && in.Elevated == nil {
		out.Plan.Desired.Elevated = definition.ElevatedDefault
	}
	out.Plan.NoOp = before != nil && updateMatches(*before, out.Plan.Desired)
	if preview {
		return out, nil
	}
	out.Plan.Mode = "perform"
	if out.Plan.NoOp {
		item := *before
		out.Result = &UpdateResult{Status: "unchanged", Item: &item}
		return out, nil
	}
	if writer == nil {
		return out, updateUsage("label update writer is not configured")
	}
	current, err := updateBaseline(ctx, reader, in)
	if err != nil {
		return out, updateFailure(in, "recheck", errs.OutcomeNotAttempted, err)
	}
	if !sameUpdateBaseline(before, current) {
		return out, updateFailure(in, "conflict", errs.OutcomeNotAttempted, fmt.Errorf("label changed after preview baseline was read"))
	}
	var saved value.ContentLabel
	if before == nil {
		saved, err = writer.SetLabel(ctx, out.Plan.Target, out.Plan.Desired)
	} else {
		saved, err = writer.UpdateLabel(ctx, before.LUID, out.Plan.Desired)
	}
	if err != nil {
		out.Result = &UpdateResult{Status: "unknown"}
		if saved.LUID != "" {
			saved.Type = value.CanonicalContentType(saved.Type)
			out.Result.Item = &saved
		}
		var acknowledged interface{ WriteAcknowledged() bool }
		if errors.As(err, &acknowledged) && acknowledged.WriteAcknowledged() {
			out.Result.Status = "verification_pending"
			out.Result.Completed = []string{"label_write"}
			return out, updateFailure(in, "verification", errs.OutcomeConfirmed, err)
		}
		return out, updateFailure(in, "submission", errs.OutcomeUnknown, err)
	}
	saved.Type = value.CanonicalContentType(saved.Type)
	out.Result = &UpdateResult{Status: "verification_pending", Item: &saved, Completed: []string{"label_write"}}
	if saved.LUID == "" || saved.TargetLUID != out.Plan.Target.LUID || saved.Type != out.Plan.Target.Type || (before != nil && saved.LUID != before.LUID) || !updateMatches(saved, out.Plan.Desired) {
		return out, updateFailure(in, "verification", errs.OutcomeConfirmed, fmt.Errorf("write response mismatch: requested attachment=%q type=%q target=%q value=%q, returned attachment=%q type=%q target=%q value=%q", in.ID, out.Plan.Target.Type, out.Plan.Target.LUID, out.Plan.Desired.Value, saved.LUID, saved.Type, saved.TargetLUID, saved.Value))
	}
	verified, err := reader.GetLabel(ctx, saved.LUID)
	if err != nil {
		return out, updateFailure(in, "verification", errs.OutcomeConfirmed, err)
	}
	verified.Type = value.CanonicalContentType(verified.Type)
	if verified.LUID != saved.LUID || verified.TargetLUID != saved.TargetLUID || verified.Type != saved.Type || !updateMatches(verified, out.Plan.Desired) {
		return out, updateFailure(in, "verification", errs.OutcomeConfirmed, fmt.Errorf("label readback differs from the acknowledged write"))
	}
	out.Result.Status = "updated"
	out.Result.Item = &verified
	return out, nil
}
func updateBaseline(ctx context.Context, reader UpdateReader, in UpdateInput) (*value.ContentLabel, error) {
	if in.ID != "" {
		v, e := reader.GetLabel(ctx, in.ID)
		if e != nil {
			return nil, e
		}
		v.Type = value.CanonicalContentType(v.Type)
		if v.LUID != in.ID || !supportedKind(v.Type) || v.TargetLUID == "" || (in.Type != "" && (v.Type != in.Type || v.TargetLUID != in.TargetID)) {
			return nil, fmt.Errorf("label identity mismatch: requested attachment=%q type=%q target=%q, returned attachment=%q type=%q target=%q", in.ID, in.Type, in.TargetID, v.LUID, v.Type, v.TargetLUID)
		}
		return &v, nil
	}
	items, e := reader.GetLabels(ctx, value.LabelTarget{Type: in.Type, LUID: in.TargetID}, nil)
	if e != nil {
		return nil, e
	}
	if len(items) > 10000 {
		return nil, fmt.Errorf("label collection exceeds bound")
	}
	var found *value.ContentLabel
	for _, v := range items {
		v.Type = value.CanonicalContentType(v.Type)
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
func updateMatches(v value.ContentLabel, p value.LabelUpdate) bool {
	return v.Value == p.Value && v.Message == p.Message && v.Active == p.Active && v.Elevated == p.Elevated
}
func sameUpdateBaseline(a, b *value.ContentLabel) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.LUID == b.LUID && a.TargetLUID == b.TargetLUID && a.Type == b.Type && a.Value == b.Value && a.Message == b.Message && a.Active == b.Active && a.Elevated == b.Elevated
}
func updateUsage(s string) error {
	return &errs.Error{ID: "content.label.update.usage", Kind: errs.KindUsage, Operation: "content.label.update", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Choose the exact label or asset, then preview explicit changes."}
}
func updateFailure(in UpdateInput, step string, outcome errs.Outcome, cause error) error {
	phase := errs.PhaseValidation
	if outcome == errs.OutcomeUnknown {
		phase = errs.PhaseSubmission
	}
	if step == "verification" {
		phase = errs.PhaseVerification
	}
	return &errs.Error{ID: "content.label.update." + step, Kind: errs.KindOperation, Operation: "content.label.update", Environment: in.Environment, Site: in.Site, Resource: in.ID, Summary: "Label update failed.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact attachment before retrying; preserve any acknowledged write.", Phase: phase, Outcome: outcome, TableauRequestID: errs.TableauRequestID(cause)}
}
