package jobmonitor

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

// Publication persists and observes accepted identity; it never submits writes.
type Publication struct {
	Store          Store
	Base           Receipt
	Deadline       func() time.Time
	StopRequested  func() bool
	OnAccepted     func(context.Context, string) error
	NotifyAccepted func(string, string)
	StartWaiting   func(context.Context)
	Suspend        func(context.Context) error
	Observe        func(context.Context, []Receipt) []CheckResult
}

func (p *Publication) waitDeadline() time.Time {
	if p.Deadline != nil {
		return p.Deadline()
	}
	return time.Time{}
}

func (p *Publication) stopRequested() bool {
	return p.StopRequested != nil && p.StopRequested()
}

func (p *Publication) Record(ctx context.Context, jobID, status, resourceID, requestID, verification string) (string, error) {
	r := p.Base
	r.Observation.ID = jobID
	if jobID != "" {
		if saved, err := p.Store.Read(r); err == nil {
			r = saved
		}
	}
	if r.AcceptedAt.IsZero() {
		r.AcceptedAt = time.Now().UTC()
	}
	r.Observation.Status, r.Observation.ResourceID = status, resourceID
	if requestID != "" {
		r.Observation.RequestID = requestID
	}
	r.Verification = verification
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	path, err := p.Store.Save(saveCtx, r)
	if err != nil {
		outcome := errs.OutcomeUnknown
		if status == "succeeded" {
			outcome = errs.OutcomeConfirmed
		}
		return path, &errs.Error{ID: p.Base.Operation + ".receipt", Kind: errs.KindOperation, Operation: p.Base.Operation, Environment: p.Base.Environment, Site: p.Base.Site, Resource: resourceID, TableauJobID: jobID, TableauRequestID: r.Observation.RequestID, Summary: "Publication returned a result, but its recovery receipt could not be saved.", Cause: err, Phase: errs.PhasePersistence, Outcome: outcome, Retryable: errs.Bool(false), CorrectiveAction: "Preserve the returned identities. Do not repeat publication to repair local receipt storage."}
	}
	if p.OnAccepted != nil {
		if err := p.OnAccepted(saveCtx, path); err != nil {
			return filepath.ToSlash(path), fmt.Errorf("link saved publication receipt: %w", err)
		}
	}
	return filepath.ToSlash(path), nil
}

func (p *Publication) Accepted(ctx context.Context, id, requestID, jobType string, bulk bool) (Receipt, string, error) {
	r := p.Base
	r.AcceptedAt = time.Now().UTC()
	r.WaitUntil = p.waitDeadline()
	r.ManualOnly = p.stopRequested()
	r.PoolAfter = r.AcceptedAt.Add(time.Minute)
	if bulk {
		r.PoolAfter = r.AcceptedAt
	}
	r.Observation = value.JobStatus{ID: id, Type: jobType, Status: "pending", RequestID: requestID}
	// Acceptance must survive cancellation that arrives just after the server's
	// response. This local persistence deadline never resubmits a remote write.
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var path string
	var err error
	if r.ManualOnly {
		path, err = p.Store.Save(saveCtx, r)
	} else {
		path, err = p.Store.Register(saveCtx, r)
	}
	if err != nil {
		return r, path, &errs.Error{
			ID: p.Base.Operation + ".receipt", Kind: errs.KindOperation, Operation: p.Base.Operation,
			Environment: p.Base.Environment, Site: p.Base.Site,
			TableauJobID: id, TableauRequestID: requestID,
			Summary: "Publication was accepted, but saving or registering its recovery receipt failed.",
			Cause:   err, Phase: errs.PhasePersistence, Outcome: errs.OutcomeUnknown,
			Retryable: new(false), CorrectiveAction: "Preserve the returned identities. Do not repeat publication to repair local receipt storage.",
		}
	}
	if p.OnAccepted != nil {
		if err := p.OnAccepted(saveCtx, path); err != nil {
			return r, path, err
		}
	}
	if p.NotifyAccepted != nil {
		p.NotifyAccepted(id, filepath.ToSlash(path))
	}
	if r.ManualOnly || bulk {
		return r, path, nil
	}
	observed, err := p.Wait(ctx, r)
	return observed, path, err
}

func (p *Publication) Wait(ctx context.Context, r Receipt) (Receipt, error) {
	if p.StartWaiting != nil {
		p.StartWaiting(ctx)
	}
	if r.AcceptedAt.IsZero() {
		stored, err := p.Store.Read(r)
		if err != nil {
			return r, err
		}
		r = stored
	}
	if p.Suspend != nil {
		if err := p.Suspend(ctx); err != nil {
			return r, err
		}
	}
	m := Monitor{Store: p.Store, Deadline: p.waitDeadline(), StopRequested: p.StopRequested, Observe: p.Observe}
	observed, err := m.Wait(ctx, r)
	if errors.Is(err, ErrWaitLimit) {
		return observed, nil
	}
	if err == nil && observed.Observation.Status != "succeeded" {
		err = fmt.Errorf("remote job is %s", observed.Observation.Status)
	}
	return observed, err
}
