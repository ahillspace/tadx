package publish

import (
	"context"
	"errors"
	"fmt"
	"github.com/ahillspace/tadx/internal/commandhint"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

type ArtifactReader interface {
	ReadDatasource(context.Context, string) (Artifact, error)
}
type Resolver interface {
	ResolveProject(context.Context, identity.Selector) (Project, error)
	FindDatasources(context.Context, string, string) ([]Datasource, error)
	ResolvePublishedDatasource(context.Context, string, string) (Datasource, error)
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
	if err := ValidateInput(input); err != nil {
		return Output{}, err
	}
	ctx = a.beginProjectResolution(ctx)
	if a == nil || a.artifacts == nil || a.resolver == nil || a.publisher == nil {
		return Output{}, runtimeError("datasource.publish.unconfigured", "Datasource publish is not configured.", nil)
	}
	plan, err := a.plan(ctx, input)
	if err != nil {
		return Output{}, err
	}
	out := Output{Plan: plan, Help: []string{"Run without --preview to publish this exact plan."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	ctx = a.beginProjectResolution(ctx)
	artifact, err := a.artifacts.ReadDatasource(ctx, input.ArtifactPath)
	if err != nil {
		return Output{}, operationError("datasource.publish.reread", "Datasource artifact revalidation failed.", input, err)
	}
	if artifact.Fingerprint != plan.ArtifactFingerprint || artifact.CompositionStatus != plan.CompositionStatus || !slices.Equal(artifact.ParentDataSourceURLs, plan.ParentDataSourceURLs) {
		return Output{}, operationError("datasource.publish.artifact_changed", "The datasource artifact changed during revalidation.", input, errors.New("datasource artifact or composition references changed during revalidation"))
	}
	project, err := a.resolveProject(ctx, input, artifact)
	if err != nil {
		return Output{}, err
	}
	if project.LUID != plan.Target.ProjectLUID || project.Path != plan.Target.ProjectPath {
		return Output{}, operationError("datasource.publish.target_changed", "The datasource publish destination changed during revalidation.", input, errors.New("datasource publish destination changed during revalidation"))
	}
	existing, err := collision(ctx, a.resolver, plan.DatasourceName, project.LUID, input, artifact)
	if err != nil {
		return Output{}, err
	}
	if existing != plan.Target.ExistingLUID {
		return Output{}, operationError("datasource.publish.collision_changed", "The datasource publish collision changed during revalidation.", input, errors.New("datasource publish collision changed during revalidation"))
	}
	prepared, err := a.publisher.Prepare(ctx, plan.request)
	if err != nil {
		return Output{}, operationError("datasource.publish.prepare", "Datasource publish preparation failed.", input, err)
	}
	result, err := prepared.Commit(ctx)
	if err != nil {
		requestID := result.TableauRequestID
		if requestID == "" {
			requestID = errs.TableauRequestID(err)
		}
		errorID, summary := "datasource.publish.failed", "Datasource publish failed."
		correctiveAction := "Review the upstream error before publishing again."
		if result.Status == "unknown" || result.Status == "timed_out" || result.Status == "cancelled" {
			errorID, summary = "datasource.publish.outcome_unknown", "The datasource publish outcome could not be determined."
			correctiveAction = "Inspect the target site and Tableau request before attempting another publish."
			if result.JobID != "" {
				correctiveAction = "Inspect the Tableau job by its exact job ID before attempting another publish."
			}
		}
		if hint := publishInspectionHint(plan, result); hint != "" {
			correctiveAction += " Run " + hint + "."
		}
		return Output{}, &errs.Error{ID: errorID, Kind: errs.KindOperation, Operation: "datasource.publish", Resource: plan.Target.ExistingLUID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: err, Retryable: errs.Bool(false), CorrectiveAction: correctiveAction, TableauJobID: result.JobID, TableauRequestID: requestID}
	}
	if plan.AsJob {
		if result.Status != "succeeded" {
			completedStatus := result.Status
			result.Status = "unknown"
			return Output{}, unknownOutcomeError(plan, input, result, fmt.Errorf("completed Tableau datasource publish job returned status %q", completedStatus))
		}
		if result.DatasourceLUID == "" {
			resolved, resolveErr := a.resolver.ResolvePublishedDatasource(ctx, plan.DatasourceName, plan.Target.ProjectLUID)
			if resolveErr != nil {
				result.Status = "unknown"
				return Output{}, unknownOutcomeError(plan, input, result, fmt.Errorf("resolve completed datasource identity: %w", resolveErr))
			}
			if strings.TrimSpace(resolved.LUID) == "" || resolved.Name != plan.DatasourceName || resolved.ProjectLUID != plan.Target.ProjectLUID {
				result.Status = "unknown"
				return Output{}, unknownOutcomeError(plan, input, result, errors.New("completed datasource resolution returned incomplete or conflicting authoritative identity"))
			}
			result.DatasourceLUID, result.DatasourceName, result.ProjectLUID = resolved.LUID, resolved.Name, resolved.ProjectLUID
		} else if result.DatasourceName != plan.DatasourceName || result.ProjectLUID != plan.Target.ProjectLUID {
			result.Status = "unknown"
			return Output{}, unknownOutcomeError(plan, input, result, errors.New("completed datasource job returned identity conflicting with the exact publish target"))
		}
	}
	out.Result = &result
	out.Help = []string{commandhint.Environment(input.Environment, "content", "datasource", "inspect", "--id", result.DatasourceLUID)}
	return out, nil
}

func unknownOutcomeError(plan Plan, input Input, result Result, cause error) error {
	correctiveAction := "Inspect the target site and Tableau request before attempting another publish."
	if result.JobID != "" {
		correctiveAction = "Inspect the Tableau job and resolve the exact datasource name in the exact project before attempting another publish."
	}
	if hint := publishInspectionHint(plan, result); hint != "" {
		correctiveAction += " Run " + hint + "."
	}
	return &errs.Error{
		ID:               "datasource.publish.outcome_unknown",
		Kind:             errs.KindOperation,
		Operation:        "datasource.publish",
		Resource:         plan.Target.ExistingLUID,
		Environment:      input.Environment,
		Site:             input.Site,
		Summary:          "The datasource publish completed, but its authoritative datasource identity could not be determined safely.",
		Cause:            cause,
		Retryable:        errs.Bool(false),
		CorrectiveAction: correctiveAction,
		TableauJobID:     result.JobID,
		TableauRequestID: result.TableauRequestID,
	}
}

func (a *Action) plan(ctx context.Context, input Input) (Plan, error) {
	if strings.TrimSpace(input.Environment) == "" || (strings.TrimSpace(input.Site) == "" && !input.TargetResolved) {
		return Plan{}, usage("environment", "datasource publish requires an explicit resolved environment and site")
	}
	if !validMode(input.Mode) {
		return Plan{}, usage("mode", "datasource publish requires an explicit create, overwrite, append, or replace mode")
	}
	artifact, err := a.artifacts.ReadDatasource(ctx, input.ArtifactPath)
	if err != nil {
		return Plan{}, operationError("datasource.publish.read", "Datasource artifact read failed.", input, err)
	}
	parents := append([]string(nil), artifact.ParentDataSourceURLs...)
	if artifact.CompositionStatus == "ordinary" && len(parents) != 0 {
		return Plan{}, operationError("datasource.publish.composition", "Ordinary datasource metadata contains parent references.", input, errors.New("ordinary datasource cannot contain parent datasource URLs"))
	}
	if artifact.CompositionStatus == "composed" && len(parents) == 0 {
		return Plan{}, operationError("datasource.publish.composition", "Composed datasource metadata omitted parent references.", input, errors.New("composed datasource requires parent datasource URLs"))
	}
	if artifact.CompositionStatus != "ordinary" && artifact.CompositionStatus != "composed" {
		return Plan{}, operationError("datasource.publish.composition_unknown", "Datasource composition could not be classified safely.", input, errors.New("datasource composition status must be ordinary or composed"))
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = artifact.Name
	}
	if name == "" {
		return Plan{}, usage("name", "datasource publish requires a datasource name")
	}
	project, err := a.resolveProject(ctx, input, artifact)
	if err != nil {
		return Plan{}, err
	}
	existing, err := collision(ctx, a.resolver, name, project.LUID, input, artifact)
	if err != nil {
		return Plan{}, err
	}
	req := PublishRequest{Name: name, ProjectLUID: project.LUID, Filename: artifact.Filename, ContentPath: artifact.PayloadPath, ContentSize: artifact.Size, ExpectedFingerprint: artifact.Fingerprint, Mode: input.Mode, ParentDataSourceURLs: parents, AsJob: input.AsJob}
	return Plan{Workspace: input.WorkspaceName, SourceLUID: artifact.TableauID, Mode: "preview", PublishMode: input.Mode, Operation: "datasource.publish", ArtifactPath: artifact.Path, ArtifactFingerprint: artifact.Fingerprint, Filename: artifact.Filename, DatasourceName: name, CompositionStatus: artifact.CompositionStatus, ParentDataSourceURLs: parents, Target: Target{Environment: input.Environment, Site: input.Site, ProjectLUID: project.LUID, ProjectPath: project.Path, ExistingLUID: existing}, Substeps: []string{"resolve exact destination", "check datasource collision", "revalidate artifact and destination", "upload native datasource package", "publish datasource", "poll asynchronous job when requested"}, AsJob: input.AsJob, request: req}, nil
}

func (a *Action) resolveProject(ctx context.Context, input Input, artifact Artifact) (Project, error) {
	selector := input.ProjectSelector
	if input.SourceDefaulted {
		selector = identity.Selector{LUID: identity.LUID(artifact.SourceProjectID), ProjectPath: artifact.SourceProjectName}
	}
	project, err := a.resolver.ResolveProject(ctx, selector)
	if err != nil {
		return Project{}, operationError("datasource.publish.project", "Publish project resolution failed.", input, err)
	}
	return project, nil
}

func collision(ctx context.Context, resolver Resolver, name, project string, input Input, artifact Artifact) (string, error) {
	items, err := resolver.FindDatasources(ctx, name, project)
	if err != nil {
		return "", operationError("datasource.publish.collision", "Datasource collision check failed.", input, err)
	}
	if len(items) > 1 {
		return "", operationError("datasource.publish.ambiguous", "Datasource publish target is ambiguous.", input, fmt.Errorf("%d authoritative datasource matches", len(items)))
	}
	if len(items) == 0 {
		if input.Mode == ModeAppend || input.Mode == ModeReplace || input.SourceDefaulted {
			return "", operationError("datasource.publish.target_missing", "The requested datasource target does not exist.", input, errors.New("publish mode requires an existing datasource"))
		}
		return "", nil
	}
	item := items[0]
	if item.LUID == "" || item.ProjectLUID != project {
		return "", operationError("datasource.publish.collision_identity", "Datasource collision omitted authoritative identity.", input, errors.New("datasource collision omitted authoritative identity"))
	}
	if input.SourceDefaulted && item.LUID != artifact.TableauID {
		return "", operationError("datasource.publish.source_changed", "The recorded source datasource identity changed.", input, errors.New("recorded source LUID no longer matches the exact collision"))
	}
	if input.Mode == ModeCreate {
		return "", operationError("datasource.publish.conflict", "A datasource with this name already exists.", input, errors.New("create mode cannot overwrite an existing datasource"))
	}
	return item.LUID, nil
}

func validMode(mode Mode) bool {
	return mode == ModeCreate || mode == ModeOverwrite || mode == ModeAppend || mode == ModeReplace
}
func usage(field, summary string) error {
	return &errs.Error{ID: "datasource.publish.usage", Kind: errs.KindUsage, Operation: "datasource.publish", Summary: summary, Retryable: errs.Bool(false), CorrectiveAction: "Correct the datasource publish input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: summary}}}
}
func runtimeError(id, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: errs.KindRuntime, Operation: "datasource.publish", Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Configure datasource publishing before retrying."}
}
func operationError(id, summary string, input Input, cause error) error {
	retryable, action := errs.CompleteRetryAdvice(cause, "Review the exact artifact, destination, and upstream error, then review a new preview.")
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "datasource.publish", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: action, TableauRequestID: errs.TableauRequestID(cause)}
}
