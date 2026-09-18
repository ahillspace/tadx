// Package operationrun stores durable records for detached TADX operation
// workers and provides the process-lifetime lease used to observe them.
//
// The package deliberately persists only Request fields. In particular, it has
// no environment or credential field, and Launch inherits the caller's
// environment only in memory.
package operationrun

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/ahillspace/tadx/internal/lock"
)

const (
	// MaxRecordBytes bounds the size of one durable record read or written by
	// this package.
	MaxRecordBytes int64 = 1 << 20
	maxIDLength          = 128
)

var (
	// ErrNotFound reports that an exact operation ID has no durable record.
	ErrNotFound = errors.New("operation run not found")
	// ErrWorkerBusy reports that another worker currently owns the run lease.
	ErrWorkerBusy = errors.New("operation run worker is already leased")
	// ErrTerminal reports that a terminal run cannot be leased again.
	ErrTerminal = errors.New("operation run is terminal")
	// ErrWorkerInterrupted reports that a previously started worker did not
	// publish a terminal or remote-pending result and cannot be replayed.
	ErrWorkerInterrupted = errors.New("operation run worker was interrupted")
	// ErrForegroundBusy reports that another process owns the foreground wait
	// marker for the operation.
	ErrForegroundBusy = errors.New("operation run foreground wait is already owned")
	// ErrRecordMismatch reports that a record's persisted ID does not match the
	// exact ID used to select its filename.
	ErrRecordMismatch = errors.New("operation run record ID does not match requested ID")
	// ErrInvalidID reports an ID that cannot safely select one record.
	ErrInvalidID = errors.New("invalid operation run ID")
	// ErrRecordTooLarge reports a record exceeding MaxRecordBytes.
	ErrRecordTooLarge = errors.New("operation run record is too large")
)

// Phase is the durable lifecycle phase of an operation worker.
type Phase string

const (
	PhaseRequested     Phase = "requested"
	PhaseRunning       Phase = "running"
	PhaseRemotePending Phase = "remote_pending"
	PhaseCompleted     Phase = "completed"
	PhaseFailed        Phase = "failed"
)

// Request is the bounded, non-secret input persisted with a run.
//
// Args are the private worker arguments supplied by the application. BatchData
// can carry a bounded, non-secret JSON snapshot for a worker. Callers must not
// put credentials in Args or BatchData. Environment variables are intentionally
// not represented here and are never persisted by this package.
type Request struct {
	Args             []string        `json:"args"`
	BatchData        json.RawMessage `json:"batch_data,omitempty"`
	ConfigPath       string          `json:"config_path,omitempty"`
	WorkingDirectory string          `json:"working_directory,omitempty"`
	Operation        string          `json:"operation"`
	NoWait           bool            `json:"no_wait,omitzero"`
}

// Record is the durable state of one detached operation worker.
//
// Version is a monotonic record revision. It starts at one and increments for
// every successful Update, including a worker lease acknowledgement.
type Record struct {
	ID            string          `json:"id"`
	Version       uint64          `json:"version"`
	Operation     string          `json:"operation"`
	RequestedAt   time.Time       `json:"requested_at"`
	StartedAt     time.Time       `json:"started_at,omitzero"`
	FinishedAt    time.Time       `json:"finished_at,omitzero"`
	Phase         Phase           `json:"phase"`
	WorkerPID     int             `json:"worker_pid,omitzero"`
	Detached      bool            `json:"detached,omitzero"`
	Activity      string          `json:"activity,omitempty"`
	Request       Request         `json:"request"`
	CompactResult json.RawMessage `json:"compact_result,omitempty"`
	FullResult    json.RawMessage `json:"full_result,omitempty"`
	ExitCode      *int            `json:"exit_code,omitzero"`
	LiveResults   json.RawMessage `json:"live_results,omitempty"`
	ReceiptPaths  []string        `json:"receipt_paths,omitempty"`
}

// UpdateFunc mutates a record while the run's update lock is held.
// ID, Operation, and Request are immutable and must not be changed.
type UpdateFunc func(*Record) error

