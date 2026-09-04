package list

// Input controls capability discovery.
type Input struct {
	Domain           string `json:"domain,omitempty"`
	Resource         string `json:"resource,omitempty"`
	Owner            string `json:"owner,omitempty"`
	Product          string `json:"product,omitempty"`
	Mutation         *bool  `json:"mutation,omitempty"`
	Cursor           string `json:"cursor,omitempty"`
	Limit            int    `json:"limit,omitempty"`
	MutationsEnabled bool   `json:"-"`
}

// Capability is the bounded discovery view of one registry entry.
type Capability struct {
	ID               string `json:"id"`
	Owner            string `json:"owner"`
	Disposition      string `json:"disposition,omitempty"`
	State            string `json:"state"`
	Command          string `json:"command,omitempty"`
	Blocked          bool   `json:"blocked"`
	Domain           string `json:"-"`
	Resource         string `json:"-"`
	Product          string `json:"-"`
	RemoteMutation   bool   `json:"-"`
	ExecutionEnabled bool   `json:"execution_enabled"`
}

// Pagination describes a bounded result page and its continuation.
type Pagination struct {
	Returned   int    `json:"returned"`
	Total      int    `json:"total"`
	Limit      int    `json:"limit"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// Output is the stable capability list result.
type Output struct {
	Page         Pagination   `json:"page"`
	Capabilities []Capability `json:"capabilities"`
	Help         []string     `json:"help"`
}
