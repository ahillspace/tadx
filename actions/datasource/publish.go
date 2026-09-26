package datasource

import (
	"context"
	"errors"
	"fmt"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"path/filepath"
	"slices"
	"strings"
)

// ErrPublishedDatasourceNotVisible identifies a successful publish whose exact
// destination is not yet exposed by Tableau's name index.
var PublishErrPublishedDatasourceNotVisible = errors.New("published datasource is not visible in the Tableau name index")

type ArtifactReader interface {
	ReadDatasource(context.Context, string) (PublishArtifact, error)
}
type PublishResolver interface {
	ProjectResolver
	CollisionReader
	ResolvePublishedDatasource(context.Context, string, string) (Record, error)
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
	if a != nil {
		ctx = beginProjectResolution(ctx, a.resolver)
	}
	if a == nil || a.artifacts == nil || a.resolver == nil || a.publisher == nil {
		return PublishOutput{}, publishRuntimeError("datasource.publish.unconfigured", "Datasource publish is not configured.", nil)
	}
	plan, err := a.plan(ctx, input)
	if err != nil {
		return PublishOutput{}, err
	}
	plan.SourceKind = "managed_artifact"
	if input.File != "" {
		plan.SourceKind = "native_file"
	}
	out := PublishOutput{Plan: plan, Help: []string{"Run without --preview to publish this exact plan."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	out.Help = nil
	ctx = beginProjectResolution(ctx, a.resolver)
	artifact, err := a.artifacts.ReadDatasource(ctx, input.ArtifactPath)
	if err != nil {
		return PublishOutput{}, publishOperationError("datasource.publish.reread", "Datasource artifact revalidation failed.", input, err)
	}
	if artifact.Fingerprint != plan.ArtifactFingerprint || artifact.CompositionStatus != plan.CompositionStatus || !slices.Equal(artifact.ParentDataSourceURLs, plan.ParentDataSourceURLs) {
		return PublishOutput{}, publishOperationError("datasource.publish.artifact_changed", "The datasource artifact changed during revalidation.", input, errors.New("datasource artifact or composition references changed during revalidation"))
	}
	project, err := a.resolveProject(ctx, input, artifact)
	if err != nil {
		return PublishOutput{}, err
	}
	if project.LUID != plan.Target.ProjectLUID || project.Path != plan.Target.ProjectPath {
		return PublishOutput{}, publishOperationError("datasource.publish.target_changed", "The datasource publish destination changed during revalidation.", input, errors.New("datasource publish destination changed during revalidation"))
	}
	existing, err := publishCollision(ctx, a.resolver, plan.DatasourceName, project.LUID, input, artifact)
	if err != nil {
		return PublishOutput{}, err
	}
	if existing != plan.Target.ExistingLUID {
		return PublishOutput{}, publishOperationError("datasource.publish.collision_changed", "The datasource publish collision changed during revalidation.", input, errors.New("datasource publish collision changed during revalidation"))
	}
	prepared, err := a.publisher.Prepare(ctx, plan.request)
	if err != nil {
		return PublishOutput{}, publishOperationError("datasource.publish.prepare", "Datasource publish preparation failed.", input, err)
	}
	result, err := prepared.Commit(ctx)
	if err != nil {
		if known, ok := errors.AsType[*errs.Error](err); ok && known.Phase != "" {
			if result.Status == "succeeded" {
				result.Verification = "destination_unavailable"
			}
			if result != (PublishResult{}) {
				out.Result = &result
			}
			return out, err
		}
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
		if hint := publishPublishInspectionHint(plan, result); hint != "" {
			correctiveAction += " Run " + hint + "."
		}
		if result != (PublishResult{}) {
			out.Result = &result
		}
		return out, &errs.Error{ID: errorID, Kind: errs.KindOperation, Operation: "datasource.publish", Resource: plan.Target.ExistingLUID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: err, Retryable: new(false), CorrectiveAction: correctiveAction, TableauJobID: result.JobID, TableauRequestID: requestID, Phase: errs.PhaseSubmission, Outcome: errs.OutcomeUnknown}
	}
	out.Result = &result
	if result.Status == "pending" || result.Status == "running" {
		return out, nil
	}
	return a.Complete(ctx, out)
}

// Complete confirms a completed publish without ever repeating its write.
func (a *Publisher) Complete(ctx context.Context, out PublishOutput) (PublishOutput, error) {
	out.Help = nil
	if out.Result == nil {
		return out, nil
	}
	plan, result := out.Plan, *out.Result
	input := PublishInput{Environment: plan.Target.Environment, Site: plan.Target.Site}
	if result.Status == "succeeded" {
		if result.DatasourceLUID == "" {
			resolved, resolveErr := a.resolver.ResolvePublishedDatasource(ctx, plan.DatasourceName, plan.Target.ProjectLUID)
			if resolveErr != nil {
				if errors.Is(resolveErr, PublishErrPublishedDatasourceNotVisible) {
					result.Verification = "destination_pending"
					out.Result = &result
					out.Help = []string{"Publication succeeded; its destination is not yet visible in the name index. Do not repeat publication.", publishPublishInspectionHint(plan, result)}
					return out, nil
				}
				result.Verification = "destination_unavailable"
				out.Result = &result
				return out, publishUnknownOutcomeError(plan, input, result, fmt.Errorf("resolve completed datasource identity: %w", resolveErr))
			}
			if strings.TrimSpace(resolved.LUID) == "" || resolved.Name != plan.DatasourceName || resolved.ProjectLUID != plan.Target.ProjectLUID {
				result.Verification = "destination_unavailable"
				out.Result = &result
				return out, publishUnknownOutcomeError(plan, input, result, errors.New("completed datasource resolution returned incomplete or conflicting authoritative identity"))
			}
			result.DatasourceLUID, result.DatasourceName, result.ProjectLUID = resolved.LUID, resolved.Name, resolved.ProjectLUID
		} else if result.DatasourceName != plan.DatasourceName || result.ProjectLUID != plan.Target.ProjectLUID {
			result.Verification = "destination_mismatch"
			out.Result = &result
			return out, publishUnknownOutcomeError(plan, input, result, errors.New("completed datasource job returned identity conflicting with the exact publish target"))
		}
	}
	out.Result = &result
	if result.Status == "succeeded" {
		out.Result.Verification = "confirmed"
	}
	return out, nil
}

func publishUnknownOutcomeError(plan PublishPlan, input PublishInput, result PublishResult, cause error) error {
	correctiveAction := "Inspect the target site and Tableau request before attempting another publish."
	if result.JobID != "" {
		correctiveAction = "Inspect the Tableau job and resolve the exact datasource name in the exact project before attempting another publish."
	}
	if hint := publishPublishInspectionHint(plan, result); hint != "" {
		correctiveAction += " Run " + hint + "."
	}
	return &errs.Error{
		ID:               "datasource.publish.destination_unavailable",
		Kind:             errs.KindOperation,
		Operation:        "datasource.publish",
		Resource:         plan.Target.ExistingLUID,
		Environment:      input.Environment,
		Site:             input.Site,
		Summary:          "The datasource publish completed, but its authoritative datasource identity could not be determined safely.",
		Cause:            cause,
		Retryable:        new(false),
		CorrectiveAction: correctiveAction,
		TableauJobID:     result.JobID,
		TableauRequestID: result.TableauRequestID,
		Phase:            errs.PhaseVerification,
		Outcome:          errs.OutcomeConfirmed,
	}
}

func (a *Publisher) plan(ctx context.Context, input PublishInput) (PublishPlan, error) {
	if strings.TrimSpace(input.Environment) == "" || (strings.TrimSpace(input.Site) == "" && !input.TargetResolved) {
		return PublishPlan{}, publishUsage("environment", "datasource publish requires an explicit resolved environment and site")
	}
	if !publishValidMode(input.Mode) {
		return PublishPlan{}, publishUsage("mode", "datasource publish requires an explicit create, overwrite, append, or replace mode")
	}
	artifact, err := a.artifacts.ReadDatasource(ctx, input.ArtifactPath)
	if err != nil {
		return PublishPlan{}, publishOperationError("datasource.publish.read", "Datasource artifact read failed.", input, err)
	}
	if (input.Mode == ModeAppend || input.Mode == ModeReplace) && !strings.EqualFold(filepath.Ext(artifact.Filename), ".hyper") {
		return PublishPlan{}, publishUsage("file", "append and replace require a prepared .hyper file; TADX does not unpack or edit datasource packages")
	}
	parents := append([]string(nil), artifact.ParentDataSourceURLs...)
	if artifact.CompositionStatus == "ordinary" && len(parents) != 0 {
		return PublishPlan{}, publishOperationError("datasource.publish.composition", "Ordinary datasource metadata contains parent references.", input, errors.New("ordinary datasource cannot contain parent datasource URLs"))
	}
	if artifact.CompositionStatus == "composed" && len(parents) == 0 {
		return PublishPlan{}, publishOperationError("datasource.publish.composition", "Composed datasource metadata omitted parent references.", input, errors.New("composed datasource requires parent datasource URLs"))
	}
	if artifact.CompositionStatus != "ordinary" && artifact.CompositionStatus != "composed" {
		return PublishPlan{}, publishOperationError("datasource.publish.composition_unknown", "Datasource composition could not be classified safely.", input, errors.New("datasource composition status must be ordinary or composed"))
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = artifact.Name
	}
	if name == "" {
		return PublishPlan{}, publishUsage("name", "datasource publish requires a datasource name")
	}
	project, err := a.resolveProject(ctx, input, artifact)
	if err != nil {
		return PublishPlan{}, err
	}
	existing, err := publishCollision(ctx, a.resolver, name, project.LUID, input, artifact)
	if err != nil {
		return PublishPlan{}, err
	}
	req := PublishRequest{Name: name, ProjectLUID: project.LUID, Filename: artifact.Filename, ContentPath: artifact.PayloadPath, ContentSize: artifact.Size, ExpectedFingerprint: artifact.Fingerprint, Mode: input.Mode, ParentDataSourceURLs: parents, AsJob: input.AsJob}
	return PublishPlan{Workspace: input.WorkspaceName, SourceLUID: artifact.TableauID, Mode: "preview", PublishMode: input.Mode, Operation: "datasource.publish", ArtifactPath: artifact.Path, ArtifactFingerprint: artifact.Fingerprint, Filename: artifact.Filename, DatasourceName: name, CompositionStatus: artifact.CompositionStatus, ParentDataSourceURLs: parents, Target: PublishTarget{Environment: input.Environment, Site: input.Site, ProjectLUID: project.LUID, ProjectPath: project.Path, ExistingLUID: existing}, Substeps: []string{"resolve exact destination", "check datasource collision", "revalidate artifact and destination", "upload prepared datasource input", "publish datasource", "automatically monitor an accepted job"}, AsJob: input.AsJob, request: req}, nil
}

func (a *Publisher) resolveProject(ctx context.Context, input PublishInput, artifact PublishArtifact) (Project, error) {
	selector := input.ProjectSelector
	if input.SourceDefaulted {
		selector = identity.Selector{LUID: identity.LUID(artifact.SourceProjectID), ProjectPath: artifact.SourceProjectName}
	}
	project, err := a.resolver.ResolveProject(ctx, selector)
	if err != nil {
		return Project{}, publishOperationError("datasource.publish.project", "Publish project resolution failed.", input, err)
	}
	return project, nil
}

func publishCollision(ctx context.Context, resolver PublishResolver, name, project string, input PublishInput, artifact PublishArtifact) (string, error) {
	items, err := resolver.FindDatasources(ctx, name, project)
	if err != nil {
		return "", publishOperationError("datasource.publish.collision", "Datasource collision check failed.", input, err)
	}
	if len(items) > 1 {
		return "", publishOperationError("datasource.publish.ambiguous", "Datasource publish target is ambiguous.", input, fmt.Errorf("%d authoritative datasource matches", len(items)))
	}
	if len(items) == 0 {
		if input.Mode == ModeAppend || input.Mode == ModeReplace || input.SourceDefaulted {
			return "", publishOperationError("datasource.publish.target_missing", "The requested datasource target does not exist.", input, errors.New("publish mode requires an existing datasource"))
		}
		return "", nil
	}
	item := items[0]
	if item.LUID == "" || item.ProjectLUID != project {
		return "", publishOperationError("datasource.publish.collision_identity", "Datasource collision omitted authoritative identity.", input, errors.New("datasource collision omitted authoritative identity"))
	}
	if input.SourceDefaulted && item.LUID != artifact.TableauID {
		return "", publishOperationError("datasource.publish.source_changed", "The recorded source datasource identity changed.", input, errors.New("recorded source LUID no longer matches the exact collision"))
	}
	if input.Mode == ModeCreate {
		return "", publishOperationError("datasource.publish.conflict", "A datasource with this name already exists.", input, errors.New("create mode cannot overwrite an existing datasource"))
	}
	return item.LUID, nil
}

func publishValidMode(mode Mode) bool {
	return mode == ModeCreate || mode == ModeOverwrite || mode == ModeAppend || mode == ModeReplace
}
func publishUsage(field, summary string) error {
	return &errs.Error{ID: "datasource.publish.usage", Kind: errs.KindUsage, Operation: "datasource.publish", Summary: summary, Retryable: new(false), CorrectiveAction: "Correct the datasource publish input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: summary}}}
}
func publishRuntimeError(id, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: errs.KindRuntime, Operation: "datasource.publish", Summary: summary, Cause: cause, Retryable: new(false), CorrectiveAction: "Configure datasource publishing before retrying."}
}
func publishOperationError(id, summary string, input PublishInput, cause error) error {
	retryable, action := errs.CompleteRetryAdvice(cause, "Review the exact artifact, destination, and upstream error, then review a new preview.")
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "datasource.publish", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: action, TableauRequestID: errs.TableauRequestID(cause)}
}

// ValidatePublishInput checks caller-controlled arguments before dependency setup.
func ValidatePublishInput(input PublishInput) error {
	if strings.TrimSpace(input.ArtifactPath) == "" && strings.TrimSpace(input.File) == "" && strings.TrimSpace(input.ArtifactID) == "" && strings.TrimSpace(input.ArtifactName) == "" {
		return publishUsage("artifact", "an explicit datasource artifact is required")
	}
	if !publishValidMode(input.Mode) {
		return publishUsage("mode", "datasource publish requires an explicit create, overwrite, append, or replace mode")
	}
	if (input.Mode == ModeAppend || input.Mode == ModeReplace) && input.File != "" && !strings.EqualFold(filepath.Ext(input.File), ".hyper") {
		return publishUsage("file", "append and replace require a prepared .hyper file; TADX does not unpack or edit datasource packages")
	}
	if input.ProjectSelector.LUID != "" && input.ProjectSelector.ProjectPath != "" && !input.SourceDefaulted {
		return publishUsage("project", "a project LUID cannot be combined with a project path")
	}
	return nil
}
