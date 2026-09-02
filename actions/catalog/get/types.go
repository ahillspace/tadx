package get

// Input selects one cached record by authoritative LUID or exact labels.
type Input struct {
	Environment  string
	Site         string
	SiteResolved bool
	LUID         string
	Kind         string
	Name         string
	ProjectPath  string
}

// Item is one bounded catalog record.
type Item struct {
	LUID        string `json:"luid"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	ProjectPath string `json:"project_path,omitempty"`
	Owner       string `json:"owner,omitempty"`
}

// Generation describes the exact local source generation.
type Generation struct {
	ID          string `json:"id"`
	Environment string `json:"environment"`
	Site        string `json:"site"`
	GeneratedAt string `json:"generated_at,omitempty"`
	Stale       bool   `json:"stale"`
}

// Result is the source-facing exact lookup result.
type Result struct {
	Item       Item
	Generation Generation
	Warnings   []string
}

// Output is the stable catalog.get document.
type Output struct {
	Item       Item
	Generation Generation
	Warnings   []string
	Help       []string
}

// CompactItem contains authoritative identity and exact project path.
type CompactItem struct {
	LUID        string `json:"luid"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	ProjectPath string `json:"project_path,omitempty"`
}

// CompactGeneration contains current-source decision fields.
type CompactGeneration struct {
	ID          string `json:"id"`
	Environment string `json:"environment"`
	Site        string `json:"site"`
	Stale       bool   `json:"stale"`
}

// CompactResult is the default bounded lookup projection.
type CompactResult struct {
	Item       CompactItem       `json:"item"`
	Generation CompactGeneration `json:"generation"`
	Warnings   []string          `json:"warnings,omitempty"`
	Details    string            `json:"details"`
	Help       []string          `json:"help"`
}

// FullResult is the expanded bounded lookup projection.
type FullResult struct {
	Item       Item       `json:"item"`
	Generation Generation `json:"generation"`
	Warnings   []string   `json:"warnings,omitempty"`
	Help       []string   `json:"help"`
}

// CompactOutput returns authoritative identity and source freshness.
func (o Output) CompactOutput() any {
	return CompactResult{Item: CompactItem{LUID: o.Item.LUID, Kind: o.Item.Kind, Name: o.Item.Name, ProjectPath: o.Item.ProjectPath}, Generation: CompactGeneration{ID: o.Generation.ID, Environment: o.Generation.Environment, Site: o.Generation.Site, Stale: o.Generation.Stale}, Warnings: o.Warnings, Details: "--full", Help: o.Help}
}

// FullOutput returns the bounded complete cached record.
func (o Output) FullOutput() any {
	return FullResult{Item: o.Item, Generation: o.Generation, Warnings: o.Warnings, Help: o.Help}
}
