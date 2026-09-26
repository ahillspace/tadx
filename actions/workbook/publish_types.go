package workbook

import "github.com/ahillspace/tadx/internal/identity"

const publishMaxFullWarnings = 20

// PublishInput selects one local artifact and explicit remote destination.
type PublishInput struct {
	File         string
	ArtifactID   string
	ArtifactName string
	// WorkspaceName is the resolved logical workspace alias used in follow-up commands.
	WorkspaceName string
	Workspace     string
	ArtifactPath  string
	Environment   string
	Site          string
	// TargetResolved reports that the composition root already resolved the
	// exact write environment and site, so the action must not re-require them.
	TargetResolved bool
	// SourceDefaulted reports that no explicit --environment was given and the
	// composition root defaulted the write target to the artifact's recorded
	// source origin. In this mode the action republishes over the exact recorded
	// source workbook and never creates a differently named or located workbook.
	SourceDefaulted bool
	Name            string
	ProjectSelector identity.Selector
	ProjectLUID     string
	ProjectPath     string
	Overwrite       bool
	AsJob           bool
}

// PublishArtifact is the current local canonical workbook.
type PublishArtifact struct {
	Path        string
	PayloadPath string
	Filename    string
	Size        int64
	Name        string
	TableauID   string
	Fingerprint string
	// Source provenance recorded at pull time, used to default the publish
	// target back to the artifact's origin when no explicit target is given.
	SourceEnvironment string
	SourceSite        string
	SourceProjectName string
	SourceProjectID   string
	// Portability and PublishedDatasourceCount drive the cross-site publish
	// warning. A "source-site-bound" workbook references published datasources
	// that will not resolve when published to a different site.
	Portability              string
	PublishedDatasourceCount int
}

// PublishTarget is the stable preview target.
type PublishTarget struct {
	// Origin is "artifact-source" when the target was defaulted from the
	// artifact's recorded provenance, and "explicit" when a write target was
	// given on the command line.
	Origin       string `json:"origin"`
	Environment  string `json:"environment"`
	Site         string `json:"site"`
	ProjectLUID  string `json:"project_luid"`
	ProjectPath  string `json:"project_path"`
	ExistingLUID string `json:"existing_workbook_luid,omitempty"`
}

// PublishPlan is the deterministic preview and the only value Apply accepts.
type PublishPlan struct {
	Workspace           string        `json:"workspace"`
	SourceLUID          string        `json:"source_luid"`
	Mode                string        `json:"mode"`
	Operation           string        `json:"operation"`
	ArtifactPath        string        `json:"artifact_path"`
	SourceKind          string        `json:"source_kind"`
	ArtifactFingerprint string        `json:"artifact_fingerprint"`
	Filename            string        `json:"filename"`
	WorkbookName        string        `json:"workbook_name"`
	Target              PublishTarget `json:"target"`
	Overwrite           bool          `json:"overwrite"`
	AsJob               bool          `json:"-"`
	Warnings            []string      `json:"warnings,omitempty"`
	Substeps            []string      `json:"substeps"`
	request             PublishRequest
	planned             bool
}

// PublishRequest is the explicit adapter mutation request.
type PublishRequest struct {
	Name                string
	ProjectLUID         string
	Filename            string
	ContentPath         string
	ContentSize         int64
	ExpectedFingerprint string
	Overwrite           bool
	AsJob               bool
}

// PublishValidationIssue is one advisory diagnostic returned by server-side TWB validation.
type PublishValidationIssue struct {
	Severity    string `json:"severity"`
	Message     string `json:"message"`
	Line        int    `json:"line,omitempty"`
	Column      int    `json:"column,omitempty"`
	ElementName string `json:"element_name,omitempty"`
}

// PublishResult is the authoritative terminal mutation result.
type PublishResult struct {
	Status             string                   `json:"status"`
	WorkbookLUID       string                   `json:"workbook_luid,omitempty"`
	WorkbookName       string                   `json:"workbook_name,omitempty"`
	ProjectLUID        string                   `json:"project_luid,omitempty"`
	JobID              string                   `json:"tableau_job_id,omitempty"`
	TableauRequestID   string                   `json:"tableau_request_id,omitempty"`
	ReceiptPath        string                   `json:"receipt_path,omitempty"`
	Verification       string                   `json:"verification,omitempty"`
	ValidationWarnings []PublishValidationIssue `json:"validation_warnings,omitempty"`
}

