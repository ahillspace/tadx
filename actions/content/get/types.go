package get

// Input identifies one resource through its type-specific adapter.
type Input struct {
	Environment  string
	Site         string
	SiteResolved bool
	Kind         string
	LUID         string
	Name         string
	ProjectPath  string
}

// Item is one normalized exact content item.
type Item struct {
	LUID        string `json:"luid"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	ProjectPath string `json:"project_path,omitempty"`
	Owner       string `json:"owner,omitempty"`
	ModifiedAt  string `json:"modified_at,omitempty"`
	URL         string `json:"url,omitempty"`
}

// Result is the adapter-facing exact lookup result.
type Result struct {
	Item     Item
	Warnings []string
}

// Output is the stable content.get document.
type Output struct {
	Item     Item
	Warnings []string
	Help     []string
}

// CompactItem contains authoritative exact identity.
type CompactItem struct {
	LUID        string `json:"luid"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	ProjectPath string `json:"project_path,omitempty"`
}

// CompactResult is the default bounded exact-read projection.
type CompactResult struct {
	Item     CompactItem `json:"item"`
	Warnings []string    `json:"warnings,omitempty"`
	Details  string      `json:"details"`
	Help     []string    `json:"help"`
}

// FullResult is the expanded bounded exact-read projection.
type FullResult struct {
	Item     Item     `json:"item"`
	Warnings []string `json:"warnings,omitempty"`
	Help     []string `json:"help"`
}

// CompactOutput returns authoritative identity fields.
func (o Output) CompactOutput() any {
	return CompactResult{Item: CompactItem{LUID: o.Item.LUID, Kind: o.Item.Kind, Name: o.Item.Name, ProjectPath: o.Item.ProjectPath}, Warnings: o.Warnings, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded resource details.
func (o Output) FullOutput() any {
	return FullResult{Item: o.Item, Warnings: o.Warnings, Help: o.Help}
}
