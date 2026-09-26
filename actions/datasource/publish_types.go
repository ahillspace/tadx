package datasource

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

type PublishInput struct {
	File         string
	ArtifactID   string
	ArtifactName string
	// WorkspaceName is the resolved logical workspace alias used in follow-up commands.
	WorkspaceName string
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved                                   bool
	Workspace, ArtifactPath, Environment, Site, Name string
	ProjectSelector                                  identity.Selector
	SourceDefaulted                                  bool
	Mode                                             Mode
	AsJob                                            bool
}

// SetProjectSelector sets one exact destination project selector.
func (i *PublishInput) SetProjectSelector(luid, projectPath string) {
	i.ProjectSelector = identity.Selector{LUID: identity.LUID(luid), ProjectPath: projectPath}
}

type PublishArtifact struct {
	Path, PayloadPath, Filename, Name, TableauID, Fingerprint, SourceEnvironment, SourceSite, SourceProjectName, SourceProjectID string
	Size                                                                                                                         int64
	CompositionStatus                                                                                                            string
	ParentDataSourceURLs                                                                                                         []string
}

type PublishTarget struct {
	Environment  string `json:"environment"`
	Site         string `json:"site"`
	ProjectLUID  string `json:"project_luid"`
	ProjectPath  string `json:"project_path"`
	ExistingLUID string `json:"existing_datasource_luid,omitempty"`
}
type PublishPlan struct {
	Kind                 string        `json:"kind"`
	Workspace            string        `json:"workspace"`
	SourceLUID           string        `json:"source_luid"`
	Mode                 string        `json:"mode"`
	PublishMode          Mode          `json:"publish_mode"`
	Operation            string        `json:"operation"`
	ArtifactPath         string        `json:"artifact_path"`
	SourceKind           string        `json:"source_kind"`
	ArtifactFingerprint  string        `json:"artifact_fingerprint"`
	Filename             string        `json:"filename"`
	DatasourceName       string        `json:"datasource_name"`
	CompositionStatus    string        `json:"composition_status"`
	ParentDataSourceURLs []string      `json:"parent_datasource_urls,omitempty"`
	Target               PublishTarget `json:"target"`
	Substeps             []string      `json:"substeps"`
	AsJob                bool          `json:"-"`
	request              PublishRequest
}

// PublishCompactPlan keeps only the fields needed to understand the publish decision.
// Diagnostic artifact and composition detail remains available through --full.
type PublishCompactPlan struct {
	Workspace      string        `json:"workspace"`
	Kind           string        `json:"kind"`
	SourceLUID     string        `json:"source_luid"`
	Mode           string        `json:"mode"`
	PublishMode    Mode          `json:"publish_mode"`
	Operation      string        `json:"operation"`
	ArtifactPath   string        `json:"artifact_path"`
	SourceKind     string        `json:"source_kind"`
	DatasourceName string        `json:"datasource_name"`
	Target         PublishTarget `json:"target"`
	AsJob          bool          `json:"-"`
}

type PublishRequest struct {
	Name, ProjectLUID, Filename, ContentPath, ExpectedFingerprint string
	ContentSize                                                   int64
	Mode                                                          Mode
	ParentDataSourceURLs                                          []string
	AsJob                                                         bool
}
type PublishResult struct {
	Status           string `json:"status"`
	DatasourceLUID   string `json:"datasource_luid,omitempty"`
	DatasourceName   string `json:"datasource_name,omitempty"`
	ProjectLUID      string `json:"project_luid,omitempty"`
	JobID            string `json:"tableau_job_id,omitempty"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
	ReceiptPath      string `json:"receipt_path,omitempty"`
	Verification     string `json:"verification,omitempty"`
}
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

type PublishCompactPublishResult struct {
	Status         string `json:"status"`
	DatasourceLUID string `json:"datasource_luid,omitempty"`
	DatasourceName string `json:"datasource_name,omitempty"`
	ProjectLUID    string `json:"project_luid,omitempty"`
	JobID          string `json:"tableau_job_id,omitempty"`
	ReceiptPath    string `json:"receipt_path,omitempty"`
	Verification   string `json:"verification,omitempty"`
}

type PublishCompactResult struct {
	Status       string                       `json:"status,omitempty"`
	Kind         string                       `json:"kind,omitempty"`
	Operation    string                       `json:"operation,omitempty"`
	Environment  string                       `json:"environment,omitempty"`
	Site         string                       `json:"site,omitempty"`
	ProjectPath  string                       `json:"project_path,omitempty"`
	Workspace    string                       `json:"workspace,omitempty"`
	ArtifactPath string                       `json:"artifact_path,omitempty"`
	Plan         *PublishCompactPlan          `json:"plan,omitempty"`
	Result       *PublishCompactPublishResult `json:"result,omitempty"`
	Details      string                       `json:"details"`
	Help         []string                     `json:"help"`
}

type PublishFullResult struct {
	Plan   PublishPlan    `json:"plan"`
	Result *PublishResult `json:"result,omitempty"`
	Help   []string       `json:"help"`
}

func (o PublishOutput) CompactOutput() any {
	plan := &PublishCompactPlan{Workspace: o.Plan.Workspace, Kind: "datasource", SourceLUID: o.Plan.SourceLUID,
		Mode:           o.Plan.Mode,
		PublishMode:    o.Plan.PublishMode,
		Operation:      o.Plan.Operation,
		ArtifactPath:   o.Plan.ArtifactPath,
		SourceKind:     o.Plan.SourceKind,
		DatasourceName: o.Plan.DatasourceName,
		Target:         o.Plan.Target,
		AsJob:          o.Plan.AsJob,
	}
	compact := PublishCompactResult{Plan: plan, Details: "--full", Help: o.Help}
	if o.Result != nil {
		compact.Result = &PublishCompactPublishResult{
			Status:         o.Result.Status,
			DatasourceLUID: o.Result.DatasourceLUID,
			DatasourceName: o.Result.DatasourceName,
			ProjectLUID:    o.Result.ProjectLUID,
			JobID:          o.Result.JobID,
			ReceiptPath:    o.Result.ReceiptPath,
			Verification:   o.Result.Verification,
		}
		if o.Plan.Mode == "execute" {
			compact.Status = o.Result.Status
			compact.Kind = "datasource"
			compact.Operation = o.Plan.Operation
			compact.Environment = o.Plan.Target.Environment
			compact.Site = o.Plan.Target.Site
			compact.ProjectPath = o.Plan.Target.ProjectPath
			compact.Workspace = o.Plan.Workspace
			compact.ArtifactPath = o.Plan.ArtifactPath
			compact.Plan = nil
		}
	}
	return compact
}

func (o PublishOutput) FullOutput() any {
	o.Plan.Kind = "datasource"
	return PublishFullResult{Plan: o.Plan, Result: o.Result, Help: o.Help}
}

type PreparedPublish interface {
	Commit(context.Context) (PublishResult, error)
}
