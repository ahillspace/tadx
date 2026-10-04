package job

import (
	"context"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/jobmonitor"
	"github.com/ahillspace/tadx/internal/value"
)

type serviceProviderFake struct {
	session                   Session
	opens, stores, operations int
}

func (p *serviceProviderFake) Open(context.Context, string) (Session, error) {
	p.opens++
	return p.session, nil
}
func (p *serviceProviderFake) Store() (jobmonitor.Store, error) {
	p.stores++
	return jobmonitor.Store{}, errors.New("store unexpectedly opened")
}
func (p *serviceProviderFake) Suspend(context.Context) error { return nil }
func (p *serviceProviderFake) RecoveryPorts() RecoveryPorts {
	p.operations++
	return RecoveryPorts{}
}

func TestServiceRejectsInvalidInputBeforeOpeningAnyDependency(t *testing.T) {
	provider := &serviceProviderFake{}
	service := New(provider)
	if _, err := service.Inspect(t.Context(), InspectInput{}); err == nil {
		t.Fatal("invalid inspect accepted")
	}
	if _, err := service.Wait(t.Context(), WaitInput{}); err == nil {
		t.Fatal("invalid wait accepted")
	}
	if _, err := service.Cancel(t.Context(), CancelInput{}); err == nil {
		t.Fatal("invalid cancel accepted")
	}
	if provider.opens != 0 || provider.stores != 0 || provider.operations != 0 {
		t.Fatalf("provider=%+v", provider)
	}
}

type nativeJobsFake struct {
	reads, cancels   int
	failConfirmation bool
}

func (n *nativeJobsFake) Inspect(context.Context, string) (value.JobStatus, error) {
	n.reads++
	if n.failConfirmation && n.reads > 1 {
		return value.JobStatus{}, errors.New("confirmation unavailable")
	}
	return value.JobStatus{ID: "job-1", Type: "refreshworkbook", Status: "pending"}, nil
}
func (n *nativeJobsFake) Cancel(context.Context, string) (string, error) {
	n.cancels++
	return "request-1", nil
}

func TestServiceCancellationPreviewDoesNotSubmit(t *testing.T) {
	native := &nativeJobsFake{}
	service := New(&serviceProviderFake{session: Session{Environment: "dev", Site: "site", Native: native}})
	output, err := service.Cancel(t.Context(), CancelInput{ID: "job-1", Preview: true})
	if err != nil || output.Status != "preview" || native.reads != 1 || native.cancels != 0 {
		t.Fatalf("output=%+v err=%v native=%+v", output, err, native)
	}
}

func TestServiceCancellationConfirmationFailureRetainsAcknowledgement(t *testing.T) {
	native := &nativeJobsFake{failConfirmation: true}
	service := New(&serviceProviderFake{session: Session{Environment: "dev", Site: "site", Native: native}})
	output, err := service.Cancel(t.Context(), CancelInput{ID: "job-1"})
	structured, ok := errors.AsType[*errs.Error](err)
	if !ok || structured.Phase != errs.PhaseVerification || structured.Outcome != errs.OutcomeUnknown || structured.TableauRequestID != "request-1" || output.RequestID != "request-1" || output.Job.ID != "job-1" || native.cancels != 1 {
		t.Fatalf("output=%+v err=%v native=%+v", output, err, native)
	}
}
