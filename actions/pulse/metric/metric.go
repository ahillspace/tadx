// Package metric owns Pulse metric observations and explicit lifecycle operations.
package metric

import "github.com/ahillspace/tadx/internal/value"

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

// Subscription is shared by followers, exact unfollow, and the saved snapshot.
type Subscription = value.PulseSubscription
