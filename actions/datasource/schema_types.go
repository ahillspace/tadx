package datasource

import (
	"github.com/ahillspace/tadx/internal/readsource"
	"github.com/ahillspace/tadx/internal/value"
)

const (
	schemaDefaultLimit = 20
	schemaMaxLimit     = 10000
	schemaMaxAllFields = 10000
)

// SchemaInput selects one published datasource schema and a bounded field view.
type SchemaInput struct {
	Descriptions   bool
	Tags           bool
	Environment    string
	Site           string
	DatasourceLUID string
	Query          string
	Role           string
	Table          string
	FieldID        string
	FieldIDs       []string
	Limit          int
	Cursor         string
	Cache          bool
	All            bool
}

// Table is one logical datasource table.
type Table = value.SchemaTable

// Field is one normalized datasource field.
type Field = value.SchemaField

// SchemaRecord is one complete normalized datasource schema before bounded projection.
type SchemaRecord struct {
	DescriptionsObserved bool
	TagsObserved         bool
	DatasourceLUID       string
	DatasourceName       string
	Tables               []Table
	Fields               []Field
	Warnings             []string
	ObservedAt           string
	RequestID            string
}

// SchemaPage describes the bounded field projection.
type SchemaPage struct {
	Returned      int    `json:"returned"`
	Total         int    `json:"total"`
	Limit         int    `json:"limit"`
	NextCursor    string `json:"-"`
	MoreAvailable bool   `json:"more_available"`
}

// SchemaOutput retains the complete bounded result before compact or full rendering.
type SchemaOutput struct {
	Status         string
	Environment    string
	Site           string
	DatasourceLUID string
	DatasourceName string
	Tables         []Table
	Page           SchemaPage
	Fields         []Field
	Warnings       []string
	Source         *readsource.Metadata
	RequestID      string
	Help           []string
}

// SchemaCompactField contains everything needed to select a field for authoring.
type SchemaCompactField struct {
	Metadata                *value.FieldDescription `json:"metadata,omitempty"`
	MetadataMatch           string                  `json:"metadata_match,omitempty"`
	ID                      string                  `json:"id"`
	Caption                 string                  `json:"caption"`
	Table                   string                  `json:"table,omitempty"`
	Role                    string                  `json:"role"`
	DataType                string                  `json:"data_type"`
	DefaultAggregation      string                  `json:"default_aggregation,omitempty"`
	RequiresUserAggregation bool                    `json:"requires_user_aggregation"`
}

// SchemaCompactResult is the default bounded output.
type SchemaCompactResult struct {
	Status         string               `json:"status"`
	Environment    string               `json:"environment,omitempty"`
	Site           string               `json:"site,omitempty"`
	DatasourceLUID string               `json:"datasource_luid"`
	DatasourceName string               `json:"datasource_name"`
	Source         *readsource.Metadata `json:"source,omitempty"`
	Tables         []Table              `json:"tables"`
	Page           SchemaPage           `json:"page"`
	Fields         []SchemaCompactField `json:"fields"`
	Warnings       []string             `json:"warnings,omitempty"`
	Details        string               `json:"details"`
	Help           []string             `json:"help,omitempty"`
}

// SchemaFullResult is the expanded bounded output.
type SchemaFullResult struct {
	Status         string               `json:"status"`
	Environment    string               `json:"environment,omitempty"`
	Site           string               `json:"site,omitempty"`
	DatasourceLUID string               `json:"datasource_luid"`
	DatasourceName string               `json:"datasource_name"`
	Source         *readsource.Metadata `json:"source,omitempty"`
	Tables         []Table              `json:"tables"`
	Page           SchemaPage           `json:"page"`
	Fields         []Field              `json:"fields"`
	Warnings       []string             `json:"warnings,omitempty"`
	RequestID      string               `json:"tableau_request_id,omitempty"`
	Help           []string             `json:"help,omitempty"`
}

// CompactOutput returns bounded field-selection details.
func (o SchemaOutput) CompactOutput() any {
	fields := make([]SchemaCompactField, len(o.Fields))
	for index, field := range o.Fields {
		fields[index] = SchemaCompactField{ID: field.ID, Caption: field.Caption, Table: field.Table, Role: field.Role, DataType: field.DataType, DefaultAggregation: field.DefaultAggregation, RequiresUserAggregation: field.RequiresUserAggregation, Metadata: field.Metadata, MetadataMatch: field.MetadataMatch}
	}
	return SchemaCompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, DatasourceLUID: o.DatasourceLUID, DatasourceName: o.DatasourceName, Source: o.Source, Tables: append([]Table(nil), o.Tables...), Page: o.Page, Fields: fields, Warnings: append([]string(nil), o.Warnings...), Details: "--full", Help: append([]string(nil), o.Help...)}
}

// FullOutput returns bounded field metadata.
func (o SchemaOutput) FullOutput() any {
	return SchemaFullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, DatasourceLUID: o.DatasourceLUID, DatasourceName: o.DatasourceName, Source: o.Source, Tables: append([]Table(nil), o.Tables...), Page: o.Page, Fields: append([]Field(nil), o.Fields...), Warnings: append([]string(nil), o.Warnings...), RequestID: o.RequestID, Help: append([]string(nil), o.Help...)}
}