// Store is a durable operation-run store rooted at Directory.
//
// Directory should be private to the current TADX configuration. Create makes
// it with mode 0700 when necessary. Each record and each worker lease has its
// own lock file, while a short-lived store lock serializes ID allocation.
type Store struct {
	Directory string
}

// Create allocates a unique durable ID and writes its initial requested record.
func (s Store) Create(request Request) (Record, error) {
	if err := validateRequest(request); err != nil {
		return Record{}, err
	}
	directory, err := s.directory(true)
	if err != nil {
		return Record{}, err
	}
	storeLock, err := lock.Acquire(filepath.Join(directory, ".store.lock"))
	if err != nil {
		return Record{}, fmt.Errorf("lock operation run store: %w", err)
	}
	defer storeLock.Release()

	for range 8 {
		id, err := newID()
		if err != nil {
			return Record{}, err
		}
		record := Record{
			ID:          id,
			Version:     1,
			Operation:   request.Operation,
			RequestedAt: time.Now().UTC(),
			Phase:       PhaseRequested,
			Request:     cloneRequest(request),
		}
		path := filepath.Join(directory, id+".json")
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return Record{}, fmt.Errorf("check operation run %q: %w", id, err)
		}
		if err := writeAtomic(path, record); err != nil {
			return Record{}, fmt.Errorf("create operation run %q: %w", id, err)
		}
		return cloneRecord(record), nil
	}
	return Record{}, errors.New("could not allocate a unique operation run ID")
}

// Read returns one exact-ID record. Reads take the record's shared lock and
// reject records larger than MaxRecordBytes before decoding them.
func (s Store) Read(id string) (Record, error) {
	return s.ReadContext(context.Background(), id)
}

// ReadContext is Read with cancellation while waiting for an update lock. A
// nil context behaves like context.Background.
func (s Store) ReadContext(ctx context.Context, id string) (Record, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	path, err := s.recordPath(id)
	if err != nil {
		return Record{}, err
	}
	lockPath := path + ".lock"
	handle, err := lock.AcquireSharedContext(ctx, lockPath)
	if err != nil {
		return Record{}, fmt.Errorf("lock operation run %q for read: %w", id, err)
	}
	defer handle.Release()
	record, err := readRecord(path)
	if err != nil {
		return Record{}, err
	}
	if record.ID != id {
		return Record{}, fmt.Errorf("%w: requested %q, stored %q", ErrRecordMismatch, id, record.ID)
	}
	return record, nil
}

// Update atomically applies mutate to an exact-ID record and returns the new
// durable revision. Calls serialize across processes and goroutines.
func (s Store) Update(id string, mutate UpdateFunc) (Record, error) {
	if mutate == nil {
		return Record{}, errors.New("operation run update function is nil")
	}
	path, err := s.recordPath(id)
	if err != nil {
		return Record{}, err
	}
	handle, err := lock.Acquire(path + ".lock")
	if err != nil {
		return Record{}, fmt.Errorf("lock operation run %q for update: %w", id, err)
	}
	defer handle.Release()

	record, err := readRecord(path)
	if err != nil {
		return Record{}, err
	}
	if record.ID != id {
		return Record{}, fmt.Errorf("%w: requested %q, stored %q", ErrRecordMismatch, id, record.ID)
	}
	original := cloneRecord(record)
	if err := mutate(&record); err != nil {
		return Record{}, err
	}
	if record.ID != original.ID || record.Operation != original.Operation || !reflect.DeepEqual(record.Request, original.Request) {
		return Record{}, errors.New("operation run identity and request are immutable")
	}
	record.Version++
	if err := validateRecord(record); err != nil {
		return Record{}, err
	}
	if err := writeAtomic(path, record); err != nil {
		return Record{}, fmt.Errorf("update operation run %q: %w", id, err)
	}
	return cloneRecord(record), nil
}

// Alive reports whether a worker currently holds the operation's process-
// lifetime lease. It does not inspect or trust WorkerPID, so a reused PID
// cannot make an interrupted worker appear alive.
func (s Store) Alive(id string) (bool, error) {
	path, err := s.recordPath(id)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, ErrNotFound
		}
		return false, fmt.Errorf("stat operation run %q: %w", id, err)
	}
	handle, err := lock.TryAcquire(path + ".worker.lock")
	if err == nil {
		_ = handle.Release()
		return false, nil
	}
	if errors.Is(err, lock.ErrLocked) {
		return true, nil
	}
	return false, fmt.Errorf("check operation run %q worker lease: %w", id, err)
}

