package status

import "github.com/ahillspace/tadx/internal/value"

// Input selects one resolved local cache generation.
type Input struct {
	Environment  string
	Site         string
	SiteResolved bool
}

// Result is the source-facing generation status.
type Result struct {
	Retained    []value.CachedObservation
	Coverage    []value.CacheCoverage
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

// Generation is the bounded status projection.
type Generation struct {
	Coverage    []value.CacheCoverage `json:"coverage,omitempty"`
	ID          string                `json:"id"`
	Environment string                `json:"environment"`
	Site        string                `json:"site"`
	GeneratedAt string                `json:"generated_at"`
	Records     int                   `json:"records"`
	Complete    bool                  `json:"complete"`
	Stale       bool                  `json:"stale"`
	Age         string                `json:"age,omitempty"`
	Source      string                `json:"source,omitempty"`
}

// Output is the stable cache.status document.
type Output struct {
	Retained   []value.CachedObservation
	Status     string
	Generation Generation
	Path       string
	Warnings   []string
	Help       []string
}

// CompactGeneration contains status decision fields.
type CompactGeneration struct {
	Coverage    []value.CacheCoverage `json:"coverage,omitempty"`
	ID          string                `json:"id"`
	Environment string                `json:"environment"`
	Site        string                `json:"site"`
	GeneratedAt string                `json:"generated_at"`
	Records     int                   `json:"records"`
	Complete    bool                  `json:"complete"`
	Stale       bool                  `json:"stale"`
}

// CompactResult is the default bounded status projection.
type CompactResult struct {
	Retained   []value.CachedObservation `json:"retained_observations,omitempty"`
	Status     string                    `json:"status"`
	Generation CompactGeneration         `json:"generation"`
	Warnings   []string                  `json:"warnings,omitempty"`
	Details    string                    `json:"details"`
	Help       []string                  `json:"help"`
}

// FullResult is the expanded bounded status projection.
type FullResult struct {
	Retained   []value.CachedObservation `json:"retained_observations,omitempty"`
	Status     string                    `json:"status"`
	Generation Generation                `json:"generation"`
	Path       string                    `json:"path,omitempty"`
	Warnings   []string                  `json:"warnings,omitempty"`
	Help       []string                  `json:"help"`
}

// UninitializedResult describes a selected cache without inventing a generation.
type UninitializedResult struct {
	Retained    []value.CachedObservation `json:"retained_observations,omitempty"`
	Status      string                    `json:"status"`
	Environment string                    `json:"environment"`
	Site        string                    `json:"site"`
	Help        []string                  `json:"help"`
}

// CompactOutput returns freshness and completeness fields.
func (o Output) CompactOutput() any {
	g := o.Generation
	if o.Status == "uninitialized" {
		return UninitializedResult{Retained: o.compactRetained(), Status: o.Status, Environment: g.Environment, Site: g.Site, Help: o.Help}
	}
	return CompactResult{Retained: o.compactRetained(), Status: o.Status, Generation: CompactGeneration{Coverage: g.Coverage, ID: g.ID, Environment: g.Environment, Site: g.Site, GeneratedAt: g.GeneratedAt, Records: g.Records, Complete: g.Complete, Stale: g.Stale}, Warnings: o.Warnings, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded generation source details.
func (o Output) FullOutput() any {
	if o.Status == "uninitialized" {
		return UninitializedResult{Retained: o.Retained, Status: o.Status, Environment: o.Generation.Environment, Site: o.Generation.Site, Help: o.Help}
	}
	return FullResult{Retained: o.Retained, Status: o.Status, Generation: o.Generation, Path: o.Path, Warnings: o.Warnings, Help: o.Help}
}

func (o Output) compactRetained() []value.CachedObservation {
	var retained []value.CachedObservation
	for _, item := range o.Retained {
		// Rows belonging to the selected generation already appear in coverage.
		covered := false
		for _, scope := range o.Generation.Coverage {
			if scope.Requested && scope.Scope == item.Kind+"s" && scope.Records == item.Records && item.Oldest == o.Generation.GeneratedAt && item.Newest == o.Generation.GeneratedAt {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		item.Oldest, item.Newest = "", ""
		retained = append(retained, item)
	}
	return retained
}
