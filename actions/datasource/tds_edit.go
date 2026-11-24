package datasource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type TDSReader interface {
	ReadTDSElements(ctx context.Context, targetKind, id, elementType, name string) (value.TDSRead, error)
}
type TDSEditor interface {
	TDSReader
	EditTDSElement(ctx context.Context, copyID, verb string, e value.TDSElement) (value.TDSReceipt, error)
}

type EditItem struct {
	Eligibility      value.TDSEligibility `json:"eligibility,omitzero"`
	Failure          *value.TDSStatus     `json:"failure,omitempty"`
	Index            int                  `json:"index"`
	Op               string               `json:"op"`
	Target           EditSelector         `json:"target"`
	Before           EditState            `json:"before"`
	Intended         EditState            `json:"intended"`
	Observed         *EditState           `json:"observed,omitempty"`
	Action           string               `json:"action,omitempty"`
	Status           string               `json:"status"`
	Submission       string               `json:"submission,omitempty"`
	TableauRequestID string               `json:"tableau_request_id,omitempty"`
}
type EditOutput struct {
	Mode        string `json:"mode"`
	Environment string `json:"environment"`
	EditTarget
	PreviewToken string     `json:"preview_token,omitempty"`
	Items        []EditItem `json:"items"`
	Help         []string   `json:"help,omitempty"`
}

func exactTDS(ctx context.Context, reader TDSReader, kind, id, typ, name string) (value.TDSRead, error) {
	r, err := reader.ReadTDSElements(ctx, kind, id, typ, name)
	if err != nil || r.Absent {
		return r, err
	}
	if len(r.Elements) != 1 || r.Elements[0].Name != name || r.Elements[0].Error != nil {
		return r, errors.New("exact TDS observation is incomplete or ambiguous")
	}
	return r, nil
}

// observeEdit returns a value only from its named TDS element, including when
// the surrounding collection is incomplete. Only provider-confirmed absence
// has absence semantics. The empty description is a present value.
func observeEdit(ctx context.Context, reader TDSReader, kind, id string, s EditSelector) (EditState, value.TDSEligibility, error) {
	unknown := EditState{Presence: "unknown"}
	if s.ElementType == "FieldDescription" {
		col, err := exactTDS(ctx, reader, kind, id, "Column", s.Name)
		if err != nil {
			return unknown, col.Eligibility, err
		}
		if col.Absent || col.Elements[0].Column == nil {
			return unknown, col.Eligibility, errors.New("field has no exact observable TDS column")
		}
	}
	r, err := exactTDS(ctx, reader, kind, id, s.ElementType, s.Name)
	if err != nil {
		return unknown, r.Eligibility, err
	}
	if r.Absent {
		return EditState{Presence: "absent"}, r.Eligibility, nil
	}
	e := r.Elements[0]
	var setting *string
	if s.ElementType == "Column" && e.Column != nil {
		switch s.Property {
		case "caption":
			setting = e.Column.Caption
		case "role":
			setting = e.Column.Role
		case "type":
			setting = e.Column.Type
		case "element":
			data, err := json.Marshal(e.Column)
			if err != nil {
				return unknown, r.Eligibility, err
			}
			return EditState{"present", string(data)}, r.Eligibility, nil
		}
	} else if s.Property == "description" {
		setting = e.Description
	}
	if setting == nil {
		return unknown, r.Eligibility, errors.New("TDS omitted the required property from the exact observation")
	}
	return EditState{"present", *setting}, r.Eligibility, nil
}

func editItem(c EditChange, i int) EditItem {
	return EditItem{Index: i + 1, Op: c.Op, Target: c.Selector(), Intended: c.Intended(), Before: EditState{Presence: "unknown"}, Status: "unattempted"}
}

