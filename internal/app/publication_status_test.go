package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	jobinspect "github.com/ahillspace/tadx/actions/job/inspect"
	"github.com/ahillspace/tadx/internal/jobmonitor"
	"github.com/ahillspace/tadx/internal/operationrun"
	"github.com/ahillspace/tadx/internal/value"
)

func TestInspectPublicationOperationUsesLocalStateForLiveWorker(t *testing.T) {
	operationDirectory := t.TempDir()
	store := operationrun.Store{Directory: operationDirectory}
	record, err := store.Create(operationrun.Request{Operation: "workbook.publish"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	lease, _, err := store.Lease(record.ID, 789)
	if err != nil {
		t.Fatalf("Lease() error = %v", err)
	}
	defer lease.Release()
	if _, err := lease.Update(func(record *operationrun.Record) error {
		record.Activity = "uploading workbook"
		record.LiveResults = json.RawMessage(`{"status":"running","completed":0}`)
		return nil
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	runtime := &runtimeDependencies{operationDirectory: operationDirectory, configPath: filepath.Join(operationDirectory, "config.yml")}
	result, err := runtime.inspectPublicationOperation(context.Background(), jobinspect.Input{OperationID: record.ID})
	if err != nil {
		t.Fatalf("inspectPublicationOperation() error = %v", err)
	}
	if result.Operation == nil || !result.Operation.Alive || result.Operation.Status != "running" || result.Operation.Activity != "uploading workbook" {
		t.Fatalf("local operation view = %#v", result.Operation)
	}
}

func TestInspectPublicationOperationChecksFinishedReceiptOnceWithoutPoolRegistration(t *testing.T) {
	operationDirectory, receiptDirectory := t.TempDir(), t.TempDir()
	operationStore := operationrun.Store{Directory: operationDirectory}
	record, err := operationStore.Create(operationrun.Request{Operation: "workbook.publish"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	now := time.Now().UTC()
	receiptStore := jobmonitor.Store{Directory: receiptDirectory}
	receipt := jobmonitor.Receipt{
		Version:         1,
		Operation:       "workbook.publish",
		Environment:     "dev",
		Server:          "https://tableau.example",
		Site:            "site",
		SiteID:          "site-id",
		CoordinationKey: "opaque-key",
		AcceptedAt:      now,
		Observation:     value.JobStatus{ID: "job-1", Type: "PublishWorkbook", Status: "succeeded", ResourceID: "wb-1", CheckedAt: now},
	}
	receiptPath, err := receiptStore.Save(context.Background(), receipt)
	if err != nil {
		t.Fatalf("Save() receipt error = %v", err)
	}
	if _, err := operationStore.Update(record.ID, func(record *operationrun.Record) error {
		record.Phase = operationrun.PhaseRemotePending
		record.ReceiptPaths = []string{receiptPath}
		record.FullResult = json.RawMessage(`{"status":"pending"}`)
		return nil
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	runtime := &runtimeDependencies{operationDirectory: operationDirectory, jobDirectory: receiptDirectory}
	result, err := runtime.inspectPublicationOperation(context.Background(), jobinspect.Input{OperationID: record.ID})
	if err != nil {
		t.Fatalf("inspectPublicationOperation() error = %v", err)
	}
	if result.Operation == nil || result.Operation.Status != "succeeded" || len(result.Operation.Items) != 1 {
		t.Fatalf("remote-pending view = %#v", result.Operation)
	}
	item := result.Operation.Items[0]
	if item.Status != "succeeded" || item.ResourceID != "wb-1" {
		t.Fatalf("receipt item = %#v", item)
	}
}

func TestInspectPublicationOperationRejectsExplicitTargetMismatch(t *testing.T) {
	operationDirectory := t.TempDir()
	store := operationrun.Store{Directory: operationDirectory}
	record, err := store.Create(operationrun.Request{Operation: "workbook.publish"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := store.Update(record.ID, func(record *operationrun.Record) error {
		record.Phase = operationrun.PhaseCompleted
		record.CompactResult = json.RawMessage(`{"environment":"prod","site":"main","status":"succeeded"}`)
		record.FullResult = record.CompactResult
		return nil
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	runtime := &runtimeDependencies{operationDirectory: operationDirectory}
	if _, err := runtime.inspectPublicationOperation(context.Background(), jobinspect.Input{OperationID: record.ID, Environment: "dev"}); err == nil {
		t.Fatal("inspectPublicationOperation() error = nil, want environment mismatch")
	}
	if _, err := runtime.inspectPublicationOperation(context.Background(), jobinspect.Input{OperationID: record.ID, Site: "other"}); err == nil {
		t.Fatal("inspectPublicationOperation() error = nil, want site mismatch")
	}
}

func TestInspectPublicationOperationPreservesSubmissionFailureWhenAcceptedItemSucceeds(t *testing.T) {
	operationDirectory, receiptDirectory := t.TempDir(), t.TempDir()
	operationStore := operationrun.Store{Directory: operationDirectory}
	record, err := operationStore.Create(operationrun.Request{Operation: "workbook.publish"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	now := time.Now().UTC()
	receiptStore := jobmonitor.Store{Directory: receiptDirectory}
	receipt := jobmonitor.Receipt{
		Version:         1,
		Operation:       "workbook.publish",
		Environment:     "dev",
		Server:          "https://tableau.example",
		Site:            "site",
		SiteID:          "site-id",
		CoordinationKey: "opaque-key",
		AcceptedAt:      now,
		Observation:     value.JobStatus{ID: "job-1", Type: "PublishWorkbook", Status: "succeeded", ResourceID: "wb-1", CheckedAt: now},
	}
	receiptPath, err := receiptStore.Save(context.Background(), receipt)
	if err != nil {
		t.Fatalf("Save() receipt error = %v", err)
	}
	snapshot := json.RawMessage(`{"status":"partial_failure","items":[{"status":"failed","selector":"bad-item"},{"status":"pending","receipt_path":"` + filepath.ToSlash(receiptPath) + `"}]}`)
	if _, err := operationStore.Update(record.ID, func(record *operationrun.Record) error {
		record.Phase = operationrun.PhaseRemotePending
		record.ReceiptPaths = []string{receiptPath}
		record.CompactResult = snapshot
		record.FullResult = snapshot
		return nil
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	runtime := &runtimeDependencies{operationDirectory: operationDirectory, jobDirectory: receiptDirectory}
	result, err := runtime.inspectPublicationOperation(context.Background(), jobinspect.Input{OperationID: record.ID})
	if err != nil {
		t.Fatalf("inspectPublicationOperation() error = %v", err)
	}
	if result.Operation == nil || result.Operation.Status != "partial_failure" {
		t.Fatalf("mixed operation view = %#v", result.Operation)
	}
	saved, err := operationStore.Read(record.ID)
	if err != nil {
		t.Fatalf("Read() reconciled record error = %v", err)
	}
	if saved.Phase != operationrun.PhaseFailed || !publicationHasFailure(saved.FullResult) {
		t.Fatalf("reconciled record = %#v", saved)
	}
	var decoded map[string]any
	if err := json.Unmarshal(saved.FullResult, &decoded); err != nil {
		t.Fatalf("decode reconciled result: %v", err)
	}
	items, ok := decoded["items"].([]any)
	if !ok || len(items) != 2 || items[0].(map[string]any)["status"] != "failed" || items[1].(map[string]any)["status"] != "succeeded" {
		t.Fatalf("reconciled item states = %#v", decoded["items"])
	}
}

func TestInspectPublicationOperationReconcilesNestedJobIdentity(t *testing.T) {
	operationDirectory, receiptDirectory := t.TempDir(), t.TempDir()
	operationStore := operationrun.Store{Directory: operationDirectory}
	record, err := operationStore.Create(operationrun.Request{Operation: "workbook.publish"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	now := time.Now().UTC()
	receiptStore := jobmonitor.Store{Directory: receiptDirectory}
	receipt := jobmonitor.Receipt{
		Version: 1, Operation: "workbook.publish", Environment: "dev", Server: "https://tableau.example", Site: "site", SiteID: "site-id",
		CoordinationKey: "opaque-key", AcceptedAt: now,
		Observation: value.JobStatus{ID: "job-1", Type: "PublishWorkbook", Status: "succeeded", ResourceID: "wb-1", CheckedAt: now},
	}
	receiptPath, err := receiptStore.Save(context.Background(), receipt)
	if err != nil {
		t.Fatalf("Save() receipt error = %v", err)
	}
	snapshot := json.RawMessage(`{"operation":"workbook.publish","status":"pending","items":[{"status":"pending","result":{"status":"pending","result":{"status":"pending","tableau_job_id":"job-1"}}}]}`)
	if _, err := operationStore.Update(record.ID, func(record *operationrun.Record) error {
		record.Phase = operationrun.PhaseRemotePending
		record.ReceiptPaths = []string{receiptPath}
		record.CompactResult, record.FullResult = snapshot, snapshot
		return nil
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	runtime := &runtimeDependencies{operationDirectory: operationDirectory, jobDirectory: receiptDirectory}
	result, err := runtime.inspectPublicationOperation(context.Background(), jobinspect.Input{OperationID: record.ID})
	if err != nil {
		t.Fatalf("inspectPublicationOperation() error = %v", err)
	}
	if result.Operation == nil || result.Operation.Status != "succeeded" {
		t.Fatalf("nested operation view = %#v", result.Operation)
	}
	saved, err := operationStore.Read(record.ID)
	if err != nil {
		t.Fatalf("Read() reconciled record error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(saved.FullResult, &decoded); err != nil {
		t.Fatalf("decode reconciled result: %v", err)
	}
	items := decoded["items"].([]any)
	outer := items[0].(map[string]any)
	inner := outer["result"].(map[string]any)
	terminal := inner["result"].(map[string]any)
	if outer["status"] != "succeeded" || inner["status"] != "succeeded" || terminal["status"] != "succeeded" || terminal["workbook_luid"] != "wb-1" {
		t.Fatalf("nested reconciled result = %#v", decoded)
	}
}

func TestInspectPublicationOperationPersistsPartialReceiptProgress(t *testing.T) {
	operationDirectory, receiptDirectory := t.TempDir(), t.TempDir()
	operationStore := operationrun.Store{Directory: operationDirectory}
	record, err := operationStore.Create(operationrun.Request{Operation: "workbook.publish"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	now := time.Now().UTC()
	receiptStore := jobmonitor.Store{Directory: receiptDirectory}
	receipt := jobmonitor.Receipt{
		Version: 1, Operation: "workbook.publish", Environment: "dev", Server: "https://tableau.example", Site: "site", SiteID: "site-id",
		CoordinationKey: "opaque-key", AcceptedAt: now,
		Observation: value.JobStatus{ID: "job-1", Type: "PublishWorkbook", Status: "succeeded", ResourceID: "wb-1", CheckedAt: now},
	}
	receiptPath, err := receiptStore.Save(context.Background(), receipt)
	if err != nil {
		t.Fatalf("Save() receipt error = %v", err)
	}
	missingPath := filepath.Join(receiptDirectory, "not-yet-written.json")
	snapshot := json.RawMessage(`{"operation":"workbook.publish","status":"pending","items":[{"status":"pending","receipt_path":"` + filepath.ToSlash(receiptPath) + `"},{"status":"pending","receipt_path":"` + filepath.ToSlash(missingPath) + `"}]}`)
	if _, err := operationStore.Update(record.ID, func(record *operationrun.Record) error {
		record.Phase = operationrun.PhaseRemotePending
		record.ReceiptPaths = []string{receiptPath, missingPath}
		record.CompactResult, record.FullResult = snapshot, snapshot
		return nil
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	runtime := &runtimeDependencies{operationDirectory: operationDirectory, jobDirectory: receiptDirectory}
	if _, err := runtime.inspectPublicationOperation(context.Background(), jobinspect.Input{OperationID: record.ID}); err != nil {
		t.Fatalf("inspectPublicationOperation() error = %v", err)
	}
	saved, err := operationStore.Read(record.ID)
	if err != nil {
		t.Fatalf("Read() reconciled record error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(saved.FullResult, &decoded); err != nil {
		t.Fatalf("decode reconciled result: %v", err)
	}
	items := decoded["items"].([]any)
	if items[0].(map[string]any)["status"] != "succeeded" || decoded["status"] != "pending" {
		t.Fatalf("partial reconciled result = %#v", decoded)
	}
	if saved.Phase != operationrun.PhaseRemotePending || !saved.FinishedAt.IsZero() {
		t.Fatalf("partial reconciled record = %#v", saved)
	}
}
