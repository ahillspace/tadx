package list

import (
	"github.com/ahillspace/tadx/internal/capability"
	"github.com/ahillspace/tadx/internal/output"
)

// Input controls capability discovery.
type Input struct {
	Domain                    string `json:"domain,omitempty"`
	Resource                  string `json:"resource,omitempty"`
	Owner                     string `json:"owner,omitempty"`
	Product                   string `json:"product,omitempty"`
	Mutation                  *bool  `json:"mutation,omitempty"`
	Cursor                    string `json:"cursor,omitempty"`
	Limit                     int    `json:"limit,omitempty"`
	Full                      bool   `json:"-"`
	JSON                      bool   `json:"-"`
	MutationsEnabled          bool   `json:"-"`
	MutationPolicyUnavailable bool   `json:"-"`
}

// Capability is the detailed discovery view retained for full output and
// saved results. Compact output projects it to capability.Summary.
type Capability = capability.Discovery

// Pagination describes a bounded result page and its continuation.
type Pagination = output.Page

// Output is the stable capability list result.
type Output struct {
	Page           Pagination   `json:"page"`
	Capabilities   []Capability `json:"capabilities"`
	Counts         Counts       `json:"counts"`
	MutationPolicy string       `json:"mutation_policy,omitempty"`
	NextCommand    string       `json:"next_command,omitempty"`
	Help           []string     `json:"help"`
}

// Counts separates the selected page, its filtered match set, and delegated
// records while preserving the combined page totals in Page.
type Counts struct {
	Returned   int `json:"returned"`
	Matched    int `json:"matched"`
	OutOfScope int `json:"out_of_scope"`
}
