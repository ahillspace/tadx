package job

import (
	"context"
	"fmt"
	"time"

	"github.com/ahillspace/tadx/internal/jobmonitor"
	"github.com/ahillspace/tadx/internal/operationrun"
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/value"
)

const maxOperationReceiptChecks = 100

type RecoverySession struct {
	Server, Site, SiteID string
	Inspect              func(context.Context, string) (value.JobStatus, error)
	ResolveDestination   func(context.Context, value.PublicationDestination) (value.ResourceDestination, error)
}

// RecoveryPorts keeps native access separate from local execution records.
type RecoveryPorts struct {
	OperationStore    func() (operationrun.Store, error)
	ReceiptStore      func() (jobmonitor.Store, error)
	Open              func(context.Context, string) (RecoverySession, error)
	SupportsOperation func(string) bool
	ConfigPath        string
	Now               func() time.Time
}

type recovery struct{ ports RecoveryPorts }

func (r recovery) inspectOperation(ctx context.Context, input InspectInput) (InspectResult, error) {
	store, err := r.ports.OperationStore()
	if err != nil {
		return InspectResult{}, err
	}
	record, err := store.Read(input.OperationID)
	if err != nil {
		return InspectResult{}, err
	}
	if !r.ports.SupportsOperation(record.Operation) {
		return InspectResult{}, fmt.Errorf("operation %q is not a supported native publish or download operation", record.ID)
	}
	alive, err := store.Alive(record.ID)
	if err != nil {
		return InspectResult{}, err
	}
	var recoveryWarnings []string
	if !alive && len(record.ReceiptIntents) != 0 {
		record, recoveryWarnings = r.recoverReceiptLinks(ctx, store, record)
	}
	view := operationView(record, alive)
	if view.Environment == "" || view.Site == "" {
		environment, site := r.savedTarget(record)
		if view.Environment == "" {
			view.Environment = environment
		}
		if view.Site == "" {
			view.Site = site
		}
	}
	if input.Environment != "" && view.Environment != "" && input.Environment != view.Environment {
		return InspectResult{}, fmt.Errorf("operation %q belongs to environment %q, not %q", record.ID, view.Environment, input.Environment)
	}
	if input.Site != "" && view.Environment != "" && input.Site != view.Site {
		return InspectResult{}, fmt.Errorf("operation %q belongs to site %q, not %q", record.ID, view.Site, input.Site)
	}
	result := InspectResult{Environment: view.Environment, Site: view.Site, Operation: view, Warnings: recoveryWarnings}
	if record.Phase == operationrun.PhaseCompleted || record.Phase == operationrun.PhaseFailed {
		warnings, items := r.readSavedReceipts(ctx, record)
		if updated, updateWarnings := r.reconcileReceipts(ctx, record, items); updated != nil {
			record = *updated
			view = operationView(record, alive)
			view.Environment, view.Site = result.Environment, result.Site
			result.Operation = view
			warnings = append(warnings, updateWarnings...)
		} else {
			warnings = append(warnings, updateWarnings...)
		}
		view.Items = items
		result.Warnings = append(result.Warnings, warnings...)
	}
	if (record.Phase == operationrun.PhaseRemotePending || record.Phase == operationrun.PhaseRunning) && !alive {
		warnings, items, environment, site := r.inspectReceipts(ctx, record)
		if view.Environment == "" {
			view.Environment = environment
			result.Environment = environment
		}
		if view.Site == "" {
			view.Site = site
			result.Site = site
		}
		if input.Environment != "" && environment != "" && input.Environment != environment {
			return InspectResult{}, fmt.Errorf("operation %q belongs to environment %q, not %q", record.ID, environment, input.Environment)
		}
		if input.Site != "" && site != "" && input.Site != site {
			return InspectResult{}, fmt.Errorf("operation %q belongs to site %q, not %q", record.ID, site, input.Site)
		}
		if record.Phase == operationrun.PhaseRemotePending {
			if updated, updateWarnings := r.reconcileReceipts(ctx, record, items); updated != nil {
				record = *updated
				view = operationView(record, alive)
				view.Environment, view.Site = result.Environment, result.Site
				result.Operation = view
				warnings = append(warnings, updateWarnings...)
			} else {
				warnings = append(warnings, updateWarnings...)
			}
		}
		view.Items = items
		result.Warnings = append(result.Warnings, warnings...)
	}
	return result, nil
}

