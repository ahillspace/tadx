package search

// Input contains bounded cross-resource remote search filters.
type Input struct {
	Environment    string
	Site           string
	SiteResolved   bool
	Terms          string
	Kinds          []string
	OwnerLUID      string
	ProjectLUID    string
	ModifiedAfter  string
	ModifiedBefore string
	Cursor         string
	Limit          int
}

// Page is the bounded continuation envelope.
type Page struct {
	Returned   int    `json:"returned"`
	Total      int    `json:"total,omitempty"`
	Limit      int    `json:"limit"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// Item is one normalized cross-resource content result.
type Item struct {
	LUID        string `json:"luid"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	ProjectPath string `json:"project_path,omitempty"`
	Owner       string `json:"owner,omitempty"`
	ModifiedAt  string `json:"modified_at,omitempty"`
}

// Result is the remote source-facing normalized result.
type Result struct {
	Page     Page
	Items    []Item
	Warnings []string
}

// Output is the stable content.search document.
type Output struct {
	Page     Page
	Items    []Item
	Warnings []string
	Help     []string
}

// CompactItem contains authoritative search identity.
type CompactItem struct {
	LUID        string `json:"luid"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	ProjectPath string `json:"project_path,omitempty"`
}

// CompactResult is the default bounded search projection.
type CompactResult struct {
	Page     Page          `json:"page"`
	Items    []CompactItem `json:"items"`
	Warnings []string      `json:"warnings,omitempty"`
	Details  string        `json:"details"`
	Help     []string      `json:"help"`
}

// FullResult is the expanded bounded search projection.
type FullResult struct {
	Page     Page     `json:"page"`
	Items    []Item   `json:"items"`
	Warnings []string `json:"warnings,omitempty"`
	Help     []string `json:"help"`
}

// CompactOutput returns authoritative identities for exact follow-up.
func (o Output) CompactOutput() any {
	items := make([]CompactItem, len(o.Items))
	for index, item := range o.Items {
		items[index] = CompactItem{LUID: item.LUID, Kind: item.Kind, Name: item.Name, ProjectPath: item.ProjectPath}
	}
	return CompactResult{Page: o.Page, Items: items, Warnings: o.Warnings, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded source details for the same page.
func (o Output) FullOutput() any {
	return FullResult{Page: o.Page, Items: o.Items, Warnings: o.Warnings, Help: o.Help}
}
