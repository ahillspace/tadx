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