// operationView projects durable worker state without inferring remote effects.
func operationView(record operationrun.Record, alive bool) *OperationView {
	status := operationStatus(record.Phase, alive)
	if record.Phase == operationrun.PhaseFailed && output.OperationSnapshotStatus(record.OutputRecord()) == "partial_failure" {
		status = "partial_failure"
	}
	stopped := record.Detached || (record.Phase == operationrun.PhaseRunning && !alive)
	view := &OperationView{
		ID:           record.ID,
		Operation:    record.Operation,
		Status:       status,
		Phase:        string(record.Phase),
		Alive:        alive,
		Activity:     record.Activity,
		RequestedAt:  record.RequestedAt,
		StartedAt:    record.StartedAt,
		FinishedAt:   record.FinishedAt,
		ExitCode:     record.ExitCode,
		Snapshot:     output.OperationSnapshot(record.OutputRecord(), false, stopped),
		FullSnapshot: output.OperationSnapshot(record.OutputRecord(), true, stopped),
	}
	view.Environment, view.Site = output.OperationTarget(record.OutputRecord())
	if record.Phase != operationrun.PhaseCompleted || output.OperationNeedsDestination(record.FullResult) {
		view.CheckStatus = output.OperationCheckCommand(record.OutputRecord())
	}
	return view
}

func operationStatus(phase operationrun.Phase, alive bool) string {
	switch phase {
	case operationrun.PhaseRequested:
		return "starting"
	case operationrun.PhaseRunning:
		if !alive {
			return "interrupted"
		}
		return "running"
	case operationrun.PhaseRemotePending:
		return "pending"
	case operationrun.PhaseCompleted:
		return "succeeded"
	case operationrun.PhaseFailed:
		return "failed"
	default:
		return string(phase)
	}
}

func (r recovery) reconcileReceipts(ctx context.Context, record operationrun.Record, items []OperationItem) (*operationrun.Record, []string) {
	if len(items) == 0 || len(items) != len(record.ReceiptPaths) {
		return nil, nil
	}
	failed := record.Phase == operationrun.PhaseFailed || output.OperationHasFailure(record.FullResult) || output.OperationHasFailure(record.CompactResult)
	mergedFull := output.MergeOperationResult(record.FullResult, items, "succeeded", record.Operation)
	pending := output.OperationHasPending(mergedFull)
	for _, item := range items {
		switch item.Status {
		case "failed", "cancelled":
			failed = true
		case "succeeded":
		case "destination_pending", "pending", "running", "unknown", "":
			pending = true
		default:
			pending = true
		}
	}
	if pending {
		status := "pending"
		if failed {
			status = "partial_failure"
		}
		updated, err := r.ports.OperationStore()
		if err != nil {
			return nil, []string{fmt.Sprintf("operation state could not be reopened for reconciliation: %v", err)}
		}
		result, err := updated.Update(record.ID, func(current *operationrun.Record) error {
			current.CompactResult = output.MergeOperationResult(current.CompactResult, items, status, current.Operation)
			current.FullResult = output.MergeOperationResult(current.FullResult, items, status, current.Operation)
			return nil
		})
		if err != nil {
			return nil, []string{fmt.Sprintf("partial receipt states could not be saved to operation %q: %v", record.ID, err)}
		}
		return &result, nil
	}
	nextPhase := operationrun.PhaseCompleted
	nextStatus := "succeeded"
	if failed {
		nextPhase = operationrun.PhaseFailed
		nextStatus = "failed"
	}
	updated, err := r.ports.OperationStore()
	if err != nil {
		return nil, []string{fmt.Sprintf("operation state could not be reopened for reconciliation: %v", err)}
	}
	result, err := updated.Update(record.ID, func(current *operationrun.Record) error {
		current.Phase = nextPhase
		if current.FinishedAt.IsZero() {
			finishedAt := time.Now().UTC()
			if r.ports.Now != nil {
				finishedAt = r.ports.Now().UTC()
			}
			current.FinishedAt = finishedAt
		}
		current.CompactResult = output.MergeOperationResult(current.CompactResult, items, nextStatus, current.Operation)
		current.FullResult = output.MergeOperationResult(current.FullResult, items, nextStatus, current.Operation)
		if current.ExitCode == nil {
			code := 0
			if failed {
				code = 1
			}
			current.ExitCode = &code
		}
		return nil
	})
	if err != nil {
		return nil, []string{fmt.Sprintf("confirmed receipt states could not be saved to operation %q: %v", record.ID, err)}
	}
	return &result, nil
}
