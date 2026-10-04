package project

import (
	"context"
	"errors"
	"fmt"
	"github.com/ahillspace/tadx/internal/commandhint"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// Resolver owns exact parent resolution and sibling collision checks.
type CreateResolver interface {
	ResolveProject(context.Context, identity.Selector) (CreateProject, error)
	FindProjectCollisions(context.Context, string, string) ([]CreateProject, error)
	BeginProjectResolution(context.Context) context.Context
}

// Creator performs one exact released project create request.
type CreateCreator interface {
	CreateProject(context.Context, CreateRequest) (CreateResult, error)
}

func (a *runner) beginCreateResolution(ctx context.Context) context.Context {
	return a.CreateResolver.BeginProjectResolution(ctx)
}

// createValidated previews or creates one exact project after immediate revalidation.
func (a *runner) createValidated(ctx context.Context, input CreateInput, preview bool) (CreateOutput, error) {
	if a == nil || a.CreateResolver == nil || a.Creator == nil {
		return CreateOutput{}, &errs.Error{ID: "project.create.unconfigured", Kind: errs.KindRuntime, Operation: "project.create", Summary: "Project create is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure project creation before retrying."}
	}
	if err := createValidateResolvedTarget(input); err != nil {
		return CreateOutput{}, err
	}
	ctx = a.beginCreateResolution(ctx)
	parent, err := a.resolveParent(ctx, input.ParentSelector)
	if err != nil {
		return CreateOutput{}, createResolutionError(input, "Parent project resolution failed.", err)
	}
	parentLUID := ""
	if parent != nil {
		parentLUID = parent.LUID
	}
	if err := a.rejectCollision(ctx, input, parentLUID); err != nil {
		return CreateOutput{}, err
	}
	plan := CreatePlan{Mode: "preview", Operation: "project.create", Environment: input.Environment, Site: input.Site, Project: CreateProjectSpec{Name: input.Name, Description: input.Description, ContentPermissions: input.ContentPermissions}, Parent: parent}
	output := CreateOutput{Plan: plan, Help: []string{"Run without --preview to create this exact project."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	ctx = a.beginCreateResolution(ctx)
	currentParent, err := a.resolveParent(ctx, input.ParentSelector)
	if err != nil {
		return CreateOutput{}, createResolutionError(input, "Parent project revalidation failed.", err)
	}
	currentParentLUID := ""
	if currentParent != nil {
		currentParentLUID = currentParent.LUID
	}
	if currentParentLUID != parentLUID {
		return CreateOutput{}, &errs.Error{ID: "project.create.parent_changed", Kind: errs.KindOperation, Operation: "project.create", Environment: input.Environment, Site: input.Site, Summary: "The project parent identity changed during revalidation.", Cause: errors.New("project parent LUID changed during revalidation"), Retryable: errs.Bool(false), CorrectiveAction: "Review a new preview before creating the project."}
	}
	if err := a.rejectCollision(ctx, input, currentParentLUID); err != nil {
		return CreateOutput{}, err
	}
	result, err := a.Creator.CreateProject(ctx, CreateRequest{Name: input.Name, Description: input.Description, ParentLUID: currentParentLUID, ContentPermissions: input.ContentPermissions})
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Inspect the remote project create outcome before retrying.")
		failure := &errs.Error{ID: "project.create.failed", Kind: errs.KindOperation, Operation: "project.create", Environment: input.Environment, Site: input.Site, Summary: "Project create failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
		if result.TableauRequestID != "" {
			failure.TableauRequestID = result.TableauRequestID
		}
		if result.Status == "unknown" {
			failure.Phase = errs.PhaseSubmission
			failure.Outcome = errs.OutcomeUnknown
			failure.Retryable = errs.Bool(false)
			failure.CorrectiveAction = "Inspect the remote project create outcome before retrying."
			output.Plan.Parent = currentParent
			output.Result = &result
			if result.Project.LUID != "" {
				failure.Resource = result.Project.LUID
				failure.Phase = errs.PhaseVerification
				inspect := commandhint.Environment(input.Environment, "content", "project", "inspect", "--project-id", result.Project.LUID)
				failure.CorrectiveAction = "Inspect the project with " + inspect + " before another mutation."
				output.Help = []string{inspect}
			}
		}
		return output, failure
	}
	output.Plan.Parent = currentParent
	output.Result = &result
	output.Help = []string{commandhint.Environment(input.Environment, "content", "project", "inspect", "--project-id", result.Project.LUID)}
	return output, nil
}

func (a *runner) resolveParent(ctx context.Context, selector identity.Selector) (*CreateProject, error) {
	if selector.LUID == "" && strings.TrimSpace(selector.ProjectPath) == "" {
		return nil, nil
	}
	project, err := a.CreateResolver.ResolveProject(ctx, selector)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(project.LUID) == "" {
		return nil, errors.New("parent project resolution omitted the authoritative LUID")
	}
	return &project, nil
}

func (a *runner) rejectCollision(ctx context.Context, input CreateInput, parentLUID string) error {
	matches, err := a.CreateResolver.FindProjectCollisions(ctx, input.Name, parentLUID)
	if err != nil {
		return createResolutionError(input, "Project collision check failed.", err)
	}
	if len(matches) == 0 {
		return nil
	}
	return &errs.Error{ID: "project.create.collision", Kind: errs.KindOperation, Operation: "project.create", Resource: matches[0].LUID, Environment: input.Environment, Site: input.Site, Summary: "A sibling project with the same case-insensitive name already exists.", Cause: fmt.Errorf("project %q already exists under the selected parent", matches[0].LUID), Retryable: errs.Bool(false), CorrectiveAction: "Choose a different name or exact parent, then review a new preview."}
}

func createValidateResolvedTarget(input CreateInput) error {
	if strings.TrimSpace(input.Environment) == "" || (strings.TrimSpace(input.Site) == "" && !input.TargetResolved) {
		return createUsage("environment", "project create requires an explicit resolved environment and site")
	}
	return nil
}

// ValidateInput checks caller-controlled arguments before local or remote setup.
func ValidateCreateInput(input CreateInput) error {
	if strings.TrimSpace(input.Environment) == "" {
		return createUsage("environment", "project create requires an explicit environment")
	}
	if strings.TrimSpace(input.Name) == "" {
		return createUsage("name", "project create requires a name")
	}
	if strings.Contains(input.Name, "/") {
		return createUsage("name", "project create name cannot contain a slash")
	}
	if input.ParentSelector.Name != "" || (input.ParentSelector.LUID != "" && strings.TrimSpace(input.ParentSelector.ProjectPath) != "") {
		return createUsage("parent", "use either a parent LUID or an exact parent project path")
	}
	if !createValidContentPermissions(input.ContentPermissions) {
		return createUsage("content_permissions", "project create content permissions are invalid")
	}
	return nil
}

func createValidContentPermissions(value string) bool {
	return value == "" || value == "ManagedByOwner" || value == "LockedToProject" || value == "LockedToProjectWithoutNested"
}

func createResolutionError(input CreateInput, summary string, err error) error {
	retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact project selector, then retry.")
	return &errs.Error{ID: "project.create.resolve", Kind: errs.KindOperation, Operation: "project.create", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
}

func createUsage(field, message string) error {
	return &errs.Error{ID: "project.create.usage", Kind: errs.KindUsage, Operation: "project.create", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the project create input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "invalid", Message: message}}}
}
