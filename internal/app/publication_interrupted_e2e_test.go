package app

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/operationrun"
)

func TestInterruptedPublicationInspectionRetainsAcceptedJobsThroughCLI(t *testing.T) {
	fixture := newPublicationStatusFixture(t, publicationStatusFixtureOptions{jobDestination: true})
	operationID, options := startNoWaitWorkbookPublication(t, fixture)
	store := operationrun.Store{Directory: options.OperationDirectory}
	// Model a worker lost after saving acceptance but before it could finish
	// the current item and persist the final aggregate projection.
	if _, err := store.Update(operationID, func(record *operationrun.Record) error {
		record.Phase = operationrun.PhaseRunning
		record.FinishedAt = time.Time{}
		record.CompactResult, record.FullResult = nil, nil
		record.LiveResults = json.RawMessage(`{"status":"running","total":2,"pending":2,"items":[{"selector":"first","status":"running"},{"selector":"second","status":"queued"}]}`)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	result := inspectPublicationOperationThroughCLI(t, operationID, options)
	if result["status"] != "interrupted" {
		t.Fatalf("status = %#v, want interrupted", result["status"])
	}
	jobs, ok := result["accepted_jobs"].([]any)
	if !ok || len(jobs) != 1 {
		t.Fatalf("accepted jobs missing from interrupted batch: %#v", result)
	}
	job := jobs[0].(map[string]any)
	if job["status"] != "succeeded" || job["workbook_luid"] != "wb-new" || job["verification"] != "confirmed" {
		t.Fatalf("confirmed job identity lost: %#v", job)
	}
	if fixture.publishes.Load() != 1 || fixture.jobGets.Load() != 1 {
		t.Fatalf("recovery requests: publishes=%d jobs=%d", fixture.publishes.Load(), fixture.jobGets.Load())
	}
	saved, err := store.Read(operationID)
	if err != nil || saved.Phase != operationrun.PhaseRunning {
		t.Fatalf("recovery incorrectly completed the interrupted batch: %+v, %v", saved, err)
	}
}
