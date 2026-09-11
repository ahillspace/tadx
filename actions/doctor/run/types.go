package run

// Status is one stable doctor check state.
type Status string

const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

// Input optionally narrows doctor checks to logical environment and workspace names.
type Input struct {
	Environment string
	Workspace   string
}

// Scope contains only logical names and never machine paths.
type Scope struct {
	Environment string `json:"environment,omitempty"`
	Workspace   string `json:"workspace,omitempty"`
}

// ConfigurationState is the non-secret configuration observation.
type ConfigurationState struct {
	Present             bool
	Valid               bool
	EnvironmentResolved bool
}

// PATState reports reference and variable presence without names or values.
type PATState struct {
	ReferencesConfigured    bool
	NameVariablePresent     bool
	SecretVariablePresent   bool
	StoredCredentialPresent bool
}

// ConnectivityState reports the result of a read-only PAT authentication probe.
type ConnectivityState struct {
	Reachable     bool
	Authenticated bool
}

// CacheState reports bounded local cache health.
type CacheState struct {
	Present  bool
	Complete bool
	Stale    bool
}

// WorkspaceState reports bounded logical workspace health.
type WorkspaceState struct {
	Selected       bool
	Available      bool
	ManifestValid  bool
	DirtyArtifacts int
}

// LoggingState reports logging configuration without a log path.
type LoggingState struct {
	Enabled bool
	Valid   bool
}

// Check is one bounded full doctor result.
type Check struct {
	ID               string `json:"id"`
	Status           Status `json:"status"`
	Summary          string `json:"summary"`
	CorrectiveAction string `json:"corrective_action"`
}

// Counts summarizes the fixed check set.
type Counts struct {
	Pass int `json:"pass"`
	Warn int `json:"warn"`
	Fail int `json:"fail"`
}

// Output retains the complete bounded doctor result.
type Output struct {
	Status  Status   `json:"status"`
	Scope   Scope    `json:"scope"`
	Counts  Counts   `json:"counts"`
	Summary string   `json:"summary"`
	Checks  []Check  `json:"checks"`
	Help    []string `json:"help"`
}

// CompactCheck omits corrective actions available through --full.
type CompactCheck struct {
	ID      string `json:"id"`
	Status  Status `json:"status"`
	Summary string `json:"summary"`
}

// CompactResult is the default bounded projection.
type CompactResult struct {
	Status  Status         `json:"status"`
	Scope   Scope          `json:"scope"`
	Counts  Counts         `json:"counts"`
	Summary string         `json:"summary"`
	Checks  []CompactCheck `json:"checks"`
	Details string         `json:"details"`
	Help    []string       `json:"help"`
}

// FullResult is the expanded bounded projection.
type FullResult = Output

// CompactOutput omits corrective actions while retaining every check.
func (o Output) CompactOutput() any {
	checks := make([]CompactCheck, len(o.Checks))
	for index, check := range o.Checks {
		checks[index] = CompactCheck{ID: check.ID, Status: check.Status, Summary: check.Summary}
	}
	return CompactResult{Status: o.Status, Scope: o.Scope, Counts: o.Counts, Summary: o.Summary, Checks: checks, Details: "--full", Help: o.Help}
}

// FullOutput returns all six checks with corrective actions.
func (o Output) FullOutput() any { return FullResult(o) }