// TryAcquireForeground takes the nonblocking marker used by the foreground
// command while it waits for a worker. The marker is intentionally separate
// from the worker lease: releasing it tells the worker that the caller has
// stopped waiting, while the worker may continue until it records a result.
func (s Store) TryAcquireForeground(id string) (*ForegroundLease, error) {
	if _, err := s.recordPath(id); err != nil {
		return nil, err
	}
	handle, err := lock.TryAcquire(filepath.Join(s.Directory, id+".foreground.lock"))
	if err != nil {
		if errors.Is(err, lock.ErrLocked) {
			return nil, ErrForegroundBusy
		}
		return nil, fmt.Errorf("acquire operation run %q foreground wait: %w", id, err)
	}
	return &ForegroundLease{handle: handle}, nil
}

// ForegroundLease represents the foreground command's wait marker.
type ForegroundLease struct {
	handle     *lock.Handle
	once       sync.Once
	releaseErr error
}

// Release drops the foreground wait marker. It is safe and idempotent.
func (l *ForegroundLease) Release() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		l.releaseErr = l.handle.Release()
	})
	return l.releaseErr
}

// Lease acquires the process-lifetime worker lease and durably acknowledges
// that the worker started before returning. The caller must Release it after
// writing any terminal or remote-pending result.
func (s Store) Lease(id string, workerPID int) (*Lease, Record, error) {
	path, err := s.recordPath(id)
	if err != nil {
		return nil, Record{}, err
	}
	workerLock, err := lock.TryAcquire(path + ".worker.lock")
	if err != nil {
		if errors.Is(err, lock.ErrLocked) {
			return nil, Record{}, ErrWorkerBusy
		}
		return nil, Record{}, fmt.Errorf("acquire operation run %q worker lease: %w", id, err)
	}
	release := func() {
		_ = workerLock.Release()
	}
	if workerPID <= 0 {
		workerPID = os.Getpid()
	}
	record, err := s.Update(id, func(record *Record) error {
		if isTerminal(record.Phase) {
			return ErrTerminal
		}
		if !record.StartedAt.IsZero() {
			return ErrWorkerInterrupted
		}
		record.StartedAt = time.Now().UTC()
		record.Phase = PhaseRunning
		record.WorkerPID = workerPID
		return nil
	})
	if err != nil {
		release()
		return nil, Record{}, err
	}
	return &Lease{store: s, id: id, workerLock: workerLock}, record, nil
}

// Lease represents an acquired process-lifetime worker lease.
type Lease struct {
	store      Store
	id         string
	workerLock *lock.Handle
	once       sync.Once
	releaseErr error
}

// ID returns the exact operation ID owned by the lease.
func (l *Lease) ID() string {
	if l == nil {
		return ""
	}
	return l.id
}

// Update applies a durable record update through the lease's store.
func (l *Lease) Update(mutate UpdateFunc) (Record, error) {
	if l == nil {
		return Record{}, errors.New("operation run lease is nil")
	}
	return l.store.Update(l.id, mutate)
}

// Release drops the worker lease. It is safe and idempotent.
func (l *Lease) Release() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		l.releaseErr = l.workerLock.Release()
	})
	return l.releaseErr
}

func (s Store) directory(create bool) (string, error) {
	if strings.TrimSpace(s.Directory) == "" {
		return "", errors.New("operation run store directory is empty")
	}
	directory := filepath.Clean(s.Directory)
	if create {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return "", fmt.Errorf("create operation run store directory: %w", err)
		}
	}
	return directory, nil
}

func (s Store) recordPath(id string) (string, error) {
	if err := validateID(id); err != nil {
		return "", err
	}
	directory, err := s.directory(false)
	if err != nil {
		return "", err
	}
	path := filepath.Join(directory, id+".json")
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("stat operation run %q: %w", id, err)
	}
	return path, nil
}

