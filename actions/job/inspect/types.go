package inspect

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

// Input selects one exact Tableau job for inspection.
type Input struct {
	Environment string
	Site        string
	ID          string
	OperationID string
}

// OperationItem is one bounded local publication outcome. Result is the
// decoded saved item projection when the worker supplied one.
type OperationItem struct {
	Key          string `json:"key,omitempty"`
	Status       string `json:"status,omitempty"`
	JobID        string `json:"tableau_job_id,omitempty"`
	ResourceID   string `json:"resource_id,omitempty"`
	Name         string `json:"name,omitempty"`
	Project      string `json:"project,omitempty"`
	ReceiptPath  string `json:"receipt_path,omitempty"`
	Error        string `json:"error,omitempty"`
	Verification string `json:"verification,omitempty"`
	Result       any    `json:"result,omitempty"`
}

// OperationView is the bounded local view of one detached operation run.
// Snapshot is selected from the saved compact or full result by the runtime.
type OperationView struct {
	ID           string          `json:"operation_id"`
	Operation    string          `json:"operation,omitempty"`
	CheckStatus  string          `json:"check_status,omitempty"`
	Status       string          `json:"status,omitempty"`
	Phase        string          `json:"phase"`
	Alive        bool            `json:"alive"`
	Detached     bool            `json:"detached,omitzero"`
	Activity     string          `json:"activity,omitempty"`
	Environment  string          `json:"environment,omitempty"`
	Site         string          `json:"site,omitempty"`
	RequestedAt  time.Time       `json:"requested_at,omitzero"`
	StartedAt    time.Time       `json:"started_at,omitzero"`
	FinishedAt   time.Time       `json:"finished_at,omitzero"`
	WorkerPID    int             `json:"worker_pid,omitzero"`
	ExitCode     *int            `json:"exit_code,omitzero"`
	Snapshot     any             `json:"snapshot,omitempty"`
	FullSnapshot any             `json:"-"`
	LiveResults  json.RawMessage `json:"live_results,omitempty"`
	ReceiptPaths []string        `json:"receipt_paths,omitempty"`
	Items        []OperationItem `json:"items,omitempty"`
	Warnings     []string        `json:"warnings,omitempty"`
	Details      []string        `json:"details,omitempty"`
}

// Result is the source-facing inspection result.
type Result struct {
	Status      value.JobStatus
	Environment string
	Site        string
	Attempts    int
	Warnings    []string
	Operation   *OperationView
}

// Output is the stable job.inspect document.
type Output struct {
	Status      string          `json:"status"`
	Environment string          `json:"environment"`
	Site        string          `json:"site"`
	Job         value.JobStatus `json:"job"`
	Operation   *OperationView  `json:"operation,omitempty"`
	Attempts    int             `json:"attempts,omitempty"`
	Warnings    []string        `json:"warnings,omitempty"`
	Help        []string        `json:"help"`
}

// ValidateInput checks exact identity.
func ValidateInput(input Input) error {
	id := strings.TrimSpace(input.ID)
	operationID := strings.TrimSpace(input.OperationID)
	if id == "" && operationID == "" {
		return &errs.Error{ID: "job.inspect.usage", Kind: errs.KindUsage, Operation: "job.inspect", Summary: "Job inspection requires one exact Tableau job ID or operation ID.", Retryable: errs.Bool(false), CorrectiveAction: "Provide exactly one of --id or --operation-id."}
	}
	if id != "" && operationID != "" {
		return &errs.Error{ID: "job.inspect.selector", Kind: errs.KindUsage, Operation: "job.inspect", Summary: "Choose an exact Tableau job ID or operation ID, not both.", Retryable: errs.Bool(false), CorrectiveAction: "Provide only --id or --operation-id."}
	}
	if input.ID != id || input.OperationID != operationID {
		return &errs.Error{ID: "job.inspect.usage", Kind: errs.KindUsage, Operation: "job.inspect", Summary: "The exact inspection ID must not contain surrounding whitespace.", Retryable: errs.Bool(false), CorrectiveAction: "Provide one exact ID without surrounding whitespace."}
	}
	if strings.ContainsAny(id+operationID, "\r\n") {
		return &errs.Error{ID: "job.inspect.usage", Kind: errs.KindUsage, Operation: "job.inspect", Summary: "The exact inspection ID must be a single line.", Retryable: errs.Bool(false), CorrectiveAction: "Provide one exact ID on a single line."}
	}
	return nil
}
