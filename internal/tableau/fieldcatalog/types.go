// Package fieldcatalog reads and normalizes published datasource schema metadata.
package fieldcatalog

// Schema is one complete provider schema snapshot.
type Schema struct {
	DatasourceLUID string
	DatasourceName string
	Tables         []Table
	Fields         []Field
	Warnings       []string
	RequestID      string
}

// Table is one logical table reported by Tableau.
type Table struct {
	ID         string
	Name       string
	FieldCount int
}

// Field is one normalized Tableau field.
type Field struct {
	ID                      string
	Name                    string
	Caption                 string
	Label                   string
	Role                    string
	DataType                string
	TimeType                string
	Table                   string
	LogicalTableID          string
	DefaultAggregation      string
	Formula                 string
	RequiresUserAggregation bool
	Excluded                bool
	ExclusionReason         string
	Provenance              string
}

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
