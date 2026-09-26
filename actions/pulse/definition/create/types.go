package create

import "github.com/ahillspace/tadx/internal/value"

// Input contains one small definition-authoring intent.
type Input struct {
	Environment string
	Site        string
	Intent      Intent
}

// Intent is the stable user-facing definition configuration.
// Tableau media types and nested provider envelopes do not cross this boundary.
type Intent struct {
	Name               string
	Description        string
	DatasourceLUID     string
	MeasureField       string
	Aggregation        string
	TimeDimension      string
	AllowedDimensions  []string
	MinimumGranularity string
	NumberFormat       string
	CurrencyCode       string
	Sentiment          string
	Temporality        string
	RunningTotal       bool
}

// FieldReferences contains exact datasource fields that a live validator must verify.
type FieldReferences struct {
	DatasourceLUID    string
	MeasureField      string
	Aggregation       string
	TimeDimension     string
	AllowedDimensions []string
}

// ExistingDefinition is one exact collision candidate.
type ExistingDefinition struct {
	LUID           string
	Name           string
	DatasourceLUID string
}

// These aliases preserve the action's authoring vocabulary and output types.
type CreateRequest = value.PulseDefinitionCreateRequest
type Specification = value.PulseSpecification
type Datasource = value.PulseDatasource
type BasicSpecification = value.PulseBasicSpecification
type Measure = value.PulseMeasure
type TimeDimension = value.PulseTimeDimension
type Filter = value.PulseFilter
type CategoricalValue = value.PulseCategoricalValue
type ExtensionOptions = value.PulseExtensionOptions
type RepresentationOptions = value.PulseRepresentationOptions
type InsightsOptions = value.PulseInsightsOptions
type InsightSetting = value.PulseInsightSetting
type Comparisons = value.PulseComparisons
type Comparison = value.PulseComparison
type CompareConfig = value.PulseCompareConfig
type Certification = value.PulseCertification

// Plan is the deterministic preview and the only value Apply accepts.
type Plan struct {
	Mode        string        `json:"mode"`
	Operation   string        `json:"operation"`
	Environment string        `json:"environment"`
	Site        string        `json:"site"`
	Name        string        `json:"name"`
	Datasource  string        `json:"datasource_luid"`
	Measure     Measure       `json:"measure"`
	TimeField   string        `json:"time_dimension"`
	Dimensions  []string      `json:"allowed_dimensions"`
	Fingerprint string        `json:"request_fingerprint"`
	Request     CreateRequest `json:"request"`
	planned     bool
}

// CreateResult is the authoritative result after bounded default-metric resolution.
type CreateResult struct {
	Status              string `json:"status"`
	DefinitionLUID      string `json:"definition_luid"`
	DefaultMetricLUID   string `json:"default_metric_luid"`
	DefaultMetricStatus string `json:"default_metric_status"`
	TableauRequestID    string `json:"tableau_request_id,omitempty"`
	PollRequestID       string `json:"poll_request_id,omitempty"`
}

// Output keeps a result attached to its exact preview.
type Output struct {
	Plan   Plan
	Result *CreateResult
	Help   []string
}

// CompactPlan contains the safety-critical target and references.
type CompactPlan struct {
	Mode                 string   `json:"mode"`
	Operation            string   `json:"operation"`
	Environment          string   `json:"environment"`
	Site                 string   `json:"site"`
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	Datasource           string   `json:"datasource_luid"`
	Measure              Measure  `json:"measure"`
	TimeField            string   `json:"time_dimension"`
	Dimensions           []string `json:"allowed_dimensions"`
	DimensionsOmitted    int      `json:"dimensions_omitted,omitempty"`
	Population           string   `json:"population"`
	MinimumGranularity   string   `json:"minimum_granularity"`
	AllowedGranularities []string `json:"allowed_granularities"`
	NumberFormat         string   `json:"number_format"`
	Currency             string   `json:"currency"`
	Sentiment            string   `json:"sentiment"`
	Temporality          string   `json:"temporality"`
	RunningTotal         bool     `json:"running_total"`
	OffsetFromToday      int      `json:"offset_from_today"`
	UseDynamicOffset     bool     `json:"use_dynamic_offset"`
	Comparisons          []string `json:"comparisons"`
	InsightsEnabled      bool     `json:"insights_enabled"`
	DisabledInsights     []string `json:"disabled_insights"`
	Certified            bool     `json:"certified"`
	RequiresFull         bool     `json:"requires_full"`
	ReviewComplete       bool     `json:"review_complete"`
}

// CompactCreateResult omits transport diagnostics.
type CompactCreateResult struct {
	Status              string `json:"status"`
	DefinitionLUID      string `json:"definition_luid"`
	DefaultMetricLUID   string `json:"default_metric_luid"`
	DefaultMetricStatus string `json:"default_metric_status"`
}

// CompactResult is the default mutation projection.
type CompactResult struct {
	Plan    CompactPlan          `json:"plan"`
	Result  *CompactCreateResult `json:"result,omitempty"`
	Details string               `json:"details"`
	Help    []string             `json:"help"`
}

// FullResult contains the exact normalized request and bounded diagnostics.
type FullResult struct {
	Plan   Plan          `json:"plan"`
	Result *CreateResult `json:"result,omitempty"`
	Help   []string      `json:"help"`
}

// CompactOutput returns the safety-critical plan and identities.
func (o Output) CompactOutput() any {
	plan := compactPlan(o.Plan)
	var result *CompactCreateResult
	if o.Result != nil {
		result = &CompactCreateResult{Status: o.Result.Status, DefinitionLUID: o.Result.DefinitionLUID, DefaultMetricLUID: o.Result.DefaultMetricLUID, DefaultMetricStatus: o.Result.DefaultMetricStatus}
	}
	return CompactResult{Plan: plan, Result: result, Details: "--full", Help: o.Help}
}

// FullOutput returns the exact normalized request.
func (o Output) FullOutput() any {
	return FullResult{Plan: o.Plan, Result: o.Result, Help: o.Help}
}
