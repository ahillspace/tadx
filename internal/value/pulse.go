package value

// PulseMetricRequest carries one desired metric specification to get or create.
type PulseMetricRequest struct {
	DefinitionLUID string
	Specification  map[string]any
}

// PulseExpectedMetric identifies the exact saved metric and ownership to verify.
type PulseExpectedMetric struct {
	MetricLUID     string
	DefinitionLUID string
	DatasourceLUID string
	SiteLUID       string
	Specification  map[string]any
}

// PulseSubscriptionRequest identifies one exact desired follower relationship.
type PulseSubscriptionRequest struct {
	MetricLUID   string
	FollowerType string
	FollowerLUID string
}

// PulseSubscription is one observed metric follower relationship.
// The same fields form the versioned follower snapshot payload.
type PulseSubscription struct {
	LUID         string `json:"luid"`
	MetricLUID   string `json:"metric_luid"`
	FollowerType string `json:"follower_type"`
	FollowerLUID string `json:"follower_luid"`
	FollowerName string `json:"follower_name,omitempty"`
	RequestID    string `json:"-"`
}
