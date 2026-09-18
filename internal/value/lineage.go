package value

// LineageNode preserves distinct Metadata and optional REST identities.
type LineageNode struct {
	MetadataID string `json:"metadata_id"`
	Kind       string `json:"kind"`
	RESTLUID   string `json:"rest_luid,omitempty"`
	Name       string `json:"name,omitempty"`
}

// LineageEdge is one factual directed relationship between Metadata identities.
type LineageEdge struct {
	FromMetadataID string `json:"from_metadata_id"`
	ToMetadataID   string `json:"to_metadata_id"`
	Relationship   string `json:"relationship"`
}

// LineageFailure records bounded, sanitized provider failure context.
type LineageFailure struct {
	Provider     string `json:"provider"`
	Relation     string `json:"relation,omitempty"`
	RootKind     string `json:"root_kind"`
	RootRESTLUID string `json:"root_rest_luid"`
	RequestID    string `json:"request_id,omitempty"`
}
