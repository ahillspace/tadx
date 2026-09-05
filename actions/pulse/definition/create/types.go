package create

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

// CreateRequest is the exact, normalized provider request owned by this action.
type CreateRequest struct {
	Name                  string                `json:"name"`
	Description           string                `json:"description"`
	Specification         Specification         `json:"specification"`
	ExtensionOptions      ExtensionOptions      `json:"extension_options"`
	RepresentationOptions RepresentationOptions `json:"representation_options"`
	InsightsOptions       InsightsOptions       `json:"insights_options"`
	Comparisons           Comparisons           `json:"comparisons"`
	DatasourceGoals       []map[string]any      `json:"datasource_goals"`
	RelatedLinks          []map[string]any      `json:"related_links"`
	Certification         Certification         `json:"certification"`
}

// Specification identifies the datasource and metric semantics.
type Specification struct {
	Datasource         Datasource         `json:"datasource"`
	BasicSpecification BasicSpecification `json:"basic_specification"`
	RunningTotal       bool               `json:"is_running_total"`
	Temporality        string             `json:"temporality"`
}

// Datasource identifies one published datasource.
type Datasource struct {
	ID string `json:"id"`
}

// BasicSpecification contains the initial default metric query.
type BasicSpecification struct {
	Measure       Measure       `json:"measure"`
	TimeDimension TimeDimension `json:"time_dimension"`
	Filters       []Filter      `json:"filters"`
}

// Measure identifies one exact measure and aggregation.
type Measure struct {
	Field       string `json:"field"`
	Aggregation string `json:"aggregation"`
}

// TimeDimension identifies one exact temporal field.
type TimeDimension struct {
	Field string `json:"field"`
}

// Filter remains empty for definition creation. Scoped filters belong to metric fork.
type Filter struct {
	Field             string             `json:"field"`
	Operator          string             `json:"operator"`
	CategoricalValues []CategoricalValue `json:"categorical_values"`
	IncludeNull       bool               `json:"include_null"`
}

// CategoricalValue is retained for exact provider serialization.
type CategoricalValue struct {
	StringValue *string `json:"string_value,omitempty"`
	BoolValue   *bool   `json:"bool_value,omitempty"`
	NullValue   *string `json:"null_value,omitempty"`
}

// ExtensionOptions controls available dimensions and time grains.
type ExtensionOptions struct {
	AllowedDimensions    []string `json:"allowed_dimensions"`
	AllowedGranularities []string `json:"allowed_granularities"`
	OffsetFromToday      int      `json:"offset_from_today"`
	UseDynamicOffset     bool     `json:"use_dynamic_offset"`
}

// RepresentationOptions controls display semantics.
type RepresentationOptions struct {
	Type          string `json:"type"`
	SentimentType string `json:"sentiment_type"`
	CurrencyCode  string `json:"currency_code,omitempty"`
}

// InsightsOptions contains the proven default insight configuration.
type InsightsOptions struct {
	ShowInsights bool             `json:"show_insights"`
	Settings     []InsightSetting `json:"settings"`
}

// InsightSetting enables or disables one supported insight.
type InsightSetting struct {
	Type     string `json:"type"`
	Disabled bool   `json:"disabled"`
}

// Comparisons contains ordered comparison settings.
type Comparisons struct {
	Comparisons []Comparison `json:"comparisons"`
}

// Comparison identifies one comparison and its display index.
type Comparison struct {
	CompareConfig CompareConfig `json:"compare_config"`
	Index         int           `json:"index"`
}

// CompareConfig contains one Tableau comparison enum.
type CompareConfig struct {
	Comparison string `json:"comparison"`
}

// Certification explicitly creates an uncertified definition.
type Certification struct {
	IsCertified bool `json:"is_certified"`
}

// Plan is the deterministic preview and the only value Apply accepts.
type Plan struct {
	Mode        string        `json:"mode"`
	Operation   string        `json:"operation"`
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
	Mode        string   `json:"mode"`
	Operation   string   `json:"operation"`
	Name        string   `json:"name"`
	Datasource  string   `json:"datasource_luid"`
	Measure     Measure  `json:"measure"`
	TimeField   string   `json:"time_dimension"`
	Dimensions  []string `json:"allowed_dimensions"`
	Fingerprint string   `json:"request_fingerprint"`
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
	plan := CompactPlan{Mode: o.Plan.Mode, Operation: o.Plan.Operation, Name: o.Plan.Name, Datasource: o.Plan.Datasource, Measure: o.Plan.Measure, TimeField: o.Plan.TimeField, Dimensions: append([]string(nil), o.Plan.Dimensions...), Fingerprint: o.Plan.Fingerprint}
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
