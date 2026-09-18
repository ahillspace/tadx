package fork

type Input struct {
	Environment   string
	Site          string
	SiteLUID      string
	MetricLUID    string
	Timeframe     string
	CustomDays    int
	CustomDaysSet bool `json:"-"`
	Filters       []Filter
}
type Filter struct {
	Field   string   `json:"field"`
	Values  []string `json:"values"`
	Exclude bool     `json:"exclude"`
}
type Metric struct {
	LUID           string
	DefinitionLUID string
	SiteLUID       string
	Specification  map[string]any
}
type Definition struct {
	LUID                 string
	DatasourceLUID       string
	AllowedDimensions    []string
	AllowedGranularities []string
	FixedFilters         []any
	FixedFiltersKnown    bool
}
type CreateRequest struct {
	DefinitionLUID string
	Specification  map[string]any
}
type CreateResult struct {
	MetricLUID string
	MetricName string
	Created    bool
	RequestID  string
}
type ExpectedMetric struct {
	MetricLUID     string
	DefinitionLUID string
	DatasourceLUID string
	SiteLUID       string
	Specification  map[string]any
}
type Reconciliation struct {
	Status                string
	Attempts              int
	OwnershipVerified     bool
	RequestID             string
	SpecificationVerified bool
	SavedSpecification    map[string]any
	MetricRequestID       string
	DefinitionRequestID   string
	SavedDefinition       SavedDefinition
}

type SavedDefinition struct {
	LUID           string `json:"luid"`
	Name           string `json:"name"`
	DatasourceLUID string `json:"datasource_luid"`
}
type Plan struct {
	Mode                   string         `json:"mode"`
	Operation              string         `json:"operation"`
	Environment            string         `json:"environment,omitempty"`
	Site                   string         `json:"site,omitempty"`
	SourceMetricLUID       string         `json:"source_metric_luid"`
	DefinitionLUID         string         `json:"definition_luid"`
	DatasourceLUID         string         `json:"datasource_luid"`
	Timeframe              string         `json:"timeframe,omitempty"`
	Filters                []Filter       `json:"filters,omitempty"`
	Specification          map[string]any `json:"specification"`
	DefinitionFilters      []any          `json:"definition_filters"`
	DefinitionFiltersKnown bool           `json:"definition_filters_known"`
	Fingerprint            string         `json:"request_fingerprint"`
}
type Result struct {
	Status                      string          `json:"status"`
	MetricLUID                  string          `json:"metric_luid"`
	MetricName                  string          `json:"metric_name,omitempty"`
	Created                     bool            `json:"created"`
	ReconciliationStatus        string          `json:"reconciliation_status"`
	ReconciliationAttempts      int             `json:"reconciliation_attempts"`
	OwnershipVerified           bool            `json:"ownership_verified"`
	SpecificationVerified       bool            `json:"specification_verified"`
	SavedSpecification          map[string]any  `json:"saved_specification"`
	SavedDefinition             SavedDefinition `json:"saved_definition"`
	MetricReadbackRequestID     string          `json:"metric_readback_request_id,omitempty"`
	DefinitionReadbackRequestID string          `json:"definition_readback_request_id,omitempty"`
	RequestID                   string          `json:"tableau_request_id,omitempty"`
	ReconciliationRequestID     string          `json:"reconciliation_request_id,omitempty"`
}
type Output struct {
	Plan   Plan     `json:"plan"`
	Result *Result  `json:"result,omitempty"`
	Help   []string `json:"help"`
}
type CompactResult struct {
	Status                string `json:"status"`
	MetricLUID            string `json:"metric_luid"`
	Created               bool   `json:"created"`
	ReconciliationStatus  string `json:"reconciliation_status"`
	OwnershipVerified     bool   `json:"ownership_verified"`
	SpecificationVerified bool   `json:"specification_verified"`
}
type CompactOutput struct {
	Plan    CompactPlan    `json:"plan"`
	Result  *CompactResult `json:"result,omitempty"`
	Details string         `json:"details"`
	Help    []string       `json:"help"`
}

func (o Output) CompactOutput() any {
	var result *CompactResult
	if o.Result != nil {
		result = &CompactResult{Status: o.Result.Status, MetricLUID: o.Result.MetricLUID, Created: o.Result.Created, ReconciliationStatus: o.Result.ReconciliationStatus, OwnershipVerified: o.Result.OwnershipVerified, SpecificationVerified: o.Result.SpecificationVerified}
	}
	return CompactOutput{Plan: compactPlan(o.Plan), Result: result, Details: "--full", Help: o.Help}
}
func (o Output) FullOutput() any { return o }
