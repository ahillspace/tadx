// Package inspect owns exact Tableau job inspection orchestration.
package inspect

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

// Source performs one exact job inspection.
type Source interface {
	Inspect(context.Context, Input) (Result, error)
}

// Action validates and projects job inspection results.
type Action struct{ source Source }

// New creates a job.inspect action.
func New(source Source) *Action { return &Action{source: source} }

// Execute inspects the requested exact job without changing remote state.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if err := ValidateInput(input); err != nil {
		return Output{}, err
	}
	if a == nil || a.source == nil {
		return Output{}, jobError("job.inspect.unconfigured", errs.KindRuntime, input, "Job inspection is not configured.", nil)
	}
	result, err := a.source.Inspect(ctx, input)
	if err != nil {
		if _, ok := errors.AsType[*errs.Error](err); ok {
			return Output{}, err
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Output{}, jobError("job.inspect.cancelled", errs.KindOperation, input, "Job inspection was canceled before an authoritative result was returned.", err)
		}
		return Output{}, jobError("job.inspect.failed", errs.KindOperation, input, "Job inspection failed.", err)
	}
	if input.OperationID != "" {
		if result.Operation == nil || result.Operation.ID != input.OperationID {
			return Output{}, jobError("job.inspect.identity", errs.KindOperation, input, "Operation inspection did not return the requested exact operation identity.", nil)
		}
		status := result.Operation.Status
		if status == "" {
			status = result.Operation.Phase
		}
		environment, site := result.Operation.Environment, result.Operation.Site
		if environment == "" {
			environment = result.Environment
		}
		if site == "" {
			site = result.Site
		}
		return Output{Status: status, Environment: environment, Site: site, Operation: result.Operation, Warnings: append([]string(nil), result.Warnings...), Help: []string{}}, nil
	}
	if result.Status.ID != input.ID || result.Status.Status == "" {
		return Output{}, jobError("job.inspect.identity", errs.KindOperation, input, "Job inspection did not return the requested exact identity and state.", nil)
	}
	environment, site := result.Environment, result.Site
	if environment == "" {
		environment = input.Environment
	}
	if site == "" {
		site = input.Site
	}
	return Output{
		Status:      result.Status.Status,
		Environment: environment,
		Site:        site,
		Job:         result.Status,
		Attempts:    result.Attempts,
		Warnings:    append([]string(nil), result.Warnings...),
		Help:        []string{},
	}, nil
}

type renderedOutput struct {
	Status      string           `json:"status"`
	Environment string           `json:"environment,omitempty"`
	Site        string           `json:"site,omitempty"`
	Job         *value.JobStatus `json:"job,omitempty"`
	Operation   any              `json:"operation,omitempty"`
	Attempts    int              `json:"attempts,omitempty"`
	Warnings    []string         `json:"warnings,omitempty"`
	Help        []string         `json:"help"`
}

// CompactOutput projects the saved compact operation snapshot and omits
// worker diagnostics that are not needed to decide the next action.
func (o Output) CompactOutput() any { return o.render(false) }

// FullOutput retains the bounded diagnostic snapshot and its original details.
func (o Output) FullOutput() any { return o.render(true) }

func (o Output) render(full bool) any {
	projected := renderedOutput{Status: o.Status, Environment: o.Environment, Site: o.Site, Attempts: o.Attempts, Warnings: append([]string(nil), o.Warnings...), Help: append([]string(nil), o.Help...)}
	if o.Job.ID != "" || o.Job.Status != "" {
		job := o.Job
		projected.Job = &job
	}
	if o.Operation != nil {
		snapshot := o.Operation.Snapshot
		if full && o.Operation.FullSnapshot != nil {
			snapshot = o.Operation.FullSnapshot
		}
		result := operationSnapshot(snapshot, *o.Operation, full)
		if object, ok := result.(map[string]any); ok {
			_, sourceContext := object["source"].(map[string]any)
			if o.Environment != "" && !sourceContext {
				object["environment"] = o.Environment
			}
			if o.Site != "" && !sourceContext {
				object["site"] = o.Site
			}
			if len(o.Warnings) > 0 {
				object["warnings"] = o.Warnings
			}
			if o.Operation.Alive && o.Operation.Activity != "" {
				object["activity"] = o.Operation.Activity
			}
			if o.Status == "interrupted" {
				object["status"] = o.Status
				// Acceptance can be saved before a batch item result. Preserve
				// these independently confirmed jobs even if the last aggregate
				// still shows an unfinished item; never infer whole-run success.
				if len(o.Operation.Items) > 0 {
					object["accepted_jobs"] = renderOperationItems(o.Operation.Items, o.Operation.Operation, full)
				}
			}
			if full {
				object["tracking"] = map[string]any{"phase": o.Operation.Phase, "worker_alive": o.Operation.Alive, "requested_at": o.Operation.RequestedAt, "started_at": o.Operation.StartedAt, "worker_finished_at": o.Operation.FinishedAt, "worker_exit_code": o.Operation.ExitCode}
			}
		}
		return result
	}
	return projected
}

