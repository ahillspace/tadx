package operationrun

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Worker owns the leased state of one private process invocation.
type Worker struct {
	Store  Store
	Record Record
	lease  *Lease
}

// OpenWorker admits only an unstarted supported operation and leases it.
func OpenWorker(store Store, id string, supported func(string) bool) (*Worker, error) {
	record, err := store.Read(id)
	if err != nil {
		return nil, err
	}
	if !supported(record.Operation) || record.Phase != PhaseRequested {
		return nil, errors.New("operation is not an unstarted supported worker request")
	}
	lease, record, err := store.Lease(id, os.Getpid())
	if err != nil {
		return nil, err
	}
	return &Worker{Store: store, Record: record, lease: lease}, nil
}

// Close releases the worker's process-lifetime lease.
func (w *Worker) Close() error { return w.lease.Release() }

// PrepareIntent records a target-bound receipt identity before submission.
func (w *Worker) PrepareIntent(operationID, operation, receiptID, scope string) error {
	_, err := w.lease.Update(func(r *Record) error {
		if operationID != r.ID || operation != r.Operation || receiptID == "" {
			return errors.New("publication receipt intent does not match its operation")
		}
		r.ReceiptIntents = append(r.ReceiptIntents, ReceiptIntent{ID: receiptID, Scope: scope, RegisteredAt: time.Now().UTC()})
		return nil
	})
	return err
}

// Detached reports whether foreground observation no longer owns its lease.
func (w *Worker) Detached() bool {
	current, err := w.Store.Read(w.Record.ID)
	if err != nil || current.Detached {
		return true
	}
	guard, err := w.Store.TryAcquireForeground(w.Record.ID)
	if err == nil {
		_ = guard.Release()
		return true
	}
	return !errors.Is(err, ErrForegroundBusy)
}

// AcceptReceipt links a separately saved receipt without submitting work.
func (w *Worker) AcceptReceipt(path string) error {
	_, err := w.lease.Update(func(r *Record) error {
		if !slices.Contains(r.ReceiptPaths, path) {
			r.ReceiptPaths = append(r.ReceiptPaths, path)
		}
		return nil
	})
	return err
}

// Finish persists encoded results or preserves accepted effects on capture failure.
func (w *Worker) Finish(compact, full json.RawMessage, exit int, unfinished bool, captureErr error) error {
	if captureErr != nil {
		_, saveErr := w.lease.Update(func(r *Record) error {
			r.ExitCode = new(1)
			r.FinishedAt = time.Now().UTC()
			r.Phase = PhaseFailed
			if len(r.ReceiptPaths) > 0 {
				r.Phase = PhaseRemotePending
			}
			r.Activity = "Final result could not be saved completely; inspect accepted identities before any retry"
			return nil
		})
		return errors.Join(captureErr, saveErr)
	}
	_, err := w.lease.Update(func(r *Record) error {
		r.CompactResult, r.FullResult = compact, full
		r.ExitCode = new(exit)
		r.FinishedAt = time.Now().UTC()
		r.Phase = PhaseCompleted
		if unfinished {
			r.Phase = PhaseRemotePending
		} else if exit != 0 {
			r.Phase = PhaseFailed
		}
		return nil
	})
	return err
}

// SaveProgress records a bounded partial result and its presentation label.
func (w *Worker) SaveProgress(compact json.RawMessage, label string) error {
	_, err := w.lease.Update(func(r *Record) error {
		r.LiveResults = compact
		r.Activity = label
		return nil
	})
	return err
}

// SetActivity records the latest caller-supplied presentation label.
func (w *Worker) SetActivity(label string) error {
	_, err := w.lease.Update(func(r *Record) error { r.Activity = label; return nil })
	return err
}

// Args materializes an immutable batch snapshot before private dispatch.
func (w *Worker) Args() ([]string, error) {
	args := slices.Clone(w.Record.Request.Args)
	if len(w.Record.Request.BatchData) > 0 {
		path := filepath.Join(w.Store.Directory, w.Record.ID+".batch.json")
		if err := os.WriteFile(path, w.Record.Request.BatchData, 0o600); err != nil {
			return nil, err
		}
		args = replaceBatchPath(args, path)
	}
	return args, nil
}

func replaceBatchPath(args []string, path string) []string {
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
