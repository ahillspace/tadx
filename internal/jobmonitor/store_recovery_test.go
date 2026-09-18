package jobmonitor

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/value"
)

func TestStoreFindByJobIDAndReadPathPreserveReceiptTarget(t *testing.T) {
	store := Store{Directory: t.TempDir()}
	receipt := Receipt{Version: 1, ReceiptID: "receipt", Operation: "workbook.publish", Environment: "dev", Server: "https://example.test", Site: "site", SiteID: "site-luid", CoordinationKey: "opaque", AcceptedAt: time.Now().UTC(), Observation: value.JobStatus{ID: "job", Type: "PublishWorkbook", Status: "pending"}}
	path, err := store.Register(t.Context(), receipt)
	if err != nil {
		t.Fatal(err)
	}
	fromPath, err := store.ReadPath(path)
	if err != nil || fromPath.Observation.ID != receipt.Observation.ID || fromPath.Environment != receipt.Environment {
		t.Fatalf("ReadPath() receipt=%+v err=%v", fromPath, err)
	}
	found, foundPath, err := store.FindByJobID(t.Context(), "job", "dev", "site")
	if err != nil || found.Observation.ID != "job" || filepath.Clean(foundPath) != filepath.Clean(path) {
		t.Fatalf("FindByJobID() receipt=%+v path=%q err=%v", found, foundPath, err)
	}
}

func TestStoreReadPathRejectsOutsideDirectory(t *testing.T) {
	store := Store{Directory: t.TempDir()}
	if _, err := store.ReadPath(filepath.Join(store.Directory, "..", "outside.json")); err == nil {
		t.Fatal("ReadPath accepted a receipt outside the configured store")
	}
}

func TestStoreAcceptsObservationReceiptWithoutInventedAcceptanceTime(t *testing.T) {
	store := Store{Directory: t.TempDir()}
	started := time.Now().UTC()
	receipt := Receipt{Version: 1, Operation: "job.wait", Environment: "dev", Server: "https://example.test", Site: "site", SiteID: "site-luid", CoordinationKey: "opaque", TrackingStartedAt: started, Observation: value.JobStatus{ID: "observed", Type: "RefreshExtract", Status: "running"}}
	path, err := store.Save(t.Context(), receipt)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.ReadPath(path)
	if err != nil || got.TrackingStartedAt != started || !got.AcceptedAt.IsZero() {
		t.Fatalf("observation receipt=%+v err=%v", got, err)
	}
}
