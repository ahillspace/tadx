package publish

import (
	"context"
	"errors"
	"fmt"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// ArtifactReader reads one canonical local workbook.
type ArtifactReader interface {
	ReadWorkbook(context.Context, string) (Artifact, error)
}

// Resolver performs authoritative destination and collision reads.
type Resolver interface {
	ResolveProject(context.Context, identity.Selector) (Project, error)
	FindWorkbooks(context.Context, string, string) ([]Workbook, error)
}

// Publisher performs the consequential remote mutation.
type Publisher interface {
	Publish(context.Context, PublishRequest) (Result, error)
}

// Action orchestrates workbook.publish preview and apply.
type Action struct {
	artifacts ArtifactReader
	resolver  Resolver
	publisher Publisher
}

// New creates workbook.publish.
func New(artifacts ArtifactReader, resolver Resolver, publisher Publisher) *Action {
	return &Action{artifacts: artifacts, resolver: resolver, publisher: publisher}
}

// Plan performs authoritative reads and returns a preview without mutation.
func (a *Action) Plan(ctx context.Context, input Input) (Plan, error) {
	if a == nil || a.artifacts == nil || a.resolver == nil || a.publisher == nil {
		return Plan{}, errs.New(errs.KindRuntime, "Workbook publish is not configured.")
	}
	if input.Environment == "" {
		return Plan{}, usage("environment", "an explicit write environment is required")
	}
	if input.Site == "" && !input.TargetResolved {
		return Plan{}, usage("site", "an explicit write site is required")
	}
	if input.ArtifactPath == "" {
		return Plan{}, usage("artifact", "an explicit workbook artifact is required")
	}
	artifact, err := a.artifacts.ReadWorkbook(ctx, input.ArtifactPath)
	if err != nil {
		return Plan{}, &errs.Error{ID: "workbook.artifact.read", Kind: errs.KindOperation, Operation: "workbook.publish", Environment: input.Environment, Site: input.Site, Summary: "Workbook artifact read failed.", Cause: err}
	}
	name := input.Name
	if name == "" {
		name = artifact.Name
	}
	if name == "" {
		return Plan{}, usage("name", "workbook name is required")
	}
	projectSelector := input.ProjectSelector
	if projectSelector.LUID == "" && projectSelector.ProjectPath == "" {
		projectSelector = identity.Selector{LUID: identity.LUID(input.ProjectLUID), ProjectPath: input.ProjectPath}
	}
	project, err := a.resolver.ResolveProject(ctx, projectSelector)
	if err != nil {
		return Plan{}, &errs.Error{ID: "project.resolve.failed", Kind: errs.KindOperation, Operation: "workbook.publish", Environment: input.Environment, Site: input.Site, Summary: "Publish project resolution failed.", Cause: err, TableauRequestID: errs.TableauRequestID(err)}
	}
	existing, err := a.resolver.FindWorkbooks(ctx, name, project.LUID)
	if err != nil {
		return Plan{}, &errs.Error{ID: "workbook.collision.read", Kind: errs.KindOperation, Operation: "workbook.publish", Environment: input.Environment, Site: input.Site, Summary: "Workbook collision check failed.", Cause: err, TableauRequestID: errs.TableauRequestID(err)}
	}
	if len(existing) > 1 {
		return Plan{}, &errs.Error{ID: "workbook.selector.ambiguous", Kind: errs.KindOperation, Operation: "workbook.publish", Environment: input.Environment, Site: input.Site, Summary: "Workbook publish target is ambiguous.", Cause: fmt.Errorf("%d workbooks named %q exist in project %q", len(existing), name, project.Path)}
	}
	if len(existing) == 1 && !input.Overwrite {
		return Plan{}, &errs.Error{ID: "workbook.collision", Kind: errs.KindOperation, Operation: "workbook.publish", Resource: existing[0].LUID, Environment: input.Environment, Site: input.Site, Summary: "A workbook with this name already exists.", Cause: errors.New("use --overwrite to replace the exact existing workbook")}
	}
	existingLUID := ""
	if len(existing) == 1 {
		existingLUID = existing[0].LUID
	}
	request := PublishRequest{Name: name, ProjectLUID: project.LUID, Filename: artifact.Filename, ContentPath: artifact.PayloadPath, ContentSize: artifact.Size, ExpectedFingerprint: artifact.Fingerprint, Overwrite: input.Overwrite, AsJob: input.AsJob}
	return Plan{
		Mode: "preview", Operation: "workbook.publish", ArtifactPath: artifact.Path,
		ArtifactFingerprint: artifact.Fingerprint, Filename: artifact.Filename, WorkbookName: name,
		Target:    Target{Environment: input.Environment, Site: input.Site, ProjectLUID: project.LUID, ProjectPath: project.Path, ExistingLUID: existingLUID},
		Overwrite: input.Overwrite, AsJob: input.AsJob,
		Substeps: []string{"resolve exact destination", "check workbook collision", "upload workbook", "publish workbook", "poll asynchronous job when requested"},
		request:  request, planned: true,
	}, nil
}

// Apply performs only the exact mutation request captured by Plan.
func (a *Action) Apply(ctx context.Context, plan Plan) (Result, error) {
	if a == nil || a.resolver == nil || a.publisher == nil {
		return Result{}, errs.New(errs.KindRuntime, "Workbook publish is not configured.")
	}
	if !plan.planned || plan.Operation != "workbook.publish" || plan.request.Name == "" || plan.request.ProjectLUID == "" {
		return Result{}, usage("plan", "workbook publish apply requires a plan produced by Plan")
	}
	if plan.request.Overwrite {
		if err := a.verifyOverwriteTarget(ctx, plan); err != nil {
			return Result{}, err
		}
	}
	result, err := a.publisher.Publish(ctx, plan.request)
	if err != nil {
		requestID := result.TableauRequestID
		if requestID == "" {
			requestID = errs.TableauRequestID(err)
		}
		errorID := "workbook.publish.failed"
		summary := "Workbook publish failed."
		correctiveAction := "Review the upstream error before publishing again."
		if result.Status == "unknown" || result.Status == "timed_out" || result.Status == "cancelled" {
			errorID = "workbook.publish.outcome_unknown"
			summary = "The workbook publish outcome could not be determined."
			correctiveAction = "Inspect the target site and Tableau request before attempting another publish."
			if result.JobID != "" {
				correctiveAction = "Inspect the Tableau job by its exact job ID before attempting another publish."
			}
		}
		return Result{}, &errs.Error{ID: errorID, Kind: errs.KindOperation, Operation: "workbook.publish", Resource: plan.Target.ExistingLUID, Environment: plan.Target.Environment, Site: plan.Target.Site, Summary: summary, Cause: err, Retryable: errs.Bool(false), CorrectiveAction: correctiveAction, TableauJobID: result.JobID, TableauRequestID: requestID}
	}
	return result, nil
}

func (a *Action) verifyOverwriteTarget(ctx context.Context, plan Plan) error {
	current, err := a.resolver.FindWorkbooks(ctx, plan.request.Name, plan.request.ProjectLUID)
	if err != nil {
		retryable, correctiveAction := errs.RetryAdvice(err)
		if retryable == nil {
			retryable = errs.Bool(false)
		}
		if correctiveAction == "" {
			correctiveAction = "Resolve the exact destination again before publishing."
		}
		return &errs.Error{ID: "workbook.overwrite.revalidate", Kind: errs.KindOperation, Operation: "workbook.publish", Resource: plan.Target.ExistingLUID, Environment: plan.Target.Environment, Site: plan.Target.Site, Summary: "Workbook overwrite target revalidation failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	expected := plan.Target.ExistingLUID
	if len(current) == 0 && expected == "" {
		return nil
	}
	if len(current) == 1 && current[0].LUID == expected {
		return nil
	}
	return &errs.Error{ID: "workbook.overwrite.target_changed", Kind: errs.KindOperation, Operation: "workbook.publish", Resource: expected, Environment: plan.Target.Environment, Site: plan.Target.Site, Summary: "Workbook overwrite target changed after planning.", Cause: fmt.Errorf("planned workbook LUID %q no longer matches the exact destination", expected), Retryable: errs.Bool(false), CorrectiveAction: "Review a new preview before publishing."}
}

// Execute plans every invocation and applies only when explicitly requested.
func (a *Action) Execute(ctx context.Context, input Input, apply bool) (Output, error) {
	plan, err := a.Plan(ctx, input)
	if err != nil {
		return Output{}, err
	}
	output := Output{Plan: plan, Applied: false}
	if !apply {
		return output, nil
	}
	result, err := a.Apply(ctx, plan)
	if err != nil {
		return Output{}, err
	}
	output.Applied = true
	output.Result = &result
	return output, nil
}

func usage(field, message string) error {
	return &errs.Error{ID: "workbook.publish.usage", Kind: errs.KindUsage, Operation: "workbook.publish", Summary: message, Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}
