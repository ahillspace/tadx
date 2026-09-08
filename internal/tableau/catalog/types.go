package catalog

import (
	"context"
	"net/url"
)

// Scope identifies one fixed catalog table and collector.
type Scope string

const (
	ScopeUsers       Scope = "users"
	ScopeGroups      Scope = "groups"
	ScopeProjects    Scope = "projects"
	ScopeWorkbooks   Scope = "workbooks"
	ScopeDatasources Scope = "datasources"
	ScopeFlows       Scope = "flows"
	ScopeViews       Scope = "views"
	ScopePermissions Scope = "permissions"
)

// ColumnType describes one fixed catalog column's storage type.
type ColumnType string

const (
	ColumnText      ColumnType = "text"
	ColumnInteger   ColumnType = "integer"
	ColumnTimestamp ColumnType = "timestamp"
	ColumnBoolean   ColumnType = "boolean"
)

// Column is one fixed positional batch column.
type Column struct {
	Name string
	Type ColumnType
}

// Request is one authenticated, site-relative Tableau GET request.
// The injected executor adds the server, API version, site, and session.
type Request struct {
	Scope            Scope
	Path             string
	Query            url.Values
	Operation        string
	PageNumber       int
	PageSize         int
	ItemID           string
	MaxResponseBytes int64
}

// Response is the bounded successful response returned by an Executor.
type Response struct {
	StatusCode       int
	Body             []byte
	TableauRequestID string
}

// Executor performs one request through TADX authentication and transport.
// It never exposes credentials or session values to this package.
type Executor interface {
	Do(context.Context, Request) (Response, error)
}

// Batch is one bounded positional row batch for a fixed catalog scope.
type Batch struct {
	Scope            Scope
	Columns          []Column
	Rows             [][]any
	TableauRequestID string
}

// BatchWriter accepts serialized batches. It must not render catalog rows.
type BatchWriter interface {
	WriteBatch(context.Context, Batch) error
}

// Config controls bounded collection and adaptive concurrency.
type Config struct {
	PageSize           int
	InitialConcurrency int
	MaxConcurrency     int
	MaxRetries         int
	RequestQueueSize   int
	BatchQueueSize     int
	MaxBatchRows       int
	MaxResponseBytes   int64
}

// RunRequest selects public scopes. An empty selection means inventory scopes;
// permissions requires an explicit selection.
type RunRequest struct {
	RequestedScopes []Scope
}

// ScopePlan separates public requests from required dependency collection.
type ScopePlan struct {
	Requested []Scope
	Implicit  []Scope
	Collected []Scope
}

// Result contains bounded operational metadata and never contains catalog rows.
type Result struct {
	RequestedScopes   []Scope
	ImplicitScopes    []Scope
	Counts            map[Scope]int64
	Requests          int64
	TableauRequestIDs []string
	FinalConcurrency  int
	SkippedRows       map[Scope]int
	DeniedPermissions int
}
