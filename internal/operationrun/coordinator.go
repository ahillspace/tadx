package operationrun

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/ahillspace/tadx/internal/errs"
)

// WaitLimit bounds foreground observation without cancelling remote work.
const WaitLimit = 20 * time.Minute

// NewStore resolves the private operation directory without creating it.
func NewStore(directory string) (Store, error) {
	if directory == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return Store{}, err
		}
		directory = filepath.Join(cache, "tadx", "operations")
	}
	directory, err := filepath.Abs(directory)
	return Store{Directory: directory}, err
}

// PrepareRequest captures bounded invocation inputs before starting a worker.
func PrepareRequest(args []string, configPath, operation, batchPath string, noWait bool) (Request, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return Request{}, err
	}
	configPath, err = filepath.Abs(configPath)
	if err != nil {
		return Request{}, err
	}
	request := Request{Args: slices.Clone(args), ConfigPath: configPath, WorkingDirectory: cwd, Operation: operation, NoWait: noWait}
	if batchPath != "" {
		file, err := os.Open(batchPath)
		if err != nil {
			return Request{}, err
		}
		data, readErr := io.ReadAll(io.LimitReader(file, 1<<20+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil {
			return Request{}, errors.Join(readErr, closeErr)
		}
		if len(data) > 1<<20 || !json.Valid(data) {
			return Request{}, WorkerError("batch", "The operation batch is not bounded valid JSON; nothing was started.", nil)
		}
		request.BatchData = data
	}
	return request, nil
}

// Activity receives foreground progress without coupling coordination to a UI.
type Activity interface {
	SetLabel(string) bool
	Stop()
}

// Coordinator launches and observes a detached worker through durable state.
type Coordinator struct {
	Store         Store
	Launch        func(context.Context, string, string) error
	StartActivity func(context.Context, string) Activity
}

// Outcome preserves a saved identity and distinguishes foreground presentation.
type Outcome struct {
	Record   Record
	Present  bool
	Stopped  bool
	Rendered bool
}

// Begin records the request and observes startup before optionally waiting.
func (c Coordinator) Begin(ctx context.Context, request Request) (Outcome, error) {
	store := c.Store
	record, err := store.Create(request)
	if err != nil {
		return Outcome{}, WorkerError("store", "The operation could not be recorded; nothing was started.", err)
	}
	var waiting *ForegroundLease
	if !request.NoWait {
		waiting, err = store.TryAcquireForeground(record.ID)
		if err != nil {
			return Outcome{}, WorkerError("wait", "Foreground wait ownership could not be established; nothing was started.", err)
		}
		defer waiting.Release()
	}
	if err := c.Launch(ctx, store.Directory, record.ID); err != nil {
		return Outcome{Record: record, Present: true}, WorkerError("start", "The worker launch was not confirmed. Check this operation before submitting again.", err)
	}
	// Startup acknowledgement is local, not a Tableau completion probe.
	ackDeadline := time.Now().Add(5 * time.Second)
	for record.StartedAt.IsZero() {
		if !time.Now().Before(ackDeadline) || ctx.Err() != nil {
			return Outcome{Record: record, Present: true}, WorkerError("start_unknown", "Worker startup was not confirmed. Check this operation before submitting again.", ctx.Err())
		}
		waitTick(ctx, 10*time.Millisecond)
		record, err = store.Read(record.ID)
		if err != nil {
			return Outcome{}, err
		}
	}
	if request.NoWait {
		return Outcome{Record: record, Present: true}, nil
	}
	var activity Activity
	if c.StartActivity != nil {
		activity = c.StartActivity(ctx, record.Operation)
		if activity != nil {
			defer activity.Stop()
		}
	}
	deadline := record.RequestedAt.Add(WaitLimit)
	for {
		if !record.FinishedAt.IsZero() {
			result := Outcome{Record: record, Present: true}
			if record.ExitCode != nil && *record.ExitCode != 0 {
				result.Rendered = true
				return result, WorkerError("failed", "The operation has one or more unsuccessful outcomes; inspect the saved results.", nil)
			}
			return result, nil
		}
		if ctx.Err() != nil || !time.Now().Before(deadline) {
			record, err = store.Update(record.ID, func(r *Record) error { r.Detached = true; return nil })
			if err != nil {
				return Outcome{}, err
			}
			return Outcome{Record: record, Present: true, Stopped: true}, nil
		}
		alive, err := store.Alive(record.ID)
		if err != nil {
			return Outcome{}, err
		}
		if !alive {
			record, err = store.Read(record.ID)
			if err != nil {
				return Outcome{}, err
			}
			if !record.FinishedAt.IsZero() {
				continue
			}
			return Outcome{Record: record, Present: true, Stopped: true}, WorkerError("interrupted", "The worker stopped without a final result. Preserve known effects and check status before repeating the operation.", nil)
		}
		if record.Activity != "" && activity != nil {
			activity.SetLabel(record.Activity)
		}
		waitTick(ctx, min(time.Second, time.Until(deadline)))
		record, err = store.Read(record.ID)
		if err != nil {
			return Outcome{}, err
		}
	}
}

func waitTick(ctx context.Context, interval time.Duration) {
	timer := time.NewTimer(interval)
	select {
	case <-ctx.Done():
		timer.Stop()
	case <-timer.C:
	}
}

// WorkerError reports a local coordination failure without suggesting replay.
func WorkerError(id, summary string, cause error) error {
	return &errs.Error{ID: "operation.worker." + id, Kind: errs.KindOperation, Operation: "operation", Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Use the returned operation ID to check status. Do not repeat the operation to recover its result."}
}
