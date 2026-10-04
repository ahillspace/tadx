package jobmonitor

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartPublicationBindsIntentBeforeSubmission(t *testing.T) {
	store := Store{Directory: t.TempDir()}
	target := PublicationTarget{Kind: "workbook", Environment: "prod", Server: "https://example.test", Site: "team", SiteID: "site-1", ConfigPath: "config.yaml", SourcePath: filepath.Join("books", "Finance.twb"), ProjectID: "project-1", Name: "Finance", CoordinationKey: "key", OperationID: "operation-1"}
	called := 0
	p, err := StartPublication(t.Context(), store, target, PublicationHooks{Prepare: func(_ context.Context, receipt Receipt) error {
		called++
		if receipt.ReceiptID == "" || receipt.Operation != "workbook.publish" || receipt.OperationID != "operation-1" || receipt.SourcePath != "books/Finance.twb" || receipt.CoordinationKey != "key" || receipt.SiteID != "site-1" {
			t.Fatalf("intent receipt=%+v", receipt)
		}
		return nil
	}})
	if err != nil || p == nil || called != 1 || p.Base.ReceiptID == "" {
		t.Fatalf("publication=%+v called=%d err=%v", p, called, err)
	}
}

func TestStartPublicationPrepareFailurePreventsPublication(t *testing.T) {
	want := errors.New("intent storage failed")
	p, err := StartPublication(t.Context(), Store{Directory: t.TempDir()}, PublicationTarget{Kind: "datasource"}, PublicationHooks{Prepare: func(context.Context, Receipt) error { return want }})
	if p != nil || !errors.Is(err, want) || !strings.Contains(err.Error(), "record publication receipt intent before submission") {
		t.Fatalf("publication=%+v err=%v", p, err)
	}
}
