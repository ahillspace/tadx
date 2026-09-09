package pulse

import "encoding/json"

// PageRequest selects one bounded Pulse page.
type PageRequest struct {
	PageSize  int
	PageToken string
}

// Definition is one normalized Pulse definition.
type Definition struct {
	LUID                 string
	Name                 string
	Description          string
	DatasourceLUID       string
	MeasureField         string
	Aggregation          string
	TimeDimension        string
	RunningTotal         bool
	Temporality          string
	AllowedDimensions    []string
	AllowedGranularities []string
	FixedFilters         []any
	FixedFiltersKnown    bool
	Configuration        json.RawMessage
	TableauRequestID     string
}

// DefinitionPage is one bounded Pulse definition page.
type DefinitionPage struct {
	Definitions      []Definition
	NextPageToken    string
	TableauRequestID string
}

// Page retains the definition-list contract name used by action adapters.
type Page = DefinitionPage

// Metric is one normalized exact Pulse metric specification.
type Metric struct {
	LUID             string
	Name             string
	DefinitionLUID   string
	SiteLUID         string
	IsDefault        bool
	DefaultKnown     bool
	Specification    map[string]any
	Configuration    json.RawMessage
	TableauRequestID string
}

// MetricPage is one bounded definition-scoped metric page.
type MetricPage struct {
	Metrics          []Metric
	NextPageToken    string
	TableauRequestID string
}

// GetOrCreateRequest is the exact desired-state metric payload.
type GetOrCreateRequest struct {
	DefinitionLUID string
	Specification  map[string]any
}

// GetOrCreateResult is the authoritative get-or-create outcome.
type GetOrCreateResult struct {
	MetricLUID       string
	MetricName       string
	Created          bool
	TableauRequestID string
}

// ExpectedMetric contains the authoritative ownership expected after get-or-create.
type ExpectedMetric struct {
	MetricLUID     string
	DefinitionLUID string
	DatasourceLUID string
	SiteLUID       string
	Specification  map[string]any
}

// Reconciliation reports verified saved configuration, not numerical metric values.
type Reconciliation struct {
	Status                string
	Attempts              int
	OwnershipVerified     bool
	TableauRequestID      string
	SpecificationVerified bool
	Metric                Metric
	Definition            Definition
}

// Subscription is one exact metric follower relationship.
type Subscription struct {
	LUID             string
	MetricLUID       string
	FollowerType     string
	FollowerLUID     string
	FollowerName     string
	TableauRequestID string
}

// CreateSubscriptionRequest identifies one exact desired follower relationship.
type CreateSubscriptionRequest struct {
	MetricLUID   string
	FollowerType string
	FollowerLUID string
}

// CreateSubscriptionResult reports whether the desired relationship changed.
type CreateSubscriptionResult struct {
	Status           string
	SubscriptionLUID string
	TableauRequestID string
}

// Definition creation request contracts.
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

type Specification struct {
	Datasource         Datasource         `json:"datasource"`
	BasicSpecification BasicSpecification `json:"basic_specification"`
	RunningTotal       bool               `json:"is_running_total"`
	Temporality        string             `json:"temporality"`
}
type Datasource struct {
	ID string `json:"id"`
}
type BasicSpecification struct {
	Measure       Measure       `json:"measure"`
	TimeDimension TimeDimension `json:"time_dimension"`
	Filters       []Filter      `json:"filters"`
}
type Measure struct {
	Field       string `json:"field"`
	Aggregation string `json:"aggregation"`
}
type TimeDimension struct {
	Field string `json:"field"`
}
type Filter struct {
	Field             string             `json:"field"`
	Operator          string             `json:"operator"`
	CategoricalValues []CategoricalValue `json:"categorical_values"`
	IncludeNull       bool               `json:"include_null"`
}
type CategoricalValue struct {
	StringValue *string `json:"string_value,omitempty"`
	BoolValue   *bool   `json:"bool_value,omitempty"`
	NullValue   *string `json:"null_value,omitempty"`
}
type ExtensionOptions struct {
	AllowedDimensions    []string `json:"allowed_dimensions"`
	AllowedGranularities []string `json:"allowed_granularities"`
	OffsetFromToday      int      `json:"offset_from_today"`
	UseDynamicOffset     bool     `json:"use_dynamic_offset"`
}
type RepresentationOptions struct {
	Type          string `json:"type"`
	SentimentType string `json:"sentiment_type"`
	CurrencyCode  string `json:"currency_code,omitempty"`
}
type InsightsOptions struct {
	ShowInsights bool             `json:"show_insights"`
	Settings     []InsightSetting `json:"settings"`
}
type InsightSetting struct {
	Type     string `json:"type"`
	Disabled bool   `json:"disabled"`
}
type Comparisons struct {
	Comparisons []Comparison `json:"comparisons"`
}
type Comparison struct {
	CompareConfig CompareConfig `json:"compare_config"`
	Index         int           `json:"index"`
}
type CompareConfig struct {
	Comparison string `json:"comparison"`
}
type Certification struct {
	IsCertified bool `json:"is_certified"`
}

type CreateResult struct {
	Status              string
	DefinitionLUID      string
	DefaultMetricLUID   string
	DefaultMetricStatus string
	TableauRequestID    string
	PollRequestID       string
}

// CreateDefinitionRequest names the definition-specific create contract.
type CreateDefinitionRequest = CreateRequest

// CreateDefinitionResult names the definition-specific create result.
type CreateDefinitionResult = CreateResult
