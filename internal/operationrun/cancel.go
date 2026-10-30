package operationrun

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ahillspace/tadx/internal/fsreplace"
	"github.com/ahillspace/tadx/internal/lock"
)

// ErrCancelUnsupported means the operation has no compatible cancellation control.
var ErrCancelUnsupported = errors.New("operation worker does not support cancellation")

// CancelControl is the independent, versioned control state for an operation.
// A request is not proof that a worker observed it or that work was stopped.
type CancelControl struct {
	ID             string    `json:"id"`
	Schema         int       `json:"schema"`
	Supported      bool      `json:"supported,omitzero"`
	RequestedAt    time.Time `json:"requested_at,omitzero"`
	AcknowledgedAt time.Time `json:"acknowledged_at,omitzero"`
}

// PrepareCancel creates cancellation control before starting a compatible worker.
func (s Store) PrepareCancel(id string) error {
	_, err := s.withCancel(id, func(record Record, control *CancelControl, exists bool) (bool, error) {
		if exists {
			return false, nil
		}
		if isTerminal(record.Phase) {
			return false, ErrTerminal
		}
		if !record.StartedAt.IsZero() {
			return false, ErrCancelUnsupported
		}
		*control = CancelControl{ID: id, Schema: 1}
		return true, nil
	})
	return err
}

// RequestCancel durably requests cancellation. Repeated requests preserve the
// original timestamp. Requests made before startup remain unacknowledged until
// the worker observes them.
func (s Store) RequestCancel(id string) (CancelControl, error) {
	return s.withCancel(id, func(record Record, control *CancelControl, exists bool) (bool, error) {
		if exists && !control.RequestedAt.IsZero() {
			return false, nil
		}
		if isTerminal(record.Phase) {
			return false, ErrTerminal
		}
		if !exists {
			return false, ErrCancelUnsupported
		}
		if !record.StartedAt.IsZero() && !control.Supported {
			return false, ErrCancelUnsupported
		}
		control.RequestedAt = time.Now().UTC()
		return true, nil
	})
}

// ReadCancel reads control state without interpreting it as a final outcome.
func (s Store) ReadCancel(id string) (CancelControl, error) {
	return s.withCancel(id, func(_ Record, _ *CancelControl, exists bool) (bool, error) {
		if !exists {
			return false, ErrCancelUnsupported
		}
		return false, nil
	})
}

// AcknowledgeCancel records that the worker observed a request.
func (l *Lease) AcknowledgeCancel() error {
	if l == nil {
		return errors.New("operation run lease is nil")
	}
	_, err := l.store.withCancel(l.id, func(_ Record, control *CancelControl, exists bool) (bool, error) {
		if !exists || !control.Supported {
			return false, ErrCancelUnsupported
		}
		if control.RequestedAt.IsZero() {
			return false, errors.New("operation cancellation was not requested")
		}
		if !control.AcknowledgedAt.IsZero() {
			return false, nil
		}
		control.AcknowledgedAt = time.Now().UTC()
		return true, nil
	})
	return err
}

// BeginCommit atomically admits one irreversible transaction against a request.
// An admitted transaction may finish even if a later request is accepted.
func (l *Lease) BeginCommit() error {
	if l == nil {
		return errors.New("operation run lease is nil")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released {
		return ErrLeaseReleased
	}
	_, err := l.store.withCancel(l.id, func(record Record, control *CancelControl, exists bool) (bool, error) {
		if isTerminal(record.Phase) {
			return false, ErrTerminal
		}
		if !exists || !control.Supported {
			return false, ErrCancelUnsupported
		}
		if !control.RequestedAt.IsZero() {
			return false, context.Canceled
		}
		return false, nil
	})
	return err
}

func (s Store) withCancel(id string, mutate func(Record, *CancelControl, bool) (bool, error)) (CancelControl, error) {
	path, err := s.recordPath(id)
	if err != nil {
		return CancelControl{}, err
	}
	handle, err := lock.Acquire(path + ".lock")
	if err != nil {
		return CancelControl{}, fmt.Errorf("lock operation run %q control: %w", id, err)
	}
	defer handle.Release()
	record, err := readRecord(path)
	if err != nil {
		return CancelControl{}, err
	}
	if record.ID != id {
		return CancelControl{}, ErrRecordMismatch
	}
	control, exists, err := readCancel(path, id)
	if err != nil {
		return CancelControl{}, err
	}
	changed, err := mutate(record, &control, exists)
	if err != nil {
		return CancelControl{}, err
	}
	if changed {
		if err := writeCancel(path, control); err != nil {
			return CancelControl{}, err
		}
	}
	return control, nil
}

// advertiseCancelLocked is called under the record lock before the worker's
// started record is written. Old records without a control file stay unchanged.
func advertiseCancelLocked(path, id string) error {
	control, exists, err := readCancel(path, id)
	if err != nil || !exists {
		return err
	}
	if control.Supported {
		return nil
	}
	control.Supported = true
	return writeCancel(path, control)
}

func readCancel(path, id string) (CancelControl, bool, error) {
	file, err := os.Open(path + ".control.json")
	if errors.Is(err, os.ErrNotExist) {
		return CancelControl{}, false, nil
	}
	if err != nil {
		return CancelControl{}, false, fmt.Errorf("read operation control: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return CancelControl{}, false, fmt.Errorf("stat operation control: %w", err)
	}
	if info.Size() > MaxRecordBytes {
		return CancelControl{}, false, ErrRecordTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxRecordBytes+1))
	if err != nil {
		return CancelControl{}, false, fmt.Errorf("read operation control: %w", err)
	}
	if int64(len(data)) > MaxRecordBytes {
		return CancelControl{}, false, ErrRecordTooLarge
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var control CancelControl
	if err := decoder.Decode(&control); err != nil {
		return CancelControl{}, false, fmt.Errorf("decode operation control: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return CancelControl{}, false, errors.New("operation control has trailing data")
	}
	if control.ID != id {
		return CancelControl{}, false, ErrRecordMismatch
	}
	if control.Schema != 1 {
		return CancelControl{}, false, errors.New("unsupported operation control schema")
	}
	if !control.AcknowledgedAt.IsZero() && (control.RequestedAt.IsZero() || !control.Supported) {
		return CancelControl{}, false, errors.New("invalid operation cancellation acknowledgement")
	}
	return control, true, nil
}

func writeCancel(path string, control CancelControl) error {
	data, err := json.Marshal(control)
	if err != nil {
		return err
	}
	target := path + ".control.json"
	temporary, err := os.CreateTemp(filepath.Dir(path), ".control-*.tmp")
	if err != nil {
		return fmt.Errorf("create operation control: %w", err)
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := fsreplace.Replace(name, target); err != nil {
		return fmt.Errorf("replace operation control: %w", err)
	}
	return nil
}