func operationSnapshot(snapshot any, operation OperationView, full bool) any {
	result := map[string]any{}
	if value, ok := snapshot.(map[string]any); ok {
		for key, item := range value {
			result[key] = item
		}
	} else if snapshot != nil {
		return snapshot
	}
	if _, ok := result["operation_id"]; !ok {
		result["operation_id"] = operation.ID
	}
	if _, ok := result["operation"]; !ok && operation.Operation != "" {
		result["operation"] = operation.Operation
	}
	if _, ok := result["status"]; !ok && operation.Status != "" {
		result["status"] = operation.Status
	}
	if operation.CheckStatus != "" {
		if _, ok := result["check_status"]; !ok {
			result["check_status"] = operation.CheckStatus
		}
	}
	if _, ok := result["items"]; !ok && len(operation.Items) > 0 {
		if _, hasResult := result["result"]; !hasResult {
			result["items"] = renderOperationItems(operation.Items, operation.Operation, full)
		}
	}
	if !full {
		return compactSnapshot(result)
	}
	return result
}

func renderOperationItems(items []OperationItem, operation string, full bool) []map[string]any {
	projected := make([]map[string]any, len(items))
	for index, item := range items {
		value := map[string]any{}
		if full && item.Key != "" {
			value["key"] = item.Key
		}
		if item.Status != "" {
			value["status"] = item.Status
		}
		if full && item.JobID != "" {
			value["tableau_job_id"] = item.JobID
		}
		if item.ResourceID != "" {
			resourceKey := "resource_id"
			if strings.HasPrefix(operation, "workbook.") {
				resourceKey = "workbook_luid"
			} else if strings.HasPrefix(operation, "datasource.") {
				resourceKey = "datasource_luid"
			} else if strings.HasPrefix(operation, "flow.") {
				resourceKey = "flow_luid"
			}
			value[resourceKey] = item.ResourceID
		}
		if item.Name != "" {
			nameKey := "name"
			if strings.HasPrefix(operation, "workbook.") {
				nameKey = "workbook_name"
			} else if strings.HasPrefix(operation, "datasource.") {
				nameKey = "datasource_name"
			} else if strings.HasPrefix(operation, "flow.") {
				nameKey = "flow_name"
			}
			value[nameKey] = item.Name
		}
		if item.Project != "" {
			value["project_luid"] = item.Project
		}
		if full && item.ReceiptPath != "" {
			value["receipt_path"] = item.ReceiptPath
		}
		if item.Error != "" {
			value["error"] = item.Error
		}
		if item.Verification != "" {
			value["verification"] = item.Verification
		}
		if item.Result != nil {
			value["result"] = item.Result
		}
		projected[index] = value
	}
	return projected
}

func compactSnapshot(snapshot any) any {
	if snapshot == nil {
		return nil
	}
	if value, ok := snapshot.(map[string]any); ok {
		projected := make(map[string]any, len(value))
		for key, item := range value {
			if key == "full_result" || key == "diagnostics" {
				continue
			}
			projected[key] = item
		}
		return projected
	}
	return snapshot
}

func jobError(id string, kind errs.Kind, input Input, summary string, cause error) error {
	resource := input.ID
	if resource == "" {
		resource = input.OperationID
	}
	return &errs.Error{ID: id, Kind: kind, Operation: "job.inspect", Resource: resource, Environment: input.Environment, Site: input.Site, TableauJobID: input.ID, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Retry the exact job inspection or inspect the saved operation record.", Phase: errs.PhaseVerification, Outcome: errs.OutcomeUnknown}
}
