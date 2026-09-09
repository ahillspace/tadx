package refresh

import "time"

// Input selects one resolved site and the catalog scopes to hydrate.
type Input struct {
	Environment  string
	Site         string
	SiteResolved bool
	Scopes       []string
}

// HydrationRequest is the normalized action-owned request passed to storage-backed hydration.
type HydrationRequest struct {
	Environment     string
	Site            string
	RequestedScopes []string
	ImplicitScopes  []string
}

// ScopeCount is one bounded per-scope hydration count.
type ScopeCount struct {
	Scope   string `json:"scope"`
	Records int    `json:"records"`
}

// Diagnostics contains bounded operational hydration measurements.
type Diagnostics struct {
	Requests       int    `json:"requests"`
	FailedRequests int    `json:"failed_requests"`
	Duration       string `json:"duration,omitempty"`
}

// HydrationResult is a row-free receipt for one internally persisted generation.
type HydrationResult struct {
	GenerationID        string
	GeneratedAt         time.Time
	Complete            bool
	Source              string
	Path                string
	RecordCount         int
	HydratedRecordCount int
	RequestedScopes     []string
	ImplicitScopes      []string
	ScopeCounts         []ScopeCount
	Diagnostics         Diagnostics
	Warnings            []string
	DeniedPermissions   int
}

// GenerationOutput is the bounded refresh generation projection.
type GenerationOutput struct {
	Complete        bool         `json:"complete"`
	ID              string       `json:"id"`
	Environment     string       `json:"environment"`
	Site            string       `json:"site"`
	GeneratedAt     string       `json:"generated_at"`
	Records         int          `json:"records"`
	HydratedRecords int          `json:"hydrated_records,omitempty"`
	Source          string       `json:"source,omitempty"`
	Scopes          []string     `json:"scopes,omitempty"`
	ImplicitScopes  []string     `json:"implicit_scopes,omitempty"`
	ScopeCounts     []ScopeCount `json:"scope_counts,omitempty"`
}

// Output is the stable catalog.refresh document.
type Output struct {
	Status      string
	Generation  GenerationOutput
	Path        string
	Warnings    []string
	Diagnostics Diagnostics
	Help        []string
}

// CompactGeneration contains refresh decision fields.
type CompactGeneration struct {
	Complete    bool   `json:"complete"`
	ID          string `json:"id"`
	Environment string `json:"environment"`
	Site        string `json:"site"`
	GeneratedAt string `json:"generated_at"`
	Records     int    `json:"records"`
}

// CompactResult is the default bounded refresh projection.
type CompactResult struct {
	Status     string            `json:"status"`
	Generation CompactGeneration `json:"generation"`
	Path       string            `json:"path"`
	Warnings   []string          `json:"warnings,omitempty"`
	Details    string            `json:"details"`
	Help       []string          `json:"help"`
}

// FullResult is the expanded bounded refresh projection.
type FullResult struct {
	Status      string           `json:"status"`
	Generation  GenerationOutput `json:"generation"`
	Path        string           `json:"path"`
	Warnings    []string         `json:"warnings,omitempty"`
	Diagnostics Diagnostics      `json:"diagnostics"`
	Help        []string         `json:"help"`
}

// CompactOutput returns a row-free operational receipt.
func (o Output) CompactOutput() any {
	g := o.Generation
	return CompactResult{Status: o.Status, Generation: CompactGeneration{Complete: g.Complete, ID: g.ID, Environment: g.Environment, Site: g.Site, GeneratedAt: g.GeneratedAt, Records: g.Records}, Path: o.Path, Warnings: o.Warnings, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded generation provenance and diagnostics.
func (o Output) FullOutput() any {
	return FullResult{Status: o.Status, Generation: o.Generation, Path: o.Path, Warnings: o.Warnings, Diagnostics: o.Diagnostics, Help: o.Help}
}
