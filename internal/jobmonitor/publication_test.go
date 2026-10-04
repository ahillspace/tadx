package jobmonitor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/errs"
)

func TestPublicationAcceptanceSavesBeforeLinkAndNoticeDespiteCancellation(t *testing.T) {
	for _, bulk := range []bool{false, true} {
		t.Run(fmt.Sprintf("bulk=%t", bulk), func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			var events []string
			publication := &Publication{
				Store:         Store{Directory: t.TempDir()},
				Base:          receipt("accepted-job", time.Now().UTC(), false),
				StopRequested: func() bool { return !bulk },
			}
			publication.OnAccepted = func(saveCtx context.Context, path string) error {
				if saveCtx.Err() != nil {
					t.Fatalf("persistence inherited cancellation: %v", saveCtx.Err())
				}
				deadline, ok := saveCtx.Deadline()
				if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 5*time.Second {
					t.Fatalf("deadline=%v", deadline)
				}
				saved, err := publication.Store.ReadPath(path)
				if err != nil || saved.Observation.ID != "accepted-job" || saved.Observation.RequestID != "accepted-request" || saved.Observation.Status != "pending" {
					t.Fatalf("linked unsaved acceptance: %+v error=%v", saved, err)
				}
				if bulk && !saved.PoolAfter.Equal(saved.AcceptedAt) {
					t.Fatalf("bulk pool=%v accepted=%v", saved.PoolAfter, saved.AcceptedAt)
				}
				if !bulk && !saved.PoolAfter.Equal(saved.AcceptedAt.Add(time.Minute)) {
					t.Fatalf("individual pooling changed: %+v", saved)
				}
				events = append(events, "link")
				return nil
			}
			publication.NotifyAccepted = func(id, path string) {
				if id != "accepted-job" || path == "" {
					t.Fatalf("notice identity=%q path=%q", id, path)
				}
				events = append(events, "notice")
			}
			publication.Observe = func(context.Context, []Receipt) []CheckResult { t.Fatal("manual/bulk acceptance observed"); return nil }
			_, _, err := publication.Accepted(ctx, "accepted-job", "accepted-request", "PublishWorkbook", bulk)
			if err != nil || !slices.Equal(events, []string{"link", "notice"}) {
				t.Fatalf("events=%v error=%v", events, err)
			}
		})
	}
}

func TestPublicationRecordPreservesRequestIdentityBeforeLinkFailure(t *testing.T) {
	linkErr := errors.New("link failed")
	publication := &Publication{Store: Store{Directory: t.TempDir()}, Base: receipt("accepted-job", time.Now().UTC(), true)}
	publication.Base.Observation.RequestID = "accepted-request"
	if _, err := publication.Store.Save(t.Context(), publication.Base); err != nil {
		t.Fatal(err)
	}
	publication.OnAccepted = func(context.Context, string) error { return linkErr }
	path, err := publication.Record(t.Context(), "accepted-job", "succeeded", "resource-1", "", "confirmed")
	if !errors.Is(err, linkErr) || path == "" {
		t.Fatalf("path=%q error=%v", path, err)
	}
	saved, err := publication.Store.ReadPath(path)
	if err != nil || saved.Observation.RequestID != "accepted-request" || saved.Observation.ResourceID != "resource-1" || saved.Verification != "confirmed" {
		t.Fatalf("saved=%+v error=%v", saved, err)
	}
}

func TestPublicationAcceptedReceiptPersistenceFailureBeforeObservation(t *testing.T) {
	for _, noWait := range []bool{false, true} {
		t.Run(fmt.Sprintf("no-wait=%t", noWait), func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "blocked-receipts")
			if err := os.WriteFile(directory, []byte("receipt storage is not a directory"), 0o600); err != nil {
				t.Fatal(err)
			}
			publication := &Publication{
				StopRequested: func() bool { return noWait },
				Store:         Store{Directory: directory},
				Base: Receipt{
					Version: 1, Operation: "workbook.publish", Environment: "production",
					Server: "https://example.test", Site: "team-site", SiteID: "site-1",
					CoordinationKey: "fixture-coordination",
				},
			}
			receipt, _, err := publication.Accepted(t.Context(), "accepted-job", "accepted-request", "PublishWorkbook", false)
			failure, ok := errors.AsType[*errs.Error](err)
			if !ok || failure.Phase != errs.PhasePersistence || failure.Outcome != errs.OutcomeUnknown || failure.TableauJobID != "accepted-job" || failure.TableauRequestID != "accepted-request" {
				t.Fatalf("acceptance persistence error=%#v", err)
			}
			storageFailure, ok := errors.AsType[*os.PathError](failure.Cause)
			if !ok || storageFailure.Op != "mkdir" || storageFailure.Path != directory {
				t.Fatalf("underlying error=%#v, want blocked receipt directory failure", failure.Cause)
			}
			if receipt.Observation.ID != "accepted-job" || receipt.Observation.RequestID != "accepted-request" || receipt.Observation.Status != "pending" || receipt.ManualOnly != noWait {
				t.Fatalf("acceptance receipt=%+v", receipt)
			}
		})
	}
}
