package errs_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
)

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
