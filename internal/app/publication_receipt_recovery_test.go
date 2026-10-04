package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/jobmonitor"
	"github.com/ahillspace/tadx/internal/operationrun"
	"github.com/ahillspace/tadx/internal/value"
)

func TestOperationInspectKeepsBatchIntentOrderAfterPartialReceiptRecovery(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected remote request", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	runtime, _ := datasourceLifecycleRuntime(t, server)
	configPath := runtime.configPath
	operationDirectory, receiptDirectory := t.TempDir(), t.TempDir()
	store := operationrun.Store{Directory: operationDirectory}
	record, err := store.Create(operationrun.Request{Operation: "workbook.publish", ConfigPath: configPath})
	if err != nil {
		t.Fatal(err)
	}
	registered := time.Now().UTC()
	receiptStore := jobmonitor.Store{Directory: receiptDirectory}
	var intents []operationrun.ReceiptIntent
	var paths []string
	for index, resourceID := range []string{"workbook-first", "workbook-second"} {
		receipt := jobmonitor.Receipt{
			ReceiptID: resourceID + "-receipt", OperationID: record.ID, Version: 1,
			Operation: record.Operation, Environment: "production", Server: server.URL,
			Site: "team-site", SiteID: "site-id", ConfigPath: configPath,
			SourcePath: resourceID + ".twb", ProjectID: "project-1", Name: resourceID,
			CoordinationKey: "opaque", AcceptedAt: registered.Add(time.Duration(index+1) * time.Second),
			Observation: value.JobStatus{Status: "unknown", ResourceID: resourceID},
		}
		intents = append(intents, operationrun.ReceiptIntent{ID: receipt.ReceiptID, Scope: jobmonitor.ReceiptScope(receipt), RegisteredAt: registered})
		path, err := receiptStore.Save(t.Context(), receipt)
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	_, err = store.Update(record.ID, func(current *operationrun.Record) error {
		current.Phase = operationrun.PhaseRunning
		current.StartedAt = registered
		current.ReceiptIntents = intents
		current.ReceiptPaths = []string{paths[1]}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	options := Options{ConfigPath: configPath, OperationDirectory: operationDirectory, JobDirectory: receiptDirectory, HTTPClient: server.Client()}
	if exit := Run(t.Context(), []string{"job", "inspect", "--operation-id", record.ID, "--json", "--full"}, &output, options); exit != 0 {
		t.Fatalf("operation inspection exit=%d output=%s", exit, output.String())
	}
	var inspection struct {
		Status string `json:"status"`
		Jobs   []struct {
			ResourceID string `json:"workbook_luid"`
		} `json:"accepted_jobs"`
	}
	if err := json.Unmarshal([]byte(output.String()), &inspection); err != nil {
		t.Fatal(err)
	}
	if inspection.Status != "interrupted" || len(inspection.Jobs) != 2 || inspection.Jobs[0].ResourceID != "workbook-first" || inspection.Jobs[1].ResourceID != "workbook-second" {
		t.Fatalf("accepted jobs lost batch order: %+v", inspection)
	}
	updated, err := store.Read(record.ID)
	if err != nil || len(updated.ReceiptPaths) != 2 || updated.ReceiptPaths[0] != paths[0] || updated.ReceiptPaths[1] != paths[1] {
		t.Fatalf("saved receipt order=%v err=%v", updated.ReceiptPaths, err)
	}
	if requests.Load() != 0 {
		t.Fatalf("operation recovery made %d remote requests", requests.Load())
	}
}
