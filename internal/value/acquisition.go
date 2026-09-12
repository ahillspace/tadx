package value

// AcquisitionTarget describes a prospective managed artifact without a receipt.
type AcquisitionTarget struct {
	ResourceKind string `json:"resource_kind,omitempty"`
	Kind         string `json:"kind"`
	LUID         string `json:"luid"`
	Name         string `json:"name"`
	Path         string `json:"path"`
	Exists       bool   `json:"exists"`
	Overwrite    bool   `json:"overwrite"`
}

// AcquisitionPlan records resolved scope and the limits of a read-only preview.
type AcquisitionPlan struct {
	Environment    string              `json:"environment,omitempty"`
	Site           string              `json:"site,omitempty"`
	Status         string              `json:"status"`
	Operation      string              `json:"operation"`
	Workspace      string              `json:"workspace"`
	Target         AcquisitionTarget   `json:"target"`
	Dependencies   []AcquisitionTarget `json:"dependencies,omitempty"`
	IncludeExtract *bool               `json:"include_extract,omitempty"`
	IncludePDS     bool                `json:"include_pds,omitempty"`
	Direction      string              `json:"direction,omitempty"`
	Depth          int                 `json:"depth,omitempty"`
	MetricCount    *int                `json:"metric_count,omitempty"`
	Limitations    []string            `json:"limitations"`
}
