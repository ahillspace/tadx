package workbook

import (
	"context"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
)

func TestPublishAcceptanceKeepsConfirmedIdentityAndReceipt(t *testing.T) {
	called := false
	acceptance := PublishAcceptance{Environment: "production", Site: "site",
		Accepted: func(context.Context, string, string) (AcceptedPublication, error) {
			return AcceptedPublication{Status: "succeeded", ResourceID: "wb-1", RequestID: "request-1", ReceiptPath: "receipt.json"}, nil
		},
		Destination: func(_ context.Context, id string) (string, string, string, error) {
			called = true
			if id != "wb-1" {
				t.Fatalf("destination id = %q", id)
			}
			return id, "Finance", "project-1", nil
		},
	}
	result, err := acceptance.result(t.Context(), "job-1", "request-before")
	if err != nil || !called || result.Status != "succeeded" || result.JobID != "job-1" || result.WorkbookLUID != "wb-1" || result.WorkbookName != "Finance" || result.ProjectLUID != "project-1" || result.TableauRequestID != "request-1" || result.ReceiptPath != "receipt.json" {
		t.Fatalf("result=%#v destination=%v err=%v", result, called, err)
	}
}

func TestPublishAcceptancePreservesPersistenceErrorWithoutDestinationRead(t *testing.T) {
	persistence := &errs.Error{ID: "workbook.publish.receipt", Phase: errs.PhasePersistence, Outcome: errs.OutcomeUnknown}
	acceptance := PublishAcceptance{Accepted: func(context.Context, string, string) (AcceptedPublication, error) {
		return AcceptedPublication{Status: "pending", RequestID: "request-1", ReceiptPath: "receipt.json"}, persistence
	}, Destination: func(context.Context, string) (string, string, string, error) {
		t.Fatal("destination read after failed receipt persistence")
		return "", "", "", nil
	}}
	result, err := acceptance.result(t.Context(), "job-1", "request-1")
	if !errors.Is(err, persistence) || result.JobID != "job-1" || result.ReceiptPath != "receipt.json" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}