// PublishOutput keeps the result attached to the exact previewed plan.
type PublishOutput struct {
	Plan   PublishPlan    `json:"plan"`
	Result *PublishResult `json:"result,omitempty"`
	Help   []string       `json:"help"`
}

// OperationStatus reports the accepted publication state for generic batch
// progress without exposing action-specific result fields.
func (o PublishOutput) OperationStatus() string {
	if o.Result == nil {
		return ""
	}
	return o.Result.Status
}

// PublishCompactPlan preserves the exact target and mutation decision without diagnostics.
type PublishCompactPlan struct {
	Workspace       string        `json:"workspace"`
	Kind            string        `json:"kind"`
	SourceLUID      string        `json:"source_luid"`
	Mode            string        `json:"mode"`
	Operation       string        `json:"operation"`
	ArtifactPath    string        `json:"artifact_path"`
	SourceKind      string        `json:"source_kind"`
	Filename        string        `json:"filename"`
	WorkbookName    string        `json:"workbook_name"`
	Target          PublishTarget `json:"target"`
	Overwrite       bool          `json:"overwrite"`
	AsJob           bool          `json:"-"`
	Warnings        []string      `json:"warnings,omitempty"`
	WarningsOmitted int           `json:"warnings_omitted,omitempty"`
}

// PublishCompactPublishResult preserves authoritative identities without request diagnostics.
type PublishCompactPublishResult struct {
	Status          string `json:"status"`
	WorkbookLUID    string `json:"workbook_luid,omitempty"`
	WorkbookName    string `json:"workbook_name,omitempty"`
	ProjectLUID     string `json:"project_luid,omitempty"`
	JobID           string `json:"tableau_job_id,omitempty"`
	ReceiptPath     string `json:"receipt_path,omitempty"`
	Verification    string `json:"verification,omitempty"`
	WarningsOmitted int    `json:"validation_warnings_omitted,omitempty"`
}

// PublishCompactResult is the bounded default projection.
type PublishCompactResult struct {
	Status          string                       `json:"status,omitempty"`
	Kind            string                       `json:"kind,omitempty"`
	Operation       string                       `json:"operation,omitempty"`
	Environment     string                       `json:"environment,omitempty"`
	Site            string                       `json:"site,omitempty"`
	ProjectPath     string                       `json:"project_path,omitempty"`
	Workspace       string                       `json:"workspace,omitempty"`
	ArtifactPath    string                       `json:"artifact_path,omitempty"`
	Warnings        []string                     `json:"warnings,omitempty"`
	WarningsOmitted int                          `json:"warnings_omitted,omitempty"`
	Plan            *PublishCompactPlan          `json:"plan,omitempty"`
	Result          *PublishCompactPublishResult `json:"result,omitempty"`
	Details         string                       `json:"details"`
	Help            []string                     `json:"help"`
}

// PublishFullPlan includes bounded publish diagnostics.
type PublishFullPlan struct {
	Workspace           string        `json:"workspace"`
	Kind                string        `json:"kind"`
	SourceLUID          string        `json:"source_luid"`
	Mode                string        `json:"mode"`
	Operation           string        `json:"operation"`
	ArtifactPath        string        `json:"artifact_path"`
	SourceKind          string        `json:"source_kind"`
	ArtifactFingerprint string        `json:"artifact_fingerprint"`
	Filename            string        `json:"filename"`
	WorkbookName        string        `json:"workbook_name"`
	Target              PublishTarget `json:"target"`
	Overwrite           bool          `json:"overwrite"`
	AsJob               bool          `json:"-"`
	Warnings            []string      `json:"warnings,omitempty"`
	WarningsOmitted     int           `json:"warnings_omitted,omitempty"`
	Substeps            []string      `json:"substeps"`
}

// PublishFullPublishResult includes bounded validation and request diagnostics.
type PublishFullPublishResult struct {
	Status                    string                   `json:"status"`
	WorkbookLUID              string                   `json:"workbook_luid,omitempty"`
	WorkbookName              string                   `json:"workbook_name,omitempty"`
	ProjectLUID               string                   `json:"project_luid,omitempty"`
	JobID                     string                   `json:"tableau_job_id,omitempty"`
	TableauRequestID          string                   `json:"tableau_request_id,omitempty"`
	ReceiptPath               string                   `json:"receipt_path,omitempty"`
	Verification              string                   `json:"verification,omitempty"`
	ValidationWarnings        []PublishValidationIssue `json:"validation_warnings,omitempty"`
	ValidationWarningsOmitted int                      `json:"validation_warnings_omitted,omitempty"`
}

