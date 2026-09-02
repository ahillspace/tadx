package publish

import (
	"context"

	"github.com/ahillspace/tadx/internal/identity"
)

type Mode string

const (
	ModeCreate    Mode = "create"
	ModeOverwrite Mode = "overwrite"
	ModeAppend    Mode = "append"
	ModeReplace   Mode = "replace"
)

type Input struct {
	Workspace, ArtifactPath, Environment, Site, Name string
	ProjectSelector                                  identity.Selector
	SourceDefaulted                                  bool
	Mode                                             Mode
	AsJob                                            bool
}

// SetProjectSelector sets one exact destination project selector.
func (i *Input) SetProjectSelector(luid, projectPath string) {
	i.ProjectSelector = identity.Selector{LUID: identity.LUID(luid), ProjectPath: projectPath}
}

type Artifact struct {
	Path, PayloadPath, Filename, Name, TableauID, Fingerprint, SourceEnvironment, SourceSite, SourceProjectName, SourceProjectID string
	Size                                                                                                                         int64
	CompositionStatus                                                                                                            string
	ParentDataSourceURLs                                                                                                         []string
}

type Project struct{ LUID, Name, Path string }
type Datasource struct{ LUID, Name, ProjectLUID string }
type Target struct {
	Environment  string `json:"environment"`
	Site         string `json:"site"`
	ProjectLUID  string `json:"project_luid"`
	ProjectPath  string `json:"project_path"`
	ExistingLUID string `json:"existing_datasource_luid,omitempty"`
}
type Plan struct {
	Mode                 Mode     `json:"mode"`
	Operation            string   `json:"operation"`
	ArtifactPath         string   `json:"artifact_path"`
	ArtifactFingerprint  string   `json:"artifact_fingerprint"`
	Filename             string   `json:"filename"`
	DatasourceName       string   `json:"datasource_name"`
	CompositionStatus    string   `json:"composition_status"`
	ParentDataSourceURLs []string `json:"parent_datasource_urls,omitempty"`
	Target               Target   `json:"target"`
	Substeps             []string `json:"substeps"`
	AsJob                bool     `json:"as_job"`
	request              PublishRequest
}

// CompactPlan keeps only the fields needed to understand the publish decision.
// Diagnostic artifact and composition detail remains available through --full.
type CompactPlan struct {
	Mode           Mode   `json:"mode"`
	Operation      string `json:"operation"`
	ArtifactPath   string `json:"artifact_path"`
	DatasourceName string `json:"datasource_name"`
	Target         Target `json:"target"`
	AsJob          bool   `json:"as_job"`
}

type PublishRequest struct {
	Name, ProjectLUID, Filename, ContentPath, ExpectedFingerprint string
	ContentSize                                                   int64
	Mode                                                          Mode
	ParentDataSourceURLs                                          []string
	AsJob                                                         bool
}
type Result struct {
	Status           string `json:"status"`
	DatasourceLUID   string `json:"datasource_luid,omitempty"`
	DatasourceName   string `json:"datasource_name,omitempty"`
	ProjectLUID      string `json:"project_luid,omitempty"`
	JobID            string `json:"tableau_job_id,omitempty"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}
type Output struct {
	Plan    Plan     `json:"plan"`
	Applied bool     `json:"applied"`
	Result  *Result  `json:"result,omitempty"`
	Help    []string `json:"help"`
}

type CompactPublishResult struct {
	Status         string `json:"status"`
	DatasourceLUID string `json:"datasource_luid,omitempty"`
	DatasourceName string `json:"datasource_name,omitempty"`
	ProjectLUID    string `json:"project_luid,omitempty"`
	JobID          string `json:"tableau_job_id,omitempty"`
}

type CompactResult struct {
	Plan    CompactPlan           `json:"plan"`
	Applied bool                  `json:"applied"`
	Result  *CompactPublishResult `json:"result,omitempty"`
	Details string                `json:"details"`
	Help    []string              `json:"help"`
}

type FullResult struct {
	Plan    Plan     `json:"plan"`
	Applied bool     `json:"applied"`
	Result  *Result  `json:"result,omitempty"`
	Help    []string `json:"help"`
}

func (o Output) CompactOutput() any {
	compact := CompactResult{
		Plan: CompactPlan{
			Mode:           o.Plan.Mode,
			Operation:      o.Plan.Operation,
			ArtifactPath:   o.Plan.ArtifactPath,
			DatasourceName: o.Plan.DatasourceName,
			Target:         o.Plan.Target,
			AsJob:          o.Plan.AsJob,
		},
		Applied: o.Applied,
		Details: "--full",
		Help:    o.Help,
	}
	if o.Result != nil {
		compact.Result = &CompactPublishResult{
			Status:         o.Result.Status,
			DatasourceLUID: o.Result.DatasourceLUID,
			DatasourceName: o.Result.DatasourceName,
			ProjectLUID:    o.Result.ProjectLUID,
			JobID:          o.Result.JobID,
		}
	}
	return compact
}

func (o Output) FullOutput() any {
	return FullResult{Plan: o.Plan, Applied: o.Applied, Result: o.Result, Help: o.Help}
}

type PreparedPublish interface {
	Commit(context.Context) (Result, error)
}
