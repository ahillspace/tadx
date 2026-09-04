package list

import "github.com/ahillspace/tadx/internal/readsource"

type Input struct {
	Environment    string
	Site           string
	DefinitionLUID string
	Cursor         string
	Limit          int
	Catalog        bool
}
type PageRequest struct {
	PageSize  int
	PageToken string
}
type Metric struct {
	LUID           string         `json:"luid"`
	Name           string         `json:"name,omitempty"`
	DefinitionLUID string         `json:"definition_luid"`
	IsDefault      bool           `json:"is_default"`
	Specification  map[string]any `json:"specification,omitempty"`
}
type Page struct {
	Metrics       []Metric
	NextPageToken string
	RequestID     string
}
type OutputPage struct {
	Returned   int    `json:"returned"`
	Limit      int    `json:"limit"`
	NextCursor string `json:"next_cursor,omitempty"`
}
type Output struct {
	Status         string
	Environment    string
	Site           string
	DefinitionLUID string
	Page           OutputPage
	Metrics        []Metric
	RequestID      string
	Help           []string
	Source         *readsource.Metadata
}
type CompactMetric struct {
	LUID      string `json:"luid"`
	Name      string `json:"name,omitempty"`
	IsDefault bool   `json:"is_default"`
}
type CompactResult struct {
	Status         string               `json:"status"`
	Environment    string               `json:"environment,omitempty"`
	Site           string               `json:"site,omitempty"`
	DefinitionLUID string               `json:"definition_luid"`
	Page           OutputPage           `json:"page"`
	Metrics        []CompactMetric      `json:"metrics"`
	Details        string               `json:"details"`
	Help           []string             `json:"help"`
	Source         *readsource.Metadata `json:"source,omitempty"`
}
type FullResult struct {
	Status         string               `json:"status"`
	Environment    string               `json:"environment,omitempty"`
	Site           string               `json:"site,omitempty"`
	DefinitionLUID string               `json:"definition_luid"`
	Page           OutputPage           `json:"page"`
	Metrics        []Metric             `json:"metrics"`
	RequestID      string               `json:"tableau_request_id,omitempty"`
	Help           []string             `json:"help"`
	Source         *readsource.Metadata `json:"source,omitempty"`
}

func (o Output) CompactOutput() any {
	items := make([]CompactMetric, len(o.Metrics))
	for i, item := range o.Metrics {
		items[i] = CompactMetric{LUID: item.LUID, Name: item.Name, IsDefault: item.IsDefault}
	}
	return CompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, DefinitionLUID: o.DefinitionLUID, Page: o.Page, Metrics: items, Details: "--full", Help: o.Help, Source: o.Source}
}
func (o Output) FullOutput() any {
	return FullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, DefinitionLUID: o.DefinitionLUID, Page: o.Page, Metrics: o.Metrics, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}
