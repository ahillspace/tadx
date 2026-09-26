package value

// PulseDefinitionCreateRequest is the normalized typed definition submission.
// Raw definition bundles use a separate document contract.
type PulseDefinitionCreateRequest struct {
	Name                  string                     `json:"name"`
	Description           string                     `json:"description"`
	Specification         PulseSpecification         `json:"specification"`
	ExtensionOptions      PulseExtensionOptions      `json:"extension_options"`
	RepresentationOptions PulseRepresentationOptions `json:"representation_options"`
	InsightsOptions       PulseInsightsOptions       `json:"insights_options"`
	Comparisons           PulseComparisons           `json:"comparisons"`
	DatasourceGoals       []map[string]any           `json:"datasource_goals"`
	RelatedLinks          []map[string]any           `json:"related_links"`
	Certification         PulseCertification         `json:"certification"`
}

type PulseSpecification struct {
	Datasource         PulseDatasource         `json:"datasource"`
	BasicSpecification PulseBasicSpecification `json:"basic_specification"`
	RunningTotal       bool                    `json:"is_running_total"`
	Temporality        string                  `json:"temporality"`
}

type PulseDatasource struct {
	ID string `json:"id"`
}

type PulseBasicSpecification struct {
	Measure       PulseMeasure       `json:"measure"`
	TimeDimension PulseTimeDimension `json:"time_dimension"`
	Filters       []PulseFilter      `json:"filters"`
}

type PulseMeasure struct {
	Field       string `json:"field"`
	Aggregation string `json:"aggregation"`
}

type PulseTimeDimension struct {
	Field string `json:"field"`
}

type PulseFilter struct {
	Field             string                  `json:"field"`
	Operator          string                  `json:"operator"`
	CategoricalValues []PulseCategoricalValue `json:"categorical_values"`
	IncludeNull       bool                    `json:"include_null"`
}

type PulseCategoricalValue struct {
	StringValue *string `json:"string_value,omitempty"`
	BoolValue   *bool   `json:"bool_value,omitempty"`
	NullValue   *string `json:"null_value,omitempty"`
}

type PulseExtensionOptions struct {
	AllowedDimensions    []string `json:"allowed_dimensions"`
	AllowedGranularities []string `json:"allowed_granularities"`
	OffsetFromToday      int      `json:"offset_from_today"`
	UseDynamicOffset     bool     `json:"use_dynamic_offset"`
}

type PulseRepresentationOptions struct {
	Type          string `json:"type"`
	SentimentType string `json:"sentiment_type"`
	CurrencyCode  string `json:"currency_code,omitempty"`
}

type PulseInsightsOptions struct {
	ShowInsights bool                  `json:"show_insights"`
	Settings     []PulseInsightSetting `json:"settings"`
}

type PulseInsightSetting struct {
	Type     string `json:"type"`
	Disabled bool   `json:"disabled"`
}

type PulseComparisons struct {
	Comparisons []PulseComparison `json:"comparisons"`
}

type PulseComparison struct {
	CompareConfig PulseCompareConfig `json:"compare_config"`
	Index         int                `json:"index"`
}

type PulseCompareConfig struct {
	Comparison string `json:"comparison"`
}

type PulseCertification struct {
	IsCertified bool `json:"is_certified"`
}
