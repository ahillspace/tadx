package publish

import "github.com/ahillspace/tadx/internal/identity"

const maxFullWarnings = 20

// Input selects one local artifact and explicit remote destination.
type Input struct {
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

// Artifact is the current local canonical workbook.
type Artifact struct {
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

// Project is one authoritative remote destination.
type Project struct {
	LUID string
	Name string
	Path string
}

// Workbook is an exact remote collision candidate.
type Workbook struct {
	LUID        string
	Name        string
	ProjectLUID string
}

// Target is the stable preview target.
type Target struct {
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

// Plan is the deterministic preview and the only value Apply accepts.
type Plan struct {
	Mode                string   `json:"mode"`
	Operation           string   `json:"operation"`
	ArtifactPath        string   `json:"artifact_path"`
	ArtifactFingerprint string   `json:"artifact_fingerprint"`
	Filename            string   `json:"filename"`
	WorkbookName        string   `json:"workbook_name"`
	Target              Target   `json:"target"`
	Overwrite           bool     `json:"overwrite"`
	AsJob               bool     `json:"as_job"`
	Warnings            []string `json:"warnings,omitempty"`
	Substeps            []string `json:"substeps"`
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

// ValidationIssue is one advisory diagnostic returned by server-side TWB validation.
type ValidationIssue struct {
	Severity    string `json:"severity"`
	Message     string `json:"message"`
	Line        int    `json:"line,omitempty"`
	Column      int    `json:"column,omitempty"`
	ElementName string `json:"element_name,omitempty"`
}

// Result is the authoritative terminal mutation result.
type Result struct {
	Status             string            `json:"status"`
	WorkbookLUID       string            `json:"workbook_luid,omitempty"`
	WorkbookName       string            `json:"workbook_name,omitempty"`
	ProjectLUID        string            `json:"project_luid,omitempty"`
	JobID              string            `json:"tableau_job_id,omitempty"`
	TableauRequestID   string            `json:"tableau_request_id,omitempty"`
	ValidationWarnings []ValidationIssue `json:"validation_warnings,omitempty"`
}

// Output keeps the result attached to the exact previewed plan.
type Output struct {
	Plan   Plan     `json:"plan"`
	Result *Result  `json:"result,omitempty"`
	Help   []string `json:"help"`
}

// CompactPlan preserves the exact target and mutation decision without diagnostics.
type CompactPlan struct {
	Mode            string   `json:"mode"`
	Operation       string   `json:"operation"`
	ArtifactPath    string   `json:"artifact_path"`
	Filename        string   `json:"filename"`
	WorkbookName    string   `json:"workbook_name"`
	Target          Target   `json:"target"`
	Overwrite       bool     `json:"overwrite"`
	AsJob           bool     `json:"as_job"`
	Warnings        []string `json:"warnings,omitempty"`
	WarningsOmitted int      `json:"warnings_omitted,omitempty"`
}

// CompactPublishResult preserves authoritative identities without request diagnostics.
type CompactPublishResult struct {
	Status          string `json:"status"`
	WorkbookLUID    string `json:"workbook_luid,omitempty"`
	WorkbookName    string `json:"workbook_name,omitempty"`
	ProjectLUID     string `json:"project_luid,omitempty"`
	JobID           string `json:"tableau_job_id,omitempty"`
	WarningsOmitted int    `json:"validation_warnings_omitted,omitempty"`
}

// CompactResult is the bounded default projection.
type CompactResult struct {
	Plan    CompactPlan           `json:"plan"`
	Result  *CompactPublishResult `json:"result,omitempty"`
	Details string                `json:"details"`
	Help    []string              `json:"help"`
}

// FullPlan includes bounded publish diagnostics.
type FullPlan struct {
	Mode                string   `json:"mode"`
	Operation           string   `json:"operation"`
	ArtifactPath        string   `json:"artifact_path"`
	ArtifactFingerprint string   `json:"artifact_fingerprint"`
	Filename            string   `json:"filename"`
	WorkbookName        string   `json:"workbook_name"`
	Target              Target   `json:"target"`
	Overwrite           bool     `json:"overwrite"`
	AsJob               bool     `json:"as_job"`
	Warnings            []string `json:"warnings,omitempty"`
	WarningsOmitted     int      `json:"warnings_omitted,omitempty"`
	Substeps            []string `json:"substeps"`
}

// FullPublishResult includes bounded validation and request diagnostics.
type FullPublishResult struct {
	Status                    string            `json:"status"`
	WorkbookLUID              string            `json:"workbook_luid,omitempty"`
	WorkbookName              string            `json:"workbook_name,omitempty"`
	ProjectLUID               string            `json:"project_luid,omitempty"`
	JobID                     string            `json:"tableau_job_id,omitempty"`
	TableauRequestID          string            `json:"tableau_request_id,omitempty"`
	ValidationWarnings        []ValidationIssue `json:"validation_warnings,omitempty"`
	ValidationWarningsOmitted int               `json:"validation_warnings_omitted,omitempty"`
}

// FullResult is the bounded expanded projection.
type FullResult struct {
	Plan   FullPlan           `json:"plan"`
	Result *FullPublishResult `json:"result,omitempty"`
	Help   []string           `json:"help"`
}

// CompactOutput returns the target, safety decision, and resulting identities.
func (o Output) CompactOutput() any {
	warnings, omitted := boundWarnings(o.Plan.Warnings)
	plan := CompactPlan{
		Mode: o.Plan.Mode, Operation: o.Plan.Operation, ArtifactPath: o.Plan.ArtifactPath,
		Filename: o.Plan.Filename, WorkbookName: o.Plan.WorkbookName, Target: o.Plan.Target,
		Overwrite: o.Plan.Overwrite, AsJob: o.Plan.AsJob, Warnings: warnings, WarningsOmitted: omitted,
	}
	var result *CompactPublishResult
	if o.Result != nil {
		result = &CompactPublishResult{
			Status: o.Result.Status, WorkbookLUID: o.Result.WorkbookLUID, WorkbookName: o.Result.WorkbookName,
			ProjectLUID: o.Result.ProjectLUID, JobID: o.Result.JobID,
			WarningsOmitted: len(o.Result.ValidationWarnings),
		}
	}
	return CompactResult{Plan: plan, Result: result, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded diagnostics for the same publish operation.
func (o Output) FullOutput() any {
	warnings, warningsOmitted := boundWarnings(o.Plan.Warnings)
	full := FullResult{
		Plan: FullPlan{
			Mode: o.Plan.Mode, Operation: o.Plan.Operation, ArtifactPath: o.Plan.ArtifactPath,
			ArtifactFingerprint: o.Plan.ArtifactFingerprint, Filename: o.Plan.Filename,
			WorkbookName: o.Plan.WorkbookName, Target: o.Plan.Target, Overwrite: o.Plan.Overwrite,
			AsJob: o.Plan.AsJob, Warnings: warnings, WarningsOmitted: warningsOmitted, Substeps: o.Plan.Substeps,
		},
		Help: o.Help,
	}
	if o.Result != nil {
		validationWarnings, validationWarningsOmitted := boundValidationWarnings(o.Result.ValidationWarnings)
		full.Result = &FullPublishResult{
			Status: o.Result.Status, WorkbookLUID: o.Result.WorkbookLUID, WorkbookName: o.Result.WorkbookName,
			ProjectLUID: o.Result.ProjectLUID, JobID: o.Result.JobID, TableauRequestID: o.Result.TableauRequestID,
			ValidationWarnings: validationWarnings, ValidationWarningsOmitted: validationWarningsOmitted,
		}
	}
	return full
}

func boundWarnings(warnings []string) ([]string, int) {
	if len(warnings) <= maxFullWarnings {
		return warnings, 0
	}
	return warnings[:maxFullWarnings], len(warnings) - maxFullWarnings
}

func boundValidationWarnings(warnings []ValidationIssue) ([]ValidationIssue, int) {
	if len(warnings) <= maxFullWarnings {
		return warnings, 0
	}
	return warnings[:maxFullWarnings], len(warnings) - maxFullWarnings
}
