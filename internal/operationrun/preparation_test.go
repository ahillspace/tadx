package operationrun_test

import (
	"bytes"
	"testing"

	"github.com/ahillspace/tadx/internal/operationrun"
)

func TestPreparationSnapshotIsClonedAndImmutable(t *testing.T) {
	store := operationrun.Store{Directory: t.TempDir()}
	expected := []byte(`{"version":1,"destinations":[{"name":"Destination"}]}`)
	request := operationrun.Request{Operation: "workbook.publish", Args: []string{"content", "workbook", "publish"}, PreparationData: bytes.Clone(expected)}
	record, err := store.Create(request)
	if err != nil {
		t.Fatal(err)
	}
	request.PreparationData[0] = '['
	record.Request.PreparationData[0] = '['
	stored, err := store.Read(record.ID)
	if err != nil || !bytes.Equal(stored.Request.PreparationData, expected) {
		t.Fatalf("stored preparation=%s error=%v", stored.Request.PreparationData, err)
	}
	if _, err := store.Update(record.ID, func(r *operationrun.Record) error {
		r.Request.PreparationData[0] = '['
		return nil
	}); err == nil {
		t.Fatal("updating accepted preparation was permitted")
	}
	if _, err := store.Create(operationrun.Request{Operation: "workbook.publish", PreparationData: []byte("{")}); err == nil {
		t.Fatal("malformed preparation JSON was accepted")
	}
}
