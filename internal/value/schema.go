package value

// SchemaTable is one normalized logical datasource table.
type SchemaTable struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	FieldCount int    `json:"field_count"`
}

// SchemaField is one normalized datasource field, not a provider response payload.
type SchemaField struct {
	Metadata                *FieldDescription `json:"metadata,omitempty"`
	MetadataMatch           string            `json:"metadata_match,omitempty"`
	ID                      string            `json:"id"`
	Name                    string            `json:"name"`
	Caption                 string            `json:"caption"`
	Label                   string            `json:"label"`
	Role                    string            `json:"role"`
	DataType                string            `json:"data_type"`
	TimeType                string            `json:"time_type,omitempty"`
	Table                   string            `json:"table,omitempty"`
	LogicalTableID          string            `json:"logical_table_id,omitempty"`
	DefaultAggregation      string            `json:"default_aggregation,omitempty"`
	Formula                 string            `json:"formula,omitempty"`
	RequiresUserAggregation bool              `json:"requires_user_aggregation"`
	Excluded                bool              `json:"excluded"`
	ExclusionReason         string            `json:"exclusion_reason,omitempty"`
	Provenance              string            `json:"provenance,omitempty"`
}
