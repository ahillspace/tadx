package status

// Input selects one resolved local catalog generation.
type Input struct {
	Environment  string
	Site         string
	SiteResolved bool
}

// Result is the source-facing generation status.
type Result struct {
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
	ID          string `json:"id"`
	Environment string `json:"environment"`
	Site        string `json:"site"`
	GeneratedAt string `json:"generated_at"`
	Records     int    `json:"records"`
	Complete    bool   `json:"complete"`
	Stale       bool   `json:"stale"`
	Age         string `json:"age,omitempty"`
	Source      string `json:"source,omitempty"`
}

// Output is the stable catalog.status document.
type Output struct {
	Status     string
	Generation Generation
	Path       string
	Warnings   []string
	Help       []string
}

// CompactGeneration contains status decision fields.
type CompactGeneration struct {
	ID          string `json:"id"`
	Environment string `json:"environment"`
	Site        string `json:"site"`
	GeneratedAt string `json:"generated_at"`
	Records     int    `json:"records"`
	Complete    bool   `json:"complete"`
	Stale       bool   `json:"stale"`
}

// CompactResult is the default bounded status projection.
type CompactResult struct {
	Status     string            `json:"status"`
	Generation CompactGeneration `json:"generation"`
	Warnings   []string          `json:"warnings,omitempty"`
	Details    string            `json:"details"`
	Help       []string          `json:"help"`
}

// FullResult is the expanded bounded status projection.
type FullResult struct {
	Status     string     `json:"status"`
	Generation Generation `json:"generation"`
	Path       string     `json:"path,omitempty"`
	Warnings   []string   `json:"warnings,omitempty"`
	Help       []string   `json:"help"`
}

// UninitializedResult describes a selected catalog without inventing a generation.
type UninitializedResult struct {
	Status      string   `json:"status"`
	Environment string   `json:"environment"`
	Site        string   `json:"site"`
	Help        []string `json:"help"`
}

// CompactOutput returns freshness and completeness fields.
func (o Output) CompactOutput() any {
	g := o.Generation
	if o.Status == "uninitialized" {
		return UninitializedResult{Status: o.Status, Environment: g.Environment, Site: g.Site, Help: o.Help}
	}
	return CompactResult{Status: o.Status, Generation: CompactGeneration{ID: g.ID, Environment: g.Environment, Site: g.Site, GeneratedAt: g.GeneratedAt, Records: g.Records, Complete: g.Complete, Stale: g.Stale}, Warnings: o.Warnings, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded generation source details.
func (o Output) FullOutput() any {
	if o.Status == "uninitialized" {
		return o.CompactOutput()
	}
	return FullResult{Status: o.Status, Generation: o.Generation, Path: o.Path, Warnings: o.Warnings, Help: o.Help}
}
