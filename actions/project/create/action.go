package create

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// Resolver owns exact parent resolution and sibling collision checks.
type Resolver interface {
	ResolveProject(context.Context, identity.Selector) (Project, error)
	FindProjectCollisions(context.Context, string, string) ([]Project, error)
}

// Creator performs one exact released project create request.
type Creator interface {
	CreateProject(context.Context, CreateRequest) (Result, error)
}

// Action creates one project after immediate revalidation.
type Action struct {
	resolver Resolver
	creator  Creator
}

// New creates a project-create action.
func New(resolver Resolver, creator Creator) *Action {
	return &Action{resolver: resolver, creator: creator}
}

// Execute previews or creates one exact project.
func (a *Action) Execute(ctx context.Context, input Input, preview bool) (Output, error) {
	if a == nil || a.resolver == nil || a.creator == nil {
		return Output{}, &errs.Error{ID: "project.create.unconfigured", Kind: errs.KindRuntime, Operation: "project.create", Summary: "Project create is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure project creation before retrying."}
	}
	if err := validateInput(input); err != nil {
		return Output{}, err
	}
	parent, err := a.resolveParent(ctx, input.ParentSelector)
	if err != nil {
		return Output{}, resolutionError(input, "Parent project resolution failed.", err)
	}
	parentLUID := ""
	if parent != nil {
		parentLUID = parent.LUID
	}
	if err := a.rejectCollision(ctx, input, parentLUID); err != nil {
		return Output{}, err
	}
	plan := Plan{Mode: "preview", Operation: "project.create", Environment: input.Environment, Site: input.Site, Project: ProjectSpec{Name: input.Name, Description: input.Description, ContentPermissions: input.ContentPermissions}, Parent: parent}
	output := Output{Plan: plan, Help: []string{"Run without --preview to create this exact project."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	currentParent, err := a.resolveParent(ctx, input.ParentSelector)
	if err != nil {
		return Output{}, resolutionError(input, "Parent project revalidation failed.", err)
	}
	currentParentLUID := ""
	if currentParent != nil {
		currentParentLUID = currentParent.LUID
	}
	if currentParentLUID != parentLUID {
		return Output{}, &errs.Error{ID: "project.create.parent_changed", Kind: errs.KindOperation, Operation: "project.create", Environment: input.Environment, Site: input.Site, Summary: "The project parent identity changed during revalidation.", Cause: errors.New("project parent LUID changed during revalidation"), Retryable: errs.Bool(false), CorrectiveAction: "Review a new preview before creating the project."}
	}
	if err := a.rejectCollision(ctx, input, currentParentLUID); err != nil {
		return Output{}, err
	}
	result, err := a.creator.CreateProject(ctx, CreateRequest{Name: input.Name, Description: input.Description, ParentLUID: currentParentLUID, ContentPermissions: input.ContentPermissions})
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Inspect the remote project create outcome before retrying.")
		return Output{}, &errs.Error{ID: "project.create.failed", Kind: errs.KindOperation, Operation: "project.create", Environment: input.Environment, Site: input.Site, Summary: "Project create failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	output.Plan.Parent = currentParent
	output.Result = &result
	output.Help = []string{"tadx content project inspect --project-id " + result.Project.LUID}
	return output, nil
}

func (a *Action) resolveParent(ctx context.Context, selector identity.Selector) (*Project, error) {
	if selector.LUID == "" && strings.TrimSpace(selector.ProjectPath) == "" {
		return nil, nil
	}
	project, err := a.resolver.ResolveProject(ctx, selector)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(project.LUID) == "" {
		return nil, errors.New("parent project resolution omitted the authoritative LUID")
	}
	return &project, nil
}

func (a *Action) rejectCollision(ctx context.Context, input Input, parentLUID string) error {
	matches, err := a.resolver.FindProjectCollisions(ctx, input.Name, parentLUID)
	if err != nil {
		return resolutionError(input, "Project collision check failed.", err)
	}
	if len(matches) == 0 {
		return nil
	}
	return &errs.Error{ID: "project.create.collision", Kind: errs.KindOperation, Operation: "project.create", Resource: matches[0].LUID, Environment: input.Environment, Site: input.Site, Summary: "A sibling project with the same case-insensitive name already exists.", Cause: fmt.Errorf("project %q already exists under the selected parent", matches[0].LUID), Retryable: errs.Bool(false), CorrectiveAction: "Choose a different name or exact parent, then review a new preview."}
}

func validateInput(input Input) error {
	if strings.TrimSpace(input.Environment) == "" || strings.TrimSpace(input.Site) == "" {
		return usage("environment", "project create requires an explicit resolved environment and site")
	}
	if strings.TrimSpace(input.Name) == "" {
		return usage("name", "project create requires a name")
	}
	if strings.Contains(input.Name, "/") {
		return usage("name", "project create name cannot contain a slash")
	}
	if input.ParentSelector.Name != "" || (input.ParentSelector.LUID != "" && strings.TrimSpace(input.ParentSelector.ProjectPath) != "") {
		return usage("parent", "use either a parent LUID or an exact parent project path")
	}
	if !validContentPermissions(input.ContentPermissions) {
		return usage("content_permissions", "project create content permissions are invalid")
	}
	return nil
}

func validContentPermissions(value string) bool {
	return value == "" || value == "ManagedByOwner" || value == "LockedToProject" || value == "LockedToProjectWithoutNested"
}

func resolutionError(input Input, summary string, err error) error {
	retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact project selector, then retry.")
	return &errs.Error{ID: "project.create.resolve", Kind: errs.KindOperation, Operation: "project.create", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
}

func usage(field, message string) error {
	return &errs.Error{ID: "project.create.usage", Kind: errs.KindUsage, Operation: "project.create", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the project create input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "invalid", Message: message}}}
}
