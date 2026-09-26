package pulse

import (
	"encoding/json"

	"github.com/ahillspace/tadx/internal/value"
)

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

// SubscriptionPage is one bounded user-filtered subscription page.
type SubscriptionPage struct {
	Subscriptions    []Subscription
	NextPageToken    string
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

// The typed definition request is shared with the authoring action.
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
