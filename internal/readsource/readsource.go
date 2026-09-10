// Package readsource defines the source and freshness contract shared by read actions.
package readsource

import "time"

const (
	// Tableau identifies an authoritative live Tableau read.
	Tableau = "tableau"
	// Catalog identifies an explicit local catalog read.
	Catalog = "catalog"

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
	CatalogRefreshed  bool   `json:"catalog_refreshed,omitempty"`
	CatalogGeneration string `json:"catalog_generation_id,omitempty"`
	CatalogWarning    string `json:"catalog_warning,omitempty"`
}

// LiveInventoryWarning identifies an authoritative live inventory whose local
// catalog publication failed. The returned data remains authoritative.
func LiveInventoryWarning(observedAt time.Time) Metadata {
	value := Live(observedAt)
	value.CatalogWarning = "The live inventory succeeded, but the local catalog snapshot was not updated."
	return value
}

// LiveInventory identifies an authoritative live read that also published a
// complete local catalog scope snapshot.
func LiveInventory(observedAt time.Time, generationID string) Metadata {
	value := Live(observedAt)
	value.CatalogRefreshed = true
	value.CatalogGeneration = generationID
	return value
}

// Live returns metadata for an authoritative Tableau response.
func Live(observedAt time.Time) Metadata {
	return Metadata{Mode: Tableau, ObservedAt: timestamp(observedAt), Coverage: CoverageComplete}
}

// Cached returns metadata for a local catalog result.
func Cached(observedAt time.Time, coverage, generationID string, generatedAt time.Time, stale bool) Metadata {
	reason := ""
	if coverage == "" {
		coverage = CoveragePartial
	}
	if coverage == CoveragePartial {
		reason = "not_fully_observed"
	}
	return Metadata{
		Mode:              Catalog,
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
