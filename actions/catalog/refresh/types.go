package refresh

import "time"

// Input selects one resolved site inventory and its admitted scopes.
type Input struct {
	Environment  string
	Site         string
	SiteResolved bool
	Scopes       []string
}

// Record is one normalized cross-resource catalog identity.
type Record struct {
	LUID        string `json:"luid"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	ProjectPath string `json:"project_path,omitempty"`
	Owner       string `json:"owner,omitempty"`
}

// Snapshot is one complete remote inventory read.
type Snapshot struct {
	GeneratedAt time.Time
	Source      string
	Records     []Record
	Warnings    []string
}

// Generation is the complete normalized value passed to durable storage.
type Generation struct {
	Environment string
	Site        string
	GeneratedAt time.Time
	Complete    bool
	Source      string
	Scopes      []string
	Records     []Record
}

// WriteResult identifies the generation published by storage.
type WriteResult struct {
	GenerationID string
	Path         string
	RecordCount  int
}

// GenerationOutput is the bounded refresh generation projection.
type GenerationOutput struct {
	ID          string   `json:"id"`
	Environment string   `json:"environment"`
	Site        string   `json:"site"`
	GeneratedAt string   `json:"generated_at"`
	Records     int      `json:"records"`
	Source      string   `json:"source,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
}

// Output is the stable catalog.refresh document.
type Output struct {
	Status     string
	Generation GenerationOutput
	Path       string
	Warnings   []string
	Help       []string
}

// CompactGeneration contains refresh decision fields.
type CompactGeneration struct {
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
	Status     string           `json:"status"`
	Generation GenerationOutput `json:"generation"`
	Path       string           `json:"path"`
	Warnings   []string         `json:"warnings,omitempty"`
	Help       []string         `json:"help"`
}

// CompactOutput returns refresh decision fields.
func (o Output) CompactOutput() any {
	g := o.Generation
	return CompactResult{Status: o.Status, Generation: CompactGeneration{ID: g.ID, Environment: g.Environment, Site: g.Site, GeneratedAt: g.GeneratedAt, Records: g.Records}, Path: o.Path, Warnings: o.Warnings, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded generation provenance.
func (o Output) FullOutput() any {
	return FullResult{Status: o.Status, Generation: o.Generation, Path: o.Path, Warnings: o.Warnings, Help: o.Help}
}
