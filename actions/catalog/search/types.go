package search

// Input contains bounded local catalog filters.
type Input struct {
	Text         string
	Kind         string
	ProjectPath  string
	Owner        string
	Environment  string
	Site         string
	SiteResolved bool
	LUID         string
	Cursor       string
	Limit        int
}

// Page is the normalized bounded continuation envelope.
type Page struct {
	Returned   int    `json:"returned"`
	Total      int    `json:"total"`
	Limit      int    `json:"limit"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// Item is one compact catalog result.
type Item struct {
	LUID        string `json:"luid"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	ProjectPath string `json:"project_path,omitempty"`
	Owner       string `json:"owner,omitempty"`
}

// Result is the source-facing normalized search result.
type Result struct {
	Page         Page
	GenerationID string
	Environment  string
	Site         string
	GeneratedAt  string
	Stale        bool
	Items        []Item
	Warnings     []string
}

// Generation describes the exact local source generation.
type Generation struct {
	ID          string `json:"id"`
	Environment string `json:"environment"`
	Site        string `json:"site"`
	GeneratedAt string `json:"generated_at"`
	Stale       bool   `json:"stale"`
}

// Output is the stable catalog.search document.
type Output struct {
	Page       Page       `json:"page"`
	Generation Generation `json:"generation"`
	Items      []Item     `json:"items"`
	Warnings   []string   `json:"warnings,omitempty"`
	Help       []string   `json:"help"`
}
