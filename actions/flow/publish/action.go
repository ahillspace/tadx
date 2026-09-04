package publish

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

type ArtifactReader interface {
	ReadFlow(context.Context, string) (Artifact, error)
}
type Resolver interface {
	ResolveProject(context.Context, identity.Selector) (Project, error)
	FindFlows(context.Context, string, string) ([]Flow, error)
}
type PreparedPublish interface {
	Commit(context.Context) (Result, error)
}
type Publisher interface {
	Prepare(context.Context, PublishRequest) (PreparedPublish, error)
}
type Action struct {
	artifacts ArtifactReader
	resolver  Resolver
	publisher Publisher
}

func New(artifacts ArtifactReader, resolver Resolver, publisher Publisher) *Action {
	return &Action{artifacts: artifacts, resolver: resolver, publisher: publisher}
}
func (a *Action) Execute(ctx context.Context, input Input, preview bool) (Output, error) {
	if a == nil || a.artifacts == nil || a.resolver == nil || a.publisher == nil {
		return Output{}, unconfigured()
	}
	plan, err := a.plan(ctx, input)
	if err != nil {
		return Output{}, err
	}
	output := Output{Plan: plan, Help: []string{"Run without --preview to publish this exact plan."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	// Revalidate the exact artifact, destination, and collision BEFORE preparing
	// the upload. Prepare uploads the native flow (a server-side side effect); a
	// revalidation failure after Prepare would strand that upload with no cleanup
	// hook. Prepare therefore runs last, immediately before Commit, with no
	// fallible gate between them.
	current, err := a.artifacts.ReadFlow(ctx, input.ArtifactPath)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Repair or pull the exact flow artifact, then review a new preview.")
		return Output{}, &errs.Error{ID: "flow.publish.reread", Kind: errs.KindOperation, Operation: "flow.publish", Environment: input.Environment, Site: input.Site, Summary: "Flow artifact revalidation read failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction}
	}
	if current.Fingerprint != plan.ArtifactFingerprint {
		return Output{}, &errs.Error{ID: "flow.publish.artifact_changed", Kind: errs.KindOperation, Operation: "flow.publish", Environment: input.Environment, Site: input.Site, Summary: "The flow artifact changed during revalidation.", Cause: errors.New("flow artifact changed during revalidation"), Retryable: errs.Bool(false), CorrectiveAction: "Review a new preview before publishing."}
	}
	project, err := a.resolver.ResolveProject(ctx, input.ProjectSelector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact destination project and target, then retry.")
		return Output{}, &errs.Error{ID: "flow.publish.project", Kind: errs.KindOperation, Operation: "flow.publish", Environment: input.Environment, Site: input.Site, Summary: "Publish project resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	if project.LUID != plan.Target.ProjectLUID || project.Path != plan.Target.ProjectPath {
		return Output{}, &errs.Error{ID: "flow.publish.target_changed", Kind: errs.KindOperation, Operation: "flow.publish", Environment: input.Environment, Site: input.Site, Summary: "The flow publish destination changed during revalidation.", Cause: errors.New("flow publish destination changed during revalidation"), Retryable: errs.Bool(false), CorrectiveAction: "Review a new preview before publishing."}
	}
	existing, err := exactCollision(ctx, a.resolver, plan.FlowName, project.LUID, input.Environment, input.Site, input.Overwrite)
	if err != nil {
		return Output{}, err
	}
	if existing != plan.Target.ExistingLUID {
		return Output{}, &errs.Error{ID: "flow.publish.collision_changed", Kind: errs.KindOperation, Operation: "flow.publish", Environment: input.Environment, Site: input.Site, Summary: "The flow publish collision changed during revalidation.", Cause: errors.New("flow publish collision changed during revalidation"), Retryable: errs.Bool(false), CorrectiveAction: "Review a new preview before publishing."}
	}
	prepared, err := a.publisher.Prepare(ctx, plan.request)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the artifact and upload response, then prepare the publish again.")
		return Output{}, &errs.Error{ID: "flow.publish.prepare", Kind: errs.KindOperation, Operation: "flow.publish", Environment: input.Environment, Site: input.Site, Summary: "Flow publish preparation failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	result, err := prepared.Commit(ctx)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the upstream error before publishing again.")
		return Output{}, &errs.Error{ID: "flow.publish.failed", Kind: errs.KindOperation, Operation: "flow.publish", Environment: input.Environment, Site: input.Site, Summary: "Flow publish failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	output.Result = &result
	output.Help = []string{"tadx content flow inspect --id " + result.FlowLUID}
	return output, nil
}
func (a *Action) plan(ctx context.Context, input Input) (Plan, error) {
	if input.Environment == "" || input.Site == "" {
		return Plan{}, usage("environment", "flow publish requires an explicit resolved environment and site")
	}
	artifact, err := a.artifacts.ReadFlow(ctx, input.ArtifactPath)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Repair or pull the exact flow artifact, then review a new preview.")
		return Plan{}, &errs.Error{ID: "flow.publish.read", Kind: errs.KindOperation, Operation: "flow.publish", Environment: input.Environment, Site: input.Site, Summary: "Flow artifact read failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction}
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = artifact.Name
	}
	if name == "" {
		return Plan{}, usage("name", "flow publish requires a flow name")
	}
	project, err := a.resolver.ResolveProject(ctx, input.ProjectSelector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact destination project and target, then retry.")
		return Plan{}, &errs.Error{ID: "flow.publish.project", Kind: errs.KindOperation, Operation: "flow.publish", Environment: input.Environment, Site: input.Site, Summary: "Publish project resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	existing, err := exactCollision(ctx, a.resolver, name, project.LUID, input.Environment, input.Site, input.Overwrite)
	if err != nil {
		return Plan{}, err
	}
	return Plan{Mode: "preview", Operation: "flow.publish", ArtifactPath: artifact.Path, ArtifactFingerprint: artifact.Fingerprint, Filename: artifact.Filename, FlowName: name, Target: Target{Environment: input.Environment, Site: input.Site, ProjectLUID: project.LUID, ProjectPath: project.Path, ExistingLUID: existing}, Overwrite: input.Overwrite, Substeps: []string{"resolve exact destination", "check flow collision", "revalidate destination and collision", "upload native flow", "publish flow"}, request: PublishRequest{Name: name, ProjectLUID: project.LUID, Filename: artifact.Filename, ContentPath: artifact.PayloadPath, ContentSize: artifact.Size, ExpectedFingerprint: artifact.Fingerprint, Overwrite: input.Overwrite}, planned: true}, nil
}
func exactCollision(ctx context.Context, resolver Resolver, name, project, environment, site string, overwrite bool) (string, error) {
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

func usage(field, message string) error {
	return &errs.Error{ID: "flow.publish.usage", Kind: errs.KindUsage, Operation: "flow.publish", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the flow publish input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}

func unconfigured() error {
	return &errs.Error{ID: "flow.publish.unconfigured", Kind: errs.KindRuntime, Operation: "flow.publish", Summary: "Flow publish is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure flow publishing before retrying."}
}
