// Package metadataassets provides bounded upstream metadata REST and GraphQL reads and writes.
package metadataassets

import "github.com/ahillspace/tadx/internal/value"

// Page is one bounded observation. Complete concerns the selected scope only.
type Page[T any] = value.MetadataPage[T]

// Query selects an exact identity or a bounded discovery page.
// Text is supported upstream for databases and tables, not columns.
type Query = value.MetadataQuery

// Update changes only supplied properties. Empty contact clearing is not verified.
type Update = value.MetadataUpdate

type LabelTarget = value.LabelTarget
type LabelUpdate = value.LabelUpdate

type DatasourceUpstream struct {
	LUID             string
	Databases        []value.MetadataDatabase
	Tables           []value.MetadataTable
	Complete         bool
	ObservedAt       string
	TableauRequestID string
}

type DatasourceDescriptions = value.MetadataDatasourceDescriptions

// ConstraintError reports a known unsupported or unverified request without a write.
type ConstraintError struct{ Reason string }

func (e *ConstraintError) Error() string { return e.Reason }

// MutationEvidence retains known identity after a successful HTTP write with invalid read-back.
type MutationEvidence struct {
	LUID             string
	Name             string
	TableauRequestID string
	Outcome          string
}
