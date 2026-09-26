package flow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

type ArtifactReader interface {
	ReadFlow(context.Context, string) (PublishArtifact, error)
}
type PublishResolver interface {
	ProjectResolver
	CollisionReader
}
type PreparedPublish interface {
	Commit(context.Context) (PublishResult, error)
}
type PublishPreparer interface {
	Prepare(context.Context, PublishRequest) (PreparedPublish, error)
}
type Publisher struct {
	artifacts ArtifactReader
	resolver  PublishResolver
	publisher PublishPreparer
}

func NewPublish(artifacts ArtifactReader, resolver PublishResolver, publisher PublishPreparer) *Publisher {
	return &Publisher{artifacts: artifacts, resolver: resolver, publisher: publisher}
}
func (a *Publisher) Execute(ctx context.Context, input PublishInput, preview bool) (PublishOutput, error) {
	if err := ValidatePublishInput(input); err != nil {
		return PublishOutput{}, err
	}
	ctx = a.beginProjectResolution(ctx)
	if a == nil || a.artifacts == nil || a.resolver == nil || a.publisher == nil {
		return PublishOutput{}, publishUnconfigured()
	}
	plan, err := a.plan(ctx, input)
	if err != nil {
		return PublishOutput{}, err
	}
	plan.SourceKind = "managed_artifact"
	if input.File != "" {
		plan.SourceKind = "native_file"
	}
	output := PublishOutput{Plan: plan, Help: []string{"Run without --preview to publish this exact plan."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	output.Help = nil
	ctx = a.beginProjectResolution(ctx)
	// Revalidate the exact artifact, destination, and collision BEFORE preparing
	// the upload. Prepare uploads the native flow (a server-side side effect); a
	// revalidation failure after Prepare would strand that upload with no cleanup
	// hook. Prepare therefore runs last, immediately before Commit, with no
	// fallible gate between them.
	current, err := a.artifacts.ReadFlow(ctx, input.ArtifactPath)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Repair or pull the exact flow artifact, then review a new preview.")
		return PublishOutput{}, &errs.Error{ID: "flow.publish.reread", Kind: errs.KindOperation, Operation: "flow.publish", Environment: input.Environment, Site: input.Site, Summary: "Flow artifact revalidation read failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction}
	}
	if current.Fingerprint != plan.ArtifactFingerprint {
		return PublishOutput{}, &errs.Error{ID: "flow.publish.artifact_changed", Kind: errs.KindOperation, Operation: "flow.publish", Environment: input.Environment, Site: input.Site, Summary: "The flow artifact changed during revalidation.", Cause: errors.New("flow artifact changed during revalidation"), Retryable: errs.Bool(false), CorrectiveAction: "Review a new preview before publishing."}
	}
	project, err := a.resolver.ResolveProject(ctx, input.ProjectSelector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact destination project and target, then retry.")
		return PublishOutput{}, &errs.Error{ID: "flow.publish.project", Kind: errs.KindOperation, Operation: "flow.publish", Environment: input.Environment, Site: input.Site, Summary: "Publish project resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	if project.LUID != plan.Target.ProjectLUID || project.Path != plan.Target.ProjectPath {
		return PublishOutput{}, &errs.Error{ID: "flow.publish.target_changed", Kind: errs.KindOperation, Operation: "flow.publish", Environment: input.Environment, Site: input.Site, Summary: "The flow publish destination changed during revalidation.", Cause: errors.New("flow publish destination changed during revalidation"), Retryable: errs.Bool(false), CorrectiveAction: "Review a new preview before publishing."}
	}
	existing, err := publishExactCollision(ctx, a.resolver, plan.FlowName, project.LUID, input.Environment, input.Site, input.Overwrite)
	if err != nil {
		return PublishOutput{}, err
	}
	if existing != plan.Target.ExistingLUID {
		return PublishOutput{}, &errs.Error{ID: "flow.publish.collision_changed", Kind: errs.KindOperation, Operation: "flow.publish", Environment: input.Environment, Site: input.Site, Summary: "The flow publish collision changed during revalidation.", Cause: errors.New("flow publish collision changed during revalidation"), Retryable: errs.Bool(false), CorrectiveAction: "Review a new preview before publishing."}
	}
	prepared, err := a.publisher.Prepare(ctx, plan.request)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the artifact and upload response, then prepare the publish again.")
		return PublishOutput{}, &errs.Error{ID: "flow.publish.prepare", Kind: errs.KindOperation, Operation: "flow.publish", Environment: input.Environment, Site: input.Site, Summary: "Flow publish preparation failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	result, err := prepared.Commit(ctx)
	if err != nil {
		if known, ok := errors.AsType[*errs.Error](err); ok && known.Phase != "" {
			if result != (PublishResult{}) {
				output.Result = &result
			}
			return output, err
		}
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the upstream error before publishing again.")
		if result != (PublishResult{}) {
			output.Result = &result
		}
		return output, &errs.Error{ID: "flow.publish.failed", Kind: errs.KindOperation, Operation: "flow.publish", Environment: input.Environment, Site: input.Site, Summary: "Flow publish failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err), Phase: errs.PhaseSubmission, Outcome: errs.OutcomeUnknown}
	}
	output.Result = &result
	return output, nil
}
func (a *Publisher) plan(ctx context.Context, input PublishInput) (PublishPlan, error) {
	if input.Environment == "" || (input.Site == "" && !input.TargetResolved) {
		return PublishPlan{}, publishUsage("environment", "flow publish requires an explicit resolved environment and site")
	}
	artifact, err := a.artifacts.ReadFlow(ctx, input.ArtifactPath)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Repair or pull the exact flow artifact, then review a new preview.")
		return PublishPlan{}, &errs.Error{ID: "flow.publish.read", Kind: errs.KindOperation, Operation: "flow.publish", Environment: input.Environment, Site: input.Site, Summary: "Flow artifact read failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction}
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = artifact.Name
	}
	if name == "" {
		return PublishPlan{}, publishUsage("name", "flow publish requires a flow name")
	}
	project, err := a.resolver.ResolveProject(ctx, input.ProjectSelector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact destination project and target, then retry.")
		return PublishPlan{}, &errs.Error{ID: "flow.publish.project", Kind: errs.KindOperation, Operation: "flow.publish", Environment: input.Environment, Site: input.Site, Summary: "Publish project resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	existing, err := publishExactCollision(ctx, a.resolver, name, project.LUID, input.Environment, input.Site, input.Overwrite)
	if err != nil {
		return PublishPlan{}, err
	}
	return PublishPlan{Workspace: input.WorkspaceName, SourceLUID: artifact.TableauID, Mode: "preview", Operation: "flow.publish", ArtifactPath: artifact.Path, ArtifactFingerprint: artifact.Fingerprint, Filename: artifact.Filename, FlowName: name, Target: PublishTarget{Environment: input.Environment, Site: input.Site, ProjectLUID: project.LUID, ProjectPath: project.Path, ExistingLUID: existing}, Overwrite: input.Overwrite, Substeps: []string{"resolve exact destination", "check flow collision", "revalidate destination and collision", "upload native flow", "publish flow"}, request: PublishRequest{Name: name, ProjectLUID: project.LUID, Filename: artifact.Filename, ContentPath: artifact.PayloadPath, ContentSize: artifact.Size, ExpectedFingerprint: artifact.Fingerprint, Overwrite: input.Overwrite}, planned: true}, nil
}
func publishExactCollision(ctx context.Context, resolver PublishResolver, name, project, environment, site string, overwrite bool) (string, error) {
	items, err := resolver.FindFlows(ctx, name, project)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Resolve the exact destination again before publishing.")
		return "", &errs.Error{ID: "flow.publish.collision", Kind: errs.KindOperation, Operation: "flow.publish", Environment: environment, Site: site, Summary: "Flow collision check failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	if len(items) > 1 {
		return "", &errs.Error{ID: "flow.publish.ambiguous", Kind: errs.KindOperation, Operation: "flow.publish", Environment: environment, Site: site, Summary: "Flow publish target is ambiguous.", Cause: fmt.Errorf("%d authoritative flows named %q match the publish collision", len(items), name), Retryable: errs.Bool(false), CorrectiveAction: "Select or rename one authoritative flow target before publishing."}
	}
	if len(items) == 0 {
		return "", nil
	}
	if items[0].LUID == "" || items[0].ProjectLUID != project {
		return "", &errs.Error{ID: "flow.publish.collision_identity", Kind: errs.KindOperation, Operation: "flow.publish", Environment: environment, Site: site, Summary: "Flow collision omitted authoritative destination identity.", Cause: errors.New("flow collision omitted authoritative destination identity"), Retryable: errs.Bool(false), CorrectiveAction: "Resolve the exact destination again before publishing."}
	}
	if !overwrite {
		return "", &errs.Error{ID: "flow.publish.conflict", Kind: errs.KindOperation, Operation: "flow.publish", Resource: items[0].LUID, Environment: environment, Site: site, Summary: "A flow with this name already exists.", Cause: fmt.Errorf("flow %q already exists in the destination; use --overwrite", name), Retryable: errs.Bool(false), CorrectiveAction: "Review the exact existing flow, then use --overwrite only when replacement is intended."}
	}
	return items[0].LUID, nil
}

func publishUsage(field, message string) error {
	return &errs.Error{ID: "flow.publish.usage", Kind: errs.KindUsage, Operation: "flow.publish", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the flow publish input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}

func publishUnconfigured() error {
	return &errs.Error{ID: "flow.publish.unconfigured", Kind: errs.KindRuntime, Operation: "flow.publish", Summary: "Flow publish is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure flow publishing before retrying."}
}

// ValidatePublishInput checks caller-controlled arguments before dependency setup.
func ValidatePublishInput(input PublishInput) error {
	if strings.TrimSpace(input.ArtifactPath) == "" && strings.TrimSpace(input.File) == "" && strings.TrimSpace(input.ArtifactID) == "" && strings.TrimSpace(input.ArtifactName) == "" {
		return publishUsage("artifact", "an explicit flow artifact is required")
	}
	if input.ProjectSelector.LUID != "" && input.ProjectSelector.ProjectPath != "" {
		return publishUsage("project", "a project LUID cannot be combined with a project path")
	}
	return nil
}

// A resolver may share project reads within one explicit validation phase.
// Each prewrite phase starts again, never inheriting the planning snapshot.
type publishProjectResolutionPhase interface {
	BeginProjectResolution(context.Context) context.Context
}

func (a *Publisher) beginProjectResolution(ctx context.Context) context.Context {
	if a != nil {
		if resolver, ok := a.resolver.(publishProjectResolutionPhase); ok {
			return resolver.BeginProjectResolution(ctx)
		}
	}
	return ctx
}
