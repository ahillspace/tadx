// Package metric owns Pulse metric observations and explicit lifecycle operations.
package metric

// Metric contains the same live facts for inspection, forking, followers, and deletion.
// DefaultKnown distinguishes an absent default observation from an observed false value.
// JSON retains the inspect projection; list and delete use their own bounded projections.
type Metric struct {
	LUID           string         `json:"luid"`
	Name           string         `json:"name,omitempty"`
	DefinitionLUID string         `json:"definition_luid"`
	SiteLUID       string         `json:"site_luid,omitempty"`
	IsDefault      bool           `json:"is_default"`
	DefaultKnown   bool           `json:"-"`
	Specification  map[string]any `json:"specification"`
	Configuration  []byte         `json:"-"`
	RequestID      string         `json:"-"`
}

// Subscription is an observed relationship, shared by listing and exact unfollow resolution.
// Its JSON is also the versioned follower snapshot payload; unfollow projects an explicit plan.
type Subscription struct {
	LUID         string `json:"luid"`
	MetricLUID   string `json:"metric_luid"`
	FollowerType string `json:"follower_type"`
	FollowerLUID string `json:"follower_luid"`
	FollowerName string `json:"follower_name,omitempty"`
	RequestID    string `json:"-"`
}
