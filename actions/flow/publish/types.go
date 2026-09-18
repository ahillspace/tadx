package publish

import "github.com/ahillspace/tadx/internal/identity"

type Input struct {
	File         string
	ArtifactID   string
	ArtifactName string
	// WorkspaceName is the resolved logical workspace alias used in follow-up commands.
	WorkspaceName string
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved                                   bool
	Workspace, ArtifactPath, Environment, Site, Name string
	ProjectSelector                                  identity.Selector
	Overwrite                                        bool
}

// SetProjectSelector records one exact destination without exposing identity plumbing to Cobra.
func (i *Input) SetProjectSelector(luid, projectPath string) {
	i.ProjectSelector = identity.Selector{LUID: identity.LUID(luid), ProjectPath: projectPath}
}

type Artifact struct {
	TableauID                                                                                                         string
	Path, PayloadPath, Filename, Name, Fingerprint, SourceEnvironment, SourceSite, SourceProjectName, SourceProjectID string
	Size                                                                                                              int64
}
type Project struct{ LUID, Name, Path string }
type Flow struct{ LUID, Name, ProjectLUID string }
type Target struct {
	Environment  string `json:"environment"`
	Site         string `json:"site"`
	ProjectLUID  string `json:"project_luid"`
	ProjectPath  string `json:"project_path"`
	ExistingLUID string `json:"existing_flow_luid,omitempty"`
}
type Plan struct {
	Kind                string   `json:"kind"`
	Workspace           string   `json:"workspace"`
	SourceLUID          string   `json:"source_luid"`
	Mode                string   `json:"mode"`
	Operation           string   `json:"operation"`
	ArtifactPath        string   `json:"artifact_path"`
	SourceKind          string   `json:"source_kind"`
	ArtifactFingerprint string   `json:"artifact_fingerprint"`
	Filename            string   `json:"filename"`
	FlowName            string   `json:"flow_name"`
	Target              Target   `json:"target"`
	Overwrite           bool     `json:"overwrite"`
	Substeps            []string `json:"substeps"`
	request             PublishRequest
	planned             bool
}
type PublishRequest struct {
	Name, ProjectLUID, Filename, ContentPath, ExpectedFingerprint string
	ContentSize                                                   int64
	Overwrite                                                     bool
}
type Result struct {
	Status           string `json:"status"`
	FlowLUID         string `json:"flow_luid,omitempty"`
	FlowName         string `json:"flow_name,omitempty"`
	ProjectLUID      string `json:"project_luid,omitempty"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
	ReceiptPath      string `json:"receipt_path,omitempty"`
}
type Output struct {
	Plan   Plan     `json:"plan"`
	Result *Result  `json:"result,omitempty"`
	Help   []string `json:"help"`
}
type CompactPublishResult struct {
	Status      string `json:"status"`
	FlowLUID    string `json:"flow_luid,omitempty"`
	FlowName    string `json:"flow_name,omitempty"`
	ProjectLUID string `json:"project_luid,omitempty"`
	ReceiptPath string `json:"receipt_path,omitempty"`
}
type CompactResult struct {
	Plan    CompactPlan           `json:"plan"`
	Result  *CompactPublishResult `json:"result,omitempty"`
	Details string                `json:"details"`
	Help    []string              `json:"help"`
}
type FullResult struct {
	Plan   Plan     `json:"plan"`
	Result *Result  `json:"result,omitempty"`
	Help   []string `json:"help"`
}

func (o Output) CompactOutput() any {
	var result *CompactPublishResult
	if o.Result != nil {
		result = &CompactPublishResult{Status: o.Result.Status, FlowLUID: o.Result.FlowLUID, FlowName: o.Result.FlowName, ProjectLUID: o.Result.ProjectLUID, ReceiptPath: o.Result.ReceiptPath}
	}
	return CompactResult{Plan: CompactPlan{Workspace: o.Plan.Workspace, Kind: "flow", SourceLUID: o.Plan.SourceLUID, Mode: o.Plan.Mode, Operation: o.Plan.Operation, FlowName: o.Plan.FlowName, Target: o.Plan.Target, Overwrite: o.Plan.Overwrite, ArtifactPath: o.Plan.ArtifactPath, SourceKind: o.Plan.SourceKind}, Result: result, Details: "--full", Help: o.Help}
}
func (o Output) FullOutput() any {
	o.Plan.Kind = "flow"
	return FullResult{Plan: o.Plan, Result: o.Result, Help: o.Help}
}

type CompactPlan struct {
	ArtifactPath string `json:"artifact_path"`
	SourceKind   string `json:"source_kind"`
	Workspace    string `json:"workspace"`
	Kind         string `json:"kind"`
	SourceLUID   string `json:"source_luid"`
	Mode         string `json:"mode"`
	Operation    string `json:"operation"`
	FlowName     string `json:"flow_name"`
	Target       Target `json:"target"`
	Overwrite    bool   `json:"overwrite"`
}
