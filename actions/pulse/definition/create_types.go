package definition

import (
	"strings"

	"github.com/ahillspace/tadx/internal/value"
)

func createCompactPlan(plan CreatePlan) CreateCompactPlan {
	request := plan.Request
	dimensions := append([]string{}, plan.Dimensions...)
	omitted := max(0, len(dimensions)-50)
	dimensions = dimensions[:min(50, len(dimensions))]
	granularities := make([]string, len(request.ExtensionOptions.AllowedGranularities))
	for i, value := range request.ExtensionOptions.AllowedGranularities {
		granularities[i] = strings.TrimPrefix(value, "GRANULARITY_BY_")
	}
	minimum := ""
	if len(granularities) > 0 {
		minimum = granularities[0]
	}
	comparisons := []string{}
	for _, value := range request.Comparisons.Comparisons {
		comparisons = append(comparisons, strings.TrimPrefix(value.CompareConfig.Comparison, "TIME_COMPARISON_"))
	}
	disabled := []string{}
	for _, value := range request.InsightsOptions.Settings {
		if value.Disabled {
			disabled = append(disabled, strings.TrimPrefix(value.Type, "INSIGHT_TYPE_"))
		}
	}
	sentiment := strings.TrimPrefix(request.RepresentationOptions.SentimentType, "SENTIMENT_TYPE_")
	sentiment = strings.TrimSuffix(sentiment, "_IS_GOOD")
	temporality := strings.TrimPrefix(request.Specification.Temporality, "TEMPORALITY_")
	if temporality == "LATEST_POINT_IN_TIME" {
		temporality = "LATEST"
	}
	return CreateCompactPlan{Mode: plan.Mode, Operation: plan.Operation, Environment: plan.Environment, Site: plan.Site, Name: plan.Name, Description: request.Description, Datasource: plan.Datasource, Measure: CreateMeasure{Field: plan.Measure.Field, Aggregation: strings.TrimPrefix(plan.Measure.Aggregation, "AGGREGATION_")}, TimeField: plan.TimeField, Dimensions: dimensions, DimensionsOmitted: omitted, Population: "ALL_ROWS", MinimumGranularity: minimum, AllowedGranularities: granularities, NumberFormat: strings.TrimPrefix(request.RepresentationOptions.Type, "NUMBER_FORMAT_TYPE_"), Currency: strings.TrimPrefix(request.RepresentationOptions.CurrencyCode, "CURRENCY_CODE_"), Sentiment: sentiment, Temporality: temporality, RunningTotal: request.Specification.RunningTotal, OffsetFromToday: request.ExtensionOptions.OffsetFromToday, UseDynamicOffset: request.ExtensionOptions.UseDynamicOffset, Comparisons: comparisons, InsightsEnabled: request.InsightsOptions.ShowInsights, DisabledInsights: disabled, Certified: request.Certification.IsCertified, RequiresFull: omitted > 0, ReviewComplete: omitted == 0}
}

// Input contains one small definition-authoring intent.
type CreateInput struct {
	request     CreateRequest
	Environment string
	Site        string
	Intent      CreateIntent
}

