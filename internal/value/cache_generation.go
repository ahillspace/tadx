package value

import "time"

// CacheGenerationRequest is one normalized, exact-target collection plan.
type CacheGenerationRequest struct {
	Environment     string
	Site            string
	RequestedScopes []string
	ImplicitScopes  []string
}

// CacheScopeCount is the collected row count for one requested or implicit scope.
type CacheScopeCount struct {
	Scope   string `json:"scope"`
	Records int    `json:"records"`
}

// CacheGenerationDiagnostics holds bounded collector measurements.
type CacheGenerationDiagnostics struct {
	Requests       int    `json:"requests"`
	FailedRequests int    `json:"failed_requests"`
	Duration       string `json:"duration,omitempty"`
}

// CacheGenerationReceipt records what the collector published, without cache rows.
type CacheGenerationReceipt struct {
	GenerationID        string
	GeneratedAt         time.Time
	Complete            bool
	Source              string
	Path                string
	RecordCount         int
	HydratedRecordCount int
	RequestedScopes     []string
	ImplicitScopes      []string
	ScopeCounts         []CacheScopeCount
	Diagnostics         CacheGenerationDiagnostics
	Warnings            []string
	DeniedPermissions   int
}

// CacheSelection identifies the canonical environment and exact site to inspect.
type CacheSelection struct {
	Environment string
	Site        string
}

// CacheStatusObservation contains local generation facts before output projection.
type CacheStatusObservation struct {
	Retained    []CachedObservation
	Coverage    []CacheCoverage
	ID          string
	Environment string
	Site        string
	GeneratedAt string
	Age         string
	Complete    bool
	Stale       bool
	Source      string
	Path        string
	Records     int
	Warnings    []string
}
