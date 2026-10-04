package datasource

import (
	"context"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
)

func TestPublishAcceptanceKeepsConfirmedDatasourceIdentityAndReceipt(t *testing.T) {
	acceptance := PublishAcceptance{Environment: "production", Site: "site",
		Accepted: func(context.Context, string, string) (AcceptedPublication, error) {
			return AcceptedPublication{Status: "succeeded", ResourceID: "ds-1", RequestID: "request-1", ReceiptPath: "receipt.json"}, nil
		},
		Destination: func(_ context.Context, id string) (string, string, string, error) {
			if id != "ds-1" {
				t.Fatalf("destination id = %q", id)
			}
			return id, "Sales", "project-1", nil
		},
	}
	result, err := acceptance.result(t.Context(), "job-1", "request-before")
	if err != nil || result.Status != "succeeded" || result.JobID != "job-1" || result.DatasourceLUID != "ds-1" || result.DatasourceName != "Sales" || result.ProjectLUID != "project-1" || result.TableauRequestID != "request-1" || result.ReceiptPath != "receipt.json" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestPublishAcceptanceReportsConfirmedDestinationFailureWithoutRetry(t *testing.T) {
	missing := errors.New("destination read failed")
	acceptance := PublishAcceptance{Environment: "production", Site: "site",
		Accepted: func(context.Context, string, string) (AcceptedPublication, error) {
			return AcceptedPublication{Status: "succeeded", ResourceID: "ds-1", RequestID: "request-1"}, nil
		},
		Destination: func(context.Context, string) (string, string, string, error) { return "ds-1", "", "", missing },
	}
	result, err := acceptance.result(t.Context(), "job-1", "request-before")
	structured, ok := errors.AsType[*errs.Error](err)
	if !ok || !errors.Is(err, missing) || structured.ID != "datasource.publish.monitor" || structured.Outcome != errs.OutcomeConfirmed || structured.Phase != errs.PhaseVerification || result.DatasourceLUID != "ds-1" || result.JobID != "job-1" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}
