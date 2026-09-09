package output

import "encoding/json"

// Page preserves private continuation state while exposing usable result bounds.
type Page struct {
	Returned      int    `json:"returned"`
	Total         int    `json:"total"`
	Limit         int    `json:"limit"`
	NextCursor    string `json:"-"`
	MoreAvailable bool   `json:"more_available"`
}

// MarshalJSON never publishes the private continuation token.
func (p Page) MarshalJSON() ([]byte, error) {
	type public Page
	value := public(p)
	value.MoreAvailable = value.MoreAvailable || value.NextCursor != ""
	return json.Marshal(value)
}
