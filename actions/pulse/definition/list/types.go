package list

import "github.com/ahillspace/tadx/internal/readsource"

// Input selects one bounded Pulse definition page.
type Input struct {
	Environment string
	Site        string
	Cursor      string
	Limit       int
	Catalog     bool
}

// PageRequest is the action-owned upstream continuation request.
type PageRequest struct {
	PageSize  int
	PageToken string
}

// Definition is one complete bounded Pulse definition summary.
type Definition struct {
	LUID              string   `json:"luid"`
	Name              string   `json:"name"`
	Description       string   `json:"description,omitempty"`
	DatasourceLUID    string   `json:"datasource_luid"`
	MeasureField      string   `json:"measure_field,omitempty"`
	Aggregation       string   `json:"aggregation,omitempty"`
	TimeDimension     string   `json:"time_dimension,omitempty"`
	AllowedDimensions []string `json:"allowed_dimensions,omitempty"`
}

// Page is one provider page.
type Page struct {
	Definitions   []Definition
	NextPageToken string
	RequestID     string
}

// OutputPage contains stable continuation metadata.
type OutputPage struct {
	Returned   int    `json:"returned"`
	Limit      int    `json:"limit"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// Output retains complete details before projection.
type Output struct {
	Status      string
	Environment string
	Site        string
	Page        OutputPage
	Definitions []Definition
	RequestID   string
	Help        []string
	Source      *readsource.Metadata
}

// CompactDefinition identifies one definition and its datasource.
type CompactDefinition struct {
	LUID           string `json:"luid"`
	Name           string `json:"name"`
	DatasourceLUID string `json:"datasource_luid"`
}

// CompactResult is the default bounded projection.
type CompactResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Page        OutputPage           `json:"page"`
	Definitions []CompactDefinition  `json:"definitions"`
	Details     string               `json:"details"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// FullResult is the expanded bounded projection.
type FullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Page        OutputPage           `json:"page"`
	Definitions []Definition         `json:"definitions"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// CompactOutput returns stable definition identities.
func (o Output) CompactOutput() any {
	items := make([]CompactDefinition, len(o.Definitions))
	for index, item := range o.Definitions {
		items[index] = CompactDefinition{LUID: item.LUID, Name: item.Name, DatasourceLUID: item.DatasourceLUID}
	}
	return CompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Definitions: items, Details: "--full", Help: o.Help, Source: o.Source}
}

// FullOutput returns all fields from the same bounded provider page.
func (o Output) FullOutput() any {
	items := append([]Definition(nil), o.Definitions...)
	for index := range items {
		items[index].AllowedDimensions = append([]string(nil), items[index].AllowedDimensions...)
	}
	return FullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Definitions: items, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}
