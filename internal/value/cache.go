package value

// CacheCoverage describes evidence retained for one bounded inventory scope.
// Requested false identifies a dependency collected for another scope.
type CacheCoverage struct {
	Scope     string `json:"scope"`
	Requested bool   `json:"requested"`
	Complete  bool   `json:"complete"`
	Records   int    `json:"records"`
}

// CachedObservation keeps independent scope coverage separate from a refresh.
type CachedObservation struct {
	Kind     string `json:"kind"`
	Records  int    `json:"records"`
	Complete bool   `json:"complete"`
	Stale    bool   `json:"stale"`
	Oldest   string `json:"oldest_observation,omitempty"`
	Newest   string `json:"newest_observation,omitempty"`
}
