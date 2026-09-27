package contentlabel

import (
	"context"
	"fmt"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
	"strings"
)

type DeleteInput struct {
	Environment, Site, ID, Type, TargetID string
}
type DeleteReader interface {
	GetLabel(context.Context, string) (value.ContentLabel, error)
	GetLabelValue(context.Context, string) (value.LabelValue, error)
}
type DeleteWriter interface {
	DeleteLabel(context.Context, string) error
}
type DeleteOutput struct {
	Mode        string              `json:"mode"`
	Operation   string              `json:"operation"`
	Environment string              `json:"environment"`
	Site        string              `json:"site"`
	Item        *value.ContentLabel `json:"item,omitempty"`
	Status      string              `json:"status"`
}

func (o DeleteOutput) CompactOutput() any { return o }
func (o DeleteOutput) FullOutput() any    { return o }
func ValidateDeleteInput(in DeleteInput) error {
	if strings.TrimSpace(in.ID) == "" || strings.TrimSpace(in.ID) != in.ID {
		return deleteUsage("an exact attachment --id is required")
	}
	if (in.Type == "") != (in.TargetID == "") {
		return deleteUsage("--type and --target-id must be supplied together")
	}
	if in.Type != "" && !supportedKind(in.Type) {
		return deleteUsage("labels support database, table, column, datasource, or flow")
	}
	if strings.TrimSpace(in.TargetID) != in.TargetID {
		return deleteUsage("related asset ID must be exact")
	}
	return nil
}
func Delete(ctx context.Context, reader DeleteReader, writer DeleteWriter, in DeleteInput, preview bool) (DeleteOutput, error) {
	out := DeleteOutput{Mode: "preview", Operation: "content.label.delete", Environment: in.Environment, Site: in.Site, Status: "planned"}
	if reader == nil {
		return out, deleteUsage("label delete reader is not configured")
	}
	v, e := reader.GetLabel(ctx, in.ID)
	if e != nil {
		return out, deleteFailure(in, "read", errs.OutcomeNotAttempted, e)
	}
	v.Type = value.CanonicalContentType(v.Type)
	if v.LUID != in.ID || v.TargetLUID == "" || !supportedKind(v.Type) || (in.Type != "" && (v.Type != in.Type || v.TargetLUID != in.TargetID)) {
		return out, deleteFailure(in, "identity", errs.OutcomeNotAttempted, fmt.Errorf("label identity mismatch: requested attachment=%q type=%q target=%q, returned attachment=%q type=%q target=%q", in.ID, in.Type, in.TargetID, v.LUID, v.Type, v.TargetLUID))
	}
	out.Item = &v
	definition, e := reader.GetLabelValue(ctx, v.Value)
	if e != nil {
		return out, deleteFailure(in, "value", errs.OutcomeNotAttempted, e)
	}
	if definition.Name != v.Value || definition.Internal {
		return out, deleteFailure(in, "value", errs.OutcomeNotAttempted, fmt.Errorf("system-managed or mismatched label value cannot be deleted here"))
	}
	if preview {
		return out, nil
	}
	if writer == nil {
		return out, deleteUsage("label delete writer is not configured")
	}
	current, e := reader.GetLabel(ctx, in.ID)
	if e != nil {
		return out, deleteFailure(in, "recheck", errs.OutcomeNotAttempted, e)
	}
	if current != v {
		return out, deleteFailure(in, "conflict", errs.OutcomeNotAttempted, fmt.Errorf("label changed since baseline read"))
	}
	out.Mode = "perform"
	out.Status = "unknown"
	if e := writer.DeleteLabel(ctx, in.ID); e != nil {
		return out, deleteFailure(in, "submission", errs.OutcomeUnknown, e)
	}
	out.Status = "deleted"
	return out, nil
}
func deleteUsage(s string) error {
	return &errs.Error{ID: "content.label.delete.usage", Kind: errs.KindUsage, Operation: "content.label.delete", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Select an exact label attachment ID and preview deletion."}
}
func deleteFailure(in DeleteInput, step string, outcome errs.Outcome, cause error) error {
	phase := errs.PhaseValidation
	if outcome == errs.OutcomeUnknown {
		phase = errs.PhaseSubmission
	}
	return &errs.Error{ID: "content.label.delete." + step, Kind: errs.KindOperation, Operation: "content.label.delete", Environment: in.Environment, Site: in.Site, Resource: in.ID, Summary: "Label deletion failed.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the attachment before retrying deletion.", Phase: phase, Outcome: outcome, TableauRequestID: errs.TableauRequestID(cause)}
}
