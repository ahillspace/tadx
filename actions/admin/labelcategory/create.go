package labelcategory

import (
	"context"
	"errors"
	"fmt"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type CreateInput struct {
	Environment, Site string
	Name, Description string
}
type CreateReader interface {
	ListLabelCategories(context.Context) ([]value.LabelCategory, error)
	GetLabelCategory(context.Context, string) (value.LabelCategory, error)
}
type CreateWriter interface {
	CreateLabelCategory(context.Context, value.LabelCategory) (value.LabelCategory, error)
}

func ValidateCreateInput(in CreateInput) error {
	if !validName(in.Name) {
		return createUsage("an exact --name containing 1 to 128 characters is required")
	}
	if !validDescription(in.Description) {
		return createUsage("description must contain 1 to 500 characters")
	}
	return nil
}
func Create(ctx context.Context, reader CreateReader, writer CreateWriter, in CreateInput, preview bool) (WriteOutput, error) {
	out := WriteOutput{Plan: WritePlan{Mode: "preview", Operation: "admin.label.category.create", Environment: in.Environment, Site: in.Site, Name: in.Name, Effect: "Creates a new shared category; an existing exact name is rejected."}}
	before, err := createBaseline(ctx, reader, in)
	if err != nil {
		return out, createFail(in, "read", errs.OutcomeNotAttempted, err)
	}
	out.Plan.Before = before
	if before != nil {
		return out, createFail(in, "exists", errs.OutcomeNotAttempted, fmt.Errorf("category already exists; use update"))
	}
	desired := value.LabelCategory{Name: in.Name, Description: in.Description}
	out.Plan.Desired = desired
	if preview {
		return out, nil
	}
	out.Plan.Mode = "perform"
	current, err := createBaseline(ctx, reader, in)
	if err != nil {
		return out, createFail(in, "recheck", errs.OutcomeNotAttempted, err)
	}
	if current != nil {
		return out, createFail(in, "conflict", errs.OutcomeNotAttempted, fmt.Errorf("label category changed after its baseline was read"))
	}
	saved, err := writer.CreateLabelCategory(ctx, desired)
	if err != nil {
		out.Result = &WriteResult{Status: "unknown", Item: saved}
		if acknowledged, ok := errors.AsType[interface {
			error
			WriteAcknowledged() bool
		}](err); ok && acknowledged.WriteAcknowledged() {
			out.Result.Status = "verification_pending"
			out.Result.Completed = []string{"definition_write"}
			return out, createFail(in, "verification", errs.OutcomeConfirmed, err)
		}
		return out, createFail(in, "submission", errs.OutcomeUnknown, err)
	}
	out.Result = &WriteResult{Status: "verification_pending", Item: saved, Completed: []string{"definition_write"}}
	verified, err := reader.GetLabelCategory(ctx, desired.Name)
	if err != nil {
		return out, createFail(in, "verification", errs.OutcomeConfirmed, err)
	}
	if !equal(verified, desired) {
		return out, createFail(in, "verification", errs.OutcomeConfirmed, fmt.Errorf("readback differs from the acknowledged definition"))
	}
	out.Result.Status = "created"
	out.Result.Item = verified
	return out, nil
}
func createBaseline(ctx context.Context, reader CreateReader, in CreateInput) (*value.LabelCategory, error) {
	items, err := reader.ListLabelCategories(ctx)
	if err != nil {
		return nil, err
	}
	for _, v := range items {
		if v.Name == in.Name {
			copy := v
			return &copy, nil
		}
	}
	return nil, nil
}
func createUsage(s string) error {
	return &errs.Error{ID: "admin.label.category.create.usage", Kind: errs.KindUsage, Operation: "admin.label.category.create", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Correct the shared label definition and preview its changes."}
}
func createFail(in CreateInput, step string, outcome errs.Outcome, cause error) error {
	phase := errs.PhaseValidation
	if outcome == errs.OutcomeUnknown {
		phase = errs.PhaseSubmission
	}
	if step == "verification" {
		phase = errs.PhaseVerification
	}
	return &errs.Error{ID: "admin.label.category.create." + step, Kind: errs.KindOperation, Operation: "admin.label.category.create", Environment: in.Environment, Site: in.Site, Resource: in.Name, Summary: "Label category create failed.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact label definition before retrying; preserve any acknowledged write.", Phase: phase, Outcome: outcome, TableauRequestID: errs.TableauRequestID(cause)}
}