func validateRequest(request Request) error {
	if strings.TrimSpace(request.Operation) == "" {
		return errors.New("operation run operation is empty")
	}
	if len(request.Args) > 4096 {
		return errors.New("operation run has too many worker arguments")
	}
	if len(request.BatchData) != 0 && !json.Valid(request.BatchData) {
		return errors.New("operation run batch data is not valid JSON")
	}
	return nil
}

func validateRecord(record Record) error {
	if err := validateID(record.ID); err != nil {
		return err
	}
	if record.Version == 0 {
		return errors.New("operation run record version is zero")
	}
	if strings.TrimSpace(record.Operation) == "" || record.Operation != record.Request.Operation {
		return errors.New("operation run operation does not match request")
	}
	if record.RequestedAt.IsZero() {
		return errors.New("operation run requested timestamp is zero")
	}
	switch record.Phase {
	case PhaseRequested, PhaseRunning, PhaseRemotePending, PhaseCompleted, PhaseFailed:
	default:
		return fmt.Errorf("unknown operation run phase %q", record.Phase)
	}
	for name, result := range map[string]json.RawMessage{
		"compact result": record.CompactResult,
		"full result":    record.FullResult,
		"live results":   record.LiveResults,
	} {
		if len(result) != 0 && !json.Valid(result) {
			return fmt.Errorf("operation run %s is not valid JSON", name)
		}
	}
	if len(record.ReceiptPaths) > 4096 {
		return errors.New("operation run has too many receipt paths")
	}
	return nil
}

func validateID(id string) error {
	if id == "" || len(id) > maxIDLength || id == "." || id == ".." {
		return ErrInvalidID
	}
	for _, char := range id {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '-' && char != '_' {
			return ErrInvalidID
		}
	}
	return nil
}

func isTerminal(phase Phase) bool {
	return phase == PhaseRemotePending || phase == PhaseCompleted || phase == PhaseFailed
}

func newID() (string, error) {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", fmt.Errorf("generate operation run ID: %w", err)
	}
	return "run-" + hex.EncodeToString(entropy[:]), nil
}

func readRecord(path string) (Record, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Record{}, ErrNotFound
		}
		return Record{}, fmt.Errorf("open operation run record: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Record{}, fmt.Errorf("stat operation run record: %w", err)
	}
	if info.Size() > MaxRecordBytes {
		return Record{}, ErrRecordTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxRecordBytes+1))
	if err != nil {
		return Record{}, fmt.Errorf("read operation run record: %w", err)
	}
	if int64(len(data)) > MaxRecordBytes {
		return Record{}, ErrRecordTooLarge
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record Record
	if err := decoder.Decode(&record); err != nil {
		return Record{}, fmt.Errorf("decode operation run record: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Record{}, errors.New("operation run record has trailing JSON")
		}
		return Record{}, fmt.Errorf("decode trailing operation run record data: %w", err)
	}
	if err := validateRecord(record); err != nil {
		return Record{}, err
	}
	return cloneRecord(record), nil
}

func writeAtomic(path string, record Record) error {
	if err := validateRecord(record); err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode operation run record: %w", err)
	}
	if int64(len(data)) > MaxRecordBytes {
		return ErrRecordTooLarge
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return fmt.Errorf("create operation run temporary record: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect operation run temporary record: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write operation run temporary record: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync operation run temporary record: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close operation run temporary record: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("replace operation run record: %w", err)
	}
	return nil
}

func cloneRequest(request Request) Request {
	request.Args = append([]string(nil), request.Args...)
	request.BatchData = append(json.RawMessage(nil), request.BatchData...)
	return request
}

func cloneRecord(record Record) Record {
	record.Request = cloneRequest(record.Request)
	record.CompactResult = append(json.RawMessage(nil), record.CompactResult...)
	record.FullResult = append(json.RawMessage(nil), record.FullResult...)
	record.LiveResults = append(json.RawMessage(nil), record.LiveResults...)
	record.ReceiptPaths = append([]string(nil), record.ReceiptPaths...)
	if record.ExitCode != nil {
		code := *record.ExitCode
		record.ExitCode = &code
	}
	return record
}
