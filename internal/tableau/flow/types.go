// Package flow defines the docs-only Tableau flow client seam.
package flow

import "context"

// Parameter is one bounded flow parameter projection.
type Parameter struct {
	LUID        string
	Name        string
	Type        string
	Description string
	Value       string
	Required    *bool
}

// OutputStep is one direct flow output step.
type OutputStep struct {
	LUID string
	Name string
}

// Flow is one authoritative Tableau flow projection.
type Flow struct {
	LUID             string
	Name             string
	Description      string
	FileType         string
	ProjectLUID      string
	ProjectName      string
	OwnerLUID        string
	CreatedAt        string
	UpdatedAt        string
	Tags             []string
	Parameters       []Parameter
	OutputSteps      []OutputStep
	TableauRequestID string
}

// ListRequest selects one bounded REST page.
type ListRequest struct {
	PageNumber  int
	PageSize    int
	Name        string
	OwnerName   string
	ProjectLUID string
	ProjectName string
}

// Page is one normalized classic REST page.
type Page struct {
	Number           int
	Size             int
	Total            int
	Items            []Flow
	TableauRequestID string
}

// Download is one unchanged TFL or TFLX response.
type Download struct {
	Filename         string
	Content          []byte
	TableauRequestID string
}

// Client is the docs-only read seam implemented after live evidence closes the gate.
type Client interface {
	List(context.Context, ListRequest) (Page, error)
	Get(context.Context, string) (Flow, error)
	Download(context.Context, string) (Download, error)
}

// PublishRequest contains explicit native flow publication choices.
type PublishRequest struct {
	Name, ProjectLUID, Filename, ContentPath, ExpectedFingerprint string
	ContentSize                                                   int64
	Overwrite                                                     bool
}

// PublishResult is one authoritative final flow publication result.
type PublishResult struct {
	Status, FlowLUID, FlowName, ProjectLUID, TableauRequestID string
}

// PreparedPublish separates upload preparation from the final publish mutation.
type PreparedPublish interface {
	Commit(context.Context) (PublishResult, error)
}

// MutationResult is one exact move or delete result.
type MutationResult struct {
	Status, FlowLUID, ProjectLUID, TableauRequestID string
}

// MutationClient is the docs-only flow mutation seam.
type MutationClient interface {
	Prepare(context.Context, PublishRequest) (PreparedPublish, error)
	Move(context.Context, string, string) (MutationResult, error)
	Delete(context.Context, string) (MutationResult, error)
}
