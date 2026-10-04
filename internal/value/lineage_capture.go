package value

// LineageCaptureRequest identifies one exact REST root for bounded metadata capture.
type LineageCaptureRequest struct {
	Kind      string
	RESTLUID  string
	Direction string
	Depth     int
}

// LineageCaptureGraph retains Metadata and REST identities without an action owner.
type LineageCaptureGraph struct {
	RootRESTLUID   string
	RootMetadataID string
	Direction      string
	Depth          int
	Complete       bool
	Failure        *LineageFailure
	Nodes          []LineageNode
	Edges          []LineageEdge
	Warnings       []string
	RequestIDs     []string
}
