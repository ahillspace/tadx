package get

import "github.com/ahillspace/tadx/internal/readsource"

type Input struct {
	Environment string
	Site        string
	LUID        string
	Catalog     bool
}
type Metric struct {
	LUID           string         `json:"luid"`
	Name           string         `json:"name,omitempty"`
	DefinitionLUID string         `json:"definition_luid"`
	SiteLUID       string         `json:"site_luid,omitempty"`
	IsDefault      bool           `json:"is_default"`
	Specification  map[string]any `json:"specification"`
	Configuration  []byte         `json:"-"`
	RequestID      string         `json:"-"`
}
type Output struct {
	Status      string
	Environment string
	Site        string
	Metric      Metric
	RequestID   string
	Help        []string
	Source      *readsource.Metadata
}
type CompactMetric struct {
	LUID           string `json:"luid"`
	Name           string `json:"name,omitempty"`
	DefinitionLUID string `json:"definition_luid"`
	IsDefault      bool   `json:"is_default"`
}
type CompactResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Metric      CompactMetric        `json:"metric"`
	Details     string               `json:"details"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}
type FullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Metric      Metric               `json:"metric"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

func (o Output) CompactOutput() any {
	return CompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Metric: CompactMetric{LUID: o.Metric.LUID, Name: o.Metric.Name, DefinitionLUID: o.Metric.DefinitionLUID, IsDefault: o.Metric.IsDefault}, Details: "--full", Help: o.Help, Source: o.Source}
}
func (o Output) FullOutput() any {
	return FullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Metric: o.Metric, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}
