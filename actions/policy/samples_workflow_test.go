package policy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
)

// WriteSamples checks cancellation before each file after directory/preflight work.
type cancelPolicySamplesAfterFirst struct {
	context.Context
	checks int
}

func (c *cancelPolicySamplesAfterFirst) Err() error {
	c.checks++
	if c.checks > 1 {
		return context.Canceled
	}
	return nil
}

func TestPolicySamplesRetainsConfirmedFilesOnCancellation(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "samples")
	service := New(nil)
	out, err := service.WriteSamples(&cancelPolicySamplesAfterFirst{Context: t.Context()}, directory)
	structured, ok := errors.AsType[*errs.Error](err)
	want := filepath.ToSlash(filepath.Join(directory, "read-only.json"))
	if !ok || !errors.Is(err, context.Canceled) || structured.ID != "policy.samples.write" || structured.Outcome != errs.OutcomeUnknown || out.Status != "partial" || !slices.Equal(out.Files, []string{want}) || !slices.Equal(structured.Completed, out.Files) {
		t.Fatalf("output=%+v error=%v", out, err)
	}
	if _, err := os.Stat(filepath.Join(directory, "read-write-no-admin.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected second candidate: %v", err)
	}
}
