package delete

import (
	"context"
	"fmt"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
	"strings"
)

type Input struct {
	Environment, Site, ID, Type, TargetID string
	TargetResolved                        bool
}
type Reader interface {
	GetLabel(context.Context, string) (value.ContentLabel, error)
	GetLabelValue(context.Context, string) (value.LabelValue, error)
}
type Writer interface {
	DeleteLabel(context.Context, string) error
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
	Item        *value.ContentLabel `json:"item,omitempty"`
	Status      string              `json:"status"`
}

func (o Output) CompactOutput() any { return o }
func (o Output) FullOutput() any    { return o }
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
func (a *Action) Execute(ctx context.Context, in Input, preview bool) (Output, error) {
	if e := ValidateInput(in); e != nil {
		return Output{}, e
	}
	out := Output{Mode: "preview", Operation: "content.label.delete", Environment: in.Environment, Site: in.Site, Status: "planned"}
	if !in.TargetResolved || in.Environment == "" {
		return out, usage("a resolved Tableau environment is required")
	}
	if a == nil || a.reader == nil {
		return out, usage("label delete reader is not configured")
	}
	v, e := a.reader.GetLabel(ctx, in.ID)
	if e != nil {
		return out, fail(in, "read", errs.OutcomeNotAttempted, e)
	}
	v.Type = value.CanonicalContentType(v.Type)
	if v.LUID != in.ID || v.TargetLUID == "" || !allowed(v.Type) || (in.Type != "" && (v.Type != in.Type || v.TargetLUID != in.TargetID)) {
		return out, fail(in, "identity", errs.OutcomeNotAttempted, fmt.Errorf("label identity mismatch: requested attachment=%q type=%q target=%q, returned attachment=%q type=%q target=%q", in.ID, in.Type, in.TargetID, v.LUID, v.Type, v.TargetLUID))
	}
	out.Item = &v
	definition, e := a.reader.GetLabelValue(ctx, v.Value)
	if e != nil {
		return out, fail(in, "value", errs.OutcomeNotAttempted, e)
	}
	if definition.Name != v.Value || definition.Internal {
		return out, fail(in, "value", errs.OutcomeNotAttempted, fmt.Errorf("system-managed or mismatched label value cannot be deleted here"))
	}
	if preview {
		return out, nil
	}
	if a.writer == nil {
		return out, usage("label delete writer is not configured")
	}
	current, e := a.reader.GetLabel(ctx, in.ID)
	if e != nil {
		return out, fail(in, "recheck", errs.OutcomeNotAttempted, e)
	}
	if current != v {
		return out, fail(in, "conflict", errs.OutcomeNotAttempted, fmt.Errorf("label changed since baseline read"))
	}
	out.Mode = "perform"
	out.Status = "unknown"
	if e := a.writer.DeleteLabel(ctx, in.ID); e != nil {
		return out, fail(in, "submission", errs.OutcomeUnknown, e)
	}
	out.Status = "deleted"
	return out, nil
}
func allowed(s string) bool {
	return s == "database" || s == "table" || s == "column" || s == "datasource" || s == "flow"
}
func usage(s string) error {
	return &errs.Error{ID: "content.label.delete.usage", Kind: errs.KindUsage, Operation: "content.label.delete", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Select an exact label attachment ID and preview deletion."}
}
func fail(in Input, step string, outcome errs.Outcome, cause error) error {
	phase := errs.PhaseValidation
	if outcome == errs.OutcomeUnknown {
		phase = errs.PhaseSubmission
	}
	return &errs.Error{ID: "content.label.delete." + step, Kind: errs.KindOperation, Operation: "content.label.delete", Environment: in.Environment, Site: in.Site, Resource: in.ID, Summary: "Label deletion failed.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the attachment before retrying deletion.", Phase: phase, Outcome: outcome, TableauRequestID: errs.TableauRequestID(cause)}
}
