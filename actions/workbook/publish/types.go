package publish

import "github.com/ahillspace/tadx/internal/identity"

// Input selects one local artifact and explicit remote destination.
type Input struct {
	ArtifactPath string
	Environment  string
	Site         string
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

// Output keeps the applied result attached to the exact previewed plan.
type Output struct {
	Plan    Plan     `json:"plan"`
	Applied bool     `json:"applied"`
	Result  *Result  `json:"result,omitempty"`
	Help    []string `json:"help"`
}
