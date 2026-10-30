package workbook

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
)

// ArtifactReader reads one canonical local workbook.
type ArtifactReader interface {
	ReadWorkbook(context.Context, string) (PublishArtifact, error)
}

// PublishResolver performs authoritative destination, collision, and
// destination datasource reads.
type PublishResolver interface {
	ProjectResolver
	CollisionReader
	DatasourceReader
}

// DatasourceReader reads published datasources on the destination site by
// exact LUID, content URL, or name.
type DatasourceReader interface {
	ReadDatasource(context.Context, string) (value.Datasource, error)
	FindDatasourcesByContentURL(context.Context, []string) ([]value.Datasource, error)
	FindDatasourcesByName(context.Context, []string) ([]value.Datasource, error)
}

// PreparedPublish is ready for the final consequential remote mutation. Close
// releases the prepared upload content when Commit never runs.
type PreparedPublish interface {
	Commit(context.Context) (PublishResult, error)
	Close() error
}

// PublishPreparer prepares the publish content before the final target revalidation.
type PublishPreparer interface {
	Prepare(context.Context, PublishRequest) (PreparedPublish, error)
}

// Publisher orchestrates workbook.publish preview and apply.
type Publisher struct {
	artifacts ArtifactReader
	resolver  PublishResolver
	publisher PublishPreparer
}

// NewPublish creates workbook.publish.
func NewPublish(artifacts ArtifactReader, resolver PublishResolver, publisher PublishPreparer) *Publisher {
	return &Publisher{artifacts: artifacts, resolver: resolver, publisher: publisher}
}