// PublishFullResult is the bounded expanded projection.
type PublishFullResult struct {
	Plan   PublishFullPlan           `json:"plan"`
	Result *PublishFullPublishResult `json:"result,omitempty"`
	Help   []string                  `json:"help"`
}

// CompactOutput returns the target, safety decision, and resulting identities.
func (o PublishOutput) CompactOutput() any {
	warnings, omitted := publishBoundWarnings(o.Plan.Warnings)
	planValue := PublishCompactPlan{Workspace: o.Plan.Workspace, Kind: "workbook", SourceLUID: o.Plan.SourceLUID,
		Mode: o.Plan.Mode, Operation: o.Plan.Operation, ArtifactPath: o.Plan.ArtifactPath, SourceKind: o.Plan.SourceKind,
		Filename: o.Plan.Filename, WorkbookName: o.Plan.WorkbookName, Target: o.Plan.Target,
		Overwrite: o.Plan.Overwrite, AsJob: o.Plan.AsJob, Warnings: warnings, WarningsOmitted: omitted,
	}
	plan := &planValue
	var result *PublishCompactPublishResult
	if o.Result != nil {
		result = &PublishCompactPublishResult{
			Status: o.Result.Status, WorkbookLUID: o.Result.WorkbookLUID, WorkbookName: o.Result.WorkbookName,
			ProjectLUID: o.Result.ProjectLUID, JobID: o.Result.JobID, ReceiptPath: o.Result.ReceiptPath, Verification: o.Result.Verification,
			WarningsOmitted: len(o.Result.ValidationWarnings),
		}
	}
	compact := PublishCompactResult{Plan: plan, Result: result, Details: "--full", Help: o.Help}
	if o.Result != nil && o.Plan.Mode == "execute" {
		compact.Status = o.Result.Status
		compact.Kind = "workbook"
		compact.Operation = o.Plan.Operation
		compact.Environment = o.Plan.Target.Environment
		compact.Site = o.Plan.Target.Site
		compact.ProjectPath = o.Plan.Target.ProjectPath
		compact.Workspace = o.Plan.Workspace
		compact.ArtifactPath = o.Plan.ArtifactPath
		compact.Warnings = warnings
		compact.WarningsOmitted = omitted
		compact.Plan = nil
	}
	return compact
}

// FullOutput returns bounded diagnostics for the same publish operation.
func (o PublishOutput) FullOutput() any {
	warnings, warningsOmitted := publishBoundWarnings(o.Plan.Warnings)
	full := PublishFullResult{
		Plan: PublishFullPlan{Workspace: o.Plan.Workspace, Kind: "workbook", SourceLUID: o.Plan.SourceLUID,
			Mode: o.Plan.Mode, Operation: o.Plan.Operation, ArtifactPath: o.Plan.ArtifactPath, SourceKind: o.Plan.SourceKind,
			ArtifactFingerprint: o.Plan.ArtifactFingerprint, Filename: o.Plan.Filename,
			WorkbookName: o.Plan.WorkbookName, Target: o.Plan.Target, Overwrite: o.Plan.Overwrite,
			AsJob: o.Plan.AsJob, Warnings: warnings, WarningsOmitted: warningsOmitted, Substeps: o.Plan.Substeps,
		},
		Help: o.Help,
	}
	if o.Result != nil {
		validationWarnings, validationWarningsOmitted := publishBoundValidationWarnings(o.Result.ValidationWarnings)
		full.Result = &PublishFullPublishResult{
			Status: o.Result.Status, WorkbookLUID: o.Result.WorkbookLUID, WorkbookName: o.Result.WorkbookName,
			ProjectLUID: o.Result.ProjectLUID, JobID: o.Result.JobID, TableauRequestID: o.Result.TableauRequestID, ReceiptPath: o.Result.ReceiptPath, Verification: o.Result.Verification,
			ValidationWarnings: validationWarnings, ValidationWarningsOmitted: validationWarningsOmitted,
		}
	}
	return full
}

func publishBoundWarnings(warnings []string) ([]string, int) {
	if len(warnings) <= publishMaxFullWarnings {
		return warnings, 0
	}
	return warnings[:publishMaxFullWarnings], len(warnings) - publishMaxFullWarnings
}

func publishBoundValidationWarnings(warnings []PublishValidationIssue) ([]PublishValidationIssue, int) {
	if len(warnings) <= publishMaxFullWarnings {
		return warnings, 0
	}
	return warnings[:publishMaxFullWarnings], len(warnings) - publishMaxFullWarnings
}
