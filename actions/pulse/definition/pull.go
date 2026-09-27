package definition

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/pathspec"
	"github.com/ahillspace/tadx/internal/value"
)

// Reader is the action-owned exact definition seam.
type PullReader interface {
	GetDefinition(context.Context, string) (PullDefinition, error)
}

// Writer is the action-owned managed artifact seam.
type PullWriter interface {
	WriteDefinition(context.Context, PullArtifact) (PullArtifactResult, error)
	PreviewDefinition(context.Context, PullInput, PullDefinition) (value.AcquisitionPlan, error)
}

// Pull retrieves and atomically materializes one definition.
func Pull(ctx context.Context, reader PullReader, writer PullWriter, input PullInput) (PullOutput, error) {
	input.LUID = strings.TrimSpace(input.LUID)
	if strings.TrimSpace(input.Workspace) == "" {
		return PullOutput{}, pullError("pulse.definition.pull.usage", errs.KindUsage, input, "Pulse definition pull requires a workspace and exact definition LUID.", nil)
	}
	definition, err := reader.GetDefinition(ctx, input.LUID)
	if err != nil {
		retryable, corrective := errs.CompleteRetryAdvice(err, "Review the exact definition LUID and selected Tableau site, then retry.")
		return PullOutput{}, &errs.Error{ID: "pulse.definition.pull.read", Kind: errs.KindOperation, Operation: "pulse.definition.pull", Resource: input.LUID, Environment: input.Environment, Site: input.Site, Summary: "Pulse definition retrieval failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err)}
	}
	if definition.LUID != input.LUID || definition.Name == "" || definition.DatasourceLUID == "" || len(definition.Configuration) == 0 {
		return PullOutput{}, pullError("pulse.definition.pull.invalid_response", errs.KindOperation, input, "Tableau returned an incomplete or mismatched Pulse definition.", errors.New("definition requires matching LUID, name, datasource LUID, and configuration"))
	}
	if !definition.MetricsComplete || len(definition.Metrics) == 0 || len(definition.Metrics) > 10000 {
		return PullOutput{}, pullError("pulse.definition.pull.incomplete", errs.KindOperation, input, "A complete Pulse metric inventory is required for a portable bundle.", nil)
	}
	if input.Preview {
		plan, err := writer.PreviewDefinition(ctx, input, definition)
		if err != nil {
			return PullOutput{}, err
		}
		return PullOutput{Status: "preview", Definition: definition, Preview: &plan}, nil
	}
	artifact, err := writer.WriteDefinition(ctx, PullArtifact{
		Workspace: input.Workspace, DefinitionLUID: definition.LUID, Name: definition.Name, DatasourceLUID: definition.DatasourceLUID,
		Environment: input.Environment, Site: input.Site, ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID,
		Configuration: append([]byte(nil), definition.Configuration...), Metrics: definition.Metrics, Overwrite: input.Overwrite,
	})
	if err != nil {
		retryable, corrective := errs.CompleteRetryAdvice(err, "Review the workspace and managed artifact target, then pull again.")
		return PullOutput{}, &errs.Error{ID: "pulse.definition.pull.write", Kind: errs.KindOperation, Operation: "pulse.definition.pull", Resource: input.LUID, Environment: input.Environment, Site: input.Site, Summary: "Pulse definition artifact write failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective}
	}
	if err := pullNormalizeArtifactPaths(&artifact); err != nil {
		return PullOutput{}, pullError("pulse.definition.pull.normalize", errs.KindOperation, input, "Pulse definition artifact path normalization failed.", err)
	}
	return PullOutput{Status: "pulled", Definition: definition, Artifact: artifact, Provenance: PullProvenance{Environment: input.Environment, Site: input.Site, ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID, Workspace: input.WorkspaceName, DatasourceLUID: definition.DatasourceLUID}, MetricCount: len(definition.Metrics), RequestID: definition.RequestID, Help: []string{"The bundle preserves saved definition and metric specifications. Publish creates new objects and requires explicit datasource mapping."}}, nil
}

func pullNormalizeArtifactPaths(result *PullArtifactResult) error {
	for _, candidate := range []*string{&result.Path, &result.CanonicalPath} {
		if *candidate == "" {
			continue
		}
		if pathspec.IsAbs(*candidate) {
			return errors.New("artifact writer returned an absolute path")
		}
		normalized := strings.ReplaceAll(*candidate, "\\", "/")
		for _, segment := range strings.Split(normalized, "/") {
			if segment == ".." {
				return errors.New("artifact writer returned an escaping path")
			}
		}
		*candidate = normalized
	}
	return nil
}

func pullError(id string, kind errs.Kind, input PullInput, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "pulse.definition.pull", Resource: input.LUID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Provide an exact Pulse definition and a managed logical workspace."}
}

// Input selects one Pulse definition and a managed workspace.
type PullInput struct {
	Preview       bool
	Environment   string
	Site          string
	ServerOrigin  string
	SiteLUID      string
	Workspace     string
	WorkspaceName string
	LUID          string
	Overwrite     bool
}

// Definition is the canonical remote definition document.
type PullDefinition struct {
	LUID            string
	Name            string
	DatasourceLUID  string
	Configuration   []byte
	RequestID       string
	Metrics         []PullMetric
	MetricsComplete bool
}

type PullMetric struct {
	LUID           string
	DefinitionLUID string
	IsDefault      bool
	Specification  []byte
}

// Artifact is the action-owned materialization request.
type PullArtifact struct {
	Workspace      string
	DefinitionLUID string
	Name           string
	DatasourceLUID string
	Environment    string
	Site           string
	ServerOrigin   string
	SiteLUID       string
	Configuration  []byte
	Metrics        []PullMetric
	Overwrite      bool
}

// ArtifactResult identifies one managed definition artifact.
type PullArtifactResult struct {
	Path                string `json:"path"`
	CanonicalPath       string `json:"canonical_path,omitempty"`
	BaselineFingerprint string `json:"baseline_fingerprint,omitempty"`
}

// Provenance identifies the source environment without embedding credentials.
type PullProvenance struct {
	Environment    string `json:"environment,omitempty"`
	Site           string `json:"site,omitempty"`
	ServerOrigin   string `json:"server_origin,omitempty"`
	SiteLUID       string `json:"site_luid,omitempty"`
	Workspace      string `json:"workspace,omitempty"`
	DatasourceLUID string `json:"datasource_luid,omitempty"`
}

// Output retains complete details before projection.
type PullOutput struct {
	Preview     *value.AcquisitionPlan
	Status      string
	Definition  PullDefinition
	Artifact    PullArtifactResult
	Provenance  PullProvenance
	RequestID   string
	MetricCount int
	Help        []string
}

// CompactDefinition identifies the pulled definition.
type PullCompactDefinition struct {
	LUID string `json:"luid"`
	Name string `json:"name"`
}

// CompactArtifact identifies the managed artifact directory.
type PullCompactArtifact struct {
	Path string `json:"path"`
}

// CompactResult is the default projection.
type PullCompactResult struct {
	Status      string                `json:"status"`
	Definition  PullCompactDefinition `json:"definition"`
	Artifact    PullCompactArtifact   `json:"artifact"`
	MetricCount int                   `json:"metric_count"`
	Provenance  PullProvenance        `json:"provenance"`
	Details     string                `json:"details"`
	Help        []string              `json:"help"`
}

// FullResult is the expanded projection.
type PullFullResult struct {
	Status      string                `json:"status"`
	Definition  PullCompactDefinition `json:"definition"`
	Artifact    PullArtifactResult    `json:"artifact"`
	MetricCount int                   `json:"metric_count"`
	Provenance  PullProvenance        `json:"provenance"`
	RequestID   string                `json:"tableau_request_id,omitempty"`
	Help        []string              `json:"help"`
}

// CompactOutput returns the managed path and exact definition identity.
func (o PullOutput) CompactOutput() any {
	if o.Preview != nil {
		return *o.Preview
	}
	return PullCompactResult{Status: o.Status, Definition: PullCompactDefinition{LUID: o.Definition.LUID, Name: o.Definition.Name}, Artifact: PullCompactArtifact{Path: o.Artifact.Path}, MetricCount: o.MetricCount, Provenance: o.Provenance, Details: "--full", Help: o.Help}
}

// FullOutput returns artifact provenance without embedding the full resource document.
func (o PullOutput) FullOutput() any {
	if o.Preview != nil {
		return *o.Preview
	}
	return PullFullResult{Status: o.Status, Definition: PullCompactDefinition{LUID: o.Definition.LUID, Name: o.Definition.Name}, Artifact: o.Artifact, MetricCount: o.MetricCount, Provenance: o.Provenance, RequestID: o.RequestID, Help: o.Help}
}

// ValidateInput checks local selectors without resolving a site or contacting Tableau.
func PullValidateInput(input PullInput) error {
	if strings.TrimSpace(input.LUID) == "" {
		return pullError("pulse.definition.pull.usage", errs.KindUsage, input, "Pulse definition pull requires an exact LUID.", nil)
	}
	return nil
}
