package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ahillspace/tadx/internal/cli"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/ahillspace/tadx/internal/cli/progress"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/contentbatch"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/operationrun"
	"github.com/ahillspace/tadx/internal/output"
	"github.com/spf13/cobra"
)

const publicationWaitLimit = 20 * time.Minute

func nativePublication(operation string) bool {
	return operation == "workbook.publish" || operation == "datasource.publish" || operation == "flow.publish"
}

func nativeDownload(operation string) bool {
	return operation == "workbook.pull" || operation == "datasource.pull" || operation == "flow.pull"
}

func nativeLongOperation(operation string) bool {
	return nativePublication(operation) || nativeDownload(operation)
}

func operationActivity(operation string) string {
	kind, _, _ := strings.Cut(operation, ".")
	if nativeDownload(operation) {
		return "Downloading " + kind
	}
	return "Publishing " + kind
}

func publicationOperationStore(directory string) (operationrun.Store, error) {
	if directory == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return operationrun.Store{}, err
		}
		directory = filepath.Join(cache, "tadx", "operations")
	}
	directory, err := filepath.Abs(directory)
	return operationrun.Store{Directory: directory}, err
}

// bindPublicationExecution wraps only native publishing and download actions.
// Cobra argument guards run before handoff; the worker executes the original
// action with the same mutation policy and command guards.
func bindPublicationExecution(root *cobra.Command, runtime *runtimeDependencies, capture *lastCapture, argv []string, options Options) {
	var walk func(*cobra.Command)
	walk = func(command *cobra.Command) {
		if nativeLongOperation(command.Annotations[cli.CapabilityAnnotation]) && command.RunE != nil {
			original := command.RunE
			command.RunE = func(cmd *cobra.Command, args []string) error {
				preview, _ := cmd.Flags().GetBool("preview")
				if preview || options.publicationExecution != nil {
					return original(cmd, args)
				}
				noWait, _ := cmd.Flags().GetBool("no-wait")
				if !options.PublicationWorkers {
					if noWait {
						return publicationWorkerError("unconfigured", "Background execution requires the detached worker runtime.", nil)
					}
					runtime.publicationExecution = &publicationExecution{deadline: time.Now().Add(publicationWaitLimit)}
					return original(cmd, args)
				}
				return beginPublicationWorker(cmd, runtime, capture, argv, noWait, options)
			}
		}
		for _, child := range command.Commands() {
			walk(child)
		}
	}
	walk(root)
}

