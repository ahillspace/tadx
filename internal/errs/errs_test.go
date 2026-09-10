package errs_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
)

type requestIDError struct{ id string }

func (e requestIDError) Error() string     { return "upstream failed" }
func (e requestIDError) RequestID() string { return e.id }

type upstreamError struct{}

func (upstreamError) Error() string          { return "Tableau rejected the request" }
func (upstreamError) HTTPStatus() int        { return 403 }
func (upstreamError) TableauCode() string    { return "403007" }
func (upstreamError) TableauSummary() string { return "Forbidden" }
func (upstreamError) TableauDetail() string  { return "Missing permission" }
func (upstreamError) RequestID() string      { return "request-403" }

type retryAdviceError struct{}

func (retryAdviceError) Error() string            { return "upstream unavailable" }
func (retryAdviceError) Retryable() bool          { return true }
func (retryAdviceError) CorrectiveAction() string { return "Retry after recovery." }

type prerequisiteError struct{}

func (prerequisiteError) Error() string                { return "missing prerequisite" }
func (prerequisiteError) PrerequisiteKind() string     { return "project" }
func (prerequisiteError) PrerequisiteResource() string { return "project-1" }
func (prerequisiteError) PrerequisiteSummary() string  { return "Resolve the exact project first." }

func TestRetryAdviceMergesOnlyMissingFields(t *testing.T) {
	outer := &errs.Error{Retryable: errs.Bool(false), Cause: retryAdviceError{}}
	retryable, advice := errs.RetryAdvice(outer)
	if retryable == nil || *retryable || advice != "Retry after recovery." {
		t.Fatalf("retryable=%v advice=%q", retryable, advice)
	}
	outer.Retryable, outer.CorrectiveAction = nil, "Inspect the confirmed result."
	retryable, advice = errs.RetryAdvice(outer)
	if retryable == nil || !*retryable || advice != outer.CorrectiveAction {
		t.Fatalf("retryable=%v advice=%q", retryable, advice)
	}
}

func TestExitCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "success", err: nil, want: 0},
		{name: "runtime", err: errors.New("network failed"), want: 1},
		{name: "operation", err: errs.New(errs.KindOperation, "publish failed"), want: 1},
		{name: "usage", err: errs.New(errs.KindUsage, "unknown flag"), want: 2},
		{name: "wrapped usage", err: fmt.Errorf("parse: %w", errs.New(errs.KindUsage, "bad selector")), want: 2},
	}
	for _, tt := range tests {
		if got := errs.ExitCode(tt.err); got != tt.want {
			t.Errorf("%s: got %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestStructurePreservesFieldsAndCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("upstream unavailable")
	err := &errs.Error{
		ID:               "TADX-PUBLISH-001",
		Kind:             errs.KindOperation,
		Operation:        "workbook.publish",
		Selector:         "name=Sales",
		Resource:         "workbook",
		Environment:      "production",
		Site:             "analytics",
		Summary:          "Publish failed",
		Cause:            cause,
		Retryable:        errs.Bool(true),
		CorrectiveAction: "Check site permissions.",
		Validation:       []errs.ValidationDetail{{Field: "project", Code: "required", Message: "Project is required."}},
		TableauRequestID: "req-123",
		TableauJobID:     "job-456",
	}
	payload := errs.Structure(err)
	if payload.Error.ID != err.ID || payload.Error.UpstreamCause != cause.Error() || payload.Error.Retryable == nil || !*payload.Error.Retryable {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if !errors.Is(err, cause) {
		t.Fatal("structured error does not unwrap cause")
	}
}

func TestTableauRequestIDFindsWrappedCarrier(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("workbook read: %w", requestIDError{id: "request-123"})
	if got := errs.TableauRequestID(err); got != "request-123" {
		t.Fatalf("TableauRequestID() = %q", got)
	}
}

func TestRetryAdviceFindsWrappedCarrier(t *testing.T) {
	t.Parallel()

	retryable, correctiveAction := errs.RetryAdvice(fmt.Errorf("auth: %w", retryAdviceError{}))
	if retryable == nil || !*retryable || correctiveAction != "Retry after recovery." {
		t.Fatalf("RetryAdvice() = %#v, %q", retryable, correctiveAction)
	}
}

func TestRetryAdvicePreservesStructuredFields(t *testing.T) {
	t.Parallel()

	err := &errs.Error{Retryable: errs.Bool(false), CorrectiveAction: "Inspect before retrying."}
	retryable, corrective := errs.RetryAdvice(fmt.Errorf("wrapped: %w", err))
	if retryable == nil || *retryable || corrective != "Inspect before retrying." {
		t.Fatalf("RetryAdvice() = %#v, %q", retryable, corrective)
	}
}

func TestStructureExtractsPrerequisiteCarrier(t *testing.T) {
	t.Parallel()

	payload := errs.Structure(&errs.Error{Cause: prerequisiteError{}}).Error
	if payload.Prerequisite == nil || payload.Prerequisite.Resource != "project-1" {
		t.Fatalf("prerequisite = %#v", payload.Prerequisite)
	}
}

func TestStructureGeneratesBoundedRecoveryAdviceFromFacts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        *errs.Error
		want       string
		unexpected string
	}{
		{"unknown", &errs.Error{Outcome: errs.OutcomeUnknown}, "Inspect the exact target", "retry the mutation automatically"},
		{"confirmed", &errs.Error{Outcome: errs.OutcomeConfirmed}, "Retain the confirmed result", "Inspect the exact target"},
		{"prerequisite", &errs.Error{Prerequisite: &errs.Prerequisite{Kind: "project", Resource: "project-1"}}, "Resolve the required project project-1", "Inspect the exact target"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := errs.Structure(test.err).Error.Recovery
			if !strings.Contains(got, test.want) || strings.Contains(got, test.unexpected) {
				t.Fatalf("recovery = %q", got)
			}
		})
	}
}