// Plan performs authoritative reads and returns a preview without mutation.
func (a *Publisher) Plan(ctx context.Context, input PublishInput) (PublishPlan, error) {
	if err := ValidatePublishInput(input); err != nil {
		return PublishPlan{}, err
	}
	if a != nil {
		ctx = beginProjectResolution(ctx, a.resolver)
	}
	if a == nil || a.artifacts == nil || a.resolver == nil || a.publisher == nil {
		return PublishPlan{}, publishUnconfigured()
	}
	if input.Environment == "" {
		return PublishPlan{}, publishUsage("environment", "an explicit write environment is required")
	}
	if input.Site == "" && !input.TargetResolved {
		return PublishPlan{}, publishUsage("site", "an explicit write site is required")
	}
	if input.ArtifactPath == "" {
		return PublishPlan{}, publishUsage("artifact", "an explicit workbook artifact is required")
	}
	artifact, err := a.artifacts.ReadWorkbook(ctx, input.ArtifactPath)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Repair or pull the exact workbook artifact, then review a new preview.")
		return PublishPlan{}, &errs.Error{ID: "workbook.publish.read", Kind: errs.KindOperation, Operation: "workbook.publish", Environment: input.Environment, Site: input.Site, Summary: "Workbook artifact read failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction}
	}
	bindings, err := matchDatasourceBindings(input.DatasourceBindings, artifact.Datasources)
	if err != nil {
		return PublishPlan{}, err
	}
	// Artifact-home republish: with no explicit write target, the composition
	// root defaults the environment/site to the artifact's recorded source, and
	// the action targets the exact recorded source project, name, and workbook
	// LUID. Overwrite is implied because republishing to source replaces the
	// same workbook; the resolved plan and optional preview are the safeguards.
	origin := "explicit"
	name := input.Name
	overwrite := input.Overwrite
	projectSelector := input.ProjectSelector
	if projectSelector.LUID == "" && projectSelector.ProjectPath == "" {
		projectSelector = identity.Selector{LUID: identity.LUID(input.ProjectLUID), ProjectPath: input.ProjectPath}
	}
	if input.SourceDefaulted {
		origin = "artifact-source"
		name = artifact.Name
		overwrite = true
		if artifact.SourceProjectID == "" && artifact.SourceProjectName == "" {
			return PublishPlan{}, &errs.Error{ID: "workbook.publish.source_incomplete", Kind: errs.KindOperation, Operation: "workbook.publish", Resource: artifact.TableauID, Environment: input.Environment, Site: input.Site, Summary: "The artifact has no recorded source project.", Cause: errors.New("source provenance is missing the recorded project"), Retryable: new(false), CorrectiveAction: "Re-pull the workbook or publish to an explicit environment and project."}
		}
		projectSelector = identity.Selector{LUID: identity.LUID(artifact.SourceProjectID), ProjectPath: artifact.SourceProjectName}
	}
	if name == "" {
		name = artifact.Name
	}
	if name == "" {
		return PublishPlan{}, publishUsage("name", "workbook name is required")
	}
	project, err := a.resolver.ResolveProject(ctx, projectSelector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact destination project and target, then retry.")
		return PublishPlan{}, &errs.Error{ID: "workbook.publish.project", Kind: errs.KindOperation, Operation: "workbook.publish", Environment: input.Environment, Site: input.Site, Summary: "Publish project resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	existing, err := a.resolver.FindWorkbooks(ctx, name, project.LUID)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Resolve the exact destination again before publishing.")
		return PublishPlan{}, &errs.Error{ID: "workbook.publish.collision", Kind: errs.KindOperation, Operation: "workbook.publish", Environment: input.Environment, Site: input.Site, Summary: "Workbook collision check failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	if input.SourceDefaulted {
		// The recorded source workbook must still exist by name in its recorded
		// project and carry the same LUID. Zero matches (deleted or moved out of
		// the project), a different LUID (name now points to another workbook),
		// or multiple matches all mean the source changed after it was pulled.
		if len(existing) != 1 || existing[0].LUID != artifact.TableauID {
			return PublishPlan{}, &errs.Error{ID: "workbook.publish.source_changed", Kind: errs.KindOperation, Operation: "workbook.publish", Resource: artifact.TableauID, Environment: input.Environment, Site: input.Site, Summary: "The recorded source workbook changed after it was pulled.", Cause: fmt.Errorf("recorded source workbook %q named %q in project %q was renamed, moved, deleted, or replaced", artifact.TableauID, name, project.Path), Retryable: new(false), CorrectiveAction: "Re-pull the workbook or publish to an explicit environment, then review a new preview."}
		}
	} else {
		if len(existing) > 1 {
			return PublishPlan{}, &errs.Error{ID: "workbook.publish.ambiguous", Kind: errs.KindOperation, Operation: "workbook.publish", Environment: input.Environment, Site: input.Site, Summary: "Workbook publish target is ambiguous.", Cause: fmt.Errorf("%d workbooks named %q exist in project %q", len(existing), name, project.Path), Retryable: new(false), CorrectiveAction: "Select or rename one authoritative workbook target before publishing."}
		}
		if len(existing) == 1 && !input.Overwrite {
			return PublishPlan{}, &errs.Error{ID: "workbook.publish.conflict", Kind: errs.KindOperation, Operation: "workbook.publish", Resource: existing[0].LUID, Environment: input.Environment, Site: input.Site, Summary: "A workbook with this name already exists.", Cause: errors.New("use --overwrite to replace the exact existing workbook"), Retryable: new(false), CorrectiveAction: "Review the exact existing workbook, then use --overwrite only when replacement is intended."}
		}
	}
	existingLUID := ""
	if len(existing) == 1 {
		existingLUID = existing[0].LUID
	}
	contentURLs, datasourceBindings, err := a.readDatasourceBindings(ctx, input, artifact.Datasources, bindings)
	if err != nil {
		return PublishPlan{}, err
	}
	request := PublishRequest{Name: name, ProjectLUID: project.LUID, Filename: artifact.Filename, ContentPath: artifact.PayloadPath, ContentSize: artifact.Size, ExpectedFingerprint: artifact.Fingerprint, Overwrite: overwrite, DatasourceContentURLs: contentURLs}
	sourceKind := "managed_artifact"
	selection := []string{"--artifact", artifact.Path}
	if input.File != "" {
		sourceKind = "native_file"
		selection = []string{"--file", input.File}
	}
	return PublishPlan{
		Workspace: input.WorkspaceName, SourceLUID: artifact.TableauID,
		Mode: "preview", Operation: "workbook.publish", ArtifactPath: artifact.Path, SourceKind: sourceKind,
		ArtifactFingerprint: artifact.Fingerprint, Filename: artifact.Filename, WorkbookName: name,
		Target:    PublishTarget{Origin: origin, Environment: input.Environment, Site: input.Site, ProjectLUID: project.LUID, ProjectPath: project.Path, ExistingLUID: existingLUID},
		Overwrite: overwrite, DatasourceBindings: datasourceBindings,
		Substeps: []string{"resolve exact destination", "check workbook collision", "upload workbook", "publish workbook", "confirm destination"},
		request:  request, planned: true, selection: selection, datasources: artifact.Datasources, bindings: bindings,
	}, nil
}

// Apply performs only the exact mutation request captured by Plan.
func (a *Publisher) Apply(ctx context.Context, plan PublishPlan) (PublishResult, error) {
	if a != nil {
		ctx = beginProjectResolution(ctx, a.resolver)
	}
	if a == nil || a.resolver == nil || a.publisher == nil {
		return PublishResult{}, publishUnconfigured()
	}
	if !plan.planned || plan.Operation != "workbook.publish" || plan.request.Name == "" || plan.request.ProjectLUID == "" {
		return PublishResult{}, publishUsage("plan", "workbook publish apply requires a plan produced by Plan")
	}
	if plan.request.Overwrite {
		if err := a.verifyOverwriteTarget(ctx, plan); err != nil {
			return PublishResult{}, err
		}
	}
	prepared, err := a.publisher.Prepare(ctx, plan.request)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the artifact and upload response, then prepare the publish again.")
		return PublishResult{}, &errs.Error{ID: "workbook.publish.prepare", Kind: errs.KindOperation, Operation: "workbook.publish", Resource: plan.Target.ExistingLUID, Environment: plan.Target.Environment, Site: plan.Target.Site, Summary: "Workbook publish preparation failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	if prepared == nil {
		return PublishResult{}, &errs.Error{ID: "workbook.publish.prepare", Kind: errs.KindRuntime, Operation: "workbook.publish", Resource: plan.Target.ExistingLUID, Environment: plan.Target.Environment, Site: plan.Target.Site, Summary: "Workbook publish preparation returned no commit operation.", Retryable: new(false), CorrectiveAction: "Review the publish configuration before retrying."}
	}
	defer func() { _ = prepared.Close() }()
	ctx = beginProjectResolution(ctx, a.resolver)
	if plan.request.Overwrite {
		if err := a.verifyOverwriteTarget(ctx, plan); err != nil {
			return PublishResult{}, err
		}
	}
	result, err := prepared.Commit(ctx)
	if err != nil {
		if denied, ok := errors.AsType[interface {
			error
			NotAttempted() bool
		}](err); ok && denied.NotAttempted() {
			return result, &errs.Error{ID: "workbook.publish.not_attempted", Kind: errs.KindOperation, Operation: "workbook.publish", Environment: plan.Target.Environment, Site: plan.Target.Site, Summary: "Publication was not submitted.", Cause: err, Retryable: new(false), Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
		}
		requestID := result.TableauRequestID
		if requestID == "" {
			requestID = errs.TableauRequestID(err)
		}
		// Only a definitive Tableau rejection of the publish request itself
		// proves nothing was applied; transport failures stay unknown.
		outcome := errs.OutcomeUnknown
		lookupNote := ""
		var status interface{ HTTPStatus() int }
		if result.Status == "" && errors.As(err, &status) && status.HTTPStatus() >= 400 && status.HTTPStatus() < 500 {
			outcome = errs.OutcomeNotApplied
			var rejection error
			if lookupNote, rejection = a.unmappedDatasourceRejection(ctx, plan, err); rejection != nil {
				return result, rejection
			}
		}
		errorID := "workbook.publish.failed"
		summary := "Workbook publish failed."
		correctiveAction := "Review the upstream error before publishing again."
		if result.Status == "unknown" {
			errorID = "workbook.publish.outcome_unknown"
			summary = "The workbook publish outcome could not be determined."
			correctiveAction = "Inspect the target site and Tableau request before attempting another publish."
		}
		if hint := publishInspectionHint(plan, result); hint != "" {
			correctiveAction += " Run " + hint + "."
		}
		correctiveAction += lookupNote
		return result, &errs.Error{ID: errorID, Kind: errs.KindOperation, Operation: "workbook.publish", Resource: plan.Target.ExistingLUID, Environment: plan.Target.Environment, Site: plan.Target.Site, Summary: summary, Cause: err, Retryable: new(false), CorrectiveAction: correctiveAction, TableauRequestID: requestID, Phase: errs.PhaseSubmission, Outcome: outcome}
	}
	return result, nil
}

func (a *Publisher) verifyOverwriteTarget(ctx context.Context, plan PublishPlan) error {
	current, err := a.resolver.FindWorkbooks(ctx, plan.request.Name, plan.request.ProjectLUID)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Resolve the exact destination again before publishing.")
		return &errs.Error{ID: "workbook.publish.revalidate", Kind: errs.KindOperation, Operation: "workbook.publish", Resource: plan.Target.ExistingLUID, Environment: plan.Target.Environment, Site: plan.Target.Site, Summary: "Workbook overwrite target revalidation failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	expected := plan.Target.ExistingLUID
	if len(current) == 0 && expected == "" {
		return nil
	}
	if len(current) == 1 && current[0].LUID == expected {
		return nil
	}
	return &errs.Error{ID: "workbook.publish.target_changed", Kind: errs.KindOperation, Operation: "workbook.publish", Resource: expected, Environment: plan.Target.Environment, Site: plan.Target.Site, Summary: "Workbook overwrite target changed after planning.", Cause: fmt.Errorf("planned workbook LUID %q no longer matches the exact destination", expected), Retryable: new(false), CorrectiveAction: "Review a new preview before publishing."}
}

// Execute plans every invocation and publishes unless preview is requested.
func (a *Publisher) Execute(ctx context.Context, input PublishInput, preview bool) (PublishOutput, error) {
	plan, err := a.Plan(ctx, input)
	if err != nil {
		return PublishOutput{}, err
	}
	output := PublishOutput{Plan: plan, Help: []string{"Run without --preview to publish this exact plan."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	output.Help = nil
	result, err := a.Apply(ctx, plan)
	if err != nil {
		if result.Status != "" || result.WorkbookLUID != "" || result.TableauRequestID != "" || result.ReceiptPath != "" || len(result.ValidationWarnings) != 0 {
			output.Result = &result
		}
		return output, err
	}
	output.Result = &result
	return a.Complete(output)
}

// Complete confirms the publication destination without submitting it again.
func (a *Publisher) Complete(output PublishOutput) (PublishOutput, error) {
	output.Help = nil
	if output.Result == nil || output.Result.Status != "succeeded" {
		return output, nil
	}
	result, plan := output.Result, output.Plan
	if result.WorkbookName != plan.WorkbookName || result.ProjectLUID != plan.Target.ProjectLUID {
		result.Verification = "destination_mismatch"
		return output, &errs.Error{ID: "workbook.publish.destination_mismatch", Kind: errs.KindOperation, Operation: "workbook.publish", Resource: result.WorkbookLUID, Environment: plan.Target.Environment, Site: plan.Target.Site, Summary: "Workbook publication succeeded, but its returned destination differs from the requested target.", Phase: errs.PhaseVerification, Outcome: errs.OutcomeConfirmed, Retryable: new(false), CorrectiveAction: "Inspect the returned exact identity before any further write. Do not repeat publication."}
	}
	result.Verification = "confirmed"
	return output, nil
}

func publishUsage(field, message string) error {
	return &errs.Error{ID: "workbook.publish.usage", Kind: errs.KindUsage, Operation: "workbook.publish", Summary: message, Retryable: new(false), CorrectiveAction: "Correct the publish input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}

func publishUnconfigured() error {
	return &errs.Error{ID: "workbook.publish.unconfigured", Kind: errs.KindRuntime, Operation: "workbook.publish", Summary: "Workbook publish is not configured.", Retryable: new(false), CorrectiveAction: "Configure workbook publishing before retrying."}
}

// ValidatePublishInput checks caller-controlled arguments before dependency setup.
func ValidatePublishInput(input PublishInput) error {
	if strings.TrimSpace(input.ArtifactPath) == "" && strings.TrimSpace(input.File) == "" && strings.TrimSpace(input.ArtifactID) == "" && strings.TrimSpace(input.ArtifactName) == "" {
		return publishUsage("artifact", "an explicit workbook artifact is required")
	}
	if input.ProjectSelector.LUID != "" && input.ProjectSelector.ProjectPath != "" && !input.SourceDefaulted {
		return publishUsage("project", "a project LUID cannot be combined with a project path")
	}
	return nil
}
