package publish

import "encoding/json"

type Input struct {
	Environment   string
	Site          string
	SiteLUID      string
	Workspace     string
	WorkspaceName string
	Artifact      string
	ArtifactID    string
	ArtifactName  string
	DatasourceMap []string
	Preview       bool
}

type Bundle struct {
	DefinitionLUID string
	DatasourceLUID string
	Configuration  json.RawMessage
	Metrics        []Metric
}

type Metric struct {
	LUID          string          `json:"source_metric_luid"`
	IsDefault     bool            `json:"is_default"`
	Specification json.RawMessage `json:"specification"`
}

type Mapping struct {
	Kind            string `json:"kind"`
	SourceLUID      string `json:"source_luid"`
	DestinationLUID string `json:"destination_luid"`
}

type Plan struct {
	Environment               string          `json:"environment"`
	Site                      string          `json:"site"`
	Artifact                  string          `json:"artifact"`
	SourceDefinitionLUID      string          `json:"source_definition_luid"`
	Name                      string          `json:"name"`
	SourceDatasourceLUID      string          `json:"source_datasource_luid"`
	DestinationDatasourceLUID string          `json:"destination_datasource_luid"`
	NewObjectsOnly            bool            `json:"new_objects_only"`
	MetricCount               int             `json:"metric_count"`
	DefinitionConfiguration   json.RawMessage `json:"definition_configuration,omitempty"`
	Metrics                   []Metric        `json:"metrics"`
	ReviewComplete            bool            `json:"review_complete"`
}

type DefinitionResult struct{ LUID, DefaultMetricLUID, RequestID string }
type MetricResult struct{ LUID, RequestID string }

type Output struct {
	Status   string    `json:"status"`
	Plan     Plan      `json:"plan"`
	Mappings []Mapping `json:"mappings"`
	Complete bool      `json:"complete"`
	Warnings []string  `json:"warnings,omitempty"`
	Help     []string  `json:"help"`
}

type compactResult struct {
	Status   string    `json:"status"`
	Plan     Plan      `json:"plan"`
	Mappings []Mapping `json:"mappings"`
	Complete bool      `json:"complete"`
	Warnings []string  `json:"warnings,omitempty"`
	Details  string    `json:"details"`
	Help     []string  `json:"help"`
}

func (o Output) CompactOutput() any {
	plan := o.Plan
	if len(plan.Metrics) > 10 {
		plan.Metrics = plan.Metrics[:10]
		plan.ReviewComplete = false
	}
	if len(plan.DefinitionConfiguration) > 16384 {
		plan.DefinitionConfiguration = nil
		plan.ReviewComplete = false
	}
	metrics := make([]Metric, len(plan.Metrics))
	copy(metrics, plan.Metrics)
	for i := range metrics {
		if len(metrics[i].Specification) > 4096 {
			metrics[i].Specification = nil
			plan.ReviewComplete = false
		}
	}
	plan.Metrics = metrics
	mappings := o.Mappings
	if len(mappings) > 20 {
		mappings = mappings[:20]
		plan.ReviewComplete = false
	}
	return compactResult{Status: o.Status, Plan: plan, Mappings: mappings, Complete: o.Complete, Warnings: o.Warnings, Details: "--full", Help: o.Help}
}
func (o Output) FullOutput() any { return o }
