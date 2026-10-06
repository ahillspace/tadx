package project

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// UpdateResolver owns exact project identity resolution.
type UpdateResolver interface {
	ResolveProject(context.Context, identity.Selector) (UpdateProject, error)
}

// Updater performs one exact released project update request.
type Updater interface {
	UpdateProject(context.Context, UpdateRequest) (UpdateResult, error)
}

// UpdateAction updates bounded metadata on one exact project.
type UpdateAction struct {
	resolver UpdateResolver
	updater  Updater
}

// NewUpdate creates a project-update action.
func NewUpdate(resolver UpdateResolver, updater Updater) *UpdateAction {
	return &UpdateAction{resolver: resolver, updater: updater}
}

// Execute previews or updates one exact project.
func (a *UpdateAction) Execute(ctx context.Context, input UpdateInput, preview bool) (UpdateOutput, error) {
	if a == nil || a.resolver == nil || a.updater == nil {
		return UpdateOutput{}, &errs.Error{ID: "project.update.unconfigured", Kind: errs.KindRuntime, Operation: "project.update", Summary: "Project update is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure project update before retrying."}
	}
	if err := validateUpdateInput(input); err != nil {
		return UpdateOutput{}, err
	}
	project, err := a.resolver.ResolveProject(ctx, input.Selector)
	if err != nil {
		return UpdateOutput{}, updateResolutionError(input, "Project resolution failed.", err)
	}
	request, noOp := changedRequest(project, input)
	plan := UpdatePlan{Mode: "preview", Operation: "project.update", Environment: input.Environment, Site: input.Site, Target: project, Changes: Changes{Name: input.Name, Description: input.Description, ContentPermissions: input.ContentPermissions}, NoOp: noOp}
	output := UpdateOutput{Plan: plan, Help: []string{"Run without --preview to update this exact project."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	current, err := a.resolver.ResolveProject(ctx, input.Selector)
	if err != nil {
		return UpdateOutput{}, updateResolutionError(input, "Project revalidation failed.", err)
	}
	if current.LUID != project.LUID {
		return UpdateOutput{}, &errs.Error{ID: "project.update.target_changed", Kind: errs.KindOperation, Operation: "project.update", Resource: project.LUID, Environment: input.Environment, Site: input.Site, Summary: "The project identity changed during revalidation.", Cause: errors.New("project LUID changed during revalidation"), Retryable: errs.Bool(false), CorrectiveAction: "Review a new preview before updating the project."}
	}
	request, noOp = changedRequest(current, input)
	output.Plan.Target = current
	output.Plan.NoOp = noOp
	if noOp {
		output.Result = &UpdateResult{Status: "unchanged", Project: current}
		output.Help = []string{commandhint.Environment(input.Environment, "content", "project", "inspect", "--project-id", current.LUID)}
		return output, nil
	}
	result, err := a.updater.UpdateProject(ctx, request)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Inspect the remote project update outcome before retrying: "+commandhint.Environment(input.Environment, "content", "project", "inspect", "--project-id", current.LUID))
		return UpdateOutput{}, &errs.Error{ID: "project.update.failed", Kind: errs.KindOperation, Operation: "project.update", Resource: current.LUID, Environment: input.Environment, Site: input.Site, Summary: "Project update failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	output.Result = &result
	output.Help = []string{commandhint.Environment(input.Environment, "content", "project", "inspect", "--project-id", current.LUID)}
	return output, nil
}

func changedRequest(project UpdateProject, input UpdateInput) (UpdateRequest, bool) {
	request := UpdateRequest{LUID: project.LUID}
	if input.Name != nil && *input.Name != project.Name {
		request.Name = input.Name
	}
	if input.Description != nil && *input.Description != project.Description {
		request.Description = input.Description
	}
	if input.ContentPermissions != nil && *input.ContentPermissions != project.ContentPermissions {
		request.ContentPermissions = input.ContentPermissions
	}
	return request, request.Name == nil && request.Description == nil && request.ContentPermissions == nil
}

func validateUpdateInput(input UpdateInput) error {
	if strings.TrimSpace(input.Environment) == "" || (strings.TrimSpace(input.Site) == "" && !input.TargetResolved) {
		return updateUsage("environment", "project update requires an explicit resolved environment and site")
	}
	return ValidateUpdateInput(input)
}

// ValidateUpdateInput checks caller-controlled arguments before local or remote setup.
func ValidateUpdateInput(input UpdateInput) error {
	if strings.TrimSpace(input.Environment) == "" {
		return updateUsage("environment", "project update requires an explicit environment")
	}
	if input.Selector.LUID == "" && strings.TrimSpace(input.Selector.ProjectPath) == "" {
		return updateUsage("selector", "project update requires a project LUID or exact project path")
	}
	if input.Selector.Name != "" || (input.Selector.LUID != "" && strings.TrimSpace(input.Selector.ProjectPath) != "") {
		return updateUsage("selector", "use either a project LUID or an exact project path")
	}
	if input.Name == nil && input.Description == nil && input.ContentPermissions == nil {
		return updateUsage("changes", "project update requires at least one explicit metadata change")
	}
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return updateUsage("name", "project update name cannot be empty")
	}
	if input.Name != nil && strings.Contains(*input.Name, "/") {
		return updateUsage("name", "project update name cannot contain a slash")
	}
	if input.ContentPermissions != nil && !validUpdateContentPermissions(*input.ContentPermissions) {
		return updateUsage("content_permissions", "project update content permissions are invalid")
	}
	return nil
}

func validUpdateContentPermissions(value string) bool {
	return value == "ManagedByOwner" || value == "LockedToProject" || value == "LockedToProjectWithoutNested"
}

func updateResolutionError(input UpdateInput, summary string, err error) error {
	retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact project selector, then retry.")
	return &errs.Error{ID: "project.update.resolve", Kind: errs.KindOperation, Operation: "project.update", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
}

func updateUsage(field, message string) error {
	return &errs.Error{ID: "project.update.usage", Kind: errs.KindUsage, Operation: "project.update", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the project update input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "invalid", Message: message}}}
}

// UpdateInput selects one project and explicit bounded metadata changes.
type UpdateInput struct {
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved     bool
	Environment        string
	Site               string
	Selector           identity.Selector
	Name               *string
	Description        *string
	ContentPermissions *string
}

// SetSelector records one exact project selector.
func (i *UpdateInput) SetSelector(luid, projectPath string) {
	i.Selector = identity.Selector{LUID: identity.LUID(luid), ProjectPath: projectPath}
}

// UpdateProject is one authoritative project identity and bounded metadata projection.
type UpdateProject struct {
	LUID                            string `json:"luid"`
	Name                            string `json:"name"`
	Path                            string `json:"path"`
	ParentLUID                      string `json:"parent_luid,omitempty"`
	Description                     string `json:"description,omitempty"`
	ContentPermissions              string `json:"content_permissions,omitempty"`
	ControllingPermissionsProjectID string `json:"controlling_permissions_project_luid,omitempty"`
}

// Changes contains only explicit requested metadata fields.
type Changes struct {
	Name               *string `json:"name,omitempty"`
	Description        *string `json:"description,omitempty"`
	ContentPermissions *string `json:"content_permissions,omitempty"`
}

// UpdateRequest carries only changed fields to the released REST adapter.
type UpdateRequest struct {
	LUID               string
	Name               *string
	Description        *string
	ContentPermissions *string
}

// UpdatePlan is the complete bounded project-update preview.
type UpdatePlan struct {
	Mode        string        `json:"mode"`
	Operation   string        `json:"operation"`
	Environment string        `json:"environment"`
	Site        string        `json:"site"`
	Target      UpdateProject `json:"target"`
	Changes     Changes       `json:"changes"`
	NoOp        bool          `json:"no_op"`
}

// UpdateResult is the authoritative project-update result.
type UpdateResult struct {
	Status           string        `json:"status"`
	Project          UpdateProject `json:"project"`
	TableauRequestID string        `json:"tableau_request_id,omitempty"`
}

// UpdateOutput retains complete details before projection.
type UpdateOutput struct {
	Plan   UpdatePlan    `json:"plan"`
	Result *UpdateResult `json:"result,omitempty"`
	Help   []string      `json:"help"`
}

// UpdateCompactProject is the exact identity needed by a later action.
type UpdateCompactProject struct {
	LUID                            string `json:"luid"`
	Name                            string `json:"name"`
	Path                            string `json:"path"`
	ParentLUID                      string `json:"parent_luid,omitempty"`
	ContentPermissions              string `json:"content_permissions,omitempty"`
	ControllingPermissionsProjectID string `json:"controlling_permissions_project_luid,omitempty"`
}

// UpdateCompactMutationResult omits successful request diagnostics.
type UpdateCompactMutationResult struct {
	Status  string               `json:"status"`
	Project UpdateCompactProject `json:"project"`
}

// UpdateCompactResult is the default projection.
type UpdateCompactResult struct {
	Plan    UpdatePlan                   `json:"plan"`
	Result  *UpdateCompactMutationResult `json:"result,omitempty"`
	Details string                       `json:"details"`
	Help    []string                     `json:"help"`
}

// UpdateFullResult is the expanded projection.
type UpdateFullResult = UpdateOutput

// CompactOutput returns exact mutation identity without the successful request ID.
func (o UpdateOutput) CompactOutput() any {
	var result *UpdateCompactMutationResult
	if o.Result != nil {
		result = &UpdateCompactMutationResult{Status: o.Result.Status, Project: UpdateCompactProject{LUID: o.Result.Project.LUID, Name: o.Result.Project.Name, Path: o.Result.Project.Path, ParentLUID: o.Result.Project.ParentLUID, ContentPermissions: o.Result.Project.ContentPermissions, ControllingPermissionsProjectID: o.Result.Project.ControllingPermissionsProjectID}}
	}
	return UpdateCompactResult{Plan: o.Plan, Result: result, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded mutation details.
func (o UpdateOutput) FullOutput() any { return UpdateFullResult(o) }
