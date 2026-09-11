// Package readsource defines the source and freshness contract shared by read actions.
package readsource

import "time"

const (
	// Tableau identifies an authoritative live Tableau read.
	Tableau = "tableau"
	// Cache identifies an explicit local cache read.
	Cache = "cache"

	// CoverageComplete means the source satisfies the requested read projection.
	CoverageComplete = "complete"
	// CoveragePartial means the source contains known records without exhaustive scope coverage.
	CoveragePartial = "partial"
)

// Metadata describes where a read result came from and how current its coverage is.
type Metadata struct {
	Mode              string `json:"mode"`
	ObservedAt        string `json:"observed_at,omitempty"`
	Coverage          string `json:"coverage"`
	CoverageReason    string `json:"coverage_reason,omitempty"`
	Stale             bool   `json:"stale"`
	GenerationID      string `json:"generation_id,omitempty"`
	GenerationCreated string `json:"generation_generated_at,omitempty"`
	CacheRefreshed    bool   `json:"cache_refreshed,omitempty"`
	CacheGeneration   string `json:"cache_generation_id,omitempty"`
	CacheWarning      string `json:"cache_warning,omitempty"`
}

// LiveInventoryWarning identifies an authoritative live inventory whose local
// cache publication failed. The returned data remains authoritative.
func LiveInventoryWarning(observedAt time.Time) Metadata {
	value := Live(observedAt)
	value.CacheWarning = "The live inventory succeeded, but the local cache snapshot was not updated."
	return value
}

// LiveInventory identifies an authoritative live read that also published a
// complete local cache scope snapshot.
func LiveInventory(observedAt time.Time, generationID string) Metadata {
	value := Live(observedAt)
	value.CacheRefreshed = true
	value.CacheGeneration = generationID
	return value
}

// Live returns metadata for an authoritative Tableau response.
func Live(observedAt time.Time) Metadata {
	return Metadata{Mode: Tableau, ObservedAt: timestamp(observedAt), Coverage: CoverageComplete}
}

// Cached returns metadata for a local cache result.
func Cached(observedAt time.Time, coverage, generationID string, generatedAt time.Time, stale bool) Metadata {
	reason := ""
	if coverage == "" {
		coverage = CoveragePartial
	}
	if coverage == CoveragePartial {
		reason = "not_fully_observed"
	}
	return Metadata{
		Mode:              Cache,
		ObservedAt:        timestamp(observedAt),
		Coverage:          coverage,
		CoverageReason:    reason,
		Stale:             stale,
		GenerationID:      generationID,
		GenerationCreated: timestamp(generatedAt),
	}
}

func timestamp(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