func itemAction(row EditItem) (string, error) {
	if row.Target.ElementType == "Column" && row.Intended.Presence == "present" && row.Before.Presence == "absent" {
		return "", errors.New("cannot update a missing column")
	}
	switch {
	case row.Before == row.Intended:
		return "unchanged", nil
	case row.Intended.Presence == "absent":
		return "delete", nil
	case row.Before.Presence == "absent":
		return "add", nil
	default:
		return "update", nil
	}
}

func elementForEdit(i EditItem) value.TDSElement {
	e := value.TDSElement{ElementType: i.Target.ElementType, Name: i.Target.Name}
	if i.Action == "delete" {
		return e
	}
	switch i.Target.Property {
	case "caption":
		e.Column = &value.TDSColumn{Caption: &i.Intended.Value}
	case "type":
		e.Column = &value.TDSColumn{Type: &i.Intended.Value}
	case "role":
		e.Column = &value.TDSColumn{Role: &i.Intended.Value}
	case "description":
		e.Description = &i.Intended.Value
	}
	return e
}

func (o EditOutput) inspectCommand(item EditItem) string {
	selector := "--field"
	if item.Target.ElementType == "TableDescription" {
		selector = "--table"
	}
	return commandhint.Environment(o.Environment, "content", "datasource", "work-copy", "--work-copy", o.WorkCopyLUID, selector, item.Target.Name)
}

func (o EditOutput) failure(item EditItem, summary string, cause error, phase errs.Phase, outcome errs.Outcome) error {
	failure := &errs.Error{
		ID: "datasource.tds." + item.Status, Kind: errs.KindOperation,
		Operation: "datasource." + item.Op, Selector: item.Target.Name,
		Environment: o.Environment, Site: o.SiteLUID, Resource: o.WorkCopyLUID,
		Summary: summary, Cause: cause, Phase: phase, Outcome: outcome,
		Retryable: new(false), TableauRequestID: item.TableauRequestID,
		CorrectiveAction: o.inspectCommand(item),
	}
	if o.Mode == "execute" && len(o.Items) > 1 {
		failure.Failed = fmt.Sprintf("%d: %s", item.Index, item.Op)
		for _, previous := range o.Items[:item.Index-1] {
			if previous.Status == "verified" || previous.Status == "unchanged" {
				failure.Completed = append(failure.Completed, fmt.Sprintf("%d: %s", previous.Index, previous.Op))
			}
		}
	}
	return failure
}