// Intent is the stable user-facing definition configuration.
// Tableau media types and nested provider envelopes do not cross this boundary.
type CreateIntent struct {
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
type CreateFieldReferences struct {
	DatasourceLUID    string
	MeasureField      string
	Aggregation       string
	TimeDimension     string
	AllowedDimensions []string
}

// These aliases preserve the action's authoring vocabulary and output types.
type CreateRequest = value.PulseDefinitionCreateRequest
type CreateSpecification = value.PulseSpecification
type CreateDatasource = value.PulseDatasource
type CreateBasicSpecification = value.PulseBasicSpecification
type CreateMeasure = value.PulseMeasure
type CreateTimeDimension = value.PulseTimeDimension
type CreateFilter = value.PulseFilter
type CreateCategoricalValue = value.PulseCategoricalValue
type CreateExtensionOptions = value.PulseExtensionOptions
type CreateRepresentationOptions = value.PulseRepresentationOptions
type CreateInsightsOptions = value.PulseInsightsOptions
type CreateInsightSetting = value.PulseInsightSetting
type CreateComparisons = value.PulseComparisons
type CreateComparison = value.PulseComparison
type CreateCompareConfig = value.PulseCompareConfig
type CreateCertification = value.PulseCertification

// Plan is the deterministic projection of a fresh create operation.
type CreatePlan struct {
	Mode        string        `json:"mode"`
	Operation   string        `json:"operation"`
	Environment string        `json:"environment"`
	Site        string        `json:"site"`
	Name        string        `json:"name"`
	Datasource  string        `json:"datasource_luid"`
	Measure     CreateMeasure `json:"measure"`
	TimeField   string        `json:"time_dimension"`
	Dimensions  []string      `json:"allowed_dimensions"`
	Fingerprint string        `json:"request_fingerprint"`
	Request     CreateRequest `json:"request"`
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
type CreateOutput struct {
	Plan   CreatePlan
	Result *CreateResult
	Help   []string
}

// CompactPlan contains the safety-critical target and references.
type CreateCompactPlan struct {
	Mode                 string        `json:"mode"`
	Operation            string        `json:"operation"`
	Environment          string        `json:"environment"`
	Site                 string        `json:"site"`
	Name                 string        `json:"name"`
	Description          string        `json:"description"`
	Datasource           string        `json:"datasource_luid"`
	Measure              CreateMeasure `json:"measure"`
	TimeField            string        `json:"time_dimension"`
	Dimensions           []string      `json:"allowed_dimensions"`
	DimensionsOmitted    int           `json:"dimensions_omitted,omitempty"`
	Population           string        `json:"population"`
	MinimumGranularity   string        `json:"minimum_granularity"`
	AllowedGranularities []string      `json:"allowed_granularities"`
	NumberFormat         string        `json:"number_format"`
	Currency             string        `json:"currency"`
	Sentiment            string        `json:"sentiment"`
	Temporality          string        `json:"temporality"`
	RunningTotal         bool          `json:"running_total"`
	OffsetFromToday      int           `json:"offset_from_today"`
	UseDynamicOffset     bool          `json:"use_dynamic_offset"`
	Comparisons          []string      `json:"comparisons"`
	InsightsEnabled      bool          `json:"insights_enabled"`
	DisabledInsights     []string      `json:"disabled_insights"`
	Certified            bool          `json:"certified"`
	RequiresFull         bool          `json:"requires_full"`
	ReviewComplete       bool          `json:"review_complete"`
}

// CompactCreateResult omits transport diagnostics.
type CreateCompactCreateResult struct {
	Status              string `json:"status"`
	DefinitionLUID      string `json:"definition_luid"`
	DefaultMetricLUID   string `json:"default_metric_luid"`
	DefaultMetricStatus string `json:"default_metric_status"`
}

// CompactResult is the default mutation projection.
type CreateCompactResult struct {
	Plan    CreateCompactPlan          `json:"plan"`
	Result  *CreateCompactCreateResult `json:"result,omitempty"`
	Details string                     `json:"details"`
	Help    []string                   `json:"help"`
}

// FullResult contains the exact normalized request and bounded diagnostics.
type CreateFullResult struct {
	Plan   CreatePlan    `json:"plan"`
	Result *CreateResult `json:"result,omitempty"`
	Help   []string      `json:"help"`
}

// CompactOutput returns the safety-critical plan and identities.
func (o CreateOutput) CompactOutput() any {
	plan := createCompactPlan(o.Plan)
	var result *CreateCompactCreateResult
	if o.Result != nil {
		result = &CreateCompactCreateResult{Status: o.Result.Status, DefinitionLUID: o.Result.DefinitionLUID, DefaultMetricLUID: o.Result.DefaultMetricLUID, DefaultMetricStatus: o.Result.DefaultMetricStatus}
	}
	return CreateCompactResult{Plan: plan, Result: result, Details: "--full", Help: o.Help}
}

// FullOutput returns the exact normalized request.
func (o CreateOutput) FullOutput() any {
	return CreateFullResult{Plan: o.Plan, Result: o.Result, Help: o.Help}
}
