package schema

import (
	"github.com/ahillspace/tadx/internal/readsource"
	"github.com/ahillspace/tadx/internal/value"
)

const (
	defaultLimit = 20
	maxLimit     = 10000
	maxAllFields = 10000
)

// Input selects one published datasource schema and a bounded field view.
type Input struct {
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
	Catalog        bool
	All            bool
}

// Table is one logical datasource table.
type Table = value.SchemaTable

// Field is one normalized datasource field.
type Field = value.SchemaField

// Schema is one complete normalized datasource schema before bounded projection.
type Schema struct {
	DatasourceLUID string
	DatasourceName string
	Tables         []Table
	Fields         []Field
	Warnings       []string
	ObservedAt     string
	RequestID      string
}

// Page describes the bounded field projection.
type Page struct {
	Returned      int    `json:"returned"`
	Total         int    `json:"total"`
	Limit         int    `json:"limit"`
	NextCursor    string `json:"-"`
	MoreAvailable bool   `json:"more_available"`
}

// Output retains the complete bounded result before compact or full rendering.
type Output struct {
	Status         string
	Environment    string
	Site           string
	DatasourceLUID string
	DatasourceName string
	Tables         []Table
	Page           Page
	Fields         []Field
	Warnings       []string
	Source         *readsource.Metadata
	RequestID      string
	Help           []string
}

// CompactField contains everything needed to select a field for authoring.
type CompactField struct {
	ID                      string `json:"id"`
	Caption                 string `json:"caption"`
	Table                   string `json:"table,omitempty"`
	Role                    string `json:"role"`
	DataType                string `json:"data_type"`
	DefaultAggregation      string `json:"default_aggregation,omitempty"`
	RequiresUserAggregation bool   `json:"requires_user_aggregation"`
}

// CompactResult is the default bounded output.
type CompactResult struct {
	Status         string               `json:"status"`
	Environment    string               `json:"environment,omitempty"`
	Site           string               `json:"site,omitempty"`
	DatasourceLUID string               `json:"datasource_luid"`
	DatasourceName string               `json:"datasource_name"`
	Source         *readsource.Metadata `json:"source,omitempty"`
	Tables         []Table              `json:"tables"`
	Page           Page                 `json:"page"`
	Fields         []CompactField       `json:"fields"`
	Warnings       []string             `json:"warnings,omitempty"`
	Details        string               `json:"details"`
	Help           []string             `json:"help,omitempty"`
}

// FullResult is the expanded bounded output.
type FullResult struct {
	Status         string               `json:"status"`
	Environment    string               `json:"environment,omitempty"`
	Site           string               `json:"site,omitempty"`
	DatasourceLUID string               `json:"datasource_luid"`
	DatasourceName string               `json:"datasource_name"`
	Source         *readsource.Metadata `json:"source,omitempty"`
	Tables         []Table              `json:"tables"`
	Page           Page                 `json:"page"`
	Fields         []Field              `json:"fields"`
	Warnings       []string             `json:"warnings,omitempty"`
	RequestID      string               `json:"tableau_request_id,omitempty"`
	Help           []string             `json:"help,omitempty"`
}

// CompactOutput returns bounded field-selection details.
func (o Output) CompactOutput() any {
	fields := make([]CompactField, len(o.Fields))
	for index, field := range o.Fields {
		fields[index] = CompactField{ID: field.ID, Caption: field.Caption, Table: field.Table, Role: field.Role, DataType: field.DataType, DefaultAggregation: field.DefaultAggregation, RequiresUserAggregation: field.RequiresUserAggregation}
	}
	return CompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, DatasourceLUID: o.DatasourceLUID, DatasourceName: o.DatasourceName, Source: o.Source, Tables: append([]Table(nil), o.Tables...), Page: o.Page, Fields: fields, Warnings: append([]string(nil), o.Warnings...), Details: "--full", Help: append([]string(nil), o.Help...)}
}

// FullOutput returns bounded field metadata.
func (o Output) FullOutput() any {
	return FullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, DatasourceLUID: o.DatasourceLUID, DatasourceName: o.DatasourceName, Source: o.Source, Tables: append([]Table(nil), o.Tables...), Page: o.Page, Fields: append([]Field(nil), o.Fields...), Warnings: append([]string(nil), o.Warnings...), RequestID: o.RequestID, Help: append([]string(nil), o.Help...)}
}
