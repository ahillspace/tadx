package project

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// DeleteResolver resolves an authoritative project LUID.
type DeleteResolver interface {
	ResolveProject(context.Context, identity.Selector) (DeleteProject, error)
}

// Deleter performs one exact remote project deletion.
type Deleter interface {
	DeleteProject(context.Context, string) (DeleteResult, error)
}

// DeleteAction previews and applies one exact project deletion.
type DeleteAction struct {
	resolver DeleteResolver
	deleter  Deleter
}

// NewDelete creates a project-delete action.
func NewDelete(resolver DeleteResolver, deleter Deleter) *DeleteAction {
	return &DeleteAction{resolver: resolver, deleter: deleter}
}

// Execute previews a deletion or revalidates its exact LUID before applying it.
func (a *DeleteAction) Execute(ctx context.Context, input DeleteInput, preview bool) (DeleteOutput, error) {
	if err := ValidateDeleteInput(input); err != nil {
		return DeleteOutput{}, err
	}
	if a == nil || a.resolver == nil || a.deleter == nil {
		return DeleteOutput{}, runtimeError()
	}
	input.Environment = strings.TrimSpace(input.Environment)
	input.Site = strings.TrimSpace(input.Site)
	input.ProjectLUID = strings.TrimSpace(input.ProjectLUID)
	if input.Environment == "" || (input.Site == "" && !input.TargetResolved) {
		return DeleteOutput{}, deleteUsage("environment", "project delete requires an explicit resolved environment and site")
	}
	target, err := a.resolve(ctx, input, "Project resolution failed.", "Review the exact project LUID, then retry.")
	if err != nil {
		return DeleteOutput{}, err
	}
	plan := DeletePlan{Mode: "preview", Operation: "project.delete", Environment: input.Environment, Site: input.Site, Target: target}
	output := DeleteOutput{Plan: plan, Warnings: []string{CascadeWarning}, Help: []string{"Run without --preview to delete this exact project."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	current, err := a.resolve(ctx, input, "Project revalidation failed.", "Review a new preview before deleting.")
	if err != nil {
		return DeleteOutput{}, err
	}
	if current.LUID != target.LUID {
		return DeleteOutput{}, deleteOperationError("project.delete.target_changed", input, target.LUID, "The project identity changed during revalidation.", "Review a new preview before deleting.", errors.New("project LUID changed during revalidation"))
	}
	result, err := a.deleter.DeleteProject(ctx, target.LUID)
	if err != nil {
		return DeleteOutput{}, deleteOperationError("project.delete.failed", input, target.LUID, "Project delete failed.", "Inspect the remote project delete outcome before retrying: "+commandhint.Environment(input.Environment, "content", "project", "inspect", "--project-id", target.LUID), err)
	}
	if result.ProjectLUID != target.LUID {
		return DeleteOutput{}, deleteOperationError("project.delete.invalid_response", input, target.LUID, "Project delete returned a different authoritative identity.", "Review the remote project state before retrying.", errors.New("project delete result LUID did not match the requested LUID"))
	}
	output.Result = &result
	output.Help = []string{commandhint.Environment(input.Environment, "content", "project", "list")}
	return output, nil
}

func (a *DeleteAction) resolve(ctx context.Context, input DeleteInput, summary, correctiveAction string) (DeleteProject, error) {
	project, err := a.resolver.ResolveProject(ctx, identity.Selector{LUID: identity.LUID(input.ProjectLUID)})
	if err != nil {
		return DeleteProject{}, deleteOperationError("project.delete.resolve", input, input.ProjectLUID, summary, correctiveAction, err)
	}
	if strings.TrimSpace(project.LUID) == "" {
		return DeleteProject{}, deleteOperationError("project.delete.resolve", input, input.ProjectLUID, summary, correctiveAction, errors.New("resolved project omitted its authoritative LUID"))
	}
	if project.LUID != input.ProjectLUID {
		return DeleteProject{}, deleteOperationError("project.delete.resolve", input, input.ProjectLUID, summary, correctiveAction, errors.New("resolved project LUID did not match the requested LUID"))
	}
	return project, nil
}

func runtimeError() error {
	return &errs.Error{ID: "project.delete.unconfigured", Kind: errs.KindRuntime, Operation: "project.delete", Summary: "Project delete is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure project delete before retrying."}
}

// ValidateDeleteInput checks caller-controlled arguments before dependency setup.
func ValidateDeleteInput(input DeleteInput) error {
	if strings.TrimSpace(input.Environment) == "" {
		return deleteUsage("environment", "project delete requires an explicit environment")
	}
	if strings.TrimSpace(input.ProjectLUID) == "" {
		return deleteUsage("project_id", "project delete requires an authoritative project LUID")
	}
	return nil
}

func deleteUsage(field, message string) error {
	return &errs.Error{ID: "project.delete.usage", Kind: errs.KindUsage, Operation: "project.delete", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Provide an exact project LUID and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}

func deleteOperationError(id string, input DeleteInput, resource, summary, fallback string, cause error) error {
	retryable, correctiveAction := errs.CompleteRetryAdvice(cause, fallback)
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "project.delete", Resource: resource, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(cause)}
}

// CascadeWarning describes Tableau's documented project-deletion behavior.
const CascadeWarning = "Deleting this project also deletes all Tableau assets inside it. TADX cannot verify that the project is empty."

// DeleteInput selects one exact project on one resolved Tableau target.
type DeleteInput struct {
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved bool
	Environment    string
	Site           string
	ProjectLUID    string
}

// DeleteProject is the authoritative delete target shown during preview.
type DeleteProject struct {
	LUID string `json:"luid"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// DeletePlan is the immutable preview of one project deletion.
type DeletePlan struct {
	Mode        string        `json:"mode"`
	Operation   string        `json:"operation"`
	Environment string        `json:"environment"`
	Site        string        `json:"site"`
	Target      DeleteProject `json:"target"`
}

// DeleteResult is the authoritative remote outcome.
type DeleteResult struct {
	Status           string `json:"status"`
	ProjectLUID      string `json:"project_luid"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}

// DeleteOutput contains the preview and optional applied outcome.
type DeleteOutput struct {
	Plan     DeletePlan    `json:"plan"`
	Result   *DeleteResult `json:"result,omitempty"`
	Warnings []string      `json:"warnings"`
	Help     []string      `json:"help"`
}

// CompactDeleteResult omits successful request diagnostics.
type CompactDeleteResult struct {
	Status      string `json:"status"`
	ProjectLUID string `json:"project_luid"`
}

// DeleteCompactResult is the bounded default projection.
type DeleteCompactResult struct {
	Plan     DeletePlan           `json:"plan"`
	Result   *CompactDeleteResult `json:"result,omitempty"`
	Warnings []string             `json:"warnings"`
	Details  string               `json:"details"`
	Help     []string             `json:"help"`
}

// CompactOutput returns the bounded default projection.
func (o DeleteOutput) CompactOutput() any {
	var result *CompactDeleteResult
	if o.Result != nil {
		result = &CompactDeleteResult{Status: o.Result.Status, ProjectLUID: o.Result.ProjectLUID}
	}
	return DeleteCompactResult{Plan: o.Plan, Result: result, Warnings: append([]string(nil), o.Warnings...), Details: "--full", Help: o.Help}
}

// FullOutput returns bounded request diagnostics.
func (o DeleteOutput) FullOutput() any { return o }
