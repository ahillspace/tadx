package value

import "strings"

// WorkbookSchema keeps each normalized field group with its original metadata identity.
type WorkbookSchema struct {
	Metadata MetadataWorkbookDatasources
	Fields   [][]SchemaField
}

// Matches applies the common schema text, role, and logical-table filters.
func (f SchemaField) Matches(query, role, table string) bool {
	if role != "" && !strings.EqualFold(strings.TrimSpace(f.Role), role) {
		return false
	}
	if table != "" && !strings.EqualFold(f.Table, table) {
		return false
	}
	return query == "" || strings.Contains(strings.ToLower(strings.Join([]string{f.ID, f.Name, f.Caption, f.Label, f.Formula}, "\x00")), strings.ToLower(query))
}

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
