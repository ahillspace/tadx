package inspect

import "github.com/ahillspace/tadx/internal/readsource"

// Input selects one authoritative Pulse definition.
type Input struct {
	Environment string
	Site        string
	LUID        string
	Cache       bool
}

// Definition is one complete normalized Pulse definition.
type Definition struct {
	LUID                 string         `json:"luid"`
	Name                 string         `json:"name"`
	Description          string         `json:"description,omitempty"`
	DatasourceLUID       string         `json:"datasource_luid"`
	MeasureField         string         `json:"measure_field"`
	Aggregation          string         `json:"aggregation"`
	TimeDimension        string         `json:"time_dimension"`
	RunningTotal         bool           `json:"running_total"`
	Temporality          string         `json:"temporality,omitempty"`
	AllowedDimensions    []string       `json:"allowed_dimensions,omitempty"`
	AllowedGranularities []string       `json:"allowed_granularities,omitempty"`
	Configuration        map[string]any `json:"configuration,omitempty"`
	RequestID            string         `json:"-"`
}

// Output retains complete details before projection.
type Output struct {
	Status      string
	Environment string
	Site        string
	Definition  Definition
	RequestID   string
	Help        []string
	Source      *readsource.Metadata
}

// CompactDefinition identifies the exact Pulse definition.
type CompactDefinition struct {
	LUID                 string   `json:"luid"`
	Name                 string   `json:"name"`
	DatasourceLUID       string   `json:"datasource_luid"`
	MeasureField         string   `json:"measure_field"`
	Aggregation          string   `json:"aggregation"`
	TimeDimension        string   `json:"time_dimension"`
	RunningTotal         bool     `json:"running_total"`
	Temporality          string   `json:"temporality,omitempty"`
	AllowedDimensions    []string `json:"allowed_dimensions,omitempty"`
	DimensionsOmitted    int      `json:"dimensions_omitted,omitempty"`
	AllowedGranularities []string `json:"allowed_granularities,omitempty"`
	GranularitiesOmitted int      `json:"granularities_omitted,omitempty"`
}

// CompactResult is the default projection.
type CompactResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Definition  CompactDefinition    `json:"definition"`
	Details     string               `json:"details"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// FullResult is the expanded bounded projection.
type FullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Definition  Definition           `json:"definition"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// CompactOutput returns stable identity fields.
func (o Output) CompactOutput() any {
	return CompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Definition: compactDefinition(o.Definition), Details: "--full", Help: o.Help, Source: o.Source}
}

func compactDefinition(definition Definition) CompactDefinition {
	const limit = 50
	dimensions := append([]string(nil), definition.AllowedDimensions...)
	granularities := append([]string(nil), definition.AllowedGranularities...)
	result := CompactDefinition{
		LUID: definition.LUID, Name: definition.Name, DatasourceLUID: definition.DatasourceLUID,
		MeasureField: definition.MeasureField, Aggregation: definition.Aggregation, TimeDimension: definition.TimeDimension,
		RunningTotal: definition.RunningTotal, Temporality: definition.Temporality,
		DimensionsOmitted: max(0, len(dimensions)-limit), GranularitiesOmitted: max(0, len(granularities)-limit),
	}
	result.AllowedDimensions = dimensions[:min(limit, len(dimensions))]
	result.AllowedGranularities = granularities[:min(limit, len(granularities))]
	return result
}

// FullOutput returns the normalized released configuration.
func (o Output) FullOutput() any {
	item := o.Definition
	item.AllowedDimensions = append([]string(nil), item.AllowedDimensions...)
	item.AllowedGranularities = append([]string(nil), item.AllowedGranularities...)
	return FullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Definition: item, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}
