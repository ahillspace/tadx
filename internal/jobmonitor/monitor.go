package jobmonitor

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/ahillspace/tadx/internal/lock"
	"github.com/ahillspace/tadx/internal/value"
)

const (
	fastInterval    = 5 * time.Second
	poolInterval    = 30 * time.Second
	maxReadFailures = 3
)

type CheckResult struct {
	Status value.JobStatus
	Err    error
}

// Monitor serializes observation rounds across processes sharing one credential
// and site. Observe must use exact job identities and must never submit writes.
type Monitor struct {
	Store   Store
	Observe func(context.Context, []Receipt) []CheckResult
	Now     func() time.Time
	Sleep   func(context.Context, time.Duration) error
}

func (m Monitor) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now().UTC()
}
func (m Monitor) sleep(ctx context.Context, d time.Duration) error {
	if m.Sleep != nil {
		return m.Sleep(ctx, d)
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Wait has no duration deadline for an active remote job. Cancellation and
// repeated read failures stop local monitoring without claiming remote failure.
func (m Monitor) Wait(ctx context.Context, accepted Receipt) (Receipt, error) {
	latest := accepted
	for {
		stored, err := m.Store.Read(accepted)
		if err != nil {
			return latest, fmt.Errorf("read accepted job receipt: %w", err)
		}
		latest = stored
		if latest.Observation.Terminal() {
			return latest, nil
		}
		if err := ctx.Err(); err != nil {
			return latest, err
		}
		if latest.ReadFailures >= maxReadFailures {
			return latest, errors.New("job status remains unavailable; remote outcome is unknown")
		}
		if delay := latest.NextCheck.Sub(m.now()); delay > 0 {
			if err := m.sleep(ctx, delay); err != nil {
				return latest, err
			}
			continue
		}
		if err := m.round(ctx, latest); err != nil {
			return latest, err
		}
	}
}

func (m Monitor) round(ctx context.Context, target Receipt) error {
	if m.Observe == nil {
		return errors.New("job observer is unavailable")
	}
	guard, err := lock.AcquireContext(ctx, m.Store.indexPath(target.CoordinationKey)+".round.lock")
	if err != nil {
		return err
	}
	defer guard.Release()
	now := m.now()
	current, err := m.Store.Read(target)
	if err != nil {
		return err
	}
	if current.Observation.Terminal() || current.NextCheck.After(now) {
		return nil
	}
	jobs := []Receipt{current}
	interval := fastInterval
	if !current.PoolAfter.After(now) {
		interval = poolInterval
		paths, err := readIndex(m.Store.indexPath(current.CoordinationKey))
		if err != nil {
			return err
		}
		for _, p := range paths {
			if p == filepath.Base(m.Store.Path(current)) {
				continue
			}
			other, err := readReceipt(filepath.Join(m.Store.Directory, p))
			if err != nil {
				return fmt.Errorf("read shared job receipt: %w", err)
			}
			if other.CoordinationKey != current.CoordinationKey {
				return errors.New("shared job receipt has mismatched coordination identity")
			}
			if !other.Observation.Terminal() && other.ReadFailures < maxReadFailures && !other.PoolAfter.After(now) && !other.NextCheck.After(now) {
				jobs = append(jobs, other)
			}
		}
	}
	results := m.Observe(ctx, jobs)
	if len(results) != len(jobs) {
		return errors.New("job observer returned incomplete results; remote outcomes are unchanged")
	}
	// Preserve observations already returned even when cancellation arrives at
	// the response boundary. This deadline covers local persistence only.
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	for i, job := range jobs {
		result := results[i]
		if result.Err == nil && (result.Status.ID != job.Observation.ID || !validStatus(result.Status.Status)) {
			result.Err = errors.New("job observation is missing its exact identity or state")
		}
		if result.Err != nil {
			job.ReadFailures++
			job.LastReadError = "job status could not be read"
		} else {
			if result.Status.CheckedAt.IsZero() {
				result.Status.CheckedAt = now
			}
			job.Observation = result.Status
			job.ReadFailures = 0
			job.LastReadError = ""
		}
		job.NextCheck = now.Add(interval)
		if _, err := m.Store.Save(saveCtx, job); err != nil {
			return fmt.Errorf("save observed job state: %w", err)
		}
	}
	return nil
}

func validStatus(status string) bool {
	switch status {
	case "pending", "running", "succeeded", "failed", "cancelled":
		return true
	default:
		return false
	}
}