// Edit uses one prepared set of changes for both direct and file entry points.
// The caller holds the per-copy lock throughout execution. Tableau provides no
// compare-and-swap for element edits, so independent clients can still race.
func Edit(ctx context.Context, client TDSEditor, target EditTarget, env string, changes []EditChange, preview bool, expectToken string) (EditOutput, error) {
	out := EditOutput{Mode: "execute", Environment: env, EditTarget: target}
	if preview {
		out.Mode = "preview"
	}
	if err := ValidateEditChanges(changes); err != nil {
		return out, err
	}
	if !value.ValidTDSLUID(target.WorkCopyLUID) {
		return out, editUsage("Editing requires the exact work-copy LUID.")
	}
	for i, c := range changes {
		out.Items = append(out.Items, editItem(c, i))
	}
	for i := range out.Items {
		row := &out.Items[i]
		before, _, err := observeEdit(ctx, client, "workcopy", target.WorkCopyLUID, row.Target)
		row.Before = before
		if err == nil {
			row.Action, err = itemAction(*row)
		}
		if err != nil {
			row.Status = "unreadable"
			out.Help = []string{out.inspectCommand(*row)}
			return out, out.failure(*row, "Edit preflight failed before submission.", err, errs.PhaseValidation, errs.OutcomeNotAttempted)
		}
	}
	digestInput := struct {
		Target EditTarget
		Items  []EditItem
	}{target, out.Items}
	data, err := json.Marshal(digestInput)
	if err != nil {
		return out, err
	}
	digest := sha256.Sum256(data)
	out.PreviewToken = "sha256:" + hex.EncodeToString(digest[:])
	if expectToken != "" && expectToken != out.PreviewToken {
		return out, &errs.Error{ID: "datasource.tds.stale_preview", Kind: errs.KindOperation, Summary: "The selected copy, request or observed values changed since preview.", Resource: target.WorkCopyLUID, Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted, Retryable: new(false)}
	}
	if preview {
		for i := range out.Items {
			out.Items[i].Status = "projected"
		}
		return out, nil
	}
	for i := range out.Items {
		row := &out.Items[i]
		before, _, err := observeEdit(ctx, client, "workcopy", target.WorkCopyLUID, row.Target)
		if err != nil || before != row.Before {
			row.Observed = &before
			row.Status = "stale"
			out.Help = []string{out.inspectCommand(*row)}
			return out, out.failure(*row, "Fresh pre-write observation differs or is unavailable.", err, errs.PhaseValidation, errs.OutcomeNotAttempted)
		}
		if row.Action == "unchanged" {
			row.Observed = &before
			row.Status = "unchanged"
			continue
		}
		receipt, submitErr := client.EditTDSElement(ctx, target.WorkCopyLUID, row.Action, elementForEdit(*row))
		row.Eligibility = receipt.Eligibility
		row.Failure = receipt.Failure
		row.Submission = receipt.Status
		row.TableauRequestID = receipt.TableauRequestID
		if receipt.Status == "rejected" || receipt.Status == "not_attempted" || (receipt.Status == "" && submitErr != nil) {
			row.Status = "rejected"
			outcome := errs.OutcomeNotApplied
			phase := errs.PhaseSubmission
			if receipt.Status == "" || receipt.Status == "not_attempted" {
				row.Status = "unattempted"
				outcome = errs.OutcomeNotAttempted
				phase = errs.PhaseSetup
			}
			out.Help = []string{out.inspectCommand(*row)}
			return out, out.failure(*row, "Tableau edit did not complete.", submitErr, phase, outcome)
		}
		observed, _, readErr := observeEdit(ctx, client, "workcopy", target.WorkCopyLUID, row.Target)
		row.Observed = &observed
		if submitErr != nil || receipt.Status != "acknowledged" {
			row.Status = "unknown"
			out.Help = []string{out.inspectCommand(*row)}
			return out, out.failure(*row, "The effect of this submission is uncertain. Review the current value.", errors.Join(submitErr, readErr), errs.PhaseSubmission, errs.OutcomeUnknown)
		}
		if readErr != nil || observed != row.Intended {
			row.Status = "unverified"
			out.Help = []string{out.inspectCommand(*row)}
			return out, out.failure(*row, "Tableau acknowledged the edit. Its intended value could not be verified.", readErr, errs.PhaseVerification, errs.OutcomeConfirmed)
		}
		row.Status = "verified"
	}
	return out, nil
}

// ReviewEdits checks all requested properties even if an earlier one differs.
func ReviewEdits(ctx context.Context, reader TDSReader, target EditTarget, env string, changes []EditChange) (EditOutput, error) {
	out := EditOutput{Mode: "review", Environment: env, EditTarget: target}
	if err := ValidateEditChanges(changes); err != nil {
		return out, err
	}
	var first error
	for i, c := range changes {
		item := editItem(c, i)
		current, eligibility, err := observeEdit(ctx, reader, "workcopy", target.WorkCopyLUID, item.Target)
		item.Eligibility = eligibility
		item.Before = current
		item.Observed = &current
		switch {
		case err != nil:
			item.Status = "unknown"
		case current != item.Intended:
			item.Status = "different"
		default:
			item.Status = "matched"
		}
		if item.Status != "matched" && first == nil {
			first = out.failure(item, fmt.Sprintf("Review entry %d is %s.", item.Index, item.Status), err, errs.PhaseVerification, errs.OutcomeNotAttempted)
			out.Help = []string{out.inspectCommand(item)}
		}
		out.Items = append(out.Items, item)
	}
	return out, first
}
