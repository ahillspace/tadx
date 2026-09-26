// Package definition owns Pulse definition observations and explicit lifecycle operations.
package definition

// Definition holds observed identity and configuration facts.
// Inspection decodes Configuration, while collision checks and deletion need only identity.
// JSON retains the inspect projection; list, pull, and delete keep separate output contracts.
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
