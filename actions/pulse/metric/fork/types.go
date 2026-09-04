package fork

type Input struct {
	Environment string
	Site        string
	SiteLUID    string
	MetricLUID  string
	Timeframe   string
	CustomDays  int
	Filters     []Filter
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
	LUID              string
	DatasourceLUID    string
	AllowedDimensions []string
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
}
type Reconciliation struct {
	Status            string
	Attempts          int
	OwnershipVerified bool
	InventoryVisible  bool
	RequestID         string
}
type Plan struct {
	Mode             string         `json:"mode"`
	Operation        string         `json:"operation"`
	Environment      string         `json:"environment,omitempty"`
	Site             string         `json:"site,omitempty"`
	SourceMetricLUID string         `json:"source_metric_luid"`
	DefinitionLUID   string         `json:"definition_luid"`
	DatasourceLUID   string         `json:"datasource_luid"`
	Timeframe        string         `json:"timeframe,omitempty"`
	Filters          []Filter       `json:"filters,omitempty"`
	Specification    map[string]any `json:"specification"`
	Fingerprint      string         `json:"request_fingerprint"`
}
type Result struct {
	Status                  string `json:"status"`
	MetricLUID              string `json:"metric_luid"`
	MetricName              string `json:"metric_name,omitempty"`
	Created                 bool   `json:"created"`
	ReconciliationStatus    string `json:"reconciliation_status"`
	ReconciliationAttempts  int    `json:"reconciliation_attempts"`
	OwnershipVerified       bool   `json:"ownership_verified"`
	InventoryVisible        bool   `json:"inventory_visible"`
	RequestID               string `json:"tableau_request_id,omitempty"`
	ReconciliationRequestID string `json:"reconciliation_request_id,omitempty"`
}
type Output struct {
	Plan    Plan     `json:"plan"`
	Applied bool     `json:"applied"`
	Result  *Result  `json:"result,omitempty"`
	Help    []string `json:"help"`
}
type CompactResult struct {
	Status               string `json:"status"`
	MetricLUID           string `json:"metric_luid"`
	Created              bool   `json:"created"`
	ReconciliationStatus string `json:"reconciliation_status"`
}
type CompactOutput struct {
	Plan    Plan           `json:"plan"`
	Applied bool           `json:"applied"`
	Result  *CompactResult `json:"result,omitempty"`
	Details string         `json:"details"`
	Help    []string       `json:"help"`
}

func (o Output) CompactOutput() any {
	var result *CompactResult
	if o.Result != nil {
		result = &CompactResult{Status: o.Result.Status, MetricLUID: o.Result.MetricLUID, Created: o.Result.Created, ReconciliationStatus: o.Result.ReconciliationStatus}
	}
	return CompactOutput{Plan: o.Plan, Applied: o.Applied, Result: result, Details: "--full", Help: o.Help}
}
func (o Output) FullOutput() any { return o }
