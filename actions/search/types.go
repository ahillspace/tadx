// Package search owns bounded live and cache search orchestration.
package search

// Input selects one source and a bounded search page.
type Input struct {
	Terms, Type, Environment, Site, ProjectPath, Owner, Cursor string
	SiteResolved, Cache                                        bool
	Limit                                                      int
}

// Page is the continuation envelope for the selected source.
type Page struct {
	UnresolvedMoreAvailable bool   `json:"-"`
	Returned                int    `json:"returned"`
	Total                   int    `json:"total,omitempty"`
	Limit                   int    `json:"limit"`
	NextCursor              string `json:"-"`
	MoreAvailable           bool   `json:"more_available"`
}

// Item provides authoritative identity for exact follow-up commands.
type Item struct {
	LUID        string `json:"luid"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	ProjectPath string `json:"project_path,omitempty"`
	Owner       string `json:"owner,omitempty"`
	ModifiedAt  string `json:"modified_at,omitempty"`
}

// Generation identifies the local cache snapshot when selected.
type Generation struct {
	ID          string `json:"id"`
	Environment string `json:"environment"`
	Site        string `json:"site"`
	GeneratedAt string `json:"generated_at"`
	Stale       bool   `json:"stale"`
}

// Result is a normalized page. The source owns ordering across its cursors.
type Result struct {
	Page       Page
	Items      []Item
	Warnings   []string
	Generation *Generation
	Source     string
}

// Output is the stable search document.
type Output struct {
	Scope      *Scope
	Source     string
	Page       Page
	Items      []Item
	Warnings   []string
	Generation *Generation
	Help       []string
}

// Scope identifies a resolved live target once, including the default site's empty URL.
type Scope struct {
	Environment string `json:"environment"`
	Site        string `json:"site"`
}

type CompactItem struct {
	LUID        string `json:"luid"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	ProjectPath string `json:"project_path,omitempty"`
}
type CompactResult struct {
	Scope      *Scope        `json:"scope,omitempty"`
	Source     string        `json:"source"`
	Page       Page          `json:"page"`
	Generation *Generation   `json:"generation,omitempty"`
	Items      []CompactItem `json:"items"`
	Warnings   []string      `json:"warnings,omitempty"`
	Details    string        `json:"details"`
	Help       []string      `json:"help"`
}
type FullResult struct {
	Scope      *Scope      `json:"scope,omitempty"`
	Source     string      `json:"source"`
	Page       Page        `json:"page"`
	Generation *Generation `json:"generation,omitempty"`
	Items      []Item      `json:"items"`
	Warnings   []string    `json:"warnings,omitempty"`
	Help       []string    `json:"help"`
}

func (o Output) CompactOutput() any {
	items := make([]CompactItem, len(o.Items))
	for i, item := range o.Items {
		items[i] = CompactItem{LUID: item.LUID, Type: item.Type, Name: item.Name, ProjectPath: item.ProjectPath}
	}
	return CompactResult{Scope: o.Scope, Source: o.Source, Page: o.Page, Generation: o.Generation, Items: items, Warnings: o.Warnings, Details: "--full", Help: o.Help}
}
func (o Output) FullOutput() any {
	return FullResult{Scope: o.Scope, Source: o.Source, Page: o.Page, Generation: o.Generation, Items: o.Items, Warnings: o.Warnings, Help: o.Help}
}
