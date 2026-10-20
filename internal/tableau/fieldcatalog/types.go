// Package fieldcatalog reads and normalizes published datasource schema metadata.
package fieldcatalog

import sharedvalue "github.com/ahillspace/tadx/internal/value"

// Schema is one complete provider schema snapshot.
type Schema struct {
	DatasourceLUID string
	DatasourceName string
	Tables         []Table
	Fields         []Field
	// DependencyFields retains hidden dependency semantics before display hiding.
	// GraphQL omits these fields from Fields; VDS retains display-hidden placeholders
	// in Fields, whose dependency semantics are supplied by this inventory instead.
	DependencyFields []Field
	Warnings         []string
	RequestID        string
}

// Table is one logical table reported by Tableau.
type Table = sharedvalue.SchemaTable

// Field is one normalized Tableau field.
type Field = sharedvalue.SchemaField

type rawField struct {
	Name                    string
	Caption                 string
	DataType                string
	PhysicalType            string
	ColumnClass             string
	DefaultAggregation      string
	Formula                 string
	Role                    string
	LogicalTableID          string
	Hidden                  bool
	Internal                bool
	ExclusionHint           string
	RequiresUserAggregation bool
	Provenance              string
}