func beginPublicationWorker(command *cobra.Command, runtime *runtimeDependencies, capture *lastCapture, argv []string, noWait bool, options Options) error {
	store, err := publicationOperationStore(options.OperationDirectory)
	if err != nil {
		return publicationWorkerError("store", "Operation tracking storage is unavailable; nothing was started.", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	configPath, err := filepath.Abs(runtime.configPath)
	if err != nil {
		return err
	}
	request := operationrun.Request{Args: slices.Clone(argv), ConfigPath: configPath, WorkingDirectory: cwd, Operation: command.Annotations[cli.CapabilityAnnotation], NoWait: noWait}
	if path, _ := command.Flags().GetString("batch-file"); path != "" {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(io.LimitReader(file, 1<<20+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil {
			return errors.Join(readErr, closeErr)
		}
		if len(data) > 1<<20 || !json.Valid(data) {
			return publicationWorkerError("batch", "The operation batch is not bounded valid JSON; nothing was started.", nil)
		}
		request.BatchData = data
	}
	record, err := store.Create(request)
	if err != nil {
		return publicationWorkerError("store", "The operation could not be recorded; nothing was started.", err)
	}
	var waiting *operationrun.ForegroundLease
	if !noWait {
		waiting, err = store.TryAcquireForeground(record.ID)
		if err != nil {
			return publicationWorkerError("wait", "Foreground wait ownership could not be established; nothing was started.", err)
		}
		defer waiting.Release()
	}
	launch := options.WorkerLauncher
	if launch == nil {
		launch = func(_ context.Context, directory, id string) error {
			executable, err := os.Executable()
			if err != nil {
				return err
			}
			return operationrun.Launch(executable, []string{"__publication-worker", directory, id}, cwd)
		}
	}
	if err := launch(command.Context(), store.Directory, record.ID); err != nil {
		return clierr.WithOutput(publicationRunOutput{record: record}, publicationWorkerError("start", "The worker launch was not confirmed. Check this operation before submitting again.", err))
	}
	// This is a local startup acknowledgement, not a Tableau completion probe.
	ackDeadline := time.Now().Add(5 * time.Second)
	for record.StartedAt.IsZero() {
		if !time.Now().Before(ackDeadline) || command.Context().Err() != nil {
			return clierr.WithOutput(publicationRunOutput{record: record}, publicationWorkerError("start_unknown", "Worker startup was not confirmed. Check this operation before submitting again.", command.Context().Err()))
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-command.Context().Done():
			timer.Stop()
		case <-timer.C:
		}
		record, err = store.Read(record.ID)
		if err != nil {
			return err
		}
	}
	if noWait {
		capture.value = publicationRunOutput{record: record}
		return nil
	}
	activity := progress.New(command.ErrOrStderr()).Start(command.Context(), operationActivity(record.Operation))
	defer activity.Stop()
	deadline := record.RequestedAt.Add(publicationWaitLimit)
	for {
		if !record.FinishedAt.IsZero() {
			capture.value = publicationRunOutput{record: record}
			if record.ExitCode != nil && *record.ExitCode != 0 {
				return clierr.Rendered(publicationWorkerError("failed", "The operation has one or more unsuccessful outcomes; inspect the saved results.", nil))
			}
			return nil
		}
		if command.Context().Err() != nil || !time.Now().Before(deadline) {
			record, err = store.Update(record.ID, func(r *operationrun.Record) error { r.Detached = true; return nil })
			if err != nil {
				return err
			}
			capture.value = publicationRunOutput{record: record, stopped: true}
			return nil
		}
		alive, err := store.Alive(record.ID)
		if err != nil {
			return err
		}
		if !alive {
			record, err = store.Read(record.ID)
			if err != nil {
				return err
			}
			if !record.FinishedAt.IsZero() {
				continue
			}
			return clierr.WithOutput(publicationRunOutput{record: record, stopped: true}, publicationWorkerError("interrupted", "The worker stopped without a final result. Preserve known effects and check status before repeating the operation.", nil))
		}
		if record.Activity != "" {
			activity.SetLabel(record.Activity)
		}
		timer := time.NewTimer(min(time.Second, time.Until(deadline)))
		select {
		case <-command.Context().Done():
			timer.Stop()
		case <-timer.C:
		}
		record, err = store.Read(record.ID)
		if err != nil {
			return err
		}
	}
}

func runPublicationWorker(ctx context.Context, directory, id string, options Options) int {
	store, err := publicationOperationStore(directory)
	if err != nil {
		return 1
	}
	record, err := store.Read(id)
	if err != nil || !nativeLongOperation(record.Operation) || record.Phase != operationrun.PhaseRequested {
		return 1
	}
	lease, record, err := store.Lease(id, os.Getpid())
	if err != nil {
		return 1
	}
	defer lease.Release()
	control := &publicationExecution{operation: record.Operation, noWait: record.Request.NoWait, deadline: record.RequestedAt.Add(publicationWaitLimit)}
	control.detached = func() bool {
		current, err := store.Read(id)
		if err != nil || current.Detached {
			return true
		}
		guard, err := store.TryAcquireForeground(id)
		if err == nil {
			_ = guard.Release()
			return true
		}
		return !errors.Is(err, operationrun.ErrForegroundBusy)
	}
	control.accepted = func(_ context.Context, path string) error {
		_, err := lease.Update(func(r *operationrun.Record) error {
			if !slices.Contains(r.ReceiptPaths, path) {
				r.ReceiptPaths = append(r.ReceiptPaths, path)
			}
			return nil
		})
		return err
	}
	options.ConfigPath = record.Request.ConfigPath
	options.OperationDirectory = store.Directory
	options.PublicationWorkers = false
	options.publicationExecution = control
	options.Stderr = io.Discard
	options.publicationResult = func(value any, _ bool, exit int) error {
		compact, full, err := publicationSnapshots(value, options.ConfigPath)
		if err != nil {
			// Preserve the independently saved receipts and latest bounded batch
			// snapshot even when the final expanded result exceeds storage bounds.
			_, saveErr := lease.Update(func(r *operationrun.Record) error {
				r.ExitCode = new(1)
				r.FinishedAt = time.Now().UTC()
				r.Phase = operationrun.PhaseFailed
				if len(r.ReceiptPaths) > 0 {
					r.Phase = operationrun.PhaseRemotePending
				}
				r.Activity = "Final result could not be saved completely; inspect accepted identities before any retry"
				return nil
			})
			return errors.Join(err, saveErr)
		}
		_, err = lease.Update(func(r *operationrun.Record) error {
			r.CompactResult, r.FullResult = compact, full
			r.ExitCode = new(exit)
			r.FinishedAt = time.Now().UTC()
			r.Phase = operationrun.PhaseCompleted
			if containsUnfinished(full) {
				r.Phase = operationrun.PhaseRemotePending
			} else if exit != 0 {
				r.Phase = operationrun.PhaseFailed
			}
			return nil
		})
		return err
	}
	ctx = contentbatch.WithObserver(ctx, func(out contentbatch.Output) {
		compact, _, err := publicationSnapshots(out, options.ConfigPath)
		if err != nil {
			return
		}
		_, _ = lease.Update(func(r *operationrun.Record) error {
			r.LiveResults = compact
			r.Activity = fmt.Sprintf("%s: %d completed, %d pending, %d failed", operationActivity(record.Operation), out.Succeeded, out.Pending, out.Failed)
			return nil
		})
	})
	ctx = progress.WithObserver(ctx, func(label string) {
		_, _ = lease.Update(func(r *operationrun.Record) error {
			r.Activity = label
			return nil
		})
	})
	args := slices.Clone(record.Request.Args)
	if len(record.Request.BatchData) > 0 {
		// A private immutable snapshot avoids rereading a caller-edited batch file.
		path := filepath.Join(store.Directory, id+".batch.json")
		if err := os.WriteFile(path, record.Request.BatchData, 0o600); err != nil {
			return 1
		}
		args = replacePublicationBatchPath(args, path)
	}
	return Run(ctx, args, io.Discard, options)
}

func replacePublicationBatchPath(args []string, path string) []string {
	for i, arg := range args {
		if (arg == "--batch-file" || arg == "--btf") && i+1 < len(args) {
			args[i+1] = path
		}
		if strings.HasPrefix(arg, "--batch-file=") || strings.HasPrefix(arg, "--btf=") {
			args[i] = "--batch-file=" + path
		}
	}
	return args
}

func publicationSnapshots(value any, configPath string) (json.RawMessage, json.RawMessage, error) {
	full, err := output.SnapshotWithConfig(value, int(operationrun.MaxRecordBytes)/3, configPath)
	if err != nil {
		return nil, nil, err
	}
	if projector, ok := value.(output.CompactProjector); ok {
		value = projector.CompactOutput()
	}
	if carried, ok := value.(error); ok {
		if partial, ok := errors.AsType[interface {
			error
			OperationOutput() any
		}](carried); ok {
			v := partial.OperationOutput()
			if p, ok := v.(output.CompactProjector); ok {
				v = p.CompactOutput()
			}
			value = map[string]any{"output": v, "error": errs.Structure(carried).Error}
		}
	}
	compact, err := output.SnapshotWithConfig(value, int(operationrun.MaxRecordBytes)/3, configPath)
	return compact, full, err
}

func containsUnfinished(data json.RawMessage) bool {
	var value any
	if json.Unmarshal(data, &value) != nil {
		return false
	}
	var pending func(any) bool
	pending = func(value any) bool {
		switch v := value.(type) {
		case map[string]any:
			if state, _ := v["status"].(string); state == "pending" || state == "running" || state == "queued" || state == "accepted" {
				return true
			}
			for _, child := range v {
				if pending(child) {
					return true
				}
			}
		case []any:
			for _, child := range v {
				if pending(child) {
					return true
				}
			}
		}
		return false
	}
	return pending(value)
}

func publicationCheckCommand(record operationrun.Record) string {
	args := []string{"job", "inspect", "--operation-id", record.ID}
	if record.Request.ConfigPath != "" {
		args = append(args, "--config", record.Request.ConfigPath)
	}
	return commandhint.Command(args...)
}

type publicationRunOutput struct {
	record  operationrun.Record
	stopped bool
}

func (o publicationRunOutput) CompactOutput() any {
	return publicationSnapshot(o.record, false, o.stopped)
}
func (o publicationRunOutput) FullOutput() any { return publicationSnapshot(o.record, true, o.stopped) }

func publicationSnapshot(record operationrun.Record, full, stopped bool) any {
	data := record.CompactResult
	if full {
		data = record.FullResult
	}
	if len(data) == 0 {
		data = record.LiveResults
	}
	result := map[string]any{}
	if len(data) > 0 {
		_ = json.Unmarshal(data, &result)
	}
	if result == nil {
		result = map[string]any{}
	}
	result["operation_id"], result["operation"] = record.ID, record.Operation
	if _, ok := result["status"]; !ok {
		state := "running"
		if record.Phase == operationrun.PhaseRequested {
			state = "starting"
		}
		if record.Phase == operationrun.PhaseCompleted {
			state = "succeeded"
		}
		if record.Phase == operationrun.PhaseFailed {
			state = "failed"
		}
		result["status"] = state
	}
	if record.Phase != operationrun.PhaseCompleted || containsUnfinished(record.FullResult) || publicationNeedsDestination(record.FullResult) {
		result["check_status"] = publicationCheckCommand(record)
	}
	if stopped {
		result["waiting_stopped"] = true
	}
	if !full {
		compactPublicationSnapshot(result)
		if _, checking := result["check_status"]; !checking {
			result["details"] = publicationCheckCommand(record) + " --full"
		}
	}
	return result
}

// The operation handle replaces per-item recovery paths in compact output.
// Saved full results retain the original action contracts and receipt paths.
func compactPublicationSnapshot(result map[string]any) {
	items, batch := result["items"].([]any)
	if batch {
		for _, field := range []string{"environment", "site", "workspace", "project_path", "kind"} {
			common := ""
			shared := len(items) > 0
			for index, raw := range items {
				item, _ := raw.(map[string]any)
				value, _ := item["result"].(map[string]any)
				candidate, _ := value[field].(string)
				if candidate == "" || (index > 0 && common != candidate) {
					shared = false
					break
				}
				common = candidate
			}
			if shared {
				result[field] = common
				for _, raw := range items {
					item := raw.(map[string]any)
					delete(item["result"].(map[string]any), field)
				}
			}
		}
		for _, raw := range items {
			item, _ := raw.(map[string]any)
			value, _ := item["result"].(map[string]any)
			if value == nil {
				continue
			}
			if native, ok := value["result"].(map[string]any); ok {
				delete(value, "result")
				for key, v := range native {
					value[key] = v
				}
			}
			if value["status"] == item["status"] {
				delete(value, "status")
			}
			if value["operation"] == result["operation"] {
				delete(value, "operation")
			}
		}
	}
	var trim func(any)
	trim = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			delete(v, "receipt_path")
			delete(v, "details")
			if help, ok := v["help"].([]any); ok && len(help) == 0 {
				delete(v, "help")
			}
			for _, child := range v {
				trim(child)
			}
		case []any:
			for _, child := range v {
				trim(child)
			}
		}
	}
	trim(result)
}

func publicationWorkerError(id, summary string, cause error) error {
	return &errs.Error{ID: "operation.worker." + id, Kind: errs.KindOperation, Operation: "operation", Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Use the returned operation ID to check status. Do not repeat the operation to recover its result."}
}