func TestUnknownOutcomeTakesPriorityOverPrerequisiteRecovery(t *testing.T) {
	t.Parallel()

	err := &errs.Error{Outcome: errs.OutcomeUnknown, Prerequisite: &errs.Prerequisite{Kind: "project", Resource: "project-1"}}
	got := errs.Structure(err).Error.Recovery
	if !strings.Contains(got, "Inspect the exact target") || strings.Contains(got, "Resolve the required project") {
		t.Fatalf("recovery = %q", got)
	}
}

func TestStructureInfersRetryAdviceFromCause(t *testing.T) {
	t.Parallel()

	payload := errs.Structure(&errs.Error{
		Kind: errs.KindOperation, Summary: "Authentication failed.", Cause: retryAdviceError{},
	}).Error
	if payload.Retryable == nil || !*payload.Retryable || payload.CorrectiveAction != "Retry after recovery." {
		t.Fatalf("structured error = %#v", payload)
	}
}

func TestStructurePreservesWrappedTableauUpstreamFields(t *testing.T) {
	t.Parallel()

	err := &errs.Error{Kind: errs.KindOperation, Summary: "Workbook read failed.", Cause: fmt.Errorf("adapter: %w", upstreamError{})}
	payload := errs.Structure(err).Error
	if payload.UpstreamStatus != 403 || payload.UpstreamCode != "403007" || payload.UpstreamSummary != "Forbidden" || payload.UpstreamDetail != "Missing permission" || payload.TableauRequestID != "request-403" {
		t.Fatalf("upstream payload = %#v", payload)
	}
}

func TestStructurePreservesRecoveryFactsThroughWrapper(t *testing.T) {
	t.Parallel()

	nested := &errs.Error{
		Kind:         errs.KindOperation,
		Phase:        errs.PhaseVerification,
		Outcome:      errs.OutcomeUnknown,
		Prerequisite: &errs.Prerequisite{Kind: "project", Resource: "project-1", Summary: "Project identity is required."},
		Summary:      "The result could not be verified.",
	}
	wrapped := &errs.Error{Kind: errs.KindOperation, Summary: "Publish failed.", Cause: nested}
	payload := errs.Structure(wrapped).Error
	if payload.Phase != errs.PhaseVerification || payload.Outcome != errs.OutcomeUnknown {
		t.Fatalf("recovery state = %#v", payload)
	}
	if payload.Prerequisite == nil || payload.Prerequisite.Resource != "project-1" {
		t.Fatalf("prerequisite = %#v", payload.Prerequisite)
	}
}
