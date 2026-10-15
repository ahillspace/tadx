package value

import (
	"slices"
	"strings"
)

// SchemaCompactField contains the shared field-selection contract.
type SchemaCompactField struct {
	Metadata                *FieldDescription `json:"metadata,omitempty"`
	MetadataMatch           string            `json:"metadata_match,omitempty"`
	ID                      string            `json:"id"`
	Caption                 string            `json:"caption"`
	Table                   string            `json:"table,omitempty"`
	Role                    string            `json:"role"`
	DataType                string            `json:"data_type"`
	DefaultAggregation      string            `json:"default_aggregation,omitempty"`
	RequiresUserAggregation bool              `json:"requires_user_aggregation"`
}

// CompactSchemaFields projects bounded fields without expanded semantic details.
func CompactSchemaFields(fields []SchemaField) []SchemaCompactField {
	out := make([]SchemaCompactField, len(fields))
	for i, field := range fields {
		out[i] = SchemaCompactField{ID: field.ID, Caption: field.Caption, Table: field.Table, Role: field.Role, DataType: field.DataType, DefaultAggregation: field.DefaultAggregation, RequiresUserAggregation: field.RequiresUserAggregation, Metadata: field.Metadata, MetadataMatch: field.MetadataMatch}
		if field.Metadata != nil {
			metadata := *field.Metadata
			metadata.Role, metadata.DataType, metadata.Aggregation, metadata.Formula, metadata.FolderName = "", "", "", "", ""
			metadata.Hidden = nil
			out[i].Metadata = &metadata
		}
	}
	return out
}

// ProjectMetadata suppresses unrequested descriptions and tags without changing the observation.
// Workbook field identity can remain visible independently of those optional facts.
func (f SchemaField) ProjectMetadata(descriptions, tags, retainIdentity bool) SchemaField {
	if !descriptions && !tags && !retainIdentity {
		f.Metadata, f.MetadataMatch = nil, ""
		return f
	}
	if f.Metadata == nil {
		return f
	}
	metadata := *f.Metadata
	metadata.UpstreamColumns = slices.Clone(metadata.UpstreamColumns)
	if !descriptions {
		metadata.Description, metadata.Inherited = nil, nil
		metadata.InheritedObserved = false
	}
	for i := range metadata.UpstreamColumns {
		if !descriptions {
			metadata.UpstreamColumns[i].Description = nil
		}
		if !tags {
			metadata.UpstreamColumns[i].Tags = nil
			metadata.UpstreamColumns[i].TagsObserved = false
		}
	}
	f.Metadata = &metadata
	return f
}

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
